package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type wikiSave struct {
	repo                      ghcli.RepoRef
	file, blob, text, message string
}

type wikiFakes struct {
	list      ghcli.WikiList
	texts     map[string]string
	reads     []string // the file of every page read
	listReads int
	saves     []wikiSave
	creates   []wikiSave
	deletes   []wikiSave
	err       error
	editorSaw []string // what the editor was opened on
	editorOut string   // what the editor returns
	kept      []string // text set aside because it was not sent
}

func wikiPages() []ghcli.WikiPage {
	d := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	return []ghcli.WikiPage{
		{Title: "Home", File: "Home.md", Blob: "b-Home.md", Author: "mira", Updated: d},
		{Title: "Color schemes", File: "Color-schemes.md", Blob: "b-Color-schemes.md", Author: "noor", Updated: d},
		{Title: "notes", File: "notes.txt", Blob: "b-notes.txt", Author: "jun", Updated: d, Plain: true},
	}
}

func installWiki(t *testing.T) *wikiFakes {
	t.Helper()
	f := &wikiFakes{
		list: ghcli.WikiList{State: ghcli.WikiReady, Pages: wikiPages(), Total: 3},
		texts: map[string]string{
			"Home.md":          "# Welcome\n\nSee [[Color schemes]] or [[the notes|notes]] or [[Missing page]].\n\nAlso [guide](Color-schemes) and [site](https://example.com) and [top](#top).\n\n```\n[[Hidden]]\n```\n",
			"Color-schemes.md": "Colours.\n\nBack to [[Home]].\n",
			"notes.txt":        "plain [[Home]] text\nsecond line\n",
		},
		editorOut: "# Welcome\n\nEdited.\n",
	}
	oL, oP, oS, oC, oD, oE, oK := fetchWikiList, fetchWikiPage, saveWikiPage, createWikiPage, deleteWikiPage, editorCmd, keepWikiText
	keepWikiText = func(tg ghcli.RepoRef, file, text string) string {
		f.kept = append(f.kept, text)
		return "/cache/lost/" + file
	}
	fetchWikiList = func(tg ghcli.RepoRef) (ghcli.WikiList, error) {
		f.listReads++
		return f.list, nil
	}
	fetchWikiPage = func(tg ghcli.RepoRef, file string) (ghcli.WikiContent, error) {
		f.reads = append(f.reads, file)
		text, ok := f.texts[file]
		if !ok {
			return ghcli.WikiContent{}, errors.New("no page " + file)
		}
		p := ghcli.WikiPage{Title: ghcli.WikiTitle(file), File: file, Blob: "b-" + file, Plain: strings.HasSuffix(file, ".txt")}
		return ghcli.WikiContent{Page: p, Text: text, Plain: p.Plain, Pages: f.list.Pages}, nil
	}
	saveWikiPage = func(tg ghcli.RepoRef, file, blob, text, msg string) error {
		f.saves = append(f.saves, wikiSave{tg, file, blob, text, msg})
		return f.err
	}
	createWikiPage = func(tg ghcli.RepoRef, file, text, msg string) error {
		f.creates = append(f.creates, wikiSave{tg, file, "", text, msg})
		return f.err
	}
	deleteWikiPage = func(tg ghcli.RepoRef, file, blob, msg string) error {
		f.deletes = append(f.deletes, wikiSave{tg, file, blob, "", msg})
		return f.err
	}
	editorCmd = func(initial string) tea.Cmd {
		f.editorSaw = append(f.editorSaw, initial)
		return func() tea.Msg { return editedMsg{text: f.editorOut} }
	}
	t.Cleanup(func() {
		fetchWikiList, fetchWikiPage, saveWikiPage, createWikiPage, deleteWikiPage, editorCmd, keepWikiText = oL, oP, oS, oC, oD, oE, oK
	})
	return f
}

func wikiListModel(t *testing.T) model {
	t.Helper()
	m := model{view: viewWiki, width: 130, height: 30, target: acme, prTarget: acme, previewOff: true}
	return pump(m, m.fetch())
}

func wikiPageModel(t *testing.T, file string) model {
	t.Helper()
	m := wikiListModel(t)
	for m.wikiPageUnderCursor() != file {
		m = keyAll(m, "j")
	}
	return keyAll(m, "enter")
}

