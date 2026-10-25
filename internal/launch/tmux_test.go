package launch

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// call is one recorded invocation of the fake RunFunc.
type call struct {
	dir   string
	stdin string
	name  string
	args  []string
}

func (c call) line() string { return c.name + " " + strings.Join(c.args, " ") }

// recorder is a RunFunc that records calls and answers from reply.
type recorder struct {
	calls []call
	reply func(c call) ([]byte, error)
}

func (r *recorder) run(dir string, stdin []byte, name string, args ...string) ([]byte, error) {
	c := call{dir, string(stdin), name, args}
	r.calls = append(r.calls, c)
	if r.reply != nil {
		return r.reply(c)
	}
	return nil, nil
}

func (r *recorder) count(sub string) int {
	n := 0
	for _, c := range r.calls {
		if c.args[0] == sub {
			n++
		}
	}
	return n
}

func paneReply(c call) ([]byte, error) {
	if len(c.args) > 0 && (c.args[0] == "split-window" || c.args[0] == "new-window") {
		return []byte("%7\n"), nil
	}
	return nil, nil
}

func noEnv(string) string { return "" }

func equalArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestTmuxOpenPaneHorizontal(t *testing.T) {
	cases := []struct {
		name string
		r    Request
		want []string
	}{
		{"horizontal", Request{Place: PlacePane, Split: SplitHorizontal, Dir: "/work"},
			[]string{"split-window", "-h", "-P", "-F", "#{pane_id}", "-c", "/work"}},
		{"vertical", Request{Place: PlacePane, Split: SplitVertical, Dir: "/work"},
			[]string{"split-window", "-v", "-P", "-F", "#{pane_id}", "-c", "/work"}},
		{"window", Request{Place: PlaceWindow, Dir: "/work"},
			[]string{"new-window", "-P", "-F", "#{pane_id}", "-c", "/work"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{reply: paneReply}
			pane, err := newTmux(rec.run, noEnv).open(tc.r)
			if err != nil {
				t.Fatal(err)
			}
			if pane != "%7" {
				t.Errorf("pane = %q, want %%7", pane)
			}
			if len(rec.calls) != 1 || rec.calls[0].name != "tmux" || !equalArgs(rec.calls[0].args, tc.want) {
				t.Errorf("calls = %+v, want one tmux %v", rec.calls, tc.want)
			}
		})
	}
}

func tmuxPaneEnv(k string) string {
	if k == "TMUX_PANE" {
		return "%3"
	}
	return ""
}

func TestTmuxOpenTargetsTmuxPane(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "display-message" {
			return []byte("$4\n"), nil
		}
		return paneReply(c)
	}}
	cases := []struct {
		name string
		r    Request
		want [][]string
	}{
		{"horizontal", Request{Place: PlacePane, Split: SplitHorizontal, Dir: "/work"},
			[][]string{{"split-window", "-h", "-P", "-F", "#{pane_id}", "-t", "%3", "-c", "/work"}}},
		{"vertical", Request{Place: PlacePane, Split: SplitVertical, Dir: "/work"},
			[][]string{{"split-window", "-v", "-P", "-F", "#{pane_id}", "-t", "%3", "-c", "/work"}}},
		{"window", Request{Place: PlaceWindow, Dir: "/work"},
			[][]string{
				{"display-message", "-p", "-t", "%3", "#{session_id}"},
				{"new-window", "-P", "-F", "#{pane_id}", "-t", "$4", "-c", "/work"},
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec.calls = nil
			pane, err := newTmux(rec.run, tmuxPaneEnv).open(tc.r)
			if err != nil || pane != "%7" {
				t.Fatalf("pane = %q, err = %v", pane, err)
			}
			if len(rec.calls) != len(tc.want) {
				t.Fatalf("calls = %+v, want %v", rec.calls, tc.want)
			}
			for i, w := range tc.want {
				if !equalArgs(rec.calls[i].args, w) {
					t.Errorf("call %d = %q, want %v", i, rec.calls[i].line(), w)
				}
			}
		})
	}
}

