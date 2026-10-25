package launch

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fetchCall struct{ dir, base string }

func TestLauncherFetchBeforeCreate(t *testing.T) {
	var order []string
	var fetches []fetchCall
	rec := &recorder{reply: func(c call) ([]byte, error) {
		order = append(order, "orca")
		return []byte(orcaCreateAgentOut), nil
	}}
	l := Launcher{
		Run: rec.run, Getenv: noEnv,
		Fetch: func(dir, base string) (string, string, error) {
			order = append(order, "fetch")
			fetches = append(fetches, fetchCall{dir, base})
			return base, "no remote, using the local branch", nil
		},
	}
	res, err := l.Do(orcaReq())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "fetch,orca" {
		t.Errorf("order = %v, want fetch then orca", order)
	}
	if len(fetches) != 1 || fetches[0] != (fetchCall{"/work", "origin/main"}) {
		t.Errorf("fetches = %+v, want one (/work, origin/main)", fetches)
	}
	if res.Note != "no remote, using the local branch" {
		t.Errorf("Note = %q, want the fetch note", res.Note)
	}
	if res.Branch != "MurabitoB/issue-42" || res.Where != "issue-42" {
		t.Errorf("res = %+v", res)
	}
}

func TestLauncherFetchNoteJoinsOrcaNote(t *testing.T) {
	r := orcaReq()
	r.Context = strings.Repeat("a", orcaPromptMax+1)
	rec := &recorder{reply: fallbackReply(orcaSendWarnOut)}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(_, b string) (string, string, error) { return b, "fetched", nil }}
	res, err := l.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Note, "fetched; ") || len(strings.Split(res.Note, "; ")) != 2 {
		t.Errorf("Note = %q, want the fetch note then orca's", res.Note)
	}
}

func TestLauncherFetchFailureStopsEverything(t *testing.T) {
	rec := &recorder{}
	l := Launcher{
		Run: rec.run, Getenv: noEnv,
		Fetch: func(dir, base string) (string, string, error) { return "", "", errors.New("could not resolve host") },
	}
	_, err := l.Do(orcaReq())
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepFetch {
		t.Fatalf("err = %v, want a fetch step error", err)
	}
	if !strings.Contains(err.Error(), "could not resolve host") {
		t.Errorf("err = %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("orca was called after a failed fetch: %+v", rec.calls)
	}
}

func TestLauncherWithoutFetchRefusesOrca(t *testing.T) {
	rec := &recorder{}
	_, err := Launcher{Run: rec.run, Getenv: noEnv}.Do(orcaReq())
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepFetch || len(rec.calls) != 0 {
		t.Errorf("err = %v, calls = %+v", err, rec.calls)
	}
}

func TestLauncherOrcaFailureKeepsStep(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaLostOut), errors.New("exit status 1") }}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(_, b string) (string, string, error) { return b, "", nil }}
	_, err := l.Do(orcaReq())
	var lost *LostReplyError
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepCreate || !errors.As(err, &lost) || lost.OperationID != orcaOp {
		t.Errorf("err = %v", err)
	}
}

func failFetch(t *testing.T) func(string, string) (string, string, error) {
	return func(dir, base string) (string, string, error) {
		t.Errorf("Fetch(%q, %q) was called", dir, base)
		return "", "", errors.New("unexpected fetch")
	}
}

func TestLauncherSkipsFetchForTmux(t *testing.T) {
	old := readyTimeout
	readyTimeout = time.Millisecond
	defer func() { readyTimeout = old }()
	rec := &recorder{reply: paneReply}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: failFetch(t)}
	claude, _ := LookupAgent("claude")
	// The fake tmux never shows the agent, so this stops before the paste; the
	// point is that it went to tmux and never fetched.
	_, err := l.Do(Request{Target: TargetTmux, Agent: claude, Place: PlaceWindow, Dir: "/work", Base: "origin/main", Context: "c"})
	var se *StepError
	if !errors.As(err, &se) || se.Step == StepPaste {
		t.Errorf("err = %v, want a step error before the paste", err)
	}
	if len(rec.calls) == 0 || rec.calls[0].name != "tmux" {
		t.Errorf("calls = %+v, want tmux", rec.calls)
	}
}

