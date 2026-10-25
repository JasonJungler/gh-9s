package launch

import (
	"errors"
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

func paneReply(c call) ([]byte, error) {
	if len(c.args) > 0 && (c.args[0] == "split-window" || c.args[0] == "new-window") {
		return []byte("%7\n"), nil
	}
	return nil, nil
}

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
			pane, err := newTmux(rec.run).open(tc.r)
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

func TestTmuxEveryLaterCommandNamesThePane(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "capture-pane" {
			return []byte("Claude Code v1"), nil
		}
		return paneReply(c)
	}}
	sp := newTmux(rec.run)
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
	f := newTmux(rec.run)
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

	rec := &recorder{}
	r := Request{Agent: claude, Model: "opus"}
	if err := newTmux(rec.run).start("%7", r); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
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

func TestTmuxReadyPollsCaptureUntilMarker(t *testing.T) {
	old := readyPollInterval
	readyPollInterval = time.Millisecond
	defer func() { readyPollInterval = old }()

	n := 0
	rec := &recorder{reply: func(c call) ([]byte, error) {
		n++
		if n < 3 {
			return []byte("loading..."), nil
		}
		return []byte("Claude Code v2"), nil
	}}
	r := Request{Agent: Agent{ReadyMarker: "Claude Code v"}}
	if err := newTmux(rec.run).ready("%7", r, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 3 {
		t.Fatalf("%d capture-pane calls, want 3", len(rec.calls))
	}
	for _, c := range rec.calls {
		if c.name != "tmux" || !equalArgs(c.args, []string{"capture-pane", "-p", "-t", "%7"}) {
			t.Errorf("call = %q, want tmux capture-pane -p -t %%7", c.line())
		}
	}
}

func TestTmuxReadyTimesOut(t *testing.T) {
	old := readyPollInterval
	readyPollInterval = time.Millisecond
	defer func() { readyPollInterval = old }()

	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte("nothing"), nil }}
	r := Request{Agent: Agent{ReadyMarker: "Claude Code v"}}
	if err := newTmux(rec.run).ready("%7", r, 20*time.Millisecond); err == nil {
		t.Fatal("ready returned nil without the marker")
	}
}

func TestTmuxPasteUsesBuffer(t *testing.T) {
	rec := &recorder{}
	text := "line one\n'quoted' $(x) `y`; 中文\n"
	if err := newTmux(rec.run).paste("%7", text); err != nil {
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
