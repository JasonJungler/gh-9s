package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/launch"
)

// promptInput is a dialog input with a generated prompt and two comments.
func promptInput() launchInput {
	in := tmuxInput()
	in.Issue.Comments = []ghcli.Comment{{Author: "a", Body: "x"}, {Author: "b", Body: "y"}}
	in.Context = "Please solve this issue\n\n# Fix login (#42)\nhttps://x/42\n\nbody\n"
	in.ContextLen, in.ContextBytes = len([]rune(in.Context)), len(in.Context)
	return in
}

// onPrompt expands the form and moves the focus to the Prompt row.
func onPrompt(t *testing.T, d launchDialog) launchDialog {
	t.Helper()
	d, _ = launchKeys(d, "e")
	for i := 0; d.focused() != fPrompt; i++ {
		if i > int(fieldCount) {
			t.Fatal("the Prompt row is not reachable")
		}
		d, _ = launchKeys(d, "tab")
	}
	return d
}

func TestPromptRowSummarisesTheGeneratedPrompt(t *testing.T) {
	d := onPrompt(t, newLaunchDialog(promptInput()))
	v := ansi.Strip(d.view(100))
	want := fmt.Sprintf("Prompt   title + body + 2 comments · %d characters", d.in.ContextLen)
	if !strings.Contains(v, want) {
		t.Errorf("view lacks %q:\n%s", want, v)
	}
	if strings.Contains(v, "edited") && !strings.Contains(v, "$EDITOR") {
		t.Errorf("an untouched prompt shows the edited mark:\n%s", v)
	}
	in := promptInput()
	in.Issue.Comments = in.Issue.Comments[:1]
	if got := newLaunchDialog(in).promptRow(); !strings.Contains(got, "title + body + 1 comment ·") {
		t.Errorf("one comment: %q", got)
	}
	in.Issue.Comments = nil
	if got := newLaunchDialog(in).promptRow(); !strings.HasPrefix(got, "title + body · ") {
		t.Errorf("no comments: %q", got)
	}
}

func TestEnterOnPromptRowAsksForTheEditorWithTheWholePrompt(t *testing.T) {
	d := onPrompt(t, newLaunchDialog(promptInput()))
	d, act := launchKeys(d, "enter")
	if act != actEditPrompt {
		t.Fatalf("enter on the Prompt row = %v, want actEditPrompt", act)
	}
	if d.prompt() != promptInput().Context {
		t.Errorf("prompt() = %q, want the generated text", d.prompt())
	}
	// not on the Prompt row, and not while compact, enter still launches
	if _, act := launchKeys(newLaunchDialog(promptInput()), "enter"); act != actSubmit {
		t.Errorf("compact enter = %v, want actSubmit", act)
	}
	e, _ := launchKeys(newLaunchDialog(promptInput()), "e", "tab")
	if _, act := launchKeys(e, "enter"); act != actSubmit {
		t.Errorf("enter on Agent = %v, want actSubmit", act)
	}
}

func TestKeysAreIgnoredWhileTheEditorIsOpen(t *testing.T) {
	d, _ := launchKeys(onPrompt(t, newLaunchDialog(promptInput())), "enter")
	for _, key := range []string{"enter", "esc", "r", "tab", "e"} {
		if got, act := launchKeys(d, key); act != actNone || !got.editing {
			t.Errorf("%q while the editor is open: act = %v, editing = %v", key, act, got.editing)
		}
	}
	d = d.promptEdited("new text", nil)
	if _, act := launchKeys(d, "esc"); act != actCancel {
		t.Errorf("esc after the editor closed = %v, want actCancel", act)
	}
}

