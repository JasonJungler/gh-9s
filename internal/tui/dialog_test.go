package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// dlgModel is a model with the dialog d open and a handler recording what the dialog returned.
func dlgModel(t *testing.T, d dialog) (model, *[]dialogResultMsg) {
	t.Helper()
	var got []dialogResultMsg
	dialogHandlers[d.id] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		got = append(got, r)
		return m, nil
	}
	t.Cleanup(func() { delete(dialogHandlers, d.id) })
	m := model{view: viewRuns, width: 100, height: 30}
	m.dlg = &d
	return m, &got
}

// drive presses keys, delivering any dialog result message straight back into the model.
func drive(m model, keysToPress ...string) model {
	for _, key := range keysToPress {
		next, c := m.Update(k(key))
		m = next.(model)
		if c != nil {
			if msg := c(); msg != nil {
				if _, isResult := msg.(dialogResultMsg); isResult {
					next, _ = m.Update(msg)
					m = next.(model)
				}
			}
		}
	}
	return m
}

func typeKeys(m model, s string) model {
	for _, r := range s {
		if r == ' ' {
			m = drive(m, "space")
		} else {
			m = drive(m, string(r))
		}
	}
	return m
}

var fruit = []choice{
	{label: "apple", value: "a"}, {label: "banana", value: "b"}, {label: "cherry", value: "c"},
}

func TestTextDialogCollectsAndSubmits(t *testing.T) {
	m, got := dlgModel(t, newTextDialog("t", "Comment on acme/api#7", "Your comment"))
	m = typeKeys(m, "hello world")
	m = drive(m, "enter")
	if m.dlg != nil || len(*got) != 1 || !(*got)[0].ok || (*got)[0].text != "hello world" {
		t.Errorf("dlg open=%v results=%+v; want one ok result 'hello world'", m.dlg != nil, *got)
	}
}

func TestTextDialogEmptyTextCannotSubmit(t *testing.T) {
	m, got := dlgModel(t, newTextDialog("t", "Comment", "text"))
	m = drive(m, "enter")
	m = typeKeys(m, "  ")
	m = drive(m, "enter")
	if m.dlg == nil || len(*got) != 0 || m.dlg.err == "" {
		t.Errorf("empty or blank text must keep the dialog open with a message: open=%v results=%v err=%q", m.dlg != nil, *got, m.dlg.err)
	}
}

func TestTextDialogEditorTextBecomesTheBody(t *testing.T) {
	old := editorCmd
	editorCmd = func(initial string) tea.Cmd {
		return func() tea.Msg { return editedMsg{text: initial + "line1\nline2\n"} }
	}
	t.Cleanup(func() { editorCmd = old })
	m, got := dlgModel(t, newTextDialog("t", "Comment", "text"))
	next, c := m.Update(k("ctrl+e"))
	m = next.(model)
	if c == nil {
		t.Fatal("ctrl+e should start the editor")
	}
	next, _ = m.Update(c())
	m = next.(model)
	if m.dlg.text != "line1\nline2" {
		t.Errorf("text = %q; want the edited text without the trailing newline", m.dlg.text)
	}
	m = drive(m, "enter")
	if len(*got) != 1 || (*got)[0].text != "line1\nline2" {
		t.Errorf("results = %+v; multi-line text must survive", *got)
	}
}

func TestChoiceDialogFiltersAndSelectsOne(t *testing.T) {
	m, got := dlgModel(t, newChoiceDialog("c", "Pick", fruit, false, nil))
	m = typeKeys(m, "an")
	if v := m.View(); !strings.Contains(v, "banana") || strings.Contains(v, "apple") || strings.Contains(v, "cherry") {
		t.Errorf("filter 'an' should leave only banana:\n%s", v)
	}
	m = drive(m, "enter")
	if m.dlg != nil || len(*got) != 1 || !reflect.DeepEqual((*got)[0].values, []string{"b"}) {
		t.Errorf("results = %+v; want banana", *got)
	}
}

func TestChoiceDialogMultiTogglesAndSkipsDisabled(t *testing.T) {
	cs := []choice{{label: "apple", value: "a"}, {label: "banana", value: "b"}, {label: "cherry", value: "c", disabled: true, note: "not allowed"}}
	m, got := dlgModel(t, newChoiceDialog("c", "Pick many", cs, true, []string{"a"}))
	m = drive(m, "down", "space") // banana on
	m = drive(m, "down", "space") // cherry is disabled: nothing happens
	m = drive(m, "enter")
	if len(*got) != 1 || !reflect.DeepEqual((*got)[0].values, []string{"a", "b"}) {
		t.Errorf("results = %+v; want [a b] in the original order, without the disabled one", *got)
	}
}

func TestChoiceDialogSingleCannotPickADisabledChoice(t *testing.T) {
	cs := []choice{{label: "squash", value: "s", disabled: true, note: "not allowed here"}}
	m, got := dlgModel(t, newChoiceDialog("c", "Method", cs, false, nil))
	m = drive(m, "enter")
	if m.dlg == nil || len(*got) != 0 || !strings.Contains(m.dlg.err, "not allowed here") {
		t.Errorf("open=%v results=%v err=%q; a disabled choice must be refused with its reason", m.dlg != nil, *got, m.dlg.err)
	}
}

