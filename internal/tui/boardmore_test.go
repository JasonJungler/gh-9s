package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func moveFakes(t *testing.T) *[]string {
	t.Helper()
	var log []string
	old := moveItem
	moveItem = func(h, p, it, after string) error { log = append(log, it+" after "+after); return nil }
	t.Cleanup(func() { moveItem = old })
	return &log
}

func TestMarkWithSpaceShowsDotAndEscClears(t *testing.T) {
	m, _, _ := cardsBoard(t)
	m = keyAll(m, " ")
	if !strings.Contains(ansi.Strip(m.View()), "● acme/app #1") {
		t.Fatalf("no mark:\n%s", ansi.Strip(m.View()))
	}
	m = keyAll(m, "esc")
	if strings.Contains(ansi.Strip(m.View()), "●") || m.view != viewBoard {
		t.Fatalf("esc did not clear the mark (view %v)", m.view)
	}
	if m = keyAll(m, "esc"); m.view == viewBoard {
		t.Error("a second esc should leave the board")
	}
}

func TestMarkCapIsFifty(t *testing.T) {
	m, _, _ := cardsBoard(t)
	st := m.boardSt()
	for i := 0; i < maxMarked; i++ {
		st = st.withMarks("x"+string(rune('A'+i%26))+string(rune('a'+i/26)), true)
	}
	m = m.withScreenData(viewBoard, st)
	m = keyAll(m, " ")
	if !strings.Contains(ansi.Strip(m.status), "at most 50") {
		t.Errorf("status %q", m.status)
	}
}

func TestBulkStatusConfirmsOnceAndStopsAtTheFirstFailure(t *testing.T) {
	m, _, wf := cardsBoard(t)
	m = keyAll(m, " ", "j", " ", "j", " ")
	m = keyAll(m, "s")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "3 cards") {
		t.Fatalf("bulk status dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.status", true, "", "Done")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Move 3 cards to Done?") || !strings.Contains(m.dlg.detail, "Fix login") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	if len(wf.status) != 0 {
		t.Fatal("wrote before the confirmation")
	}
	n := 0
	setProjectStatus = func(h, p, it, f, o string) error {
		n++
		wf.status = append(wf.status, it)
		if n == 2 {
			return errors.New("boom")
		}
		return nil
	}
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	if len(wf.status) != 2 || !strings.Contains(ansi.Strip(m.status), "stopped after 1 of 3") {
		t.Fatalf("writes %v status %q", wf.status, m.status)
	}
	if len(m.boardSt().marks) == 0 {
		t.Error("marks cleared after a failed run")
	}
}

func TestSuccessfulBulkClearsMarks(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, " ", "j", " ", "x")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Remove 2 cards") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.remove.ok", true, "")
	m = runCmd(m, cmd)
	if len(cf.log) != 2 || len(m.boardSt().marks) != 0 {
		t.Fatalf("log %v marks %v", cf.log, m.boardSt().marks)
	}
}

func TestBulkAssignOnlyWithinOneRepoAndOnlyIssuesAndPRs(t *testing.T) {
	m, _, _ := cardsBoard(t)
	m = keyAll(m, " ", "j", " ", "j", "j", " ") // Fix login, Add retry, Idea (a draft)
	m = keyAll(m, "a")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "only issues and pull requests") {
		t.Fatalf("draft among the marked: dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestBulkAssignAddsPeopleToEveryCard(t *testing.T) {
	m, _, wf := cardsBoard(t)
	m = keyAll(m, " ", "j", " ", "a")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "2 cards") {
		t.Fatalf("dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.assignees", true, "", "bob")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Assign bob on 2 cards") {
		t.Fatalf("confirm: %+v status %q op %+v", m.dlg, m.status, m.boardOp)
	}
	m, c := answer(m, "board.assignees.ok", true, "")
	runCmd(m, c)
	if len(wf.assigned) != 2 || wf.assigned[0] != "is1+bob-" || wf.assigned[1] != "pr2+bob-" {
		t.Fatalf("assigned %v", wf.assigned)
	}
}