func TestEditedPromptReplacesThePromptAndIsCleaned(t *testing.T) {
	d := onPrompt(t, newLaunchDialog(promptInput()))
	d, _ = launchKeys(d, "enter")
	d = d.promptEdited("Fix it\r\nnow\x1b[201~ ok\r\n\r\n", nil)
	if got, want := d.prompt(), "Fix it\nnow[201~ ok"; got != want {
		t.Errorf("prompt() = %q, want %q", got, want)
	}
	if d.editing {
		t.Error("still editing after the editor closed")
	}
	if d.in.ContextLen != len([]rune(d.prompt())) || d.in.ContextBytes != len(d.prompt()) {
		t.Errorf("sizes = %d chars, %d bytes; want those of the edited prompt", d.in.ContextLen, d.in.ContextBytes)
	}
	if s := d.summary(); !strings.Contains(s, "prompt edited") {
		t.Errorf("summary %q lacks the edited mark", s)
	}
	if v := ansi.Strip(d.view(100)); !strings.Contains(v, "edited · 18 characters") {
		t.Errorf("row lacks the edited marker:\n%s", v)
	}
}

func TestEditorFailureEmptyAndUnchangedKeepThePreviousPrompt(t *testing.T) {
	base := onPrompt(t, newLaunchDialog(promptInput()))
	first := base.promptEdited("first edit", nil)
	for _, tc := range []struct {
		name, text string
		err        error
		want       string // in err or info
	}{
		{"error", "garbage", errors.New("exit status 1"), "exit status 1"},
		{"empty", "", nil, "prompt is empty"},
		{"blank", " \r\n\t\n", nil, "prompt is empty"},
		{"only control characters", "\x1b\x00", nil, "prompt is empty"},
		{"unchanged", "first edit\n", nil, "unchanged"},
	} {
		d := first
		d, _ = launchKeys(d, "enter")
		d = d.promptEdited(tc.text, tc.err)
		if d.prompt() != "first edit" || !d.promptDone {
			t.Errorf("%s: prompt = %q (edited %v); want the previous edit kept", tc.name, d.prompt(), d.promptDone)
		}
		if shown := d.err + d.info; !strings.Contains(shown, tc.want) {
			t.Errorf("%s: message %q lacks %q", tc.name, shown, tc.want)
		}
		if v := ansi.Strip(d.view(100)); !strings.Contains(v, tc.want) {
			t.Errorf("%s: view lacks the message:\n%s", tc.name, v)
		}
		if d.editing {
			t.Errorf("%s: still editing", tc.name)
		}
	}
	// an unedited prompt stays unedited after a failure or an unchanged save
	d := base.promptEdited(promptInput().Context, nil)
	if d.promptDone || d.err != "" {
		t.Errorf("saving the generated text unchanged: edited = %v, err = %q", d.promptDone, d.err)
	}
	d = base.promptEdited("", nil)
	if d.promptDone || d.prompt() != promptInput().Context {
		t.Errorf("an empty save replaced the generated prompt: %q", d.prompt())
	}
}

func TestEditingBackToTheGeneratedTextDropsTheEditedMark(t *testing.T) {
	d := newLaunchDialog(promptInput()).promptEdited("something else", nil)
	d = d.promptEdited("Please solve this issue\r\n\r\n# Fix login (#42)\r\nhttps://x/42\r\n\r\nbody\r\n", nil)
	if d.promptDone || d.in.ContextLen != promptInput().ContextLen {
		t.Errorf("edited = %v, ContextLen = %d; want the generated prompt back", d.promptDone, d.in.ContextLen)
	}
}

func TestEditorErrorTextCannotInjectControlCharacters(t *testing.T) {
	d := newLaunchDialog(promptInput()).promptEdited("", errors.New("bad \x1b[2J\r name"))
	if hasControl(strings.ReplaceAll(d.view(40), "\x1b[", "")) && strings.Contains(d.view(40), "\x1b[2J") {
		t.Errorf("the escape sequence reached the view: %q", d.view(40))
	}
	if strings.Contains(d.err, "\x1b") || strings.Contains(d.err, "\r") {
		t.Errorf("err = %q", d.err)
	}
}

