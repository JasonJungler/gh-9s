package tui

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls the wiki screens make; tests replace them.
var (
	fetchWikiList  = func(tg ghcli.RepoRef) (ghcli.WikiList, error) { return ghcli.ListWiki(tg) }
	fetchWikiPage  = func(tg ghcli.RepoRef, file string) (ghcli.WikiContent, error) { return ghcli.ReadWiki(tg, file) }
	saveWikiPage   = ghcli.SaveWikiPage
	createWikiPage = ghcli.CreateWikiPage
	deleteWikiPage = ghcli.DeleteWikiPage
)

// wikiListState is what the list keeps: what was read.
type wikiListState struct{ l ghcli.WikiList }

// wikiPageState is what the page screen keeps. sel is the selected link, 1-based (0: none).
type wikiPageState struct {
	c      ghcli.WikiContent
	scroll int
	sel    int
}

// wikiEdit is the change being made, from the key press to the end: everything it needs is taken when the key is
// pressed, so it still means the page the user chose when the answer comes back.
type wikiEdit struct {
	kind                          string // "title" (asking for a new page's title), "save", "create", "delete"
	target                        ghcli.RepoRef
	file, title, blob, orig, text string
}

// wikiOpenedMsg is the page read (and the copy brought up to date) before the editor opens on it.
type wikiOpenedMsg struct {
	target ghcli.RepoRef
	file   string
	c      ghcli.WikiContent
	err    error
}

// wikiDoneMsg is the end of a save, a new page or a delete.
type wikiDoneMsg struct {
	target  ghcli.RepoRef
	file    string
	text    string
	deleted bool
	err     error
}

