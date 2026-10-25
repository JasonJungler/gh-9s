package tui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type assignFakes struct {
	added, removed []string
	err            error
	content        ghcli.Content
	cands          []ghcli.ProjectRef
	lerr           error
}

func installAssign(t *testing.T) *assignFakes {
	t.Helper()
	f := &assignFakes{
		content: ghcli.Content{ID: "C1", Title: "Fix login", Kind: "Issue", OnProjects: []string{"PA"},
			Memberships: []ghcli.Membership{{ItemID: "IA", ProjectID: "PA", Title: "Roadmap"}}},
		cands: []ghcli.ProjectRef{{ID: "PA", Owner: "acme", Number: 1, Title: "Roadmap"}, {ID: "PB", Owner: "acme", Number: 2, Title: "Sprint"}, {ID: "PC", Owner: "acme", Number: 3, Title: "Old", Closed: true}},
	}
	oR, oL, oA, oD := resolveContent, listProjects, addProjectItem, removeFromProject
	resolveContent = func(h, o, n string, num int) (ghcli.Content, error) { return f.content, nil }
	listProjects = func(h string) ([]ghcli.ProjectRef, error) { return f.cands, f.lerr }
	addProjectItem = func(h, p, c string) (string, error) { f.added = append(f.added, p+"|"+c); return "new", f.err }
	removeFromProject = func(h, p, it string) error { f.removed = append(f.removed, p+"|"+it); return f.err }
	t.Cleanup(func() { resolveContent, listProjects, addProjectItem, removeFromProject = oR, oL, oA, oD })
	return f
}

func assignModel() model {
	m := model{width: 120, height: 40, view: viewIssues, target: ghcli.RepoRef{Owner: "acme", Name: "app"}, previewOff: true}
	m.tbl = table{cols: []string{"#"}, rows: []row{{id: "7", cells: []string{"7"}}}}
	return m
}

func TestProjectAssignAsksThenAddsAndRemoves(t *testing.T) {
	f := installAssign(t)
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	if m.dlg == nil || m.dlg.id != "proj.assign" || len(m.dlg.choices) != 2 || !m.dlg.checked["PA"] || m.dlg.checked["PB"] {
		t.Fatalf("dialog: %+v", m.dlg)
	}
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PB") // untick Roadmap, tick Sprint
	if m.dlg == nil || m.dlg.id != "proj.assign.ok" || !strings.Contains(m.dlg.title, "Add to acme/2 Sprint") || !strings.Contains(m.dlg.title, "remove from Roadmap") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	if len(f.added)+len(f.removed) != 0 {
		t.Fatal("wrote before the confirmation")
	}
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	if len(f.added) != 1 || f.added[0] != "PB|C1" || len(f.removed) != 1 || f.removed[0] != "PA|IA" {
		t.Fatalf("added %v removed %v", f.added, f.removed)
	}
	if !strings.Contains(ansi.Strip(m.status), "set the Status") || m.projBusy {
		t.Errorf("status %q busy %v", m.status, m.projBusy)
	}
}

func TestProjectAssignNoChangeAndCancel(t *testing.T) {
	f := installAssign(t)
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PA") // unchanged
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "no Project changes") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
	m, cmd = m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PB")
	m, cmd = answer(m, "proj.assign.ok", false, "")
	runCmd(m, cmd)
	if len(f.added)+len(f.removed) != 0 {
		t.Fatalf("wrote: %v %v", f.added, f.removed)
	}
}

func TestProjectAssignLateAnswerAfterLeavingIsDropped(t *testing.T) {
	installAssign(t)
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	_ = (&m).switchTo(viewRuns) // leaves; the pending op is forgotten
	m = runCmd(m, cmd)
	if m.dlg != nil {
		t.Fatalf("a dialog opened for an item that is no longer on screen: %+v", m.dlg)
	}
}

func TestProjectAssignFailureStopsAndShows(t *testing.T) {
	f := installAssign(t)
	f.err = errors.New("boom")
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PA", "PB")
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	if len(f.added) != 1 || !strings.Contains(ansi.Strip(m.status), "boom") || m.projBusy {
		t.Fatalf("added %v status %q busy %v", f.added, m.status, m.projBusy)
	}
}

func TestProjectAssignScopeHint(t *testing.T) {
	f := installAssign(t)
	f.err = ghcli.ErrNeedsProjectScope
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PA", "PB")
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	if !strings.Contains(ansi.Strip(m.status), "gh auth refresh") {
		t.Errorf("status %q", m.status)
	}
}

func TestProjectAssignRefusesBadNumbersAndSecondQuestion(t *testing.T) {
	installAssign(t)
	m := assignModel()
	for _, n := range []string{"", "x", "-3", "0"} {
		if _, cmd := m.startProjectAssign(n); cmd != nil {
			t.Errorf("number %q accepted", n)
		}
	}
	m.projOp = &projOp{confirming: true}
	if _, cmd := m.startProjectAssign("7"); cmd != nil {
		t.Error("a second question while one is being answered")
	}
}

func TestMOpensProjectsFromTheIssueListAndTheIssue(t *testing.T) {
	installAssign(t)
	m := assignModel()
	next, cmd, ok := issuesKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if !ok || cmd == nil || next.projOp == nil {
		t.Fatal("M on the issue list did nothing")
	}
	m.issue = "7"
	next, cmd, ok = issueKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if !ok || cmd == nil || next.projOp == nil {
		t.Fatal("M on the issue did nothing")
	}
	m.pr = "7"
	next, cmd, ok = prDetailKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if !ok || cmd == nil || next.projOp == nil {
		t.Fatal("M on the PR did nothing")
	}
	n2, cmd2, ok2 := m.prListKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if !ok2 || cmd2 == nil || n2.(model).projOp == nil {
		t.Fatal("M on the PR list did nothing")
	}
}