func TestRResetsThePromptOnlyOnThePromptRow(t *testing.T) {
	d := onPrompt(t, newLaunchDialog(promptInput()))
	gen := d.in
	d = d.promptEdited("edited text", nil)
	d.retryID = "op-1"
	d, act := launchKeys(d, "r")
	if act != actNone || d.promptDone || d.prompt() != promptInput().Context {
		t.Fatalf("r: act = %v, edited = %v, prompt = %q", act, d.promptDone, d.prompt())
	}
	if d.in.ContextLen != gen.ContextLen || d.in.ContextBytes != gen.ContextBytes {
		t.Errorf("sizes after reset = %d/%d, want %d/%d", d.in.ContextLen, d.in.ContextBytes, gen.ContextLen, gen.ContextBytes)
	}
	if strings.Contains(d.summary(), "prompt edited") {
		t.Errorf("summary still marks the prompt: %q", d.summary())
	}
	if d.retryID != "" {
		t.Error("a reset left the lost-reply retry id")
	}
	if v := ansi.Strip(d.view(100)); strings.Contains(v, "edited ·") {
		t.Errorf("row still says edited:\n%s", v)
	}
	// r elsewhere is text, not a reset
	e := newLaunchDialog(tmuxInput())
	e.in.Context = "ctx"
	e = e.promptEdited("mine", nil)
	e, _ = launchKeys(e, "e", "tab", "tab", "r") // the Model row
	if !e.promptDone || e.in.Model != "r" {
		t.Errorf("r on the Model row: edited = %v, model = %q", e.promptDone, e.in.Model)
	}
}

func TestChangingOtherFieldsKeepsTheEditedPrompt(t *testing.T) {
	d := newLaunchDialog(promptInput()).promptEdited("my prompt", nil)
	d, _ = launchKeys(d, "e", "right")   // Where
	d, _ = launchKeys(d, "tab", "right") // Agent
	d, _ = launchKeys(d, "tab", "right") // Model
	d, _ = launchKeys(d, "tab", "right") // Place or Base
	d, _ = launchKeys(d, "tab", "x")
	if !d.promptDone || d.prompt() != "my prompt" {
		t.Errorf("prompt = %q (edited %v) after changing the other fields", d.prompt(), d.promptDone)
	}
	if !strings.Contains(d.summary(), "prompt edited") {
		t.Errorf("summary %q lost the mark", d.summary())
	}
}

func TestEditedPromptFeedsTheSizeWarnings(t *testing.T) {
	in := promptInput()
	in.Target = launch.TargetOrca
	d := newLaunchDialog(in)
	if s := d.summary(); strings.Contains(s, "⚠") {
		t.Fatalf("the generated prompt warns: %q", s)
	}
	big := strings.Repeat("字", launch.WarnChars+1)
	d = d.promptEdited(big, nil)
	s := d.summary()
	if !strings.Contains(s, fmt.Sprintf("large issue, %d characters", launch.WarnChars+1)) {
		t.Errorf("no character warning for the edited prompt: %q", s)
	}
	// the byte warning follows the edited prompt too; 3 bytes per character
	if lim := launch.ArgvLimit(); len(big) >= lim*9/10 && !strings.Contains(s, "bytes") {
		t.Errorf("no byte warning although %d bytes is near %d: %q", len(big), lim, s)
	}
	over := strings.Repeat("a", launch.ArgvLimit()+1)
	d = newLaunchDialog(in).promptEdited(over, nil)
	if s := d.summary(); !strings.Contains(s, "over the") || !strings.Contains(s, "refused") {
		t.Errorf("an edited prompt over the byte limit does not warn: %q", s)
	}
	// resetting takes the warnings away again
	d, _ = launchKeys(onPrompt(t, d), "r")
	if s := d.summary(); strings.Contains(s, "⚠") {
		t.Errorf("warning after the reset: %q", s)
	}
}