func TestChoiceDialogWithNothingToChooseSaysSo(t *testing.T) {
	m, _ := dlgModel(t, newChoiceDialog("c", "Labels", nil, true, nil))
	if v := m.View(); !strings.Contains(v, "nothing to choose") {
		t.Errorf("an empty list should say so:\n%s", v)
	}
	m, _ = dlgModel(t, newChoiceDialog("c2", "Labels", fruit, false, nil))
	m = typeKeys(m, "zzz")
	if v := m.View(); !strings.Contains(v, "nothing to choose") {
		t.Errorf("a filter that matches nothing should say so:\n%s", v)
	}
}

func TestConfirmDialogOnlyYConfirms(t *testing.T) {
	for key, want := range map[string]bool{"y": true, "Y": true, "n": false, "enter": false, "esc": false, "x": false} {
		m, got := dlgModel(t, newConfirmDialog("k", "Merge acme/api#7?", "squash"))
		m = drive(m, key)
		if m.dlg != nil || len(*got) != 1 || (*got)[0].ok != want {
			t.Errorf("key %q: open=%v results=%+v; want ok=%v", key, m.dlg != nil, *got, want)
		}
	}
}

func TestEscCancelsAnyDialog(t *testing.T) {
	for name, d := range map[string]dialog{
		"text": newTextDialog("e", "t", "p"), "choice": newChoiceDialog("e", "t", fruit, false, nil), "confirm": newConfirmDialog("e", "t", "d"),
	} {
		m, got := dlgModel(t, d)
		m = drive(m, "esc")
		if m.dlg != nil || len(*got) != 1 || (*got)[0].ok {
			t.Errorf("%s: open=%v results=%+v; want one cancelled result", name, m.dlg != nil, *got)
		}
	}
}

func TestDialogTakesEveryKeyWhileOpen(t *testing.T) {
	m, _ := dlgModel(t, newTextDialog("t", "Comment", "text"))
	m = typeKeys(m, "q:1")
	if m.view != viewRuns || m.dlg == nil || m.dlg.text != "q:1" {
		t.Errorf("view=%v text=%q; keys like q, : and 1 belong to the dialog while it is open", m.view, m.dlg.text)
	}
}

func TestDialogResultWithoutAHandlerIsHarmless(t *testing.T) {
	m := model{view: viewRuns}
	next, cmd := m.Update(dialogResultMsg{id: "nobody", ok: true})
	if cmd != nil || next.(model).view != viewRuns {
		t.Error("an unknown dialog id should be ignored")
	}
}

func TestConfirmToggleKeyFlipsAndReportsTheFinalState(t *testing.T) {
	d := newConfirmDialog("k", "Merge?", "detail")
	d.toggleLabel, d.toggleKey = "Delete the branch", "d"
	m, got := dlgModel(t, d)
	m = drive(m, "d")
	if m.dlg == nil || !m.dlg.toggleOn || !strings.Contains(m.View(), "[x] Delete the branch") {
		t.Fatalf("d should switch the option on and keep the dialog open:\n%s", m.View())
	}
	m = drive(m, "d", "d") // off, on
	m = drive(m, "y")
	if len(*got) != 1 || !(*got)[0].ok || !(*got)[0].toggled {
		t.Errorf("results = %+v; want ok with the option on", *got)
	}
	// declining reports the state too but ok is false
	m, got = dlgModel(t, d)
	m = drive(m, "n")
	if len(*got) != 1 || (*got)[0].ok {
		t.Errorf("declined results = %+v", *got)
	}
}

func TestNeedBangRequiresTheDeliberateKey(t *testing.T) {
	d := newChoiceDialog("b", "Merge", []choice{{label: "Squash and merge", value: "squash"}}, false, nil)
	d.needBang, d.bangHint = true, "press ! to merge anyway"
	m, got := dlgModel(t, d)
	m = drive(m, "enter")
	if m.dlg == nil || len(*got) != 0 || !strings.Contains(m.dlg.err, "press !") {
		t.Fatalf("enter must be refused with the hint: open=%v results=%v err=%q", m.dlg != nil, *got, m.dlg.err)
	}
	if m.dlg.filter != "" {
		t.Errorf("! must not be typed into the filter, filter=%q", m.dlg.filter)
	}
	m = drive(m, "!")
	if m.dlg != nil || len(*got) != 1 || !reflect.DeepEqual((*got)[0].values, []string{"squash"}) {
		t.Errorf("! should accept the choice: open=%v results=%+v", m.dlg != nil, *got)
	}
}

func TestBangIsJustACharacterWhenNotRequired(t *testing.T) {
	m, got := dlgModel(t, newChoiceDialog("b", "Pick", fruit, false, nil))
	m = drive(m, "!")
	if m.dlg == nil || len(*got) != 0 || m.dlg.filter != "!" {
		t.Errorf("open=%v results=%v filter=%q; ! is an ordinary character here", m.dlg != nil, *got, m.dlg.filter)
	}
}