func TestProjectAssignLeavesAProjectThatTheListDoesNotShow(t *testing.T) {
	f := installAssign(t)
	f.content.Memberships = append(f.content.Memberships, ghcli.Membership{ItemID: "IX", ProjectID: "PX", Title: "Elsewhere"})
	f.content.OnProjects = append(f.content.OnProjects, "PX")
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	found := false
	for _, c := range m.dlg.choices {
		if c.value == "PX" {
			found = true
		}
	}
	if !found || !m.dlg.checked["PX"] {
		t.Fatalf("a Project the item is in is missing from the list: %+v", m.dlg.choices)
	}
}

func TestProjectAssignAnswerWithoutAConfirmingOpWritesNothing(t *testing.T) {
	f := installAssign(t)
	m := assignModel()
	m.projOp = &projOp{host: "", label: "acme/app#7", content: f.content, names: map[string]string{"PB": "B"}, add: []string{"PB"}} // never asked
	m, cmd := answer(m, "proj.assign.ok", true, "")
	runCmd(m, cmd)
	if len(f.added) != 0 {
		t.Fatalf("wrote without having asked: %v", f.added)
	}
}

func TestProjectAssignRefusesASecondWriteWhileOneRuns(t *testing.T) {
	f := installAssign(t)
	m := assignModel()
	m.projBusy = true
	m.projOp = &projOp{label: "acme/app#7", content: f.content, names: map[string]string{"PB": "B"}, add: []string{"PB"}, confirming: true}
	m, cmd := answer(m, "proj.assign.ok", true, "")
	runCmd(m, cmd)
	if len(f.added) != 0 || !strings.Contains(ansi.Strip(m.status), "still working") {
		t.Fatalf("added %v status %q", f.added, m.status)
	}
}

func TestProjectAssignUsesOneNamedHostForEveryCall(t *testing.T) {
	f := installAssign(t)
	var hosts []string
	resolveContent = func(h, o, n string, num int) (ghcli.Content, error) {
		hosts = append(hosts, "resolve:"+h)
		return f.content, nil
	}
	listProjects = func(h string) ([]ghcli.ProjectRef, error) { hosts = append(hosts, "list:"+h); return f.cands, nil }
	addProjectItem = func(h, p, c string) (string, error) { hosts = append(hosts, "add:"+h); return "x", nil }
	removeFromProject = func(h, p, it string) error { hosts = append(hosts, "remove:"+h); return nil }
	m := assignModel() // the repo carries no host
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PB")
	m, cmd = answer(m, "proj.assign.ok", true, "")
	runCmd(m, cmd)
	want := "resolve:github.com list:github.com add:github.com remove:github.com"
	if got := strings.Join(hosts, " "); got != want {
		t.Errorf("hosts %q, want %q", got, want)
	}
}

func TestProjectAssignStopsAtTheCallThatDoesNotAnswer(t *testing.T) {
	old := writeTimeout
	writeTimeout = 30 * 1e6
	t.Cleanup(func() { writeTimeout = old })
	f := installAssign(t)
	var removed atomic.Int32
	addProjectItem = func(h, p, c string) (string, error) { time.Sleep(80 * time.Millisecond); return "x", nil } // slower than one call may take
	removeFromProject = func(h, p, it string) error { removed.Add(1); return nil }
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PB") // add PB, remove PA
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	time.Sleep(200 * time.Millisecond) // a run that went on would have removed by now
	if removed.Load() != 0 || !strings.Contains(ansi.Strip(m.status), "no answer") || m.projBusy {
		t.Fatalf("removed %d status %q busy %v", removed.Load(), m.status, m.projBusy)
	}
	_ = f
}

func TestProjectAssignRefusedWhileAChangeRuns(t *testing.T) {
	installAssign(t)
	m := assignModel()
	m.projBusy = true
	m, cmd := m.startProjectAssign("7")
	if cmd != nil || m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "still working") {
		t.Fatalf("cmd %v dlg %v status %q", cmd != nil, m.dlg, m.status)
	}
}

func TestProjectAssignPartialFailureSaysHowManyWereDone(t *testing.T) {
	f := installAssign(t)
	m := assignModel()
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PB") // add Sprint, remove Roadmap
	removeFromProject = func(h, p, it string) error { return errors.New("boom") }
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	if len(f.added) != 1 || !strings.Contains(ansi.Strip(m.status), "1 of 2 changes made") {
		t.Fatalf("added %v status %q", f.added, m.status)
	}
}

func TestProjectAssignChangingTheBoardsProjectForcesARefetch(t *testing.T) {
	installAssign(t)
	m := assignModel()
	m = m.withScreenData(viewBoard, boardState{data: ghcli.Project{ID: "PB"}})
	m, cmd := m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PA", "PB")
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	if st, _ := m.screenData(viewBoard).(boardState); !st.force {
		t.Error("the board of a changed Project was not marked for a re-read")
	}
	// a change to another Project leaves it alone
	m = assignModel().withScreenData(viewBoard, boardState{data: ghcli.Project{ID: "OTHER"}})
	m, cmd = m.startProjectAssign("7")
	m = runCmd(m, cmd)
	m.dlg = nil
	m, _ = answer(m, "proj.assign", true, "", "PA", "PB")
	m, cmd = answer(m, "proj.assign.ok", true, "")
	m = runCmd(m, cmd)
	if st, _ := m.screenData(viewBoard).(boardState); st.force {
		t.Error("an unrelated board was marked")
	}
}