func TestPromptRowFitsTheWidth(t *testing.T) {
	cjk := strings.Repeat("修正登入", 30)
	for _, width := range []int{20, 40, 100} {
		for _, target := range []launch.Target{launch.TargetOrca, launch.TargetTmux} {
			in := promptInput()
			in.Target = target
			in.Issue.Comments = make([]ghcli.Comment, 1234)
			states := map[string]launchDialog{}
			d := onPrompt(t, newLaunchDialog(in))
			states["focused"] = d
			states["edited"] = d.promptEdited("x"+cjk, nil)
			states["empty"] = d.promptEdited("", nil)
			states["failed"] = d.promptEdited("", errors.New(cjk+"\x1b[2J"+cjk))
			states["unchanged"] = d.promptEdited(in.Context, nil)
			states["editing"], _ = launchKeys(d, "enter")
			states["compact edited"] = newLaunchDialog(in).promptEdited(cjk, nil)
			for name, s := range states {
				v := s.view(width)
				if w, line := longestLine(v); w > width {
					t.Errorf("%s/%s width %d: line is %d columns: %q", target, name, width, w, line)
				}
				if strings.Contains(v, "\x1b[2J") || strings.Contains(v, "\r") {
					t.Errorf("%s/%s: control sequence in the view: %q", target, name, v)
				}
			}
		}
	}
}

func TestHintListsTheKeysOfThePromptRow(t *testing.T) {
	d := onPrompt(t, newLaunchDialog(promptInput()))
	v := ansi.Strip(d.view(200))
	for _, want := range []string{"enter edit the prompt in $EDITOR", "r restore", "esc cancel"} {
		if !strings.Contains(v, want) {
			t.Errorf("hint lacks %q:\n%s", want, v)
		}
	}
	e, _ := launchKeys(newLaunchDialog(promptInput()), "e")
	if v := ansi.Strip(e.view(200)); !strings.Contains(v, "enter launch") {
		t.Errorf("the other rows lost the launch hint:\n%s", v)
	}
}

// --- the dialog inside the model: the editor seam, stale answers, the launch request ---

// fakeEditor replaces the editor: it records what it was asked to edit and answers with text, err.
type fakeEditor struct {
	initial []string
	text    string
	err     error
}

func installEditor(t *testing.T, text string, err error) *fakeEditor {
	t.Helper()
	f := &fakeEditor{text: text, err: err}
	old := launchEdit
	launchEdit = func(initial string, done func(string, error) tea.Msg) tea.Cmd {
		f.initial = append(f.initial, initial)
		return func() tea.Msg { return done(f.text, f.err) }
	}
	t.Cleanup(func() { launchEdit = old })
	return f
}

// openPromptRow opens the dialog on the issue screen and moves to the Prompt row.
func openPromptRow(t *testing.T, m model) model {
	t.Helper()
	m = pressLaunch(m, "a", "e")
	for i := 0; m.launchDlg != nil && m.launchDlg.focused() != fPrompt; i++ {
		if i > int(fieldCount) {
			t.Fatal("no Prompt row")
		}
		m = pressLaunch(m, "tab")
	}
	if m.launchDlg == nil {
		t.Fatal("no dialog")
	}
	return m
}

func TestEditorRoundTripSendsTheEditedPromptOnce(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	ed := installEditor(t, "Fix the login crash.\r\nTouch only auth.go\x1b[201~\r\n", nil)
	m := openPromptRow(t, issueOnScreen(t))
	m = pressLaunch(m, "enter")
	if len(ed.initial) != 1 || ed.initial[0] != launch.BuildContext(issue42()) {
		t.Fatalf("editor was opened with %q; want the whole generated prompt", ed.initial)
	}
	d := m.launchDlg
	if d == nil || !d.promptDone || d.prompt() != "Fix the login crash.\nTouch only auth.go[201~" {
		t.Fatalf("dialog after the editor = %+v", d)
	}
	if len(f.reqs) != 0 {
		t.Fatal("the editor round trip launched something")
	}
	m = pressLaunch(m, "tab", "enter") // away from the Prompt row, then launch
	if len(f.reqs) != 1 {
		t.Fatalf("%d launches; want 1", len(f.reqs))
	}
	if got, want := f.reqs[0].Context, "Fix the login crash.\nTouch only auth.go[201~"; got != want {
		t.Errorf("Request.Context = %q, want %q", got, want)
	}
	for _, bad := range []string{"\r", "\x1b"} {
		if strings.Contains(f.reqs[0].Context, bad) {
			t.Errorf("Request.Context contains %q", bad)
		}
	}
}

