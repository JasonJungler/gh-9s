package tui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/pins"
)

type writeFakes struct {
	status   []string // projectID|itemID|fieldID|optionID
	added    []string
	assigned []string
	err      error
	content  ghcli.Content
	cerr     error
}

func writableProject() ghcli.Project {
	p := sampleProject()
	p.ID, p.StatusField = "P1", "F1"
	p.StatusIDs = map[string]string{"Todo": "o1", "In Progress": "o2", "Done": "o3"}
	for i := range p.Items {
		p.Items[i].ID = "it" + string(rune('0'+i))
	}
	return p
}

func installWrites(t *testing.T) *writeFakes {
	t.Helper()
	f := &writeFakes{content: ghcli.Content{ID: "C1", Title: "New thing", Kind: "Issue"}}
	oS, oR, oA, oP, oE, oL := setProjectStatus, resolveContent, addProjectItem, editPRAssignees, editIssue, listAssignees
	setProjectStatus = func(host, p, it, fld, opt string) error {
		f.status = append(f.status, p+"|"+it+"|"+fld+"|"+opt)
		return f.err
	}
	resolveContent = func(host, o, n string, num int) (ghcli.Content, error) { return f.content, f.cerr }
	addProjectItem = func(host, p, c string) (string, error) {
		f.added = append(f.added, p+"|"+c)
		return "newitem", f.err
	}
	editPRAssignees = func(r ghcli.RepoRef, n string, add, rm []string) error {
		f.assigned = append(f.assigned, "pr"+n+"+"+strings.Join(add, ",")+"-"+strings.Join(rm, ","))
		return f.err
	}
	editIssue = func(r ghcli.RepoRef, n string, a, b, add, rm []string) error {
		f.assigned = append(f.assigned, "is"+n+"+"+strings.Join(add, ",")+"-"+strings.Join(rm, ","))
		return f.err
	}
	listAssignees = func(ghcli.RepoRef) ([]string, error) { return []string{"ann", "bob"}, nil }
	t.Cleanup(func() {
		setProjectStatus, resolveContent, addProjectItem, editPRAssignees, editIssue, listAssignees = oS, oR, oA, oP, oE, oL
	})
	return f
}

func writeBoard(t *testing.T) (model, *boardFakes, *writeFakes) {
	t.Helper()
	f := installBoard(t)
	f.project = writableProject()
	wf := installWrites(t)
	m := pinsModel(t)
	m.previewOff = true
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Roadmap", URL: "https://github.com/orgs/acme/projects/3"})
	m = pressTab(pressTab(openFavs(t, m), "tab"), "tab")
	m = keyAll(m, "enter")
	return m, f, wf
}

// answer feeds a dialog result to the model the way the dialog would.
func answer(m model, id string, ok bool, text string, values ...string) (model, tea.Cmd) {
	m.dlg = nil // a dialog closes before it reports
	next, cmd := m.Update(dialogResultMsg{id: id, ok: ok, text: text, values: values})
	return next.(model), cmd
}

func runCmd(m model, cmd tea.Cmd) model {
	for cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.BatchMsg); ok {
			return m
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(model)
	}
	return m
}

func TestStatusChangeAsksThenWrites(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "s")
	if m.dlg == nil || m.dlg.id != "board.status" {
		t.Fatalf("s opened no status dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.status", true, "", "Done")
	if m.dlg == nil || m.dlg.id != "board.status.ok" || !strings.Contains(m.dlg.title, "Fix login") || !strings.Contains(m.dlg.title, "Done") {
		t.Fatalf("no confirm naming card and Status: %+v", m.dlg)
	}
	if len(wf.status) != 0 {
		t.Fatal("wrote before the confirmation")
	}
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	if len(wf.status) != 1 || wf.status[0] != "P1|it0|F1|o3" {
		t.Fatalf("write = %v", wf.status)
	}
	if !strings.Contains(ansi.Strip(m.status), "moved Fix login to Done") {
		t.Errorf("status = %q", m.status)
	}
}

func TestStatusCancelWritesNothing(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", false, "")
	runCmd(m, cmd)
	if len(wf.status) != 0 {
		t.Fatalf("wrote after no: %v", wf.status)
	}
}

