package launch

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// herdrSplitOut and herdrTabOut are what herdr 0.8.2 printed for
// `herdr pane split` and `herdr tab create` (see the comment above
// parseHerdrPaneID).
const herdrSplitOut = `{"id":"cli:pane:split","result":{"pane":{"agent_status":"unknown","cwd":"/work","focused":false,"foreground_cwd":"/work","pane_id":"w1:p2","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w1:t1","terminal_id":"term_65ea6f02804e82","workspace_id":"w1"},"type":"pane_info"}}` + "\n"

const herdrTabOut = `{"id":"cli:tab:create","result":{"root_pane":{"agent_status":"unknown","cwd":"/work","focused":false,"foreground_cwd":"/work","pane_id":"w1:p3","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w1:t2","terminal_id":"term_65ea6f02831273","workspace_id":"w1"},"tab":{"agent_status":"unknown","focused":false,"label":"x","number":2,"pane_count":1,"tab_id":"w1:t2","workspace_id":"w1"},"type":"tab_created"}}` + "\n"

// herdrReply answers like herdr: the JSON the real tool printed for split and
// tab create, and nothing for the rest.
func herdrReply(c call) ([]byte, error) {
	if len(c.args) >= 2 && c.args[0] == "pane" && c.args[1] == "split" {
		return []byte(herdrSplitOut), nil
	}
	if len(c.args) >= 2 && c.args[0] == "tab" && c.args[1] == "create" {
		return []byte(herdrTabOut), nil
	}
	return nil, nil
}

func herdrEnv(k string) string {
	switch k {
	case "HERDR_PANE_ID":
		return "w1:p1"
	case "HERDR_WORKSPACE_ID":
		return "w1"
	}
	return ""
}

func paneOnlyEnv(k string) string {
	if k == "HERDR_PANE_ID" {
		return "w1:p1"
	}
	return ""
}

func TestHerdrOpenPaneSplit(t *testing.T) {
	cases := []struct {
		name string
		r    Request
		want []string
	}{
		{"horizontal", Request{Place: PlacePane, Split: SplitHorizontal, Dir: "/work"},
			[]string{"pane", "split", "--pane", "w1:p1", "--direction", "right", "--cwd", "/work"}},
		{"vertical", Request{Place: PlacePane, Split: SplitVertical, Dir: "/work"},
			[]string{"pane", "split", "--pane", "w1:p1", "--direction", "down", "--cwd", "/work"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{reply: herdrReply}
			pane, err := newHerdr(rec.run, herdrEnv).open(tc.r)
			if err != nil {
				t.Fatal(err)
			}
			if pane != "w1:p2" {
				t.Errorf("pane = %q, want w1:p2", pane)
			}
			if len(rec.calls) != 1 || rec.calls[0].name != "herdr" || !equalArgs(rec.calls[0].args, tc.want) {
				t.Errorf("calls = %+v, want one herdr %v", rec.calls, tc.want)
			}
		})
	}
}

func TestHerdrOpenWindowUsesTab(t *testing.T) {
	cases := []struct {
		name string
		env  func(string) string
		r    Request
		want []string
	}{
		{"workspace", herdrEnv, Request{Place: PlaceWindow, Dir: "/work", Issue: 42},
			[]string{"tab", "create", "--workspace", "w1", "--cwd", "/work", "--label", "issue-42"}},
		{"no workspace", paneOnlyEnv, Request{Place: PlaceWindow, Dir: "/work", Issue: 42},
			[]string{"tab", "create", "--cwd", "/work", "--label", "issue-42"}},
		{"no issue", paneOnlyEnv, Request{Place: PlaceWindow, Dir: "/work"},
			[]string{"tab", "create", "--cwd", "/work", "--label", "agent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{reply: herdrReply}
			pane, err := newHerdr(rec.run, tc.env).open(tc.r)
			if err != nil {
				t.Fatal(err)
			}
			if pane != "w1:p3" {
				t.Errorf("pane = %q, want the tab's root pane w1:p3", pane)
			}
			if len(rec.calls) != 1 || rec.calls[0].name != "herdr" || !equalArgs(rec.calls[0].args, tc.want) {
				t.Errorf("calls = %+v, want one herdr %v", rec.calls, tc.want)
			}
		})
	}
}

func TestHerdrOpenOutsideAPaneIsAnError(t *testing.T) {
	for _, place := range []Place{PlacePane, PlaceWindow} {
		rec := &recorder{reply: herdrReply}
		_, err := newHerdr(rec.run, noEnv).open(Request{Place: place, Dir: "/work"})
		if err == nil || !strings.Contains(err.Error(), "not in a herdr pane") {
			t.Errorf("place %v: err = %v", place, err)
		}
		if len(rec.calls) != 0 {
			t.Errorf("place %v: ran %+v without a pane to anchor on", place, rec.calls)
		}
	}
}