// new-window rejects a pane as its target ("can't specify pane here"), so a
// -t on it must never look like %N.
func TestTmuxNewWindowNeverTargetsAPane(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "display-message" {
			return []byte("$0\n"), nil
		}
		return paneReply(c)
	}}
	if _, err := newTmux(rec.run, tmuxPaneEnv).open(Request{Place: PlaceWindow, Dir: "/w"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.calls {
		if c.args[0] != "new-window" {
			continue
		}
		for i, a := range c.args {
			if a == "-t" && (i+1 >= len(c.args) || strings.HasPrefix(c.args[i+1], "%")) {
				t.Errorf("new-window targets a pane: %q", c.line())
			}
		}
	}
}

func TestTmuxOpenWindowRejectsOddSessionID(t *testing.T) {
	for _, out := range []string{"", "0", "%3", "$", "$1 x", "oops"} {
		rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(out), nil }}
		_, err := newTmux(rec.run, tmuxPaneEnv).open(Request{Place: PlaceWindow})
		if err == nil || !strings.Contains(err.Error(), "instead of a session id") {
			t.Errorf("output %q: err = %v", out, err)
		}
		if rec.count("new-window") != 0 {
			t.Errorf("output %q: new-window ran", out)
		}
	}
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte("junk"), nil }}
	err := runSequence(newTmux(rec.run, tmuxPaneEnv), Request{Place: PlaceWindow}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepOpen || !strings.Contains(err.Error(), `"junk"`) {
		t.Errorf("err = %v, want StepOpen quoting the output", err)
	}
	rec = &recorder{reply: func(c call) ([]byte, error) { return nil, errors.New("no pane") }}
	if _, err := newTmux(rec.run, tmuxPaneEnv).open(Request{Place: PlaceWindow}); err == nil || len(rec.calls) != 1 {
		t.Errorf("display-message failure: err = %v, calls = %d", err, len(rec.calls))
	}
}