func TestReorderAsksThenMovesAfterTheRightCard(t *testing.T) {
	m, _, _ := cardsBoard(t)
	log := moveFakes(t)
	m = keyAll(m, "j", "j") // Old work (3rd)
	m = keyAll(m, "ctrl+up")
	if m.dlg == nil || m.dlg.id != "board.move.ok" || !strings.Contains(m.dlg.title, "Old work") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.move.ok", true, "")
	runCmd(m, cmd)
	if len(*log) != 1 || (*log)[0] != "it2 after it0" { // after the first card: ends up second
		t.Fatalf("log %v", *log)
	}
}

func TestReorderToTopAndDownAndEdges(t *testing.T) {
	m, _, _ := cardsBoard(t)
	log := moveFakes(t)
	m = keyAll(m, "j")
	m = keyAll(m, "ctrl+up") // second card to the top
	m, cmd := answer(m, "board.move.ok", true, "")
	runCmd(m, cmd)
	m2, _, _ := cardsBoard(t)
	m2 = keyAll(m2, "ctrl+down") // first card one down: after the second
	m2, cmd = answer(m2, "board.move.ok", true, "")
	runCmd(m2, cmd)
	m3, _, _ := cardsBoard(t)
	m3 = keyAll(m3, "ctrl+up") // already first: nothing
	if m3.dlg != nil {
		t.Error("asked to move the first card up")
	}
	if len(*log) != 2 || (*log)[0] != "it1 after " || (*log)[1] != "it0 after it1" {
		t.Fatalf("log %v", *log)
	}
}

func TestReorderRefusedInASortedView(t *testing.T) {
	m, _, _ := writeBoardWith(t, func(p *ghcli.Project) {
		p.Views = []ghcli.ProjectView{{Name: "By title", Layout: "TABLE_LAYOUT", SortField: "Title"}}
	})
	m = keyAll(m, "]", "ctrl+down")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "sorted") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestUndoStatusChange(t *testing.T) {
	m, _, wf := cardsBoard(t)
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	if m.undo == nil {
		t.Fatal("no undo record")
	}
	// the refreshed board shows the new Status
	st := m.boardSt()
	st.data.Items[0].Status = "Done"
	m = m.withScreenData(viewBoard, st)
	m = keyAll(m, "u")
	if m.dlg == nil || m.dlg.id != "board.undo.ok" || !strings.Contains(m.dlg.title, "back to Todo") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, cmd = answer(m, "board.undo.ok", true, "")
	m = runCmd(m, cmd)
	last := wf.status[len(wf.status)-1]
	if !strings.HasSuffix(last, "|o1") || m.undo != nil {
		t.Fatalf("last write %q undo %v", last, m.undo)
	}
}

func TestUndoRefusedWhenSomeoneChangedItSince(t *testing.T) {
	m, _, wf := cardsBoard(t)
	m = keyAll(m, "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	n := len(wf.status)
	st := m.boardSt()
	st.data.Items[0].Status = "In Progress" // someone else moved it again
	m = m.withScreenData(viewBoard, st)
	m = keyAll(m, "u")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "changed since") || len(wf.status) != n {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestUndoAnAddedCardRemovesIt(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "n")
	m, c := answer(m, "board.add", true, "acme/app#9")
	m = runCmd(m, c)
	m, c = answer(m, "board.add.ok", true, "")
	m = runCmd(m, c)
	m = keyAll(m, "u")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "New thing") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, c = answer(m, "board.undo.ok", true, "")
	runCmd(m, c)
	if len(cf.log) != 1 || cf.log[0] != "remove newitem" {
		t.Fatalf("log %v", cf.log)
	}
}

