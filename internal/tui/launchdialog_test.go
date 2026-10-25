package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/launch"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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

// pasteInto focuses the field with tabs from Where and sends text as one paste.
func pasteInto(d launchDialog, tabs int, text string) launchDialog {
	d, _ = launchKeys(d, "e")
	for i := 0; i < tabs; i++ {
		d, _ = launchKeys(d, "tab")
	}
	d, _ = d.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	return d
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

func TestPasteIsFilteredToPrintableText(t *testing.T) {
	const dirty = "a\nb\tc\x1b[2~d"
	const clean = "abc[2~d"
	base := func(d launchDialog) string { return d.request().Base }
	name := func(d launchDialog) string { return d.request().Name }
	model := func(d launchDialog) string { return d.request().Model }
	for _, c := range []struct {
		field string
		tabs  int
		get   func(launchDialog) string
	}{{"model", 2, model}, {"base", 3, base}, {"branch", 4, name}} {
		in := orcaInput()
		in.Base, in.Name = "", ""
		d := pasteInto(newLaunchDialog(in), c.tabs, dirty)
		if got := c.get(d); got != clean {
			t.Errorf("%s = %q, want %q", c.field, got, clean)
		}
		r := d.request()
		for _, v := range []string{r.Model, r.Base, r.Name} {
			if hasControl(v) {
				t.Errorf("%s: request field %q has control characters", c.field, v)
			}
		}
		if strings.ContainsAny(d.view(100), "\t\x1b") {
			t.Errorf("%s: view has control characters", c.field)
		}
	}
	// a paste of only control characters changes nothing, not even the model's mode
	d := pasteInto(newLaunchDialog(orcaInput()), 2, "\n\t")
	if d.typing || d.request().Model != "" {
		t.Errorf("control-only paste changed the model: %+v", d.request())
	}
	// CJK and emoji survive
	d = pasteInto(newLaunchDialog(orcaInput()), 4, "修正🙂")
	if got := d.request().Name; !strings.HasSuffix(got, "修正🙂") {
		t.Errorf("name = %q, want the CJK and emoji kept", got)
	}
	// a paste is never a submit
	d = pasteInto(newLaunchDialog(orcaInput()), 4, "x\n")
	if _, act := d.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\n")}); act != actNone {
		t.Errorf("pasted newline = %v, want actNone", act)
	}
}

func TestBackspaceRemovesWholeRune(t *testing.T) {
	in := orcaInput()
	in.Name = ""
	d := pasteInto(newLaunchDialog(in), 4, "a修🙂")
	for _, want := range []string{"a修", "a", ""} {
		d, _ = launchKeys(d, "backspace")
		if got := d.request().Name; got != want {
			t.Fatalf("after backspace name = %q, want %q", got, want)
		}
	}
}

func TestLettersAreTextWhileExpanded(t *testing.T) {
	in := orcaInput()
	in.Base, in.Name = "", ""
	d, _ := launchKeys(newLaunchDialog(in), "e", "tab", "tab", "tab") // Base
	for _, key := range []string{"e", "j", "q"} {
		var act launchAct
		d, act = launchKeys(d, key)
		if act != actNone {
			t.Fatalf("%q = %v, want actNone", key, act)
		}
	}
	if d.request().Base != "ejq" || d.focused() != fBase || !d.expanded {
		t.Errorf("base = %q focus = %s expanded = %v", d.request().Base, launchFieldName[d.focused()], d.expanded)
	}
}

func TestCompactStrayKeysDoNothing(t *testing.T) {
	d := newLaunchDialog(orcaInput())
	for _, msg := range []tea.KeyMsg{
		k("x"), k("j"), k("q"), k("space"), k("backspace"), k("tab"), k("left"),
		{Type: tea.KeyRunes, Runes: []rune("e"), Alt: true},
		{Type: tea.KeyRunes, Runes: []rune("e"), Paste: true},
	} {
		got, act := d.update(msg)
		if act != actNone || fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", d) {
			t.Errorf("%v: act = %v, state changed: %+v", msg, act, got)
		}
	}
}

func TestAltKeysAreNotTyped(t *testing.T) {
	in := orcaInput()
	in.Base = ""
	d, _ := launchKeys(newLaunchDialog(in), "e", "tab", "tab", "tab")
	d, act := d.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true})
	if act != actNone || d.request().Base != "" {
		t.Errorf("alt+x typed %q", d.request().Base)
	}
}

// longestLine is the widest line of s in display columns.
func longestLine(s string) (int, string) {
	w, at := 0, ""
	for _, l := range strings.Split(s, "\n") {
		if n := ansi.StringWidth(l); n > w {
			w, at = n, l
		}
	}
	return w, at
}