func TestTmuxOpenRejectsOddPaneID(t *testing.T) {
	for _, out := range []string{"", "7", "%7 extra", "no server running", "%7\n%8"} {
		rec := &recorder{reply: func(call) ([]byte, error) { return []byte(out), nil }}
		_, err := newTmux(rec.run, noEnv).open(Request{Place: PlaceWindow})
		if err == nil {
			t.Errorf("output %q accepted as a pane id", out)
			continue
		}
		if !strings.Contains(err.Error(), "instead of a pane id") {
			t.Errorf("err = %v", err)
		}
	}
	rec := &recorder{reply: func(call) ([]byte, error) { return []byte("junk"), nil }}
	err := runSequence(newTmux(rec.run, noEnv), Request{Place: PlaceWindow}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepOpen || !strings.Contains(err.Error(), `"junk"`) {
		t.Errorf("err = %v, want StepOpen quoting the output", err)
	}
}

func TestTmuxEveryLaterCommandNamesThePane(t *testing.T) {
	rec := &recorder{}
	sp := newTmux(rec.run, noEnv)
	// The marker is already on screen once at start, so ready needs a second.
	n := 0
	rec.reply = func(c call) ([]byte, error) {
		switch c.args[0] {
		case "capture-pane":
			n++
			if n <= 2 { // start's two settle checks
				return []byte("Claude Code v1"), nil
			}
			return []byte("Claude Code v1\nClaude Code v1"), nil
		case "display-message":
			return []byte("claude\n"), nil
		}
		return paneReply(c)
	}
	r := Request{Agent: Agent{TmuxCommand: "claude", ReadyMarker: "Claude Code v"}, Place: PlaceWindow, Dir: "/w"}
	pane, err := sp.open(r)
	if err != nil {
		t.Fatal(err)
	}
	rec.calls = nil
	if err := sp.start(pane, r); err != nil {
		t.Fatal(err)
	}
	if err := sp.ready(pane, r, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := sp.paste(pane, "text"); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) == 0 {
		t.Fatal("no calls recorded")
	}
	for _, c := range rec.calls {
		if c.args[0] == "load-buffer" {
			// load-buffer only fills a named buffer; it targets no pane.
			continue
		}
		found := false
		for i, a := range c.args {
			if a == "-t" && i+1 < len(c.args) && c.args[i+1] == "%7" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q does not name -t %%7", c.line())
		}
	}
}

func TestTmuxOpenFailureRunsNothingElse(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return nil, errors.New("no server") }}
	f := newTmux(rec.run, noEnv)
	err := runSequence(f, Request{Place: PlacePane, Split: SplitHorizontal, Context: "x"}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepOpen {
		t.Fatalf("err = %v, want StepOpen", err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("recorded %d calls, want only the failed open: %+v", len(rec.calls), rec.calls)
	}
}

func TestTmuxStartTypesQuotedCommand(t *testing.T) {
	claude, _ := LookupAgent("claude")
	cases := []struct{ model, want string }{
		{"opus", "claude --model 'opus'"},
		{"", "claude"},
		{"x'; rm -rf ~ #", `claude --model 'x'\''; rm -rf ~ #'`},
		{"a b$(id)`id`", "claude --model 'a b$(id)`id`'"},
	}
	for _, tc := range cases {
		if got := startCommand(claude, tc.model); got != tc.want {
			t.Errorf("startCommand(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}

	rec := &recorder{reply: func(call) ([]byte, error) { return []byte("$ "), nil }}
	r := Request{Agent: claude, Model: "opus"}
	if err := newTmux(rec.run, noEnv).start("%7", r); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"capture-pane", "-p", "-t", "%7"},
		{"capture-pane", "-p", "-t", "%7"},
		{"send-keys", "-t", "%7", "-l", "claude --model 'opus'"},
		{"send-keys", "-t", "%7", "Enter"},
	}
	if len(rec.calls) != len(want) {
		t.Fatalf("calls = %+v, want %v", rec.calls, want)
	}
	for i, w := range want {
		if rec.calls[i].name != "tmux" || !equalArgs(rec.calls[i].args, w) {
			t.Errorf("call %d = %q, want tmux %v", i, rec.calls[i].line(), w)
		}
	}
}

func TestTmuxStartRejectsControlCharacters(t *testing.T) {
	claude, _ := LookupAgent("claude")
	for _, model := range []string{"a\nb", "a\rb", "a\x1bb", "a\x00b", "a\x7fb", "a\tb"} {
		rec := &recorder{}
		err := newTmux(rec.run, noEnv).start("%7", Request{Agent: claude, Model: model})
		if err == nil {
			t.Errorf("model %q accepted", model)
		}
		if len(rec.calls) != 0 {
			t.Errorf("model %q: typed %d commands before rejecting", model, len(rec.calls))
		}
	}
	bad := claude
	bad.TmuxCommand = "claude\n; rm -rf ~"
	rec := &recorder{}
	if err := newTmux(rec.run, noEnv).start("%7", Request{Agent: bad}); err == nil || len(rec.calls) != 0 {
		t.Errorf("command with newline: err = %v, calls = %d", err, len(rec.calls))
	}
	rec = &recorder{reply: paneReply}
	err := runSequence(newTmux(rec.run, noEnv), Request{Agent: claude, Model: "a\nb", Place: PlaceWindow}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepStart {
		t.Errorf("err = %v, want StepStart", err)
	}
	if rec.count("send-keys") != 0 {
		t.Error("send-keys ran for a rejected model")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":         `''`,
		"plain":    `'plain'`,
		"it's":     `'it'\''s'`,
		"a;b":      `'a;b'`,
		"$(x) `y`": "'$(x) `y`'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// readyScript plays a pane for the ready tests: capture-pane answers come from
// captures in order (the last repeats) and display-message answers with cmd.
type readyScript struct {
	captures []string
	cmd      string
	n        int
}

func (s *readyScript) reply(c call) ([]byte, error) {
	switch c.args[0] {
	case "capture-pane":
		i := s.n
		if i >= len(s.captures) {
			i = len(s.captures) - 1
		}
		s.n++
		return []byte(s.captures[i]), nil
	case "display-message":
		return []byte(s.cmd + "\n"), nil
	}
	return nil, nil
}

func TestMain(m *testing.M) {
	readyPollInterval = time.Millisecond
	settleTimeout = 20 * time.Millisecond
	os.Exit(m.Run())
}

// runReady starts then waits on a scripted pane and reports whether the
// sequence got as far as pasting.
func runReady(t *testing.T, s *readyScript, marker string, timeout time.Duration) (error, bool, *recorder) {
	t.Helper()
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "split-window" || c.args[0] == "new-window" {
			return []byte("%7\n"), nil
		}
		return s.reply(c)
	}}
	r := Request{Agent: Agent{Label: "Agent", TmuxCommand: "agent", ReadyMarker: marker}, Place: PlaceWindow, Context: "secret"}
	err := runSequence(newTmux(rec.run, noEnv), r, timeout)
	return err, rec.count("paste-buffer") > 0, rec
}

func TestTmuxReadyPollsCaptureUntilMarker(t *testing.T) {
	s := &readyScript{captures: []string{"$ ", "$ ", "$ agent", "loading...", "$ agent\nAgent v1"}, cmd: "agent"}
	err, pasted, rec := runReady(t, s, "Agent v", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !pasted {
		t.Error("never pasted")
	}
	// start's two settle checks, then three polls.
	if got := rec.count("capture-pane"); got != 5 {
		t.Errorf("%d capture-pane calls, want 5", got)
	}
}

func TestTmuxReadyPromptContainingMarkerIsNotReady(t *testing.T) {
	// The shell prompt itself contains the marker, before and after the agent
	// would start, and the typed command shows it too.
	s := &readyScript{captures: []string{"❯ ", "❯ ", "❯ agent"}, cmd: "zsh"}
	err, pasted, _ := runReady(t, s, "❯", 30*time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepReady {
		t.Fatalf("err = %v, want StepReady", err)
	}
	if pasted {
		t.Error("pasted into the shell")
	}
}

func TestTmuxReadyMarkerOnlyInBaselineIsNotReady(t *testing.T) {
	s := &readyScript{captures: []string{"Agent v1 (old screen)"}, cmd: "agent"}
	err, pasted, _ := runReady(t, s, "Agent v", 30*time.Millisecond)
	if err == nil || pasted {
		t.Errorf("err = %v, pasted = %v; want not ready", err, pasted)
	}
}

func TestTmuxStartFailsWhenPaneNeverSettles(t *testing.T) {
	claude, _ := LookupAgent("claude")
	for name, reply := range map[string]func(call) ([]byte, error){
		"blank":    func(call) ([]byte, error) { return []byte("  \n"), nil },
		"changing": nil,
	} {
		t.Run(name, func(t *testing.T) {
			n := 0
			rec := &recorder{reply: func(c call) ([]byte, error) {
				if reply != nil {
					return reply(c)
				}
				n++
				return []byte(strings.Repeat("x", n)), nil
			}}
			err := newTmux(rec.run, noEnv).start("%7", Request{Agent: claude})
			if err == nil || !strings.Contains(err.Error(), "did not settle") {
				t.Fatalf("err = %v, want a settle error", err)
			}
			if rec.count("send-keys") != 0 {
				t.Error("typed into a pane that never settled")
			}
		})
	}
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "split-window" {
			return []byte("%7\n"), nil
		}
		return []byte(" "), nil
	}}
	err := runSequence(newTmux(rec.run, noEnv), Request{Agent: claude, Place: PlacePane, Split: SplitHorizontal, Context: "c"}, time.Second)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepStart {
		t.Errorf("err = %v, want StepStart", err)
	}
	if rec.count("send-keys")+rec.count("paste-buffer") != 0 {
		t.Error("sent keys after a settle failure")
	}
}

func TestTmuxReadySlowShellPromptIsInTheBaseline(t *testing.T) {
	// The shell has not drawn its prompt when start is called; the baseline
	// must wait for it, or the prompt would look like a new marker while the
	// agent is already the foreground command but has printed nothing yet.
	s := &readyScript{captures: []string{"", "", "❯ ", "❯ ", "❯ agent"}, cmd: "node"}
	err, pasted, _ := runReady(t, s, "❯", 30*time.Millisecond)
	if err == nil || pasted {
		t.Errorf("err = %v, pasted = %v; want not ready", err, pasted)
	}
}

func TestTmuxReadyNewMarkerButPaneStillShellIsNotReady(t *testing.T) {
	for _, cmd := range []string{"zsh", "-zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh", "nu", "pwsh"} {
		s := &readyScript{captures: []string{"$ ", "$ ", "$ agent\nAgent v1"}, cmd: cmd}
		err, pasted, _ := runReady(t, s, "Agent v", 30*time.Millisecond)
		if err == nil || pasted {
			t.Errorf("pane command %q: err = %v, pasted = %v; want not ready", cmd, err, pasted)
		}
	}
}

func TestTmuxReadyNewMarkerAndAgentInForeground(t *testing.T) {
	s := &readyScript{captures: []string{"❯ ", "❯ ", "❯ agent\n❯ ask me"}, cmd: "node"}
	err, pasted, rec := runReady(t, s, "❯", time.Second)
	if err != nil || !pasted {
		t.Fatalf("err = %v, pasted = %v; want ready and pasted", err, pasted)
	}
	last := rec.calls[len(rec.calls)-1]
	if !equalArgs(last.args, []string{"send-keys", "-t", "%7", "Enter"}) {
		t.Errorf("last call = %q", last.line())
	}
}

func TestTmuxReadyFailuresAreHard(t *testing.T) {
	r := Request{Agent: Agent{ReadyMarker: "Agent v"}}
	rec := &recorder{reply: func(c call) ([]byte, error) { return nil, errors.New("can't find pane") }}
	if err := newTmux(rec.run, noEnv).ready("%7", r, time.Second); err == nil || !strings.Contains(err.Error(), "can't find pane") {
		t.Errorf("capture-pane failure: err = %v", err)
	}
	rec = &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "display-message" {
			return nil, errors.New("display failed")
		}
		return []byte("Agent v1"), nil
	}}
	if err := newTmux(rec.run, noEnv).ready("%7", r, time.Second); err == nil || !strings.Contains(err.Error(), "display failed") {
		t.Errorf("display-message failure: err = %v", err)
	}
}

func TestTmuxReadyEmptyMarkerIsAnError(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte("anything"), nil }}
	err := newTmux(rec.run, noEnv).ready("%7", Request{Agent: Agent{Label: "Agent"}}, time.Second)
	if err == nil {
		t.Fatal("empty marker treated as ready")
	}
	if len(rec.calls) != 0 {
		t.Errorf("ran %d commands for an empty marker", len(rec.calls))
	}
	s := &readyScript{captures: []string{"x"}, cmd: "agent"}
	err, pasted, _ := runReady(t, s, "", time.Second)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepReady || pasted {
		t.Errorf("sequence: err = %v, pasted = %v", err, pasted)
	}
}