// wikiPageUnderCursor is a test convenience: the file the list stands on.
func (m model) wikiPageUnderCursor() string {
	r, _ := m.selected()
	return r.id
}

func wikiStatus(m model) string { return ansi.Strip(m.status) }

func TestSixKeyAndTheWikiCommandOpenTheList(t *testing.T) {
	f := installWiki(t)
	m := model{view: viewRuns, width: 130, height: 30, target: acme}
	if m = keyAll(m, "6"); m.view != viewWiki || f.listReads != 1 {
		t.Errorf("6 -> view %v after %d reads", m.view, f.listReads)
	}
	m = model{view: viewRuns, width: 130, height: 30, target: acme}
	next, cmd := m.command("wiki")
	if got := pump(next.(model), cmd); got.view != viewWiki {
		t.Errorf(":wiki -> view %v", got.view)
	}
}

func TestWikiListShowsTitlesFilesAuthorsAndFiltersOnTitlesOnly(t *testing.T) {
	installWiki(t)
	m := wikiListModel(t)
	s := screenText(m)
	for _, want := range []string{"PAGE", "FILE", "UPDATED", "AUTHOR", "Home", "Color schemes", "Color-schemes.md", "notes.txt", "mira", "noor"} {
		if !strings.Contains(s, want) {
			t.Errorf("list lacks %q:\n%s", want, s)
		}
	}
	m.filter = "noor" // an author: not a title
	if n := len(m.visible()); n != 0 {
		t.Errorf("the filter matched an author: %d rows", n)
	}
	m.filter = "colo"
	if v := m.visible(); len(v) != 1 || v[0].id != "Color-schemes.md" {
		t.Errorf("filter colo -> %+v", v)
	}
	m.filter = "color-schemes.md" // a file name: not a title
	if n := len(m.visible()); n != 0 {
		t.Errorf("the filter matched a file name: %d rows", n)
	}
}

func TestWikiListSaysWhenCutAndWhenThereIsNothing(t *testing.T) {
	f := installWiki(t)
	f.list.Total = 800
	if m := wikiListModel(t); !strings.Contains(wikiStatus(m), "first 3 of 800") {
		t.Errorf("status %q", m.status)
	}
	f.list = ghcli.WikiList{State: ghcli.WikiOff}
	if s := screenText(wikiListModel(t)); !strings.Contains(s, "wiki is turned off for acme/api") {
		t.Errorf("off:\n%s", s)
	}
	f.list = ghcli.WikiList{State: ghcli.WikiNoPages}
	if s := screenText(wikiListModel(t)); !strings.Contains(s, "no wiki pages yet") || !strings.Contains(s, "browser (o)") {
		t.Errorf("no pages:\n%s", s)
	}
	f.list = ghcli.WikiList{State: ghcli.WikiReady}
	if s := screenText(wikiListModel(t)); !strings.Contains(s, "no pages (n creates one)") {
		t.Errorf("empty:\n%s", s)
	}
}

func TestEnterReadsAPageWithItsLinks(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	if m.view != viewWikiPage || len(f.reads) != 1 || f.reads[0] != "Home.md" {
		t.Fatalf("view %v reads %v", m.view, f.reads)
	}
	s := screenText(m)
	for _, want := range []string{"page=Home", "Home.md · acme/api wiki", "Welcome", "Links", "Color schemes", "the notes → notes", "Missing page", "(no such page)", "guide → Color-schemes"} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q:\n%s", want, s)
		}
	}
	st, _ := m.screenData(viewWikiPage).(wikiPageState)
	links := wikiLinks(st.c, st.c.Text)
	var names []string
	for _, l := range links {
		names = append(names, l.name)
	}
	if got := strings.Join(names, "|"); got != "Color schemes|notes|Missing page|Color-schemes" {
		t.Errorf("links = %q (not the external, anchor or fenced ones)", got)
	}
}

func TestPlainPageIsNotRenderedOrLinked(t *testing.T) {
	installWiki(t)
	m := wikiPageModel(t, "notes.txt")
	s := screenText(m)
	if !strings.Contains(s, "not Markdown") || !strings.Contains(s, "plain [[Home]] text") || strings.Contains(s, "Links") {
		t.Errorf("plain page:\n%s", s)
	}
}