func TestViewLinesFitTheWidth(t *testing.T) {
	long := strings.Repeat("origin/feature-", 14)[:200]
	cjk := strings.Repeat("修正登入", 20)
	lost := &launch.StepError{Step: launch.StepCreate, Err: &launch.LostReplyError{OperationID: "1700000000000-0123456789abcdef0123456789abcdef", Cause: "orca did not answer in time, so it is not known whether the workspace exists"}}
	for _, width := range []int{20, 40, 100} {
		for _, target := range []launch.Target{launch.TargetOrca, launch.TargetHerdr, launch.TargetTmux} {
			for _, place := range []launch.Place{launch.PlacePane, launch.PlaceWindow} {
				in := orcaInput()
				in.Issue.Title = "Login fails on 修正 the very long title of an issue that goes on and on"
				in.Target, in.Place, in.Base, in.Name = target, place, long, cjk
				in.Model = cjk
				in.ContextLen = launch.WarnChars + 1
				states := map[string]launchDialog{"compact": newLaunchDialog(in)}
				e := newLaunchDialog(in)
				// walk the focus over every row of the expanded form
				e, _ = launchKeys(e, "e")
				for i := 0; i < 6; i++ {
					states[fmt.Sprintf("expanded focus %s", launchFieldName[e.focused()])] = e
					e, _ = launchKeys(e, "tab")
				}
				typed := pasteInto(newLaunchDialog(in), 2, cjk+long)
				states["typed model"] = typed
				states["running"] = newLaunchDialog(in).start()
				states["failed"] = newLaunchDialog(in).start().failed(lost)
				states["failed plain"] = newLaunchDialog(in).failed(errors.New(long))
				bad := in
				bad.Base = ""
				bd, _ := launchKeys(newLaunchDialog(bad), "enter")
				states["refused"] = bd
				for name, d := range states {
					v := d.view(width)
					if w, line := longestLine(v); w > width {
						t.Errorf("%s/%s/%s width %d: line is %d columns: %q", target, place, name, width, w, line)
					}
				}
			}
		}
	}
	// tiny and absent widths must not panic
	d := pasteInto(newLaunchDialog(orcaInput()), 3, "修正🙂")
	for _, w := range []int{-5, 0, 1, 2, 3, 5} {
		_ = d.view(w)
		_ = d.start().view(w)
	}
}

func TestLongValueIsClippedInTheViewOnly(t *testing.T) {
	long := strings.Repeat("x", 150) + "TAIL"
	in := orcaInput()
	in.Base = long
	d, _ := launchKeys(newLaunchDialog(in), "e", "tab", "tab", "tab")
	v := d.view(40)
	if !strings.Contains(v, "…") || !strings.Contains(v, "TAIL") {
		t.Errorf("clipped view should keep the end behind an ellipsis:\n%s", v)
	}
	if d.request().Base != long {
		t.Error("the stored base was clipped")
	}
}

func TestFailedWithoutStepAndOddErrors(t *testing.T) {
	v := newLaunchDialog(orcaInput()).failed(errors.New("boom")).view(100)
	if !strings.Contains(v, "failed: boom") || strings.Contains(v, "failed at") {
		t.Errorf("plain error view:\n%s", v)
	}
	v = newLaunchDialog(orcaInput()).failed(&launch.StepError{Step: launch.StepPaste, Err: errors.New("no tty")}).view(100)
	if !strings.Contains(v, "failed at paste: no tty") {
		t.Errorf("step error view:\n%s", v)
	}
	// neither a nil error nor a step without an error may panic
	d := newLaunchDialog(orcaInput()).start().failed(nil)
	if d.running() || !strings.Contains(d.view(100), "failed") {
		t.Errorf("failed(nil): running = %v\n%s", d.running(), d.view(100))
	}
	d = newLaunchDialog(orcaInput()).failed(&launch.StepError{Step: launch.StepOpen})
	if v := d.view(100); !strings.Contains(v, "failed at "+launch.StepOpen) {
		t.Errorf("step without error:\n%s", v)
	}
}

func TestUnknownAgentIsRefused(t *testing.T) {
	in := orcaInput()
	in.Agent = "nope"
	d, act := newLaunchDialog(in).update(k("enter"))
	if act != actNone {
		t.Errorf("enter with an unknown agent = %v, want actNone", act)
	}
	if v := d.view(100); !strings.Contains(v, "unknown agent nope") {
		t.Errorf("view does not say why:\n%s", v)
	}
}

func TestCompactRequiredMessageSaysPressE(t *testing.T) {
	in := orcaInput()
	in.Name = ""
	d, _ := newLaunchDialog(in).update(k("enter"))
	if v := d.view(100); !strings.Contains(v, "branch is required") || !strings.Contains(v, "press e to edit") {
		t.Errorf("view:\n%s", v)
	}
}