func init() {
	register(viewWiki, screen{
		name:     "wiki",
		identity: func(m model) string { return m.curTarget().String() + "|wiki" },
		fetch: func(m model) tea.Cmd {
			tg, id := m.curTarget(), m.curTarget().String()+"|wiki"
			return func() tea.Msg {
				l, err := fetchWikiList(tg)
				return screenMsg{view: viewWiki, id: id, data: l, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			l := msg.data.(ghcli.WikiList)
			m = m.withScreenData(viewWiki, wikiListState{l})
			m.tbl = wikiTable(m.curTarget(), l)
			if m.wikiSel != "" { // coming back from a page: stand on it again
				for i, r := range m.visible() {
					if r.id == m.wikiSel {
						m.cursor = i
					}
				}
				m.wikiSel = ""
			}
			if n := len(m.visible()); m.cursor >= n {
				m.cursor = max(0, n-1)
			}
			if l.Total > len(l.Pages) {
				m.status = styDim.Render(fmt.Sprintf("showing the first %d of %d pages", len(l.Pages), l.Total))
			}
			return m
		},
		key:    wikiListKey,
		body:   wikiListBody,
		footer: "enter read  e edit  n new page  x delete  o browser  / filter titles",
	})
	register(viewWikiPage, screen{
		name:     "wiki page",
		identity: func(m model) string { return m.curTarget().String() + "|wiki|" + m.wikiPage },
		fetch: func(m model) tea.Cmd {
			tg, file, id := m.curTarget(), m.wikiPage, m.curTarget().String()+"|wiki|"+m.wikiPage
			return func() tea.Msg {
				c, err := fetchWikiPage(tg, file)
				return screenMsg{view: viewWikiPage, id: id, data: c, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			old, _ := m.screenData(viewWikiPage).(wikiPageState)
			c := msg.data.(ghcli.WikiContent)
			if links := wikiLinks(c, ghcli.CleanText(c.Text)); old.sel > len(links) {
				old.sel = 0
			}
			return m.withScreenData(viewWikiPage, wikiPageState{c: c, scroll: old.scroll, sel: old.sel})
		},
		key:    wikiPageKey,
		body:   wikiPageBody,
		footer: "e edit  x delete  tab/shift+tab links  enter follow  o browser  j/k scroll  esc back",
	})
	dialogHandlers["wiki.title"] = wikiTitleAnswered
	dialogHandlers["wiki.save"] = wikiSaveAnswered
	dialogHandlers["wiki.delete"] = wikiDeleteAnswered
}

// wikiTable lays the list out.
func wikiTable(tg ghcli.RepoRef, l ghcli.WikiList) table {
	t := table{cols: []string{"PAGE", "FILE", "UPDATED", "AUTHOR"}}
	for _, p := range l.Pages {
		st, title, upd := stOK, p.Title, ""
		if p.Link {
			st, title = stOther, title+" [link]"
		}
		if !p.Updated.IsZero() {
			upd = p.Updated.Format(time.RFC3339)
		}
		t.rows = append(t.rows, row{id: p.File, state: st, url: ghcli.WikiURL(tg, p.File), updated: upd,
			cells: []string{title, p.File, orDash(age(upd)), orDash(p.Author)}})
	}
	return t
}

func wikiListBody(m model) string {
	if st, ok := m.screenData(viewWiki).(wikiListState); ok && !m.loading && m.err == nil && m.curTarget().Name != "" {
		switch {
		case st.l.State == ghcli.WikiOff:
			return " The wiki is turned off for " + displayRef(m.curTarget()) + "\n"
		case st.l.State == ghcli.WikiNoPages:
			return " " + displayRef(m.curTarget()) + " has no wiki pages yet.\n GitHub creates a wiki with its first page, which has to be made in the browser (o).\n"
		case len(st.l.Pages) == 0:
			return " The wiki has no pages (n creates one)\n"
		}
	}
	return m.tableBody()
}

// wikiBusyNote refuses a second wiki change while one is on its way.
func (m model) wikiBusyNote() (model, bool) {
	if m.wikiBusy {
		m.status = styDim.Render("a wiki change is still in progress")
		return m, true
	}
	return m, false
}

func wikiListKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	switch k.String() {
	case "R", "ctrl+r":
		if next, busy := m.wikiBusyNote(); busy {
			return next, nil, true
		}
		return m, nil, false
	case "enter":
		r, ok := m.selected()
		if !ok {
			return m, nil, true
		}
		m.wikiPage, m.wikiSel, m.wikiTrail = r.id, r.id, nil
		m = m.withScreenData(viewWikiPage, wikiPageState{})
		cmd := m.push(viewWikiPage)
		return m, cmd, true
	case "e":
		if r, ok := m.selected(); ok {
			next, cmd := m.startWikiEdit(r.id)
			return next, cmd, true
		}
		return m, nil, true
	case "x":
		if r, ok := m.selected(); ok {
			st, _ := m.screenData(viewWiki).(wikiListState)
			for _, p := range st.l.Pages {
				if p.File == r.id {
					next, cmd := m.startWikiDelete(p.File, p.Title, p.Blob)
					return next, cmd, true
				}
			}
		}
		return m, nil, true
	case "n":
		next, cmd := m.startWikiNew()
		return next, cmd, true
	case "o":
		if r, ok := m.selected(); ok && r.url != "" {
			m = m.open(r.url)
		} else if tg := m.curTarget(); tg.Name != "" {
			m = m.open(ghcli.WikiURL(tg, ""))
		}
		return m, nil, true
	}
	return m, nil, false
}

// ---- changing pages -----------------------------------------------------------------------------------------------

// startWikiEdit reads the page afresh (bringing the local copy up to date) and then opens the editor on it.
func (m model) startWikiEdit(file string) (model, tea.Cmd) {
	if next, busy := m.wikiBusyNote(); busy {
		return next, nil
	}
	m.wikiBusy = true
	m.status = styDim.Render("reading the page…")
	tg := m.curTarget()
	return m, func() tea.Msg {
		c, err := fetchWikiPage(tg, file)
		return wikiOpenedMsg{target: tg, file: file, c: c, err: err}
	}
}

func (m model) wikiOpened(msg wikiOpenedMsg) (model, tea.Cmd) {
	if msg.err != nil {
		m.wikiBusy = false
		m.status = styErr.Render("✗ " + msg.err.Error())
		return m, nil
	}
	if msg.target != m.curTarget() || (m.view != viewWiki && m.view != viewWikiPage) {
		m.wikiBusy = false // the user went elsewhere while the page was read
		m.status = ""
		return m, nil
	}
	m.status = ""
	m.wikiEdit = &wikiEdit{kind: "save", target: msg.target, file: msg.file, title: msg.c.Page.Title, blob: msg.c.Page.Blob, orig: msg.c.Text}
	return m, editorCmd(msg.c.Text)
}

func sameText(a, b string) bool {
	return strings.TrimRight(strings.ReplaceAll(a, "\r\n", "\n"), "\n") == strings.TrimRight(strings.ReplaceAll(b, "\r\n", "\n"), "\n")
}

// wikiEdited is the editor closing: nothing to save, or one question that sends the change.
func (m model) wikiEdited(msg editedMsg) (model, tea.Cmd) {
	e := m.wikiEdit
	end := func(status string) (model, tea.Cmd) {
		m.wikiEdit, m.wikiBusy = nil, false
		m.status = status
		return m, nil
	}
	if msg.err != nil {
		return end(styErr.Render("✗ the editor failed: " + msg.err.Error()))
	}
	if msg.text == "" || strings.TrimSpace(msg.text) == "" {
		if e.kind == "create" {
			return end(styDim.Render("nothing written — no page was created"))
		}
		return end(styDim.Render("a page cannot be emptied here; x deletes it"))
	}
	if e.kind == "save" && sameText(msg.text, e.orig) {
		return end(styDim.Render("no changes to save"))
	}
	e2 := *e
	e2.text = msg.text
	m.wikiEdit = &e2
	add, del := ghcli.WikiLineChange(e.orig, msg.text)
	head := fmt.Sprintf("Save %s to %s wiki  (+%d −%d lines)", e.title, displayRef(e.target), add, del)
	msgText := "Update " + e.title
	if e.kind == "create" {
		head = fmt.Sprintf("Create %s in %s wiki  (%d lines)", e.title, displayRef(e.target), add)
		msgText = "Create " + e.title
	}
	d := newTextDialog("wiki.save", head, "Commit message — sending this saves the change to GitHub right away")
	d.text = msgText
	return m.withDialog(d), nil
}

func wikiSaveAnswered(m model, r dialogResultMsg) (model, tea.Cmd) {
	e := m.wikiEdit
	if !r.ok || e == nil || (e.kind != "save" && e.kind != "create") {
		m.wikiEdit, m.wikiBusy = nil, false
		return m, nil
	}
	m.wikiEdit = nil // busy stays until the answer comes back
	message := strings.TrimSpace(r.text)
	e2 := *e
	return m, func() tea.Msg {
		var err error
		text := "saved " + e2.title + " to the wiki"
		if e2.kind == "create" {
			err = createWikiPage(e2.target, e2.file, e2.text, message)
			text = "created " + e2.title + " in the wiki"
		} else {
			err = saveWikiPage(e2.target, e2.file, e2.blob, e2.text, message)
		}
		return wikiDoneMsg{target: e2.target, file: e2.file, text: text, err: err}
	}
}

// startWikiNew asks for the title of a new page.
func (m model) startWikiNew() (model, tea.Cmd) {
	if next, busy := m.wikiBusyNote(); busy {
		return next, nil
	}
	st, _ := m.screenData(viewWiki).(wikiListState)
	switch {
	case m.curTarget().Name == "" || m.loading:
		m.status = styDim.Render("still loading the wiki")
		return m, nil
	case st.l.State != ghcli.WikiReady:
		m.status = styDim.Render("there is no wiki to add to: the first page has to be made in the browser (o)")
		return m, nil
	}
	m.wikiBusy = true
	m.wikiEdit = &wikiEdit{kind: "title", target: m.curTarget()}
	return m.withDialog(wikiTitleDialog("", "")), nil
}

func wikiTitleDialog(text, err string) dialog {
	d := newTextDialog("wiki.title", "New wiki page", "Title of the page (a space, not -; no / \\ : * ? \" < > |)")
	d.text, d.err = text, err
	return d
}

func wikiTitleAnswered(m model, r dialogResultMsg) (model, tea.Cmd) {
	e := m.wikiEdit
	if !r.ok || e == nil || e.kind != "title" {
		m.wikiEdit, m.wikiBusy = nil, false
		return m, nil
	}
	again := func(why string) (model, tea.Cmd) { return m.withDialog(wikiTitleDialog(r.text, why)), nil }
	file, err := ghcli.WikiFileForTitle(r.text)
	if err != nil {
		return again(err.Error())
	}
	st, _ := m.screenData(viewWiki).(wikiListState)
	for _, p := range st.l.Pages {
		if strings.EqualFold(ghcli.WikiTitle(p.File), ghcli.WikiTitle(file)) {
			return again("a page called " + p.Title + " already exists")
		}
	}
	e2 := *e
	e2.kind, e2.file, e2.title = "create", file, ghcli.WikiTitle(file)
	m.wikiEdit = &e2
	return m, editorCmd("")
}

// startWikiDelete asks before removing a page.
func (m model) startWikiDelete(file, title, blob string) (model, tea.Cmd) {
	if next, busy := m.wikiBusyNote(); busy {
		return next, nil
	}
	if blob == "" {
		m.status = styDim.Render("still loading the page")
		return m, nil
	}
	m.wikiBusy = true
	m.wikiEdit = &wikiEdit{kind: "delete", target: m.curTarget(), file: file, title: title, blob: blob}
	detail := "It is removed for everyone; only the wiki's git history keeps it.\n This is committed and pushed to GitHub right away."
	if strings.EqualFold(title, "home") {
		detail = "This is the wiki's front page: the wiki has no Home until one is created.\n " + detail
	}
	return m.withDialog(newConfirmDialog("wiki.delete", fmt.Sprintf("Delete the page %q from the %s wiki?", title, displayRef(m.curTarget())), detail)), nil
}

func wikiDeleteAnswered(m model, r dialogResultMsg) (model, tea.Cmd) {
	e := m.wikiEdit
	m.wikiEdit = nil
	if !r.ok || e == nil || e.kind != "delete" {
		m.wikiBusy = false
		return m, nil
	}
	e2 := *e
	return m, func() tea.Msg {
		err := deleteWikiPage(e2.target, e2.file, e2.blob, "Delete "+e2.title)
		return wikiDoneMsg{target: e2.target, file: e2.file, text: "deleted " + e2.title + " from the wiki", deleted: true, err: err}
	}
}

// wikiDone is the end of a change: say what happened. A failure is shown in full in a box, because it may name the
// file that holds the text the user wrote.
func (m model) wikiDone(msg wikiDoneMsg) (model, tea.Cmd) {
	m.wikiBusy = false
	if msg.err != nil {
		m.status = styErr.Render("✗ " + firstSentence(msg.err.Error()))
		return m.withDialog(newConfirmDialog("wiki.failed", "The wiki was not changed", strings.Join(wrap(msg.err.Error(), max(min(m.width-6, 76), 20)), "\n "))), nil
	}
	m.status = styOK.Render("✓ " + msg.text)
	if msg.target != m.curTarget() {
		return m, nil
	}
	if msg.deleted && m.view == viewWikiPage && m.wikiPage == msg.file {
		m.wikiTrail = nil
		if next, cmd, ok := m.popBack(); ok {
			return next, cmd
		}
	}
	if m.view == viewWiki || m.view == viewWikiPage {
		return m, m.fetch()
	}
	return m, nil
}

func firstSentence(s string) string {
	if i := strings.IndexAny(s, ";\n"); i > 0 {
		s = s[:i]
	}
	return s
}

// ---- one page -----------------------------------------------------------------------------------------------------

// wikiLink is a link in a page that may lead to another page.
type wikiLink struct {
	label, name, file string // file is "" when the wiki has no such page
}

var (
	wikiBracketLink = regexp.MustCompile(`\[\[([^\[\]|]+?)(?:\|([^\[\]]+?))?\]\]`)
	wikiMarkdownRef = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)
	wikiScheme      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// wikiNorm makes page names comparable: no format, dashes as spaces, no case.
func wikiNorm(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		return "\x00" + s // a path: not a page of this wiki
	}
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(ghcli.WikiTitle(s), "-", " ")), " "))
}