func TestTabSelectsLinksEnterFollowsAndEscGoesBackAlongThem(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "tab") // Color schemes
	st, _ := m.screenData(viewWikiPage).(wikiPageState)
	if st.sel != 1 || !strings.Contains(screenText(m), "▌") {
		t.Fatalf("sel %d", st.sel)
	}
	m = keyAll(m, "enter")
	if m.wikiPage != "Color-schemes.md" || m.view != viewWikiPage || !strings.Contains(screenText(m), "Colours.") {
		t.Fatalf("page %q view %v:\n%s", m.wikiPage, m.view, screenText(m))
	}
	if !strings.Contains(screenText(m), "page=Color schemes") {
		t.Error("title did not follow the link")
	}
	m = keyAll(m, "esc") // back along the link
	if m.wikiPage != "Home.md" || m.view != viewWikiPage || !strings.Contains(screenText(m), "Welcome") {
		t.Fatalf("esc went to %q view %v", m.wikiPage, m.view)
	}
	m = keyAll(m, "esc") // now to the list, standing on Home
	if m.view != viewWiki || m.wikiPageUnderCursor() != "Home.md" {
		t.Errorf("view %v on %q", m.view, m.wikiPageUnderCursor())
	}
	_ = f
}

func TestEscFromAPageFollowedFromTheListComesBackStandingOnIt(t *testing.T) {
	installWiki(t)
	m := wikiPageModel(t, "notes.txt")
	m = keyAll(m, "esc")
	if m.view != viewWiki || m.wikiPageUnderCursor() != "notes.txt" {
		t.Errorf("view %v on %q", m.view, m.wikiPageUnderCursor())
	}
}

func TestFollowingAMissingLinkSaysSo(t *testing.T) {
	installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "shift+tab") // from nothing: the last link
	st, _ := m.screenData(viewWikiPage).(wikiPageState)
	if st.sel != 4 {
		t.Fatalf("sel %d", st.sel)
	}
	m = keyAll(m, "shift+tab") // Missing page
	m = keyAll(m, "enter")
	if m.wikiPage != "Home.md" || !strings.Contains(wikiStatus(m), "no such page: Missing page") {
		t.Errorf("page %q status %q", m.wikiPage, wikiStatus(m))
	}
}

func TestEditOpensTheEditorOnTheFreshPageThenAsksForAMessage(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	n := len(f.reads)
	m = keyAll(m, "e")
	if len(f.reads) != n+1 {
		t.Errorf("the page was not read afresh before editing: %v", f.reads)
	}
	if len(f.editorSaw) != 1 || !strings.Contains(f.editorSaw[0], "See [[Color schemes]]") {
		t.Fatalf("editor saw %q", f.editorSaw)
	}
	if m.dlg == nil || m.dlg.id != "wiki.save" {
		t.Fatalf("dialog %+v", m.dlg)
	}
	if !strings.Contains(m.dlg.title, "Save Home to acme/api wiki") || !strings.Contains(m.dlg.title, "+1") || !strings.Contains(m.dlg.title, "−7") {
		t.Errorf("title %q", m.dlg.title)
	}
	if m.dlg.text != "Update Home" {
		t.Errorf("the message should start as the default: %q", m.dlg.text)
	}
	if len(f.saves) != 0 {
		t.Fatal("saved before the answer")
	}
	m, cmd := answer(m, "wiki.save", true, "Fix the intro")
	m = runCmd(m, cmd)
	if len(f.saves) != 1 {
		t.Fatalf("saves %+v", f.saves)
	}
	s := f.saves[0]
	if s.repo != acme || s.file != "Home.md" || s.blob != "b-Home.md" || s.text != f.editorOut || s.message != "Fix the intro" {
		t.Errorf("saved %+v", s)
	}
	if !strings.Contains(wikiStatus(m), "saved Home to the wiki") || m.wikiBusy {
		t.Errorf("status %q busy %v", wikiStatus(m), m.wikiBusy)
	}
	if len(f.reads) < n+2 {
		t.Errorf("the page should be read again after the save: %v", f.reads)
	}
}

func TestEditFromTheListEditsTheRowUnderTheCursor(t *testing.T) {
	f := installWiki(t)
	m := wikiListModel(t)
	m = keyAll(m, "j", "e") // Color schemes
	if len(f.reads) != 1 || f.reads[0] != "Color-schemes.md" || m.dlg == nil || !strings.Contains(m.dlg.title, "Color schemes") {
		t.Errorf("reads %v dlg %+v", f.reads, m.dlg)
	}
}