func TestRemoveAndArchiveCannotBeUndone(t *testing.T) {
	m, _, _ := cardsBoard(t)
	m = keyAll(m, "x")
	m, cmd := answer(m, "board.remove.ok", true, "")
	m = runCmd(m, cmd)
	m = keyAll(m, "u")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "cannot be undone from here") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestUndoFieldChangeRestoresTheOldValue(t *testing.T) {
	m, _, _ := writeBoardWith(t, func(p *ghcli.Project) {
		p.Fields = []ghcli.ProjectField{{ID: "F3", Name: "Notes", Type: "TEXT"}}
		p.Items[0].Values = map[string]string{"F3": "old"}
	})
	cf := installCards(t)
	var set []string
	setField = func(h, p, it, fld, typ string, v *ghcli.FieldValue) error {
		if v == nil {
			set = append(set, "clear")
		} else {
			set = append(set, v.Text)
		}
		return nil
	}
	_ = cf
	m = keyAll(m, "v")
	m, _ = answer(m, "board.field", true, "", "F3")
	m, _ = answer(m, "board.field.val", true, "new")
	m, cmd := answer(m, "board.field.ok", true, "")
	m = runCmd(m, cmd)
	st := m.boardSt()
	st.data.Items[0].Values = map[string]string{"F3": "new"}
	m = m.withScreenData(viewBoard, st)
	m = keyAll(m, "u")
	m, cmd = answer(m, "board.undo.ok", true, "")
	runCmd(m, cmd)
	if len(set) != 2 || set[1] != "old" {
		t.Fatalf("set %v", set)
	}
}

func TestNamesOfShowsThreeAndCountsTheRest(t *testing.T) {
	var items []ghcli.ProjectItem
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		items = append(items, ghcli.ProjectItem{Title: n})
	}
	if got := namesOf(items); got != "“a”, “b”, “c”, and 2 more" {
		t.Errorf("got %q", got)
	}
}

func TestBulkAssignRefusesCardsOfDifferentRepos(t *testing.T) {
	m, _, _ := writeBoardWith(t, func(p *ghcli.Project) { p.Items[1].Repo = "acme/other" })
	m = keyAll(m, " ", "j", " ", "a")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "different repos") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestSwitchingViewClearsMarks(t *testing.T) {
	m, _, _ := writeBoardWith(t, func(p *ghcli.Project) {
		p.Views = []ghcli.ProjectView{{Name: "Everything", Layout: "TABLE_LAYOUT"}}
	})
	m = keyAll(m, " ", "]")
	if len(m.boardSt().marks) != 0 {
		t.Errorf("marks survived a view switch: %v", m.boardSt().marks)
	}
}

func TestRefreshDropsMarksOfCardsThatAreGone(t *testing.T) {
	m, _, _ := writeBoard(t)
	m = keyAll(m, " ", "j", " ")
	p := writableProject()
	p.Items = p.Items[1:] // it0 is gone
	m = boardApply(m, screenMsg{view: viewBoard, id: projectKey(m.board), data: boardPayload{key: projectKey(m.board), p: p}})
	if marks := m.boardSt().marks; len(marks) != 1 || !marks["it1"] {
		t.Errorf("marks %v", marks)
	}
}

func TestBulkActsOnlyOnMarkedCardsOfTheTabOnScreen(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, " ")          // Fix login marked on All
	m = keyAll(m, "tab", "tab") // In Progress tab: Add retry only
	m = keyAll(m, "x")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Add retry") || strings.Contains(m.dlg.title, "cards") {
		t.Fatalf("acted on a card that is not on screen: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.remove.ok", true, "")
	runCmd(m, cmd)
	if len(cf.log) != 1 || cf.log[0] != "remove it1" {
		t.Fatalf("log %v", cf.log)
	}
}

func TestBulkRunEndsAtTheCardThatDoesNotAnswer(t *testing.T) {
	old := writeTimeout
	writeTimeout = 30 * 1e6 // 30ms
	t.Cleanup(func() { writeTimeout = old })
	m, _, wf := cardsBoard(t)
	calls := 0
	setProjectStatus = func(h, p, it, f, o string) error {
		calls++
		wf.status = append(wf.status, it)
		if calls == 1 {
			select {} // never answers
		}
		return nil
	}
	m = keyAll(m, " ", "j", " ", "j", " ", "s")
	m, _ = answer(m, "board.status", true, "", "Done")
	m, cmd := answer(m, "board.status.ok", true, "")
	m = runCmd(m, cmd)
	if len(wf.status) != 1 || !strings.Contains(ansi.Strip(m.status), "stopped after 0 of 3") {
		t.Fatalf("writes %v status %q", wf.status, m.status)
	}
}