// wikiLinks finds the links to other pages in text: [[Page]], [[Text|Page]] and relative Markdown links, in order,
// skipping fenced code. Each is matched with a page of the wiki by title.
func wikiLinks(c ghcli.WikiContent, text string) []wikiLink {
	if c.Plain {
		return nil
	}
	byTitle := map[string]string{}
	for _, p := range c.Pages {
		byTitle[wikiNorm(p.File)] = p.File
	}
	var out []wikiLink
	seen := map[string]bool{}
	add := func(label, name string) {
		label, name = strings.TrimSpace(label), strings.TrimSpace(name)
		if name == "" {
			return
		}
		if label == "" {
			label = name
		}
		key := label + "\x00" + name
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, wikiLink{label: label, name: name, file: byTitle[wikiNorm(name)]})
	}
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range wikiBracketLink.FindAllStringSubmatch(line, -1) {
			if m[2] != "" {
				add(m[1], m[2])
			} else {
				add(m[1], m[1])
			}
		}
		for _, m := range wikiMarkdownRef.FindAllStringSubmatch(line, -1) {
			t := m[2]
			if wikiScheme.MatchString(t) || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
				continue
			}
			if i := strings.IndexAny(t, "#?"); i >= 0 {
				t = t[:i]
			}
			if u, err := url.PathUnescape(t); err == nil {
				t = u
			}
			add(m[1], t)
		}
	}
	return out
}