func TestUnchangedOrEmptiedEditSavesNothingAndFreesTheKeys(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	f.editorOut = f.texts["Home.md"] + "\n" // only a final newline more
	m = keyAll(m, "e")
	if m.dlg != nil || !strings.Contains(wikiStatus(m), "no changes") || m.wikiBusy || m.wikiEdit != nil {
		t.Errorf("unchanged: dlg %v status %q busy %v", m.dlg, wikiStatus(m), m.wikiBusy)
	}
	f.editorOut = " \n\n"
	m = keyAll(m, "e")
	if m.dlg != nil || !strings.Contains(wikiStatus(m), "x deletes it") || m.wikiBusy {
		t.Errorf("emptied: dlg %v status %q busy %v", m.dlg, wikiStatus(m), m.wikiBusy)
	}
	if len(f.saves) != 0 {
		t.Errorf("saves %+v", f.saves)
	}
}

func TestCancellingTheMessageSavesNothingAndFreesTheKeys(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e")
	m, cmd := answer(m, "wiki.save", false, "")
	runCmd(m, cmd)
	if len(f.saves) != 0 || m.wikiBusy || m.wikiEdit != nil {
		t.Errorf("saves %+v busy %v edit %v", f.saves, m.wikiBusy, m.wikiEdit)
	}
	m = keyAll(m, "e")
	if m.dlg == nil {
		t.Error("e does not work after a cancelled edit")
	}
}

func TestOnlyOneWikiChangeAtATime(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e") // the message question is open: busy
	if !m.wikiBusy {
		t.Fatal("not busy while the question is open")
	}
	m.dlg = nil // pretend the question was closed by something else
	n := len(f.reads)
	for _, k := range []string{"e", "x", "R"} {
		m = keyAll(m, k)
		if !strings.Contains(wikiStatus(m), "still in progress") || m.dlg != nil {
			t.Errorf("%s while busy: dlg %v status %q", k, m.dlg, wikiStatus(m))
		}
	}
	if len(f.reads) != n {
		t.Error("a refresh ran while busy")
	}
	m = wikiListModel(t)
	m.wikiBusy = true
	if m = keyAll(m, "n"); m.dlg != nil || !strings.Contains(wikiStatus(m), "still in progress") {
		t.Errorf("n while busy: %v %q", m.dlg, wikiStatus(m))
	}
}

func TestPageReadForAnotherRepoOrViewDoesNotOpenTheEditor(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m.wikiBusy = true
	late := wikiOpenedMsg{target: ghcli.RepoRef{Owner: "other", Name: "repo"}, file: "Home.md", c: ghcli.WikiContent{Page: ghcli.WikiPage{File: "Home.md"}}}
	next, cmd := m.Update(late)
	if cmd != nil || next.(model).wikiEdit != nil || next.(model).wikiBusy || len(f.editorSaw) != 0 {
		t.Error("the editor opened for a page of another repo")
	}
	m.wikiBusy = true
	m.view = viewIssues
	late.target = acme
	next, cmd = m.Update(late)
	if cmd != nil || next.(model).wikiEdit != nil || next.(model).wikiBusy {
		t.Error("the editor opened after leaving the wiki")
	}
}

func TestFailedReadBeforeEditingShowsTheErrorAndFreesTheKeys(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	delete(f.texts, "Home.md")
	m = keyAll(m, "e")
	if m.dlg != nil || m.wikiBusy || !strings.Contains(wikiStatus(m), "no page Home.md") || len(f.editorSaw) != 0 {
		t.Errorf("dlg %v busy %v status %q", m.dlg, m.wikiBusy, wikiStatus(m))
	}
}

func TestFailedSaveShowsAllOfItAndFreesTheKeys(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e")
	f.err = errors.New("Home changed on GitHub since you opened it, so nothing was saved; your text is saved in /cache/gh-9s/wiki/lost/github.com/acme/api/Home.md.20261103-101500")
	m, cmd := answer(m, "wiki.save", true, "Update Home")
	m = runCmd(m, cmd)
	if m.wikiBusy || !strings.Contains(wikiStatus(m), "✗") {
		t.Errorf("busy %v status %q", m.wikiBusy, wikiStatus(m))
	}
	if m.dlg == nil || m.dlg.id != "wiki.failed" || !strings.Contains(strings.ReplaceAll(m.dlg.detail, "\n ", " "), "/cache/gh-9s/wiki/lost/github.com/acme/api/Home.md.20261103-101500") {
		t.Errorf("the saved-text path must be on screen in full: %+v", m.dlg)
	}
}