func TestWithoutAnEditThePromptIsTheGeneratedOne(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	m := pressLaunch(issueOnScreen(t), "a", "enter")
	if len(f.reqs) != 1 || f.reqs[0].Context != launch.BuildContext(issue42()) {
		t.Errorf("requests = %+v", f.reqs)
	}
	_ = m
}

func TestEditedPromptIsPerLaunchAndNeverStored(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	installEditor(t, "SECRET-PROMPT-TEXT", nil)
	m := issueOnScreen(t)
	prefsPath := t.TempDir() + "/launch.json"
	m.launchPrefs, _ = launch.LoadPrefs(prefsPath)
	f.results = []launch.Result{{Where: "tmux pane"}}
	m = openPromptRow(t, m)
	m = pressLaunch(m, "enter", "tab", "enter")
	if len(f.reqs) != 1 || f.reqs[0].Context != "SECRET-PROMPT-TEXT" {
		t.Fatalf("requests = %+v", f.reqs)
	}
	if m.launchDlg != nil {
		t.Fatal("the dialog stayed open after the launch")
	}
	if b, err := os.ReadFile(prefsPath); err == nil && strings.Contains(string(b), "SECRET") {
		t.Errorf("launch.json holds the prompt: %s", b)
	}
	if b, err := os.ReadFile(m.settings.Path()); err == nil && strings.Contains(string(b), "SECRET") {
		t.Errorf("settings hold the prompt: %s", b)
	}
	// the next dialog starts from the generated prompt again
	m = pressLaunch(m, "a")
	if m.launchDlg == nil || m.launchDlg.promptDone || m.launchDlg.prompt() != launch.BuildContext(issue42()) {
		t.Errorf("the next dialog kept the edit: %+v", m.launchDlg)
	}
}

func TestEditorFailureInTheModelKeepsThePromptAndTheDialog(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	installEditor(t, "", errors.New("exit status 2"))
	m := openPromptRow(t, issueOnScreen(t))
	m = pressLaunch(m, "enter")
	if m.launchDlg == nil || m.launchDlg.promptDone || m.launchDlg.editing || !strings.Contains(m.launchDlg.err, "exit status 2") {
		t.Fatalf("dialog after a failing editor = %+v", m.launchDlg)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "exit status 2") {
		t.Errorf("the failure is not shown:\n%s", v)
	}
	m = pressLaunch(m, "esc")
	if m.launchDlg != nil || len(f.reqs) != 0 {
		t.Errorf("esc after the editor: dialog open = %v, launches = %d", m.launchDlg != nil, len(f.reqs))
	}
}

func TestLateEditorAnswersAreDropped(t *testing.T) {
	installLaunch(t, tmuxEnv)
	ed := installEditor(t, "from the editor", nil)
	m := openPromptRow(t, issueOnScreen(t))
	seq := m.launchSeq

	// the dialog is not waiting for an editor
	got := m.launchPromptEdited(launchPromptMsg{seq: seq, text: "late"})
	if got.launchDlg.promptDone {
		t.Error("an answer nobody asked for was taken")
	}

	// waiting, but the answer is for an older dialog
	m = pressLaunch(m, "enter")
	if len(ed.initial) != 1 {
		t.Fatal("the editor was not opened")
	}
	d := *m.launchDlg
	d.editing, d.promptDone, d.promptText = true, false, ""
	m.launchDlg = &d
	if got := m.launchPromptEdited(launchPromptMsg{seq: seq - 1, text: "old"}); got.launchDlg.promptDone || !got.launchDlg.editing {
		t.Errorf("an answer for an older dialog was taken: %+v", got.launchDlg)
	}
	// the dialog was closed
	closed := m
	closed.launchDlg = nil
	if got := closed.launchPromptEdited(launchPromptMsg{seq: seq, text: "x"}); got.launchDlg != nil {
		t.Error("an answer reopened a closed dialog")
	}
	// the answer for the dialog that is open is taken
	if got := m.launchPromptEdited(launchPromptMsg{seq: seq, text: "fresh"}); !got.launchDlg.promptDone || got.launchDlg.prompt() != "fresh" {
		t.Errorf("the right answer was dropped: %+v", got.launchDlg)
	}
}