func TestTmuxReadyTimesOut(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte("nothing"), nil }}
	r := Request{Agent: Agent{ReadyMarker: "Claude Code v"}}
	if err := newTmux(rec.run, noEnv).ready("%7", r, 20*time.Millisecond); err == nil {
		t.Fatal("ready returned nil without the marker")
	}
}

func TestTmuxPasteUsesBuffer(t *testing.T) {
	rec := &recorder{}
	text := "line one\n'quoted' $(x) `y`; 中文\n"
	if err := newTmux(rec.run, noEnv).paste("%7", text); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 3 {
		t.Fatalf("calls = %+v, want 3", rec.calls)
	}
	load, paste, enter := rec.calls[0], rec.calls[1], rec.calls[2]
	if len(load.args) != 4 || load.args[0] != "load-buffer" || load.args[1] != "-b" || load.args[3] != "-" {
		t.Fatalf("load = %q, want load-buffer -b <name> -", load.line())
	}
	name := load.args[2]
	if load.stdin != text {
		t.Errorf("stdin = %q, want the text", load.stdin)
	}
	if !equalArgs(paste.args, []string{"paste-buffer", "-p", "-d", "-b", name, "-t", "%7"}) {
		t.Errorf("paste = %q", paste.line())
	}
	if !equalArgs(enter.args, []string{"send-keys", "-t", "%7", "Enter"}) {
		t.Errorf("enter = %q", enter.line())
	}
}

func TestTmuxPasteBufferFailureDeletesBuffer(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) {
		switch c.args[0] {
		case "paste-buffer":
			return nil, errors.New("pane gone")
		case "delete-buffer":
			return nil, errors.New("ignored")
		}
		return nil, nil
	}}
	err := newTmux(rec.run, noEnv).paste("%7", "text")
	if err == nil || err.Error() != "pane gone" {
		t.Fatalf("err = %v, want the paste-buffer error", err)
	}
	if len(rec.calls) != 3 {
		t.Fatalf("calls = %+v, want load, paste, delete", rec.calls)
	}
	name := rec.calls[0].args[2]
	if !equalArgs(rec.calls[2].args, []string{"delete-buffer", "-b", name}) {
		t.Errorf("third call = %q", rec.calls[2].line())
	}
}