func TestNewPageAsksForATitleThenEditsThenAsksForAMessage(t *testing.T) {
	f := installWiki(t)
	f.editorOut = "# Install\n\nSteps.\n"
	m := wikiListModel(t)
	m = keyAll(m, "n")
	if m.dlg == nil || m.dlg.id != "wiki.title" || !m.wikiBusy {
		t.Fatalf("dialog %+v", m.dlg)
	}
	for _, bad := range []struct{ title, want string }{{"Install-guide", "space"}, {"a/b", "cannot contain"}, {"home", "already exists"}, {"COLOR SCHEMES", "already exists"}} {
		var cmd tea.Cmd
		m, cmd = answer(m, "wiki.title", true, bad.title)
		m = runCmd(m, cmd)
		if m.dlg == nil || m.dlg.id != "wiki.title" || !strings.Contains(m.dlg.err, bad.want) || m.dlg.text != bad.title || len(f.editorSaw) != 0 {
			t.Fatalf("%q: dlg %+v saw %v", bad.title, m.dlg, f.editorSaw)
		}
	}
	m, cmd := answer(m, "wiki.title", true, "Install  guide")
	m = runCmd(m, cmd)
	if len(f.editorSaw) != 1 || f.editorSaw[0] != "" {
		t.Fatalf("editor saw %q (a new page starts empty)", f.editorSaw)
	}
	if m.dlg == nil || m.dlg.id != "wiki.save" || !strings.Contains(m.dlg.title, "Create Install guide in acme/api wiki") || m.dlg.text != "Create Install guide" {
		t.Fatalf("dialog %+v", m.dlg)
	}
	m, cmd = answer(m, "wiki.save", true, "Add install steps")
	m = runCmd(m, cmd)
	if len(f.creates) != 1 || f.creates[0].file != "Install-guide.md" || f.creates[0].text != f.editorOut || f.creates[0].message != "Add install steps" || f.creates[0].repo != acme {
		t.Fatalf("creates %+v", f.creates)
	}
	if !strings.Contains(wikiStatus(m), "created Install guide") || m.wikiBusy {
		t.Errorf("status %q busy %v", wikiStatus(m), m.wikiBusy)
	}
	if f.listReads < 2 {
		t.Error("the list should be read again to show the new page")
	}
}

func TestNewPageWithNothingWrittenCreatesNothing(t *testing.T) {
	f := installWiki(t)
	f.editorOut = "  \n"
	m := wikiListModel(t)
	m = keyAll(m, "n")
	m, cmd := answer(m, "wiki.title", true, "Empty one")
	m = runCmd(m, cmd)
	if m.dlg != nil || m.wikiBusy || !strings.Contains(wikiStatus(m), "nothing written") || len(f.creates) != 0 {
		t.Errorf("dlg %v busy %v status %q creates %v", m.dlg, m.wikiBusy, wikiStatus(m), f.creates)
	}
}

func TestCancellingTheTitleFreesTheKeys(t *testing.T) {
	installWiki(t)
	m := wikiListModel(t)
	m = keyAll(m, "n")
	m, _ = answer(m, "wiki.title", false, "")
	if m.wikiBusy || m.wikiEdit != nil {
		t.Error("still busy after cancelling the title")
	}
}

func TestNewPageIsRefusedWhereThereIsNoWikiYet(t *testing.T) {
	f := installWiki(t)
	for _, st := range []ghcli.WikiState{ghcli.WikiOff, ghcli.WikiNoPages} {
		f.list = ghcli.WikiList{State: st}
		m := wikiListModel(t)
		if m = keyAll(m, "n"); m.dlg != nil || m.wikiBusy || !strings.Contains(wikiStatus(m), "browser") {
			t.Errorf("state %v: dlg %v busy %v status %q", st, m.dlg, m.wikiBusy, wikiStatus(m))
		}
	}
}