func TestLauncherSkipsFetchForHerdr(t *testing.T) {
	rec := &recorder{reply: herdrReply}
	l := Launcher{Run: rec.run, Getenv: herdrEnv, Fetch: failFetch(t)}
	claude, _ := LookupAgent("claude")
	res, err := l.Do(Request{Target: TargetHerdr, Agent: claude, Place: PlaceWindow, Dir: "/work", Base: "origin/main", Context: "c", Issue: 42})
	if err != nil {
		t.Fatal(err)
	}
	if res.Where != "herdr tab" || res.Branch != "" || res.Note != "" {
		t.Errorf("res = %+v", res)
	}
}

func TestLauncherPicksBackendByTarget(t *testing.T) {
	claude, _ := LookupAgent("claude")
	fetch := func(_, b string) (string, string, error) { return b, "", nil }

	t.Run("orca", func(t *testing.T) {
		rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
		_, err := Launcher{Run: rec.run, Getenv: noEnv, Fetch: fetch}.Do(orcaReq())
		if err != nil {
			t.Fatal(err)
		}
		if len(rec.calls) != 1 || rec.calls[0].name != "orca" {
			t.Errorf("calls = %+v, want one orca call", rec.calls)
		}
	})
	t.Run("herdr", func(t *testing.T) {
		rec := &recorder{reply: herdrReply}
		_, err := Launcher{Run: rec.run, Getenv: herdrEnv, Fetch: fetch}.Do(Request{Target: TargetHerdr, Agent: claude, Place: PlacePane, Split: SplitHorizontal, Dir: "/work", Context: "c"})
		if err != nil {
			t.Fatal(err)
		}
		if len(rec.calls) == 0 {
			t.Fatal("no calls")
		}
		for _, c := range rec.calls {
			if c.name != "herdr" {
				t.Errorf("call %s is not herdr", c.line())
			}
		}
	})
	t.Run("tmux", func(t *testing.T) {
		old := readyTimeout
		readyTimeout = time.Millisecond
		defer func() { readyTimeout = old }()
		rec := &recorder{reply: paneReply}
		// The fake tmux never shows the agent, so the launch stops at readiness.
		_, err := Launcher{Run: rec.run, Getenv: noEnv, Fetch: fetch}.Do(Request{Target: TargetTmux, Agent: claude, Place: PlacePane, Split: SplitHorizontal, Dir: "/work", Context: "c"})
		var se *StepError
		if !errors.As(err, &se) || se.Step == StepPaste {
			t.Errorf("err = %v, want a step error before the paste", err)
		}
		if len(rec.calls) == 0 {
			t.Fatal("no calls")
		}
		for _, c := range rec.calls {
			if c.name != "tmux" {
				t.Errorf("call %s is not tmux", c.line())
			}
		}
	})
	t.Run("unknown", func(t *testing.T) {
		rec := &recorder{}
		_, err := Launcher{Run: rec.run, Getenv: noEnv, Fetch: fetch}.Do(Request{Target: "zellij"})
		if err == nil || !strings.Contains(err.Error(), "zellij") || len(rec.calls) != 0 {
			t.Errorf("err = %v, calls = %+v", err, rec.calls)
		}
	})
}

func TestLauncherTmuxReadyTimeoutIs30Seconds(t *testing.T) {
	if readyTimeout != 30*time.Second {
		t.Errorf("readyTimeout = %v, want 30s", readyTimeout)
	}
}

func TestArgvCeiling(t *testing.T) {
	for goos, want := range map[string]int{"linux": 131071, "darwin": 524288, "windows": 30000, "freebsd": 30000, "": 30000} {
		if got := argvCeiling(goos); got != want {
			t.Errorf("argvCeiling(%q) = %d, want %d", goos, got, want)
		}
	}
}

func TestArgvLimitIsThisPlatformsCeiling(t *testing.T) {
	for _, name := range []string{"linux", "darwin", "plan9"} {
		withGOOS(t, name)
		if got := ArgvLimit(); got != argvCeiling(name) {
			t.Errorf("ArgvLimit() on %s = %d, want %d", name, got, argvCeiling(name))
		}
	}
}

func withGOOS(t *testing.T, name string) {
	old := goos
	goos = name
	t.Cleanup(func() { goos = old })
}