type wikiDoc struct {
	lines  []string
	linkAt []int // the line of each link in the Links section
	links  []wikiLink
}

func wikiPageDoc(tg ghcli.RepoRef, st wikiPageState, width int) wikiDoc {
	var d wikiDoc
	c := st.c
	if c.Page.File == "" {
		d.lines = []string{"loading…"}
		return d
	}
	var doc detailDoc
	addWrapped(&doc, c.Page.Title, width, bold)
	doc.add(ui.Dim.Render(c.Page.File + " · " + displayRef(tg) + " wiki"))
	text := ghcli.CleanText(c.Text)
	doc.add("")
	if c.Plain {
		doc.add(ui.Dim.Render("(not Markdown: shown as plain text)"), "")
		for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			if l == "" {
				doc.add("")
				continue
			}
			doc.add(wrap(l, width)...)
		}
	} else {
		doc.addMarkdown(text, width, "")
	}
	d.lines = doc.lines
	d.links = wikiLinks(c, text)
	if len(d.links) > 0 {
		d.lines = append(d.lines, "", ui.Dim.Render("── Links (tab selects, enter follows) ──"))
		for i, l := range d.links {
			line := "  " + l.label
			if l.name != l.label {
				line += " → " + l.name
			}
			switch {
			case l.file == "":
				line = ui.Dim.Render(line + "   (no such page)")
			case st.sel == i+1:
				line = ui.Key.Render(line)
			}
			d.linkAt = append(d.linkAt, len(d.lines))
			d.lines = append(d.lines, line)
		}
	}
	return d
}