func TestDeleteAsksNamingThePageAndTheRepoThenDeletes(t *testing.T) {
	f := installWiki(t)
	m := wikiListModel(t)
	m = keyAll(m, "j", "x") // Color schemes
	if m.dlg == nil || m.dlg.id != "wiki.delete" || !strings.Contains(m.dlg.title, `"Color schemes"`) || !strings.Contains(m.dlg.title, "acme/api") {
		t.Fatalf("dialog %+v", m.dlg)
	}
	if strings.Contains(m.dlg.detail, "front page") {
		t.Error("the front-page warning belongs to Home only")
	}
	if len(f.deletes) != 0 {
		t.Fatal("deleted before the answer")
	}
	m, cmd := answer(m, "wiki.delete", true, "")
	m = runCmd(m, cmd)
	if len(f.deletes) != 1 || f.deletes[0].file != "Color-schemes.md" || f.deletes[0].blob != "b-Color-schemes.md" || f.deletes[0].message != "Delete Color schemes" || f.deletes[0].repo != acme {
		t.Fatalf("deletes %+v", f.deletes)
	}
	if !strings.Contains(wikiStatus(m), "deleted Color schemes") || m.wikiBusy {
		t.Errorf("status %q busy %v", wikiStatus(m), m.wikiBusy)
	}
}

func TestDeletingHomeWarnsAndCancellingDeletesNothing(t *testing.T) {
	f := installWiki(t)
	m := wikiListModel(t)
	m = keyAll(m, "x")
	if m.dlg == nil || !strings.Contains(m.dlg.detail, "front page") {
		t.Fatalf("dialog %+v", m.dlg)
	}
	m, cmd := answer(m, "wiki.delete", false, "")
	runCmd(m, cmd)
	if len(f.deletes) != 0 || m.wikiBusy {
		t.Errorf("deletes %+v busy %v", f.deletes, m.wikiBusy)
	}
}

func TestDeleteUsesThePageChosenAtKeyPressNotWhatIsOnScreenLater(t *testing.T) {
	f := installWiki(t)
	m := wikiListModel(t)
	m = keyAll(m, "x") // Home
	m.cursor = 1       // the cursor moves meanwhile
	m, cmd := answer(m, "wiki.delete", true, "")
	runCmd(m, cmd)
	if len(f.deletes) != 1 || f.deletes[0].file != "Home.md" {
		t.Errorf("deletes %+v", f.deletes)
	}
}

func TestDeleteFromAPageGoesBackToTheList(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "notes.txt")
	m = keyAll(m, "x")
	if m.dlg == nil || !strings.Contains(m.dlg.title, `"notes"`) {
		t.Fatalf("dialog %+v", m.dlg)
	}
	f.list.Pages = f.list.Pages[:2]
	m, cmd := answer(m, "wiki.delete", true, "")
	m = runCmd(m, cmd)
	if m.view != viewWiki || len(m.visible()) != 2 || len(f.deletes) != 1 || f.deletes[0].blob != "b-notes.txt" {
		t.Errorf("view %v rows %d deletes %+v", m.view, len(m.visible()), f.deletes)
	}
}

func TestAnswersWithoutAChangeInProgressDoNothing(t *testing.T) {
	f := installWiki(t)
	m := wikiListModel(t)
	for _, id := range []string{"wiki.save", "wiki.delete", "wiki.title"} {
		m2, cmd := answer(m, id, true, "orphan")
		runCmd(m2, cmd)
	}
	if len(f.saves)+len(f.creates)+len(f.deletes) != 0 || len(f.editorSaw) != 0 {
		t.Error("an orphan answer changed the wiki")
	}
}

func TestTheMessageDialogDoesNotLoseTheEditorForTheCommitMessage(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e")
	// ctrl+e inside the message dialog opens the editor for the message; its result is the message, not a new edit
	next, _ := m.Update(editedMsg{text: "A longer message\n"})
	m = next.(model)
	if m.dlg == nil || m.dlg.text != "A longer message" || m.wikiEdit == nil {
		t.Errorf("dlg %+v edit %v", m.dlg, m.wikiEdit)
	}
	_ = f
}

