package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type cardFakes struct {
	log []string
	err error
}

func installCards(t *testing.T) *cardFakes {
	t.Helper()
	f := &cardFakes{}
	oR, oA, oD, oE, oF := removeItem, archiveItem, addDraft, editDraft, setField
	removeItem = func(h, p, it string) error { f.log = append(f.log, "remove "+it); return f.err }
	archiveItem = func(h, p, it string) error { f.log = append(f.log, "archive "+it); return f.err }
	addDraft = func(h, p, title string) (string, error) {
		f.log = append(f.log, "draft "+title)
		return "newdraft", f.err
	}
	editDraft = func(h, d, title, body string) error {
		f.log = append(f.log, "edit "+d+"|"+title+"|"+body)
		return f.err
	}
	setField = func(h, p, it, fld, typ string, v *ghcli.FieldValue) error {
		s := "field " + it + "|" + fld + "|" + typ
		switch {
		case v == nil:
			s += "|clear"
		case typ == "NUMBER":
			s += "|" + strings.TrimRight(strings.TrimRight(strings.Replace(string(rune('0'+int(*v.Number))), "x", "", 1), "0"), ".")
		case v.Option != "":
			s += "|" + v.Option
		default:
			s += "|" + v.Text
		}
		f.log = append(f.log, s)
		return f.err
	}
	t.Cleanup(func() { removeItem, archiveItem, addDraft, editDraft, setField = oR, oA, oD, oE, oF })
	return f
}

func cardsBoard(t *testing.T) (model, *cardFakes, *writeFakes) {
	t.Helper()
	cf := installCards(t)
	m, _, wf := writeBoardWith(t, func(p *ghcli.Project) {
		p.Fields = []ghcli.ProjectField{
			{ID: "F1", Name: "Priority", Type: "SINGLE_SELECT", Options: []ghcli.FieldOption{{ID: "p1", Name: "High"}}},
			{ID: "F2", Name: "Size", Type: "NUMBER"},
			{ID: "F3", Name: "Notes", Type: "TEXT"},
		}
		p.Items[3].DraftID = "DR1"
		p.Items[3].Body = "old text"
	})
	return m, cf, wf
}

// gotoCard selects the card whose title is title on the All tab.
func gotoCard(t *testing.T, m model, title string) model {
	t.Helper()
	for i := 0; i < 8; i++ {
		if r, ok := m.boardSelectedRow(); ok && len(r.cells) > 1 && r.cells[1] == title {
			return m
		}
		m = keyAll(m, "j")
	}
	t.Fatalf("no card %q", title)
	return m
}

func TestRemoveAsksThenRemovesAndSaysWhatStays(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "x")
	if m.dlg == nil || m.dlg.id != "board.remove.ok" || !strings.Contains(m.dlg.title, "Fix login") || !strings.Contains(m.dlg.detail, "itself is not changed") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	if len(cf.log) != 0 {
		t.Fatal("wrote before the confirmation")
	}
	m, cmd := answer(m, "board.remove.ok", true, "")
	runCmd(m, cmd)
	if len(cf.log) != 1 || cf.log[0] != "remove it0" {
		t.Fatalf("log %v", cf.log)
	}
}

func TestRemovingADraftWarnsItIsGoneForGood(t *testing.T) {
	m, _, _ := cardsBoard(t)
	m = gotoCard(t, m, "Idea")
	m = keyAll(m, "x")
	if m.dlg == nil || !strings.Contains(m.dlg.detail, "cannot be restored") {
		t.Fatalf("no warning: %+v", m.dlg)
	}
}

func TestArchiveSaysItCannotBeUndoneHere(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "X")
	if m.dlg == nil || m.dlg.id != "board.archive.ok" || !strings.Contains(m.dlg.detail, "browser") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.archive.ok", true, "")
	runCmd(m, cmd)
	if len(cf.log) != 1 || cf.log[0] != "archive it0" {
		t.Fatalf("log %v", cf.log)
	}
}

func TestRemoveCancelAndStaleProjectWriteNothing(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "x")
	m2, cmd := answer(m, "board.remove.ok", false, "")
	runCmd(m2, cmd)
	m.board.Number = 99
	m3, cmd := answer(m, "board.remove.ok", true, "")
	runCmd(m3, cmd)
	if len(cf.log) != 0 {
		t.Fatalf("wrote: %v", cf.log)
	}
}