func TestEditedPromptOverTheCeilingIsWarnedAndRefusedLikeAGeneratedOne(t *testing.T) {
	f := installLaunch(t, tmuxEnv)
	lim := launch.ArgvLimit()
	big := strings.Repeat("a", lim+1)
	installEditor(t, big, nil)
	// Where = orca, which passes the text as an argument
	m := issueOnScreen(t)
	m = pressLaunch(m, "a", "e", "right") // Where: orca
	if m.launchDlg.in.Target != launch.TargetOrca {
		t.Fatalf("Where = %s", m.launchDlg.in.Target)
	}
	for m.launchDlg.focused() != fPrompt {
		m = pressLaunch(m, "tab")
	}
	m = pressLaunch(m, "enter")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, fmt.Sprintf("%d bytes is over the %d-byte limit", lim+1, lim)) {
		t.Errorf("no over-limit warning for the edited prompt:\n%s", v)
	}
	if got := m.launchDlg.prompt(); len(got) != lim+1 {
		t.Errorf("prompt is %d bytes", len(got))
	}
	// the real Launcher.Do refuses what the request carries, before it runs or fetches anything
	var ran [][]string
	launchDo = func(r launch.Request) (launch.Result, error) {
		l := launch.Launcher{
			Run: func(_ string, _ []byte, name string, args ...string) ([]byte, error) {
				ran = append(ran, append([]string{name}, args...))
				return nil, nil
			},
			Fetch: func(_, b string) (string, string, error) { ran = append(ran, []string{"fetch"}); return b, "", nil },
		}
		return l.Do(r)
	}
	for m.launchDlg.focused() == fPrompt {
		m = pressLaunch(m, "tab")
	}
	m = pressLaunch(m, "enter")
	if m.launchDlg == nil || m.launchDlg.running() || !strings.Contains(m.launchDlg.failure, "create workspace") || !strings.Contains(m.launchDlg.failure, fmt.Sprint(lim+1)) {
		t.Fatalf("an edited prompt over the ceiling was not refused at create: %+v", m.launchDlg)
	}
	if len(ran) != 0 || len(f.reqs) != 0 {
		t.Errorf("something ran for a refused prompt: %v", ran)
	}
}

func TestEditorExecReportsAFailureToPrepareTheFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()+"/does/not/exist")
	msg := editorExec("text", func(text string, err error) tea.Msg { return launchPromptMsg{seq: 7, text: text, err: err} })()
	got, ok := msg.(launchPromptMsg)
	if !ok || got.seq != 7 || got.err == nil {
		t.Errorf("msg = %#v; want the failure in the caller's message", msg)
	}
}

func TestAByteOrderMarkFromTheEditorIsNotAnEdit(t *testing.T) {
	d := onPrompt(t, newLaunchDialog(promptInput()))
	d = d.promptEdited("\ufeff"+promptInput().Context, nil)
	if d.promptDone || d.info != "prompt unchanged" {
		t.Errorf("promptDone = %v, info = %q; want the prompt unchanged", d.promptDone, d.info)
	}
	d = d.promptEdited("\ufeffFix it", nil)
	if got := d.prompt(); got != "Fix it" {
		t.Errorf("prompt() = %q; want the mark dropped", got)
	}
}