func TestSwitchingRepoForgetsTheWikiPage(t *testing.T) {
	installWiki(t)
	m := wikiListModel(t)
	m.wikiPage, m.wikiTrail, m.wikiSel = "Home.md", []string{"x"}, "Home.md"
	m.switchRepo(ghcli.RepoRef{Owner: "other", Name: "repo"})
	if m.wikiPage != "" || m.wikiTrail != nil || m.view != viewWiki {
		t.Errorf("page %q trail %v view %v", m.wikiPage, m.wikiTrail, m.view)
	}
}

func TestLatePageOfAnotherFileIsDropped(t *testing.T) {
	installWiki(t)
	m := wikiPageModel(t, "Home.md")
	late := screenMsg{view: viewWikiPage, id: m.curTarget().String() + "|wiki|Other.md", data: ghcli.WikiContent{Page: ghcli.WikiPage{Title: "Other", File: "Other.md"}, Text: "stale"}}
	next, _ := m.Update(late)
	if strings.Contains(screenText(next.(model)), "stale") {
		t.Error("a page we left was shown")
	}
}

func TestWikiTextIsCleanedBeforeItIsDrawn(t *testing.T) {
	f := installWiki(t)
	f.texts["Home.md"] = "hello \x1b]0;pwned\x07 \x1b[31mred\x1b[0m world\n"
	m := wikiPageModel(t, "Home.md")
	if raw := m.View(); strings.Contains(raw, "pwned") || strings.Contains(raw, "\x1b]0") {
		t.Errorf("an escape from the page reached the terminal: %q", raw)
	}
}

func TestOOpensThePageOrTheWikiInTheBrowser(t *testing.T) {
	installWiki(t)
	var opened []string
	old := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openURL = old })
	m := wikiListModel(t)
	m = keyAll(m, "j", "o")
	m = keyAll(m, "enter", "o")
	if len(opened) != 2 || opened[0] != "https://github.com/acme/api/wiki/Color-schemes" || opened[1] != "https://github.com/acme/api/wiki/Color-schemes" {
		t.Errorf("opened %v", opened)
	}
	f := installWiki(t)
	f.list = ghcli.WikiList{State: ghcli.WikiNoPages}
	m = wikiListModel(t)
	opened = nil
	m = keyAll(m, "o")
	if len(opened) != 1 || opened[0] != "https://github.com/acme/api/wiki" {
		t.Errorf("opened %v", opened)
	}
}

func TestHelpAndFootersMentionTheWikiKeys(t *testing.T) {
	installWiki(t)
	m := wikiListModel(t)
	for _, want := range []string{"e edit", "n new page", "x delete"} {
		if !strings.Contains(screenText(m), want) {
			t.Errorf("list footer lacks %q", want)
		}
	}
	m = keyAll(m, "?")
	if s := screenText(m); !strings.Contains(s, "new page") || !strings.Contains(s, "delete the page") {
		t.Errorf("help:\n%s", s)
	}
}

func TestListStandsOnThePageOnlyOnceAfterComingBack(t *testing.T) {
	installWiki(t)
	m := wikiPageModel(t, "notes.txt")
	m = keyAll(m, "esc")
	if m.wikiPageUnderCursor() != "notes.txt" {
		t.Fatalf("on %q", m.wikiPageUnderCursor())
	}
	m = keyAll(m, "k", "R") // the user moves, then refreshes: the cursor must not jump back
	if m.wikiPageUnderCursor() != "Color-schemes.md" {
		t.Errorf("after k and R the list stands on %q", m.wikiPageUnderCursor())
	}
}

func TestEndOfAChangeInAnotherRepoNeitherRefetchesNorLeavesThePage(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	n := len(f.reads)
	next, cmd := m.Update(wikiDoneMsg{target: ghcli.RepoRef{Owner: "other", Name: "repo"}, file: "Home.md", text: "deleted Home from the wiki", deleted: true})
	got := next.(model)
	if cmd != nil || got.view != viewWikiPage || len(f.reads) != n || !strings.Contains(wikiStatus(got), "deleted Home") {
		t.Errorf("view %v cmd %v reads %d status %q", got.view, cmd != nil, len(f.reads), wikiStatus(got))
	}
}