func TestLauncherRefusesTextOverTheCeilingBeforeAnyCall(t *testing.T) {
	claude, _ := LookupAgent("claude")
	for _, name := range []string{"linux", "darwin", "plan9"} {
		max := argvCeiling(name)
		for _, target := range []Target{TargetOrca, TargetHerdr} {
			t.Run(name+"/"+string(target), func(t *testing.T) {
				withGOOS(t, name)
				rec := &recorder{reply: func(c call) ([]byte, error) {
					if target == TargetHerdr {
						return herdrReply(c)
					}
					if c.args[0] == "worktree" && strings.Contains(strings.Join(c.args, " "), "--prompt") {
						return []byte(orcaCreateAgentOut), nil
					}
					return fallbackReply(orcaSendOkOut)(c)
				}}
				fetched := 0
				l := Launcher{Run: rec.run, Getenv: herdrEnv, Fetch: func(_, b string) (string, string, error) { fetched++; return b, "", nil }}
				r := orcaReq()
				r.Target = target
				r.Agent = claude
				r.Place = PlaceWindow
				r.Context = strings.Repeat("a", max+1)
				_, err := l.Do(r)
				var se *StepError
				if !errors.As(err, &se) || se.Step != StepCreate {
					t.Fatalf("err = %v, want a create step error", err)
				}
				for _, want := range []string{fmt.Sprint(max + 1), fmt.Sprint(max), string(target), "trim the issue"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("err = %q does not mention %q", err, want)
					}
				}
				if fetched != 0 || len(rec.calls) != 0 {
					t.Errorf("fetched %d times, %d calls: the refusal must come first", fetched, len(rec.calls))
				}

				// Exactly at the ceiling is not refused for the ceiling.
				r.Context = strings.Repeat("a", max)
				if _, err := l.Do(r); err != nil {
					t.Errorf("a context of exactly %d bytes: %v", max, err)
				}
			})
		}
	}
}

func TestLauncherCeilingDoesNotLimitTmux(t *testing.T) {
	withGOOS(t, "plan9")
	old := readyTimeout
	readyTimeout = time.Millisecond
	defer func() { readyTimeout = old }()
	rec := &recorder{reply: paneReply}
	claude, _ := LookupAgent("claude")
	_, err := Launcher{Run: rec.run, Getenv: noEnv, Fetch: failFetch(t)}.Do(Request{Target: TargetTmux, Agent: claude, Place: PlaceWindow, Dir: "/work", Context: strings.Repeat("a", 1000000)})
	var se *StepError
	if errors.As(err, &se) && se.Step == StepCreate {
		t.Errorf("tmux was refused for size: %v", err)
	}
	if len(rec.calls) == 0 {
		t.Error("tmux was never called")
	}
}

func TestLauncherChecksOrcaArgumentsBeforeFetch(t *testing.T) {
	for _, mod := range []func(r *Request){
		func(r *Request) { r.Name = "--evil" },
		func(r *Request) { r.Base = "-x" },
		func(r *Request) { r.Model = "a\nb" },
	} {
		rec := &recorder{}
		fetched := 0
		l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(_, b string) (string, string, error) { fetched++; return b, "", nil }}
		r := orcaReq()
		mod(&r)
		_, err := l.Do(r)
		var se *StepError
		if !errors.As(err, &se) || se.Step != StepCreate {
			t.Errorf("err = %v, want a create step error", err)
		}
		if fetched != 0 || len(rec.calls) != 0 {
			t.Errorf("fetched %d times, %d calls: a bad argument must cost nothing", fetched, len(rec.calls))
		}
	}
}

func TestLauncherPassesTheResolvedBaseToOrca(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	var fetched []string
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(_, b string) (string, string, error) {
		fetched = append(fetched, b)
		return "origin/" + b, "", nil
	}}
	r := orcaReq()
	r.Base = "main"
	if _, err := l.Do(r); err != nil {
		t.Fatal(err)
	}
	if len(fetched) != 1 || fetched[0] != "main" {
		t.Errorf("fetched %v, want the base as typed", fetched)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("calls = %+v", rec.calls)
	}
	args := rec.calls[0].args
	for i, a := range args {
		if a == "--base-branch" {
			if args[i+1] != "origin/main" {
				t.Errorf("orca got --base-branch %q, want origin/main (the stale local main is not the base)", args[i+1])
			}
			return
		}
	}
	t.Errorf("no --base-branch in %q", args)
}