func wikiPageBody(m model) string {
	st, _ := m.screenData(viewWikiPage).(wikiPageState)
	h := m.logHeight()
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.Err.Render(" "+m.err.Error()) + "\n")
		h--
	}
	doc := wikiPageDoc(m.curTarget(), st, m.contentWidth())
	n := 0
	for i := min(st.scroll, max(0, len(doc.lines)-1)); i < len(doc.lines) && n < h; i++ {
		margin := " "
		if st.sel > 0 && st.sel <= len(doc.linkAt) && i == doc.linkAt[st.sel-1] {
			margin = ui.Key.Render("▌")
		}
		b.WriteString(margin + doc.lines[i] + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

func wikiPageKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewWikiPage).(wikiPageState)
	h := m.logHeight()
	doc := wikiPageDoc(m.curTarget(), st, m.contentWidth())
	maxScroll := max(0, len(doc.lines)-h)
	switch k.String() {
	case "j", "down":
		st.scroll = min(st.scroll+1, maxScroll)
	case "k", "up":
		st.scroll = max(st.scroll-1, 0)
	case "ctrl+d", "pgdown", " ":
		st.scroll = min(st.scroll+h/2, maxScroll)
	case "ctrl+u", "pgup":
		st.scroll = max(st.scroll-h/2, 0)
	case "g", "home":
		st.scroll = 0
	case "G", "end":
		st.scroll = maxScroll
	case "tab", "shift+tab":
		if len(doc.links) == 0 {
			m.status = styDim.Render("no links to other pages here")
			return m, nil, true
		}
		switch {
		case k.String() == "tab":
			st.sel = min(st.sel+1, len(doc.links))
		case st.sel == 0:
			st.sel = len(doc.links)
		default:
			st.sel = max(st.sel-1, 1)
		}
		if at := doc.linkAt[st.sel-1]; at < st.scroll || at >= st.scroll+h {
			st.scroll = min(max(at-h/2, 0), maxScroll)
		}
	case "enter":
		if st.sel == 0 || st.sel > len(doc.links) {
			return m, nil, true
		}
		l := doc.links[st.sel-1]
		if l.file == "" {
			m.status = styDim.Render("no such page: " + l.name)
			return m, nil, true
		}
		m.wikiTrail = append(m.wikiTrail, m.wikiPage)
		m.wikiPage = l.file
		m = m.withScreenData(viewWikiPage, wikiPageState{})
		m.loading, m.err = true, nil
		return m, m.fetch(), true
	case "esc", "backspace":
		if n := len(m.wikiTrail); n > 0 { // back along the links followed
			m.wikiPage, m.wikiTrail = m.wikiTrail[n-1], m.wikiTrail[:n-1]
			m = m.withScreenData(viewWikiPage, wikiPageState{})
			m.loading, m.err = true, nil
			return m, m.fetch(), true
		}
		return m, nil, false
	case "e":
		if st.c.Page.File == "" {
			m.status = styDim.Render("still loading the page")
			return m, nil, true
		}
		next, cmd := m.startWikiEdit(m.wikiPage)
		return next, cmd, true
	case "x":
		next, cmd := m.startWikiDelete(m.wikiPage, st.c.Page.Title, st.c.Page.Blob)
		return next, cmd, true
	case "o":
		m = m.open(ghcli.WikiURL(m.curTarget(), m.wikiPage))
		return m, nil, true
	case "R", "ctrl+r":
		if next, busy := m.wikiBusyNote(); busy {
			return next, nil, true
		}
		return m, nil, false
	default:
		return m, nil, false
	}
	return m.withScreenData(viewWikiPage, st), nil, true
}
