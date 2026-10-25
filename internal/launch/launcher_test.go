package launch

import (
	"errors"
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
		Fetch: func(dir, base string) (string, error) {
			order = append(order, "fetch")
			fetches = append(fetches, fetchCall{dir, base})
			return "no remote, using the local branch", nil
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
	rec := &recorder{reply: func(c call) ([]byte, error) {
		switch c.args[0] {
		case "worktree":
			return []byte(orcaCreateBareOut), nil
		case "agent":
			return []byte(orcaAgentLaunchOut), nil
		}
		return []byte(orcaSendWarnOut), nil
	}}
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(string, string) (string, error) { return "fetched", nil }}
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
		Fetch: func(dir, base string) (string, error) { return "", errors.New("could not resolve host") },
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
	l := Launcher{Run: rec.run, Getenv: noEnv, Fetch: func(string, string) (string, error) { return "", nil }}
	_, err := l.Do(orcaReq())
	var lost *LostReplyError
	var se *StepError
	if !errors.As(err, &se) || se.Step != StepCreate || !errors.As(err, &lost) || lost.OperationID != orcaOp {
		t.Errorf("err = %v", err)
	}
}

func failFetch(t *testing.T) func(string, string) (string, error) {
	return func(dir, base string) (string, error) {
		t.Errorf("Fetch(%q, %q) was called", dir, base)
		return "", errors.New("unexpected fetch")
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
	fetch := func(string, string) (string, error) { return "", nil }

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
		Launcher{Run: rec.run, Getenv: noEnv, Fetch: fetch}.Do(Request{Target: TargetTmux, Agent: claude, Place: PlacePane, Split: SplitHorizontal, Dir: "/work", Context: "c"})
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