func TestLauncherRetrySkipsFetchAndUsesTheSameBase(t *testing.T) {
	rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: failFetch(t),
		Resolve: func(_, b string) (string, string, error) { return "origin/" + b, "", nil }}
	r := orcaReq()
	r.Base = "main"
	r.RetryID = orcaOp
	if _, err := l.Do(r); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(rec.calls[0].args, " ")
	if !strings.Contains(args, "--base-branch origin/main") || !strings.Contains(args, "--retry-launch "+orcaOp) {
		t.Errorf("retry args = %q", args)
	}
}

func TestLauncherRetryWithoutResolveStillNeverFetches(t *testing.T) {
	rec := &recorder{}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: failFetch(t)}
	r := orcaReq()
	r.RetryID = orcaOp
	_, err := l.Do(r)
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepFetch || len(rec.calls) != 0 {
		t.Errorf("err = %v, calls = %d: with no way to resolve the base the retry must stop before orca", err, len(rec.calls))
	}
}

func TestLauncherLocalRefAndItsNoteSurviveFetchAndRetry(t *testing.T) {
	const note = "origin has no wip; using the local ref wip"
	local := func(_, b string) (string, string, error) { return b, note, nil }
	for _, retry := range []string{"", orcaOp} {
		rec := &recorder{reply: func(c call) ([]byte, error) { return []byte(orcaCreateAgentOut), nil }}
		l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: local, Resolve: local}
		r := orcaReq()
		r.Base = "wip"
		r.RetryID = retry
		res, err := l.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		args := rec.calls[0].args
		got := ""
		for i, a := range args {
			if a == "--base-branch" {
				got = args[i+1]
			}
		}
		if got != "wip" || !strings.Contains(res.Note, note) {
			t.Errorf("retry %q: base %q, note %q; want the local ref and its note", retry, got, res.Note)
		}
	}
	// a base that is not found stops the launch at the fetch step, before orca
	rec := &recorder{}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(_, b string) (string, string, error) { return "", "", errors.New("base not found") }}
	_, err := l.Do(orcaReq())
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepFetch || len(rec.calls) != 0 {
		t.Errorf("err = %v, calls = %d; want a fetch step error and no orca call", err, len(rec.calls))
	}
}

func TestLauncherPasteOnlyReachesEveryTarget(t *testing.T) {
	claude, _ := LookupAgent("claude")
	t.Run("tmux presses no Enter after the paste", func(t *testing.T) {
		captures := 0
		rec := &recorder{reply: func(c call) ([]byte, error) {
			switch c.args[0] {
			case "split-window":
				return []byte("%7\n"), nil
			case "capture-pane":
				captures++
				if captures <= 2 { // start's two settle checks
					return []byte("$ "), nil
				}
				return []byte("$ \nClaude Code v1"), nil
			case "display-message":
				if c.args[len(c.args)-1] == "#{bracket_paste_flag}" {
					return []byte("1\n"), nil
				}
				return []byte("claude\n"), nil
			}
			return nil, nil
		}}
		r := Request{Target: TargetTmux, Agent: claude, Dir: "/work", Context: "text", PasteOnly: true}
		if _, err := (Launcher{Run: rec.run, Getenv: noEnv}).Do(r); err != nil {
			t.Fatal(err)
		}
		last := rec.calls[len(rec.calls)-1]
		if last.args[0] != "paste-buffer" {
			t.Errorf("last call = %q, want paste-buffer", last.line())
		}
	})
	t.Run("herdr keeps the byte ceiling", func(t *testing.T) {
		rec := &recorder{reply: herdrReply}
		r := Request{Target: TargetHerdr, Agent: claude, Dir: "/work", Context: strings.Repeat("a", argvCeiling(goos)+1), PasteOnly: true}
		_, err := (Launcher{Run: rec.run, Getenv: herdrEnv}).Do(r)
		var se *StepError
		if !errors.As(err, &se) || se.Step != StepCreate || len(rec.calls) != 0 {
			t.Errorf("err = %v, calls = %+v", err, rec.calls)
		}
	})
	t.Run("orca keeps the byte ceiling", func(t *testing.T) {
		rec := &recorder{}
		r := orcaReq()
		r.PasteOnly = true
		r.Context = strings.Repeat("a", argvCeiling(goos)+1)
		_, err := (Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(_, b string) (string, string, error) { return b, "", nil }}).Do(r)
		var se *StepError
		if !errors.As(err, &se) || se.Step != StepCreate || len(rec.calls) != 0 {
			t.Errorf("err = %v, calls = %+v", err, rec.calls)
		}
	})
}