func TestNewDraftTakesTheColumnStatus(t *testing.T) {
	m, cf, wf := cardsBoard(t)
	m = keyAll(m, "tab", "tab") // In Progress tab
	m = keyAll(m, "N")
	if m.dlg == nil || m.dlg.id != "board.draft" {
		t.Fatalf("no title dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.draft", true, "Write docs")
	if m.dlg == nil || m.dlg.id != "board.draft.ok" || !strings.Contains(m.dlg.title, "Write docs") || !strings.Contains(m.dlg.title, "In Progress") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.draft.ok", true, "")
	runCmd(m, cmd)
	if len(cf.log) != 1 || cf.log[0] != "draft Write docs" || len(wf.status) != 1 || !strings.HasSuffix(wf.status[0], "|o2") || !strings.Contains(wf.status[0], "newdraft") {
		t.Fatalf("log %v status %v", cf.log, wf.status)
	}
}

func TestEditDraftOnlyOnDraftsAndOnlyWhenChanged(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "e")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "not a draft") {
		t.Fatalf("e on an issue: dlg=%v status=%q", m.dlg, m.status)
	}
	m = gotoCard(t, m, "Idea")
	m = keyAll(m, "e")
	if m.dlg == nil || m.dlg.id != "board.edit.title" || m.dlg.text != "Idea" {
		t.Fatalf("title dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.edit.title", true, "Idea")
	if m.dlg == nil || m.dlg.id != "board.edit.body" || m.dlg.text != "old text" {
		t.Fatalf("body dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.edit.body", true, "old text")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "no changes") {
		t.Fatalf("unchanged draft asked: dlg=%v status=%q", m.dlg, m.status)
	}
	m = keyAll(m, "e")
	m, _ = answer(m, "board.edit.title", true, "Better idea")
	m, _ = answer(m, "board.edit.body", true, "-")
	m, cmd := answer(m, "board.edit.ok", true, "")
	runCmd(m, cmd)
	if len(cf.log) != 1 || cf.log[0] != "edit DR1|Better idea|" {
		t.Fatalf("log %v", cf.log)
	}
}

func TestFieldChangeByType(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "v")
	if m.dlg == nil || m.dlg.id != "board.field" {
		t.Fatalf("no field dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.field", true, "", "F1")
	if m.dlg == nil || m.dlg.id != "board.field.val" || len(m.dlg.choices) != 2 {
		t.Fatalf("option dialog: %+v", m.dlg)
	}
	m, _ = answer(m, "board.field.val", true, "", "p1")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Priority") || !strings.Contains(m.dlg.title, "High") {
		t.Fatalf("confirm: %+v", m.dlg)
	}
	m, cmd := answer(m, "board.field.ok", true, "")
	m = runCmd(m, cmd)
	if len(cf.log) != 1 || cf.log[0] != "field it0|F1|SINGLE_SELECT|p1" {
		t.Fatalf("log %v", cf.log)
	}
	// text field: "-" clears
	m = keyAll(m, "v")
	m, _ = answer(m, "board.field", true, "", "F3")
	m, _ = answer(m, "board.field.val", true, "-")
	if m.dlg == nil || !strings.HasPrefix(m.dlg.title, "Clear Notes") {
		t.Fatalf("clear confirm: %+v", m.dlg)
	}
}

func TestFieldNumberMustBeANumber(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "v")
	m, _ = answer(m, "board.field", true, "", "F2")
	m, _ = answer(m, "board.field.val", true, "lots")
	if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "not a number") || len(cf.log) != 0 {
		t.Fatalf("dlg=%v status=%q log=%v", m.dlg, m.status, cf.log)
	}
}

func TestFieldOptionRemovedMeanwhile(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	m = keyAll(m, "v")
	m, _ = answer(m, "board.field", true, "", "F1")
	m, _ = answer(m, "board.field.val", true, "", "gone")
	if m.dlg != nil || len(cf.log) != 0 || !strings.Contains(ansi.Strip(m.status), "option is gone") {
		t.Fatalf("dlg=%v status=%q", m.dlg, m.status)
	}
}

func TestCardWriteFailureIsShown(t *testing.T) {
	m, cf, _ := cardsBoard(t)
	cf.err = errors.New("nope")
	m = keyAll(m, "x")
	m, cmd := answer(m, "board.remove.ok", true, "")
	m = runCmd(m, cmd)
	if !strings.Contains(ansi.Strip(m.status), "nope") {
		t.Errorf("status %q", m.status)
	}
}

func TestSecondQuestionRefusedForTheNewKeys(t *testing.T) {
	for _, key := range []string{"x", "X", "N", "e", "v"} {
		m, _, _ := cardsBoard(t)
		m = keyAll(m, "x")
		m.dlg = nil // y pressed, answer in flight
		m = keyAll(m, key)
		if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "answer the question first") {
			t.Errorf("key %s: dlg=%v status=%q", key, m.dlg, m.status)
		}
	}
}
