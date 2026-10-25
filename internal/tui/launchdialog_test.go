package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/launch"
	tea "github.com/charmbracelet/bubbletea"
)

var testModels = []launch.Model{{ID: ""}, {ID: "opus"}, {ID: "sonnet"}}

func orcaInput() launchInput {
	return launchInput{
		Issue:      ghcli.IssueDetail{Number: 42, Title: "Fix login"},
		Target:     launch.TargetOrca,
		AutoTarget: true,
		Agent:      "claude",
		Models:     testModels,
		Base:       "origin/main",
		Name:       "issue-42-fix-login",
		Place:      launch.PlacePane,
		Split:      launch.SplitHorizontal,
		ContextLen: 500,
	}
}

func tmuxInput() launchInput {
	in := orcaInput()
	in.Target = launch.TargetTmux
	in.Model = "opus"
	in.Base, in.Name = "", ""
	return in
}

func tabBack() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyShiftTab} }

// launchKeys feeds keys to d and returns the dialog and the last action.
func launchKeys(d launchDialog, keys ...string) (launchDialog, launchAct) {
	act := actNone
	for _, s := range keys {
		if s == "shift+tab" {
			d, act = d.update(tabBack())
			continue
		}
		d, act = d.update(k(s))
	}
	return d, act
}

func TestSummaryOrca(t *testing.T) {
	got := newLaunchDialog(orcaInput()).summary()
	want := "orca ◆ new workspace · claude · default model · origin/main → issue-42-fix-login"
	if got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestSummaryTmuxPaneAndWindow(t *testing.T) {
	in := tmuxInput()
	if got, want := newLaunchDialog(in).summary(), "tmux ◆ new pane (right) · claude · opus"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	in.Split = launch.SplitVertical
	if got, want := newLaunchDialog(in).summary(), "tmux ◆ new pane (below) · claude · opus"; got != want {
		t.Errorf("vertical summary = %q, want %q", got, want)
	}
	in.Place = launch.PlaceWindow
	if got, want := newLaunchDialog(in).summary(), "tmux ◆ new window · claude · opus"; got != want {
		t.Errorf("window summary = %q, want %q", got, want)
	}
	in.Target = launch.TargetHerdr
	in.AutoTarget = false
	got := newLaunchDialog(in).summary()
	if !strings.HasPrefix(got, "herdr ") || strings.Contains(got, "◆") || !strings.Contains(got, "new window") {
		t.Errorf("herdr summary = %q", got)
	}
}

func TestEnterSubmitsCompact(t *testing.T) {
	d := newLaunchDialog(orcaInput())
	if _, act := d.update(k("enter")); act != actSubmit {
		t.Errorf("enter = %v, want actSubmit", act)
	}
	if _, act := d.update(k("x")); act != actNone {
		t.Errorf("stray key = %v, want actNone", act)
	}
}

func TestEscCancels(t *testing.T) {
	d := newLaunchDialog(orcaInput())
	if _, act := d.update(k("esc")); act != actCancel {
		t.Errorf("esc = %v, want actCancel", act)
	}
	d, _ = launchKeys(d, "e")
	if _, act := d.update(k("esc")); act != actCancel {
		t.Errorf("esc expanded = %v, want actCancel", act)
	}
	if _, act := d.start().update(k("esc")); act != actNone {
		t.Errorf("esc while running = %v, want actNone", act)
	}
}

// focusOrder presses tab from the first field and lists the field labels
// reached until the focus wraps around.
func focusOrder(t *testing.T, d launchDialog) []string {
	t.Helper()
	d, _ = launchKeys(d, "e")
	var got []string
	for i := 0; i < 12; i++ {
		got = append(got, launchFieldName[d.focused()])
		d, _ = launchKeys(d, "tab")
		if d.focused() == fWhere {
			break
		}
	}
	return got
}

func TestEExpandsAndTabCyclesApplicableFields(t *testing.T) {
	d := newLaunchDialog(orcaInput())
	if d.expanded {
		t.Fatal("dialog starts expanded")
	}
	if got, want := fmt.Sprint(focusOrder(t, d)), "[Where Agent Model Base Branch]"; got != want {
		t.Errorf("orca order = %s, want %s", got, want)
	}
	in := tmuxInput()
	if got, want := fmt.Sprint(focusOrder(t, newLaunchDialog(in))), "[Where Agent Model Place Split]"; got != want {
		t.Errorf("tmux pane order = %s, want %s", got, want)
	}
	in.Target = launch.TargetHerdr
	in.Place = launch.PlaceWindow
	if got, want := fmt.Sprint(focusOrder(t, newLaunchDialog(in))), "[Where Agent Model Place]"; got != want {
		t.Errorf("herdr window order = %s, want %s", got, want)
	}
	// shift+tab goes backwards and also skips what does not apply
	e, _ := launchKeys(newLaunchDialog(tmuxInput()), "e")
	e, _ = e.update(tabBack())
	if e.focused() != fSplit {
		t.Errorf("shift+tab from Where reached %s, want Split", launchFieldName[e.focused()])
	}
}

func TestAgentCycleReloadsModels(t *testing.T) {
	d, _ := launchKeys(newLaunchDialog(tmuxInput()), "e", "tab")
	d, act := launchKeys(d, "right")
	if act != actAgentChanged {
		t.Fatalf("right on Agent = %v, want actAgentChanged", act)
	}
	if d.request().Agent.ID != "devin" {
		t.Errorf("agent = %q, want devin", d.request().Agent.ID)
	}
	if d.request().Model != "" {
		t.Errorf("model = %q after agent change, want default", d.request().Model)
	}
	d = d.withModels([]launch.Model{{ID: ""}, {ID: "swe"}})
	d, _ = launchKeys(d, "tab", "right")
	if d.request().Model != "swe" {
		t.Errorf("model = %q, want swe from the reloaded list", d.request().Model)
	}
	if _, act := launchKeys(d, "shift+tab", "left"); act != actAgentChanged {
		t.Errorf("left on Agent = %v, want actAgentChanged", act)
	}
}

func TestModelFieldCycleAndFreeText(t *testing.T) {
	in := orcaInput() // default model first
	d, _ := launchKeys(newLaunchDialog(in), "e", "tab", "tab")
	if d.focused() != fModel {
		t.Fatalf("focus = %s, want Model", launchFieldName[d.focused()])
	}
	d, _ = launchKeys(d, "right")
	if got := d.request().Model; got != "opus" {
		t.Errorf("right: model = %q, want opus", got)
	}
	d, _ = launchKeys(d, "left", "left")
	if got := d.request().Model; got != "sonnet" {
		t.Errorf("left twice wraps to the last: model = %q, want sonnet", got)
	}
	// typing replaces the selection
	d, act := launchKeys(d, "g", "p", "t")
	if act != actNone {
		t.Errorf("typing = %v, want actNone", act)
	}
	if got := d.request().Model; got != "gpt" {
		t.Errorf("typed model = %q, want gpt", got)
	}
	d, _ = launchKeys(d, "backspace")
	if got := d.request().Model; got != "gp" {
		t.Errorf("after backspace model = %q, want gp", got)
	}
	// empty text means default
	d, _ = launchKeys(d, "backspace", "backspace")
	if got := d.request().Model; got != "" {
		t.Errorf("empty text: model = %q, want default", got)
	}
	if !strings.Contains(d.summary(), "default model") {
		t.Errorf("summary = %q, want default model", d.summary())
	}
	// cycling after free text goes back to the list
	d, _ = launchKeys(d, "right")
	if got := d.request().Model; got != "" {
		t.Errorf("right from empty text: model = %q, want the first entry (default)", got)
	}
}

func TestFreeTextModelHint(t *testing.T) {
	d, _ := launchKeys(newLaunchDialog(orcaInput()), "e", "tab", "tab", "x")
	v := d.view(100)
	if !strings.Contains(v, "only fails in the agent pane") {
		t.Errorf("view lacks the free-text hint:\n%s", v)
	}
}

func TestOrcaBranchHint(t *testing.T) {
	d, _ := launchKeys(newLaunchDialog(orcaInput()), "e")
	if v := d.view(100); !strings.Contains(v, "prefix") {
		t.Errorf("view lacks the Orca prefix hint:\n%s", v)
	}
}

func TestWarnsAbove100000(t *testing.T) {
	in := orcaInput()
	in.ContextLen = launch.WarnChars
	if s := newLaunchDialog(in).summary(); strings.Contains(s, "⚠") {
		t.Errorf("exactly %d characters warns: %q", launch.WarnChars, s)
	}
	in.ContextLen = launch.WarnChars + 1
	d := newLaunchDialog(in)
	if s := d.summary(); !strings.Contains(s, "⚠") || !strings.Contains(s, "100001") {
		t.Errorf("no size warning above the limit: %q", s)
	}
	if _, act := d.update(k("enter")); act != actSubmit {
		t.Errorf("enter with a warning = %v, want actSubmit", act)
	}
	// 40 000 CJK runes are 120 000 bytes: the caller counts runes, so no warning
	cjk := strings.Repeat("字", 40000)
	in.ContextLen = len([]rune(cjk))
	if s := newLaunchDialog(in).summary(); strings.Contains(s, "⚠") {
		t.Errorf("a 40000-rune context warns: %q", s)
	}
}

func TestEnterIgnoredWhileRunning(t *testing.T) {
	d := newLaunchDialog(orcaInput())
	d, act := d.update(k("enter"))
	if act != actSubmit {
		t.Fatalf("first enter = %v", act)
	}
	d = d.start()
	if !d.running() {
		t.Fatal("not running after start")
	}
	if _, act := d.update(k("enter")); act != actNone {
		t.Errorf("enter while running = %v, want actNone", act)
	}
	if _, act := d.update(k("e")); act != actNone {
		t.Errorf("e while running = %v", act)
	}
	if v := d.view(100); !strings.Contains(v, "launching…") {
		t.Errorf("view lacks launching…:\n%s", v)
	}
}

func TestFailedShowsStepAndKeepsDialog(t *testing.T) {
	d := newLaunchDialog(orcaInput()).start()
	d = d.failed(&launch.StepError{Step: launch.StepFetch, Err: errors.New("network down")})
	if d.running() {
		t.Error("still running after failure")
	}
	v := d.view(100)
	for _, want := range []string{launch.StepFetch, "network down", "issue-42-fix-login"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "launching…") {
		t.Errorf("view still says launching…:\n%s", v)
	}
	if d.request().RetryID != "" {
		t.Errorf("RetryID = %q for a plain step error", d.request().RetryID)
	}
	if _, act := d.update(k("enter")); act != actSubmit {
		t.Errorf("enter after failure = %v, want actSubmit", act)
	}
	// a plain error without a step is shown as it is
	p := newLaunchDialog(orcaInput()).failed(errors.New("boom"))
	if v := p.view(100); !strings.Contains(v, "boom") {
		t.Errorf("view lacks the plain error:\n%s", v)
	}
}

func TestRetryRequestCarriesOperationIDUntilEdited(t *testing.T) {
	const id = "1700000000000-0123456789abcdef0123456789abcdef"
	lost := &launch.StepError{Step: launch.StepCreate, Err: &launch.LostReplyError{OperationID: id, Cause: "no reply"}}
	d := newLaunchDialog(orcaInput()).start().failed(lost)
	if got := d.request().RetryID; got != id {
		t.Fatalf("RetryID = %q, want %q", got, id)
	}
	if v := d.view(100); !strings.Contains(v, "retr") {
		t.Errorf("view does not say enter retries:\n%s", v)
	}
	// moving the focus is not an edit
	e, _ := launchKeys(d, "e", "tab")
	if got := e.request().RetryID; got != id {
		t.Errorf("RetryID after moving focus = %q, want it kept", got)
	}
	// any edit drops it: text, cycle, agent
	for name, keys := range map[string][]string{
		"text":  {"e", "tab", "tab", "tab", "x"},
		"cycle": {"e", "right"},
		"agent": {"e", "tab", "right"},
	} {
		x, _ := launchKeys(d, keys...)
		if got := x.request().RetryID; got != "" {
			t.Errorf("%s edit: RetryID = %q, want cleared", name, got)
		}
	}
	// wrapped errors are found too
	w := newLaunchDialog(orcaInput()).failed(fmt.Errorf("launch: %w", lost))
	if got := w.request().RetryID; got != id {
		t.Errorf("wrapped: RetryID = %q, want %q", got, id)
	}
}

func TestOrcaRequiresBaseAndBranch(t *testing.T) {
	in := orcaInput()
	in.Base = ""
	d, _ := launchKeys(newLaunchDialog(in), "e")
	d, act := launchKeys(d, "enter")
	if act != actNone {
		t.Errorf("enter with empty base = %v, want actNone", act)
	}
	if v := d.view(100); !strings.Contains(v, "base") {
		t.Errorf("view does not name the missing base:\n%s", v)
	}
	d, _ = launchKeys(d, "tab", "tab", "tab", "o", "r", "i", "g", "i", "n", "/", "m")
	if _, act := launchKeys(d, "enter"); act != actSubmit {
		t.Errorf("enter with base set = %v, want actSubmit", act)
	}
	d, _ = launchKeys(d, "tab", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	if _, act := launchKeys(d, "enter"); act != actNone {
		t.Errorf("enter with empty branch = %v, want actNone", act)
	}
	// tmux needs neither
	tm := tmuxInput()
	tm.Base, tm.Name = "", ""
	if _, act := launchKeys(newLaunchDialog(tm), "e", "enter"); act != actSubmit {
		t.Errorf("tmux without base/branch = %v, want actSubmit", act)
	}
	// withBase fills a base that was loaded later, unless the user typed one
	in.Base = ""
	if got := newLaunchDialog(in).withBase("origin/dev").request().Base; got != "origin/dev" {
		t.Errorf("withBase = %q", got)
	}
}

func TestChangingTargetClearsAutoMark(t *testing.T) {
	d := newLaunchDialog(orcaInput())
	if !strings.Contains(d.summary(), "◆") {
		t.Fatal("auto target has no mark")
	}
	d, _ = launchKeys(d, "e", "right") // Where: orca -> herdr
	s := d.summary()
	if strings.Contains(s, "◆") || !strings.HasPrefix(s, "herdr") {
		t.Errorf("summary after changing target = %q", s)
	}
	if got := d.request().Target; got != launch.TargetHerdr {
		t.Errorf("target = %q, want herdr", got)
	}
	d, _ = launchKeys(d, "right", "right") // herdr -> tmux -> orca
	if got := d.request().Target; got != launch.TargetOrca {
		t.Errorf("target after wrap = %q, want orca", got)
	}
}

func TestRequestFields(t *testing.T) {
	r := newLaunchDialog(orcaInput()).request()
	if r.Target != launch.TargetOrca || r.Agent.ID != "claude" || r.Base != "origin/main" ||
		r.Name != "issue-42-fix-login" || r.Issue != 42 || r.Context != "" || r.Dir != "" {
		t.Errorf("request = %+v", r)
	}
	tm := newLaunchDialog(tmuxInput()).request()
	if tm.Model != "opus" || tm.Place != launch.PlacePane || tm.Split != launch.SplitHorizontal {
		t.Errorf("tmux request = %+v", tm)
	}
}