func TestHerdrOpenRejectsOddOutput(t *testing.T) {
	outs := []string{
		"",
		"w1:p2",
		"junk",
		"no server running",
		`{"error":{"code":"pane_not_found","message":"pane not found"}}`,
		`{"result":{"pane":{}}}`,
		`{"result":{"pane":{"pane_id":""}}}`,
		`{"result":{"pane":{"pane_id":"w1:p2 extra"}}}`,
		`{"result":{"pane":{"pane_id":"%7"}}}`,
		`{"result":{"pane":{"pane_id":"w1:t2"}}}`,
		`{"result":{"pane":{"pane_id":"w1:p2; rm -rf ~"}}}`,
		`{"result":{"pane":{"pane_id":"--help"}}}`,
		`{"result":{"pane":{"pane_id":7}}}`,
		`[]`,
	}
	for _, place := range []Place{PlacePane, PlaceWindow} {
		for _, out := range outs {
			rec := &recorder{reply: func(call) ([]byte, error) { return []byte(out), nil }}
			pane, err := newHerdr(rec.run, herdrEnv).open(Request{Place: place})
			if err == nil {
				t.Errorf("place %v: output %q accepted as pane %q", place, out, pane)
				continue
			}
			if !strings.Contains(err.Error(), "instead of a pane id") {
				t.Errorf("place %v: err = %v", place, err)
			}
		}
	}
	// A tab's reply must carry root_pane; a split's reply must carry pane.
	rec := &recorder{reply: func(call) ([]byte, error) { return []byte(herdrSplitOut), nil }}
	if _, err := newHerdr(rec.run, herdrEnv).open(Request{Place: PlaceWindow}); err == nil {
		t.Error("split reply accepted for a tab create")
	}
	rec = &recorder{reply: func(call) ([]byte, error) { return []byte(herdrTabOut), nil }}
	if _, err := newHerdr(rec.run, herdrEnv).open(Request{Place: PlacePane, Split: SplitHorizontal}); err == nil {
		t.Error("tab reply accepted for a split")
	}

	rec = &recorder{reply: func(call) ([]byte, error) { return []byte("junk"), nil }}
	err := runSequence(newHerdr(rec.run, herdrEnv), Request{Place: PlaceWindow}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepOpen || !strings.Contains(err.Error(), `"junk"`) {
		t.Errorf("err = %v, want StepOpen quoting the output", err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("ran %d commands after an unusable open: %+v", len(rec.calls), rec.calls)
	}
}

func TestHerdrOpenFailureRunsNothingElse(t *testing.T) {
	rec := &recorder{reply: func(call) ([]byte, error) { return nil, errors.New("no server") }}
	claude, _ := LookupAgent("claude")
	err := runSequence(newHerdr(rec.run, herdrEnv), Request{Agent: claude, Place: PlacePane, Split: SplitHorizontal, Context: "x"}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepOpen {
		t.Fatalf("err = %v, want StepOpen", err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("recorded %d calls, want only the failed open: %+v", len(rec.calls), rec.calls)
	}
}

func TestHerdrStartPassesModel(t *testing.T) {
	claude, _ := LookupAgent("claude")
	cases := []struct {
		name  string
		model string
		want  []string
	}{
		{"model", "opus", []string{"agent", "start", "gh9s-w1-p2", "--kind", "claude", "--pane", "w1:p2", "--timeout", "30000", "--", "--model", "opus"}},
		{"no model", "", []string{"agent", "start", "gh9s-w1-p2", "--kind", "claude", "--pane", "w1:p2", "--timeout", "30000"}},
		// The model is one argv element, so the shell never sees it.
		{"hostile model", "x'; rm -rf ~ #$(id)", []string{"agent", "start", "gh9s-w1-p2", "--kind", "claude", "--pane", "w1:p2", "--timeout", "30000", "--", "--model", "x'; rm -rf ~ #$(id)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			if err := newHerdr(rec.run, herdrEnv).start("w1:p2", Request{Agent: claude, Model: tc.model}); err != nil {
				t.Fatal(err)
			}
			if len(rec.calls) != 1 || rec.calls[0].name != "herdr" || !equalArgs(rec.calls[0].args, tc.want) {
				t.Errorf("calls = %+v, want one herdr %v", rec.calls, tc.want)
			}
		})
	}
}

func TestHerdrStartNameIsValidForHerdr(t *testing.T) {
	// herdr names must match [a-z][a-z0-9_-]{0,31}.
	for _, pane := range []string{"w1:p2", "w12:p345", "w1000000:p1000000"} {
		name := herdrAgentName(pane)
		if !herdrNamePattern.MatchString(name) {
			t.Errorf("name %q for pane %q is not a valid herdr agent name", name, pane)
		}
	}
}

func TestHerdrStartRejectsControlCharacters(t *testing.T) {
	claude, _ := LookupAgent("claude")
	for _, model := range []string{"a\nb", "a\rb", "a\x1bb", "a\x00b", "a\x7fb", "a\tb"} {
		rec := &recorder{}
		err := newHerdr(rec.run, herdrEnv).start("w1:p2", Request{Agent: claude, Model: model})
		if err == nil {
			t.Errorf("model %q accepted", model)
		}
		if len(rec.calls) != 0 {
			t.Errorf("model %q: ran %d herdr commands before rejecting", model, len(rec.calls))
		}
	}
	rec := &recorder{reply: herdrReply}
	err := runSequence(newHerdr(rec.run, herdrEnv), Request{Agent: claude, Model: "a\nb", Place: PlaceWindow, Context: "c"}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepStart {
		t.Errorf("err = %v, want StepStart", err)
	}
	for _, c := range rec.calls {
		if c.args[0] == "agent" {
			t.Errorf("ran %q for a rejected model", c.line())
		}
	}
}

func TestHerdrStartFailureStopsTheSequence(t *testing.T) {
	claude, _ := LookupAgent("claude")
	rec := &recorder{reply: func(c call) ([]byte, error) {
		if c.args[0] == "agent" {
			return nil, errors.New(`{"error":{"code":"agent_not_ready"}}`)
		}
		return herdrReply(c)
	}}
	err := runSequence(newHerdr(rec.run, herdrEnv), Request{Agent: claude, Place: PlaceWindow, Context: "secret"}, time.Millisecond)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepStart || !strings.Contains(err.Error(), "agent_not_ready") {
		t.Fatalf("err = %v, want StepStart with herdr's error", err)
	}
	if len(rec.calls) != 2 {
		t.Errorf("calls = %+v, want open and the failed start only", rec.calls)
	}
}

func TestHerdrReadyIsANoOp(t *testing.T) {
	rec := &recorder{}
	if err := newHerdr(rec.run, herdrEnv).ready("w1:p2", Request{}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("ready ran %+v", rec.calls)
	}
}

func TestHerdrPasteUsesAgentPrompt(t *testing.T) {
	rec := &recorder{}
	text := "請解決這個 issue\n\n# Title (#1)\n'quoted' $(x) `y`; \"z\"\n"
	if err := newHerdr(rec.run, herdrEnv).paste("w1:p2", text, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent", "prompt", "w1:p2", text}
	if len(rec.calls) != 1 || rec.calls[0].name != "herdr" || !equalArgs(rec.calls[0].args, want) {
		t.Errorf("calls = %+v, want one herdr %q", rec.calls, want)
	}
	if rec.calls[0].stdin != "" {
		t.Errorf("stdin = %q, want none", rec.calls[0].stdin)
	}

	rec = &recorder{reply: func(call) ([]byte, error) { return nil, errors.New("agent_blocked") }}
	if err := newHerdr(rec.run, herdrEnv).paste("w1:p2", "x", true); err == nil || err.Error() != "agent_blocked" {
		t.Errorf("err = %v, want herdr's error", err)
	}
}

func TestHerdrEveryLaterCommandNamesThePane(t *testing.T) {
	for _, place := range []Place{PlacePane, PlaceWindow} {
		rec := &recorder{reply: herdrReply}
		claude, _ := LookupAgent("claude")
		r := Request{Agent: claude, Model: "opus", Place: place, Split: SplitHorizontal, Dir: "/work", Context: "text"}
		if err := runSequence(newHerdr(rec.run, herdrEnv), r, time.Second); err != nil {
			t.Fatal(err)
		}
		pane := "w1:p2"
		if place == PlaceWindow {
			pane = "w1:p3"
		}
		if len(rec.calls) != 3 {
			t.Fatalf("calls = %+v, want open, start, prompt", rec.calls)
		}
		for _, c := range rec.calls[1:] {
			found := false
			for _, a := range c.args {
				if a == pane {
					found = true
				}
				if a == "w1:p1" {
					t.Errorf("%q names the calling pane", c.line())
				}
			}
			if !found {
				t.Errorf("%q does not name pane %s", c.line(), pane)
			}
		}
	}
}

// Measured live with herdr 0.8.2 in a pane that echoed raw input (stty raw; cat > file): `herdr pane
// send-text` writes the text byte for byte, adds no Enter, does not wrap it in a bracketed paste even
// when the program asked for bracketed paste, and passes ESC through untouched. A newline inside it
// arrives as a bare LF, which an agent could take for Enter. So paste-only wraps the text itself.
func TestHerdrPasteOnlyWrapsInBracketedPasteAndNeverSubmits(t *testing.T) {
	rec := &recorder{}
	text := "請解決這個 issue\n\n# Title (#1)\n'quoted' $(x) `y`; \"z\"\n"
	if err := newHerdr(rec.run, herdrEnv).paste("w1:p2", text, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"pane", "send-text", "w1:p2", "\x1b[200~" + text + "\x1b[201~"}
	if len(rec.calls) != 1 || rec.calls[0].name != "herdr" || !equalArgs(rec.calls[0].args, want) {
		t.Fatalf("calls = %+v, want one herdr %q", rec.calls, want)
	}
	if rec.calls[0].stdin != "" {
		t.Errorf("stdin = %q, want none", rec.calls[0].stdin)
	}
	if strings.Contains(rec.calls[0].line(), "prompt") || strings.Contains(rec.calls[0].line(), "run") {
		t.Errorf("paste-only must not use a submitting herdr command: %q", rec.calls[0].line())
	}

	rec = &recorder{reply: func(call) ([]byte, error) { return nil, errors.New("pane_not_found") }}
	if err := newHerdr(rec.run, herdrEnv).paste("w1:p2", "x", false); err == nil || err.Error() != "pane_not_found" {
		t.Errorf("err = %v, want herdr's error", err)
	}
}

// BuildContext strips ESC, so the text cannot end the paste early; the herdr space checks it again and
// sends nothing if an ESC or other control character is left.
func TestHerdrPasteOnlyRefusesAnEscapeInTheText(t *testing.T) {
	for _, text := range []string{"a\x1b[201~echo hi\r", "a\x1bb", "a\rb", "a\x7fb", "a\x00b", "a\u009bb", "a\x9b201~b", "a\xffb", "a\xc0\x80b"} {
		rec := &recorder{}
		err := newHerdr(rec.run, herdrEnv).paste("w1:p2", text, false)
		if err == nil || !strings.Contains(err.Error(), "control character") {
			t.Errorf("%q: err = %v, want a control character refusal", text, err)
		}
		if len(rec.calls) != 0 {
			t.Errorf("%q: ran %+v, want nothing sent", text, rec.calls)
		}
	}
}

func TestHerdrPasteOnlyKeepsNewlineAndTab(t *testing.T) {
	rec := &recorder{}
	if err := newHerdr(rec.run, herdrEnv).paste("w1:p2", "a\n\tb", false); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("calls = %+v", rec.calls)
	}
}

func TestHerdrSequencePasteOnlyEndsWithSendText(t *testing.T) {
	rec := &recorder{reply: herdrReply}
	claude, _ := LookupAgent("claude")
	r := Request{Agent: claude, Place: PlacePane, Split: SplitHorizontal, Dir: "/work", Context: "text", PasteOnly: true}
	if err := runSequence(newHerdr(rec.run, herdrEnv), r, time.Second); err != nil {
		t.Fatal(err)
	}
	last := rec.calls[len(rec.calls)-1]
	if !equalArgs(last.args, []string{"pane", "send-text", "w1:p2", "\x1b[200~text\x1b[201~"}) {
		t.Errorf("last call = %q", last.line())
	}
}

// An unsafe text is refused before a pane is opened, so nothing is left behind for a launch that cannot
// succeed. Submit mode is not checked (herdr agent prompt handles the text itself).
func TestHerdrSequencePasteOnlyRefusesUnsafeTextBeforeOpening(t *testing.T) {
	claude, _ := LookupAgent("claude")
	for _, text := range []string{"a\x1bb", "a\x9b201~b", "a\xffb"} {
		rec := &recorder{reply: herdrReply}
		r := Request{Agent: claude, Place: PlacePane, Split: SplitHorizontal, Dir: "/work", Context: text, PasteOnly: true}
		err := runSequence(newHerdr(rec.run, herdrEnv), r, time.Second)
		var se *StepError
		if !errors.As(err, &se) || !strings.Contains(err.Error(), "control character") {
			t.Errorf("%q: err = %v, want a refusal", text, err)
		}
		if len(rec.calls) != 0 {
			t.Errorf("%q: ran %+v, want nothing opened", text, rec.calls)
		}
	}
	rec := &recorder{reply: herdrReply}
	r := Request{Agent: claude, Place: PlacePane, Split: SplitHorizontal, Dir: "/work", Context: "a\x1bb"}
	if err := runSequence(newHerdr(rec.run, herdrEnv), r, time.Second); err != nil {
		t.Errorf("submit mode: err = %v, want it unchanged", err)
	}
}