func TestPlainPageTextIsCleanedToo(t *testing.T) {
	f := installWiki(t)
	f.texts["notes.txt"] = "plain \x1b]0;pwned\x07 \x1b[31mred\x1b[0m text\n"
	m := wikiPageModel(t, "notes.txt")
	if raw := m.View(); strings.Contains(raw, "pwned") || strings.Contains(raw, "\x1b]0") || strings.Contains(raw, "\x07") {
		t.Errorf("an escape from the page reached the terminal: %q", raw)
	}
}

func TestRefreshOnTheListIsRefusedWhileAChangeIsUnderWay(t *testing.T) {
	f := installWiki(t)
	m := wikiListModel(t)
	m.wikiBusy = true
	n := f.listReads
	m = keyAll(m, "R")
	if f.listReads != n || !strings.Contains(wikiStatus(m), "still in progress") {
		t.Errorf("reads %d->%d status %q", n, f.listReads, wikiStatus(m))
	}
}

func TestEscOnTheMessageKeepsTheEditedTextAndSaysWhere(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e")
	m, _ = answer(m, "wiki.save", false, "")
	if len(f.kept) != 1 || f.kept[0] != f.editorOut || len(f.saves) != 0 {
		t.Fatalf("kept %q saves %v", f.kept, f.saves)
	}
	if s := wikiStatus(m); !strings.Contains(s, "not saved") || !strings.Contains(s, "/cache/lost/Home.md") {
		t.Errorf("status %q", s)
	}
}

func TestFailureDoesNotReplaceAQuestionTheUserIsAnswering(t *testing.T) {
	f := installWiki(t)
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e")
	f.err = errors.New("GitHub did not accept the change; your text is saved in /cache/lost/Home.md.1")
	m, cmd := answer(m, "wiki.save", true, "Update Home")
	m = m.withDialog(newTextDialog("pr.comment", "Comment on acme/api#1", "Your comment"))
	m = runCmd(m, cmd)
	if m.dlg == nil || m.dlg.id != "pr.comment" {
		t.Fatalf("the open question was replaced: %+v", m.dlg)
	}
	if !strings.Contains(wikiStatus(m), "/cache/lost/Home.md.1") {
		t.Errorf("the saved-text path must still reach the user: %q", wikiStatus(m))
	}
}

func TestCRLFPageStaysCRLFWhenTheEditorWritesLF(t *testing.T) {
	f := installWiki(t)
	f.texts["Home.md"] = "one\r\ntwo\r\nthree\r\n"
	f.editorOut = "one\nTWO\nthree\n"
	m := wikiPageModel(t, "Home.md")
	m = keyAll(m, "e")
	title := m.dlg.title
	m, cmd := answer(m, "wiki.save", true, "m")
	runCmd(m, cmd)
	if len(f.saves) != 1 || f.saves[0].text != "one\r\nTWO\r\nthree\r\n" {
		t.Fatalf("saved %v", f.saves)
	}
	if !strings.Contains(title, "+1 −1") {
		t.Errorf("title %q", title)
	}
}

func TestPageHeaderIsCleanedToo(t *testing.T) {
	f := installWiki(t)
	old := fetchWikiPage
	fetchWikiPage = func(tg ghcli.RepoRef, file string) (ghcli.WikiContent, error) {
		c, err := old(tg, file)
		c.Page.File = "bad\x1b]0;pwned\x07.md"
		return c, err
	}
	t.Cleanup(func() { fetchWikiPage = old })
	_ = f
	m := wikiPageModel(t, "Home.md")
	if raw := m.View(); strings.Contains(raw, "pwned") || strings.Contains(raw, "\x1b]0") {
		t.Errorf("an escape in a file name reached the terminal: %q", raw)
	}
}

func TestEmptyWikiScreensSayHowToLeave(t *testing.T) {
	f := installWiki(t)
	for _, l := range []ghcli.WikiList{{State: ghcli.WikiOff}, {State: ghcli.WikiNoPages}, {State: ghcli.WikiReady}} {
		f.list = l
		if s := screenText(wikiListModel(t)); !strings.Contains(s, "Press 1-5 or 0 for another view, q to quit.") {
			t.Errorf("state %v does not say how to leave:\n%s", l.State, s)
		}
	}
	f.list = ghcli.WikiList{State: ghcli.WikiReady, Pages: wikiPages(), Total: 3}
	if s := screenText(wikiListModel(t)); strings.Contains(s, "Press 1-5") {
		t.Error("the hint belongs to the empty screens only")
	}
}
