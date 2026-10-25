package launch

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// goos is the platform the ceiling is taken from. A variable so tests can try
// every platform.
var goos = runtime.GOOS

// argvCeiling is the most bytes of issue text one program argument can carry
// on goos: Linux allows 131071 bytes in a single argument, macOS allows 524288
// bytes for all arguments together, and any other platform gets a cautious
// 30000. orca and herdr take the text as an argument; tmux gets it on stdin.
func argvCeiling(goos string) int {
	switch goos {
	case "linux":
		return 131071
	case "darwin":
		return 524288
	}
	return 30000
}

// readyTimeout is how long a tmux agent gets to become ready. A variable so
// tests can shorten it.
var readyTimeout = 30 * time.Second

// Result says where a launch went.
type Result struct {
	Where  string // the workspace, pane or window the agent runs in
	Branch string // the final branch name (Orca only)
	Note   string // something the user should know, or ""
}

// Launcher sends a Request to the environment named by Request.Target.
type Launcher struct {
	Run    RunFunc
	Getenv func(string) string
	// Fetch brings the remote of base up to date before an Orca workspace is
	// made from it. It returns the ref the workspace must branch from (a bare
	// branch becomes <remote>/<branch>, so Orca does not use a stale local
	// branch), a note to show the user, or an error.
	Fetch func(dir, base string) (ref, note string, err error)
	// Resolve is Fetch without the fetching: it only says which ref base
	// means. A retry uses it, since the base was fetched for the original
	// request.
	Resolve func(dir, base string) (ref, note string, err error)
}

// Do launches r. The text size and (for Orca) the arguments are checked before
// anything runs. For Orca it then fetches the base and creates nothing if that
// fails; Orca is given the ref Fetch resolved, never the base as typed. A retry
// (Request.RetryID) resolves the base the same way but does not fetch again. A failure is a *StepError naming the stage; whatever was already
// created is left in place and nothing is retried.
func (l Launcher) Do(r Request) (Result, error) {
	if r.Target == TargetOrca || r.Target == TargetHerdr {
		if max := argvCeiling(goos); len(r.Context) > max {
			return Result{}, &StepError{StepCreate, fmt.Errorf("issue text is %d bytes; this platform can pass at most %d bytes to %s; trim the issue and try again", len(r.Context), max, r.Target)}
		}
	}
	switch r.Target {
	case TargetOrca:
		if err := (orcaBackend{}).check(r); err != nil {
			return Result{}, &StepError{StepCreate, err}
		}
		prepare, what := l.Fetch, "fetch"
		if r.RetryID != "" {
			prepare, what = l.Resolve, "resolve"
		}
		if prepare == nil {
			return Result{}, &StepError{StepFetch, errors.New("no way to " + what + " the base was configured")}
		}
		ref, note, err := prepare(r.Dir, r.Base)
		if err != nil {
			return Result{}, &StepError{StepFetch, err}
		}
		r.Base = ref
		res, err := orcaBackend{run: l.Run}.launch(r)
		if err != nil {
			return Result{}, err
		}
		res.Note = joinNotes(note, res.Note)
		return res, nil
	case TargetHerdr:
		if err := runSequence(newHerdr(l.Run, l.Getenv), r, readyTimeout); err != nil {
			return Result{}, err
		}
		return Result{Where: where(r)}, nil
	case TargetTmux:
		if err := runSequence(newTmux(l.Run, l.Getenv), r, readyTimeout); err != nil {
			return Result{}, err
		}
		return Result{Where: where(r)}, nil
	}
	return Result{}, fmt.Errorf("unknown launch target %q", r.Target)
}

// where names the kind of place a herdr or tmux agent was opened in.
func where(r Request) string {
	place := "pane"
	if r.Place == PlaceWindow {
		place = "window"
		if r.Target == TargetHerdr {
			place = "tab"
		}
	}
	return fmt.Sprintf("%s %s", r.Target, place)
}

func joinNotes(notes ...string) string {
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}