func TestStatusMoveOtherProjectDropped(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "s")
	m.board.Number = 99 // the screen is another Project by the time the answer comes
	m, cmd := answer(m, "board.status", true, "", "Done")
	runCmd(m, cmd)
	if m.dlg != nil || len(wf.status) != 0 {
		t.Fatalf("stale answer acted: dlg=%v writes=%v", m.dlg, wf.status)
	}
}

func TestStatusOptionRemovedMeanwhile(t *testing.T) {
	m, f, wf := writeBoard(t)
	m = keyAll(m, "s")
	st := m.boardSt()
	delete(st.data.StatusIDs, "Done")
	m = m.withScreenData(viewBoard, st)
	_ = f
	m, _ = answer(m, "board.status", true, "", "Done")
	if m.dlg != nil || len(wf.status) != 0 || !strings.Contains(ansi.Strip(m.status), "not one of") {
		t.Fatalf("dlg=%v writes=%v status=%q", m.dlg, wf.status, m.status)
	}
}

func TestSecondWriteWhileBusyRefused(t *testing.T) {
	m, _, wf := writeBoard(t)
	m.boardBusy = true
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	runCmd(m, cmd)
	if len(wf.status) != 0 || !strings.Contains(ansi.Strip(m.status), "still working") {
		t.Fatalf("writes=%v status=%q", wf.status, m.status)
	}
}

func TestWriteFailureShownAndRefreshed(t *testing.T) {
	m, f, wf := writeBoard(t)
	wf.err = errors.New("boom")
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	before := len(f.calls)
	m = runCmd(m, cmd)
	if !strings.Contains(ansi.Strip(m.status), "boom") || m.boardBusy {
		t.Errorf("status=%q busy=%v", m.status, m.boardBusy)
	}
	if len(f.calls) <= before {
		t.Error("no refresh after a failed write")
	}
}

func TestMissingScopeShowsHint(t *testing.T) {
	m, _, wf := writeBoard(t)
	wf.err = ghcli.ErrNeedsProjectScope
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	if !strings.Contains(ansi.Strip(m.status), "gh auth refresh") {
		t.Errorf("status = %q", m.status)
	}
}

func TestSidewaysMoveInColumns(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "b")
	m = keyAll(m, "L")
	if m.dlg == nil || m.dlg.id != "board.status.ok" || !strings.Contains(m.dlg.title, "In Progress") {
		t.Fatalf("L: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.status.ok", true, "")
	runCmd(m, cmd)
	if len(wf.status) != 1 || wf.status[0] != "P1|it0|F1|o2" {
		t.Fatalf("write = %v", wf.status)
	}
}

func TestSidewaysOutsideColumnsExplains(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "L")
	if m.dlg != nil || len(wf.status) != 0 || !strings.Contains(ansi.Strip(m.status), "columns") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestAssigneesOnPRUsePRCommand(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "tab", "tab") // the In Progress tab: the PR
	m, cmd := m.startAssign()
	m = runCmd(m, cmd)
	if m.dlg == nil || m.dlg.id != "board.assignees" {
		t.Fatalf("no assignee dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.assignees", true, "", "bob")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "bob") {
		t.Fatalf("no confirm: %+v", m.dlg)
	}
	m, cmd = answer(m, "board.assignees.ok", true, "")
	runCmd(m, cmd)
	if len(wf.assigned) != 1 || wf.assigned[0] != "pr2+bob-ann" {
		t.Fatalf("assigned = %v", wf.assigned)
	}
}

func TestAssigneesNotForDrafts(t *testing.T) {
	m, _, _ := writeBoard(t)
	m = keyAll(m, "tab", "tab", "tab") // the draft card's tab, wherever it is
	m, cmd := m.startAssign()
	if cmd != nil && m.dlg != nil {
		t.Fatal("assignee dialog for a card that is not an issue or PR")
	}
}

func TestAddItemResolvesConfirmsThenAdds(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "n")
	if m.dlg == nil || m.dlg.id != "board.add" {
		t.Fatalf("no add dialog: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.add", true, "acme/app#9")
	m = runCmd(m, cmd)
	if m.dlg == nil || m.dlg.id != "board.add.ok" || !strings.Contains(m.dlg.title, "New thing") {
		t.Fatalf("no confirm: %+v", m.dlg)
	}
	if len(wf.added) != 0 {
		t.Fatal("added before the confirmation")
	}
	m, cmd = answer(m, "board.add.ok", true, "")
	runCmd(m, cmd)
	if len(wf.added) != 1 || wf.added[0] != "P1|C1" {
		t.Fatalf("added = %v", wf.added)
	}
}

