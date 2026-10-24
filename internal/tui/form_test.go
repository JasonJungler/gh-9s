package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func k(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

var formInputs = []workflow.DispatchInput{
	{Name: "env", Type: "choice", Options: []string{"staging", "prod"}, Default: "staging"},
	{Name: "dry", Type: "boolean", Default: "true"},
	{Name: "note", Type: "string"},
	{Name: "tag", Type: "string", Required: true},
}

func newForm() triggerForm {
	return newTriggerForm(ghcli.RepoRef{Owner: "acme", Name: "api"}, "deploy.yml", "main",
		[]string{"main", "dev"}, formInputs)
}

func press(f triggerForm, keys ...string) (triggerForm, bool, bool) {
	var submit, cancel bool
	for _, key := range keys {
		f, submit, cancel = f.update(k(key))
	}
	return f, submit, cancel
}

func TestFormArgs(t *testing.T) {
	f := newForm()
	f, _, _ = press(f, "tab", "tab", "tab", "tab", "v", "1") // focus tag; type v1
	f, _, _ = press(f, "tab", "tab")                         // wrap to env? ensure no crash
	want := []string{"workflow", "run", "deploy.yml", "-R", "acme/api", "--ref", "main",
		"-f", "env=staging", "-f", "dry=true", "-f", "tag=v1"}
	if got := f.args(); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v\nwant %v", got, want)
	}
}

func TestFormArgsZeroTargetHasNoRepoFlag(t *testing.T) {
	f := newTriggerForm(ghcli.RepoRef{}, "ci.yml", "dev", nil, nil)
	want := []string{"workflow", "run", "ci.yml", "--ref", "dev"}
	if got := f.args(); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v; want %v", got, want)
	}
}

func TestFormRequiredBlocksSubmit(t *testing.T) {
	f := newForm()
	f, submit, _ := press(f, "enter")
	if submit || !strings.Contains(f.err, "tag") {
		t.Errorf("submit=%v err=%q; want blocked with message naming tag", submit, f.err)
	}
	f, _, _ = press(f, "tab", "tab", "tab", "tab", "x")
	if _, submit, _ = press(f, "enter"); !submit {
		t.Error("filled required field should allow submit")
	}
}

func TestFormBooleanToggle(t *testing.T) {
	f := newForm()
	f, _, _ = press(f, "tab", "tab", "space") // focus dry
	if v := f.fields[1].Value; v != "false" {
		t.Errorf("dry = %q after one toggle; want false", v)
	}
	f, _, _ = press(f, "space")
	if v := f.fields[1].Value; v != "true" {
		t.Errorf("dry = %q after two toggles; want true", v)
	}
}

func TestFormChoiceCycleWraps(t *testing.T) {
	f := newForm()
	f, _, _ = press(f, "tab", "right") // focus env, next
	if f.fields[0].Value != "prod" {
		t.Fatalf("env = %q; want prod", f.fields[0].Value)
	}
	if f, _, _ = press(f, "right"); f.fields[0].Value != "staging" {
		t.Errorf("env = %q; want wrap to staging", f.fields[0].Value)
	}
	if f, _, _ = press(f, "left"); f.fields[0].Value != "prod" {
		t.Errorf("env = %q; want wrap back to prod", f.fields[0].Value)
	}
}

func TestFormUnsupportedTypeIsText(t *testing.T) {
	f := newTriggerForm(ghcli.RepoRef{}, "w.yml", "main", nil, []workflow.DispatchInput{
		{Name: "region", Type: "environment"}, {Name: "n", Type: "number"}})
	f, _, _ = press(f, "tab", "e", "u", "tab", "4", "2")
	if f.fields[0].Value != "eu" || f.fields[1].Value != "42" {
		t.Errorf("values = %q, %q", f.fields[0].Value, f.fields[1].Value)
	}
}

func TestFormRefCycleRequestsReload(t *testing.T) {
	f := newForm()
	f, _, _ = press(f, "right") // focus starts on ref
	if f.ref != "dev" || !f.pendingReload {
		t.Errorf("ref=%q pendingReload=%v", f.ref, f.pendingReload)
	}
}

func TestFormKeepsValuesOnInputReload(t *testing.T) {
	f := newForm()
	f, _, _ = press(f, "tab", "tab", "tab", "n", "o", "t", "e")
	f = f.withInputs([]workflow.DispatchInput{{Name: "note", Type: "string"}, {Name: "fresh", Type: "string", Default: "d"}})
	if len(f.fields) != 2 || f.fields[0].Value != "note" || f.fields[1].Value != "d" {
		t.Errorf("fields = %+v", f.fields)
	}
}

func TestFormCancel(t *testing.T) {
	if _, _, cancel := press(newForm(), "esc"); !cancel {
		t.Error("esc should cancel")
	}
}

func TestFormEnterAfterTypingRefReadsInputsFirst(t *testing.T) {
	f := newForm()          // focus starts on the ref field
	f, _, _ = press(f, "x") // ref is now "mainx"; its inputs have not been read
	f, submit, _ := press(f, "enter")
	if submit || !f.pendingReload || !strings.Contains(f.err, "reading inputs") {
		t.Fatalf("submit=%v pendingReload=%v err=%q; want a reload before submitting", submit, f.pendingReload, f.err)
	}
	f.pendingReload, f.loadedRef = false, f.ref         // the caller re-read the inputs
	f, _, _ = press(f, "tab", "tab", "tab", "tab", "v") // fill the required tag input
	if _, submit, _ = press(f, "enter"); !submit {
		t.Error("enter should submit once the inputs match the ref")
	}
}