func TestAddItemBadRefAndLookupFailure(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "n")
	m, _ = answer(m, "board.add", true, "--evil")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "OWNER/REPO#N") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
	wf.cerr = errors.New("not found")
	m = keyAll(m, "n")
	m, cmd := answer(m, "board.add", true, "acme/app#9")
	m = runCmd(m, cmd)
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "not found") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestParseContentRef(t *testing.T) {
	cases := []struct {
		in, host string
		ok       bool
	}{
		{"acme/app#9", "github.com", true},
		{"https://github.com/acme/app/pull/12", "github.com", true},
		{"https://github.com/acme/app/issues/12/", "github.com", true},
		{"https://ghe.corp/acme/app/issues/3", "ghe.corp", true},
		{"https://evil.com/acme/app/issues/3", "github.com", false},
		{"http://github.com/acme/app/issues/3", "github.com", false},
		{"-x/app#1", "github.com", false},
		{"acme/app", "github.com", false},
	}
	for _, c := range cases {
		_, _, _, err := parseContentRef(c.in, c.host)
		if (err == nil) != c.ok {
			t.Errorf("%q on %s: err=%v", c.in, c.host, err)
		}
	}
}

func TestWritesLiveOnlyInWriteFiles(t *testing.T) {
	for _, f := range []string{"board.go", "favprojects.go", "favtabs.go", "../ghcli/project.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), "mutation") {
			t.Errorf("%s holds a mutation; writes belong in the write files", f)
		}
	}
}

func TestConfirmAfterLeavingProjectWritesNothing(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m.board.Number = 99 // the board on screen is another Project when "yes" arrives
	m, cmd := answer(m, "board.status.ok", true, "")
	runCmd(m, cmd)
	if len(wf.status) != 0 {
		t.Fatalf("wrote to a Project that is no longer on screen: %v", wf.status)
	}
}

func TestAssigneesRefuseOddRepoNames(t *testing.T) {
	m, f, _ := writeBoard(t)
	f.project.Items[0].Repo = "-evil/app"
	m = keyAll(m, "R") // may be throttled; set the data directly as well
	st := m.boardSt()
	st.data.Items[0].Repo = "-evil/app"
	m = m.withScreenData(viewBoard, st)
	m = keyAll(m, "tab") // the Todo tab: Fix login
	m, cmd := m.startAssign()
	if cmd != nil || m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "will not use") {
		t.Fatalf("cmd=%v dlg=%v status=%q", cmd != nil, m.dlg, m.status)
	}
}

func TestSecondQuestionRefusedWhileConfirmAnswerPending(t *testing.T) {
	m, _, wf := writeBoard(t)
	m = keyAll(m, "b")
	m = keyAll(m, "L") // confirm for the first card
	m.dlg = nil        // y was pressed: the dialog is gone, the answer is queued
	m = keyAll(m, "L")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "answer the question first") {
		t.Fatalf("a second question opened: dlg=%v status=%q", m.dlg, m.status)
	}
	m, cmd := answer(m, "board.status.ok", true, "")
	runCmd(m, cmd)
	if len(wf.status) != 1 || wf.status[0] != "P1|it0|F1|o2" {
		t.Fatalf("the confirmed card was not the one written: %v", wf.status)
	}
}

func TestAddRefusedWhenAlreadyOnProject(t *testing.T) {
	m, _, wf := writeBoard(t)
	wf.content.OnProjects = []string{"P1"}
	m = keyAll(m, "n")
	m, cmd := answer(m, "board.add", true, "acme/app#9")
	m = runCmd(m, cmd)
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "already on this Project") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
	if len(wf.added) != 0 || len(wf.status) != 0 {
		t.Fatalf("wrote anyway: %v %v", wf.added, wf.status)
	}
}

func TestWriteTimeoutFreesTheBoard(t *testing.T) {
	old := writeTimeout
	writeTimeout = 20 * time.Millisecond
	t.Cleanup(func() { writeTimeout = old })
	m, _, wf := writeBoard(t)
	setProjectStatus = func(string, string, string, string, string) error { time.Sleep(500 * time.Millisecond); return nil }
	_ = wf
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	if m.boardBusy || !strings.Contains(ansi.Strip(m.status), "may or may not") {
		t.Fatalf("busy=%v status=%q", m.boardBusy, m.status)
	}
}
