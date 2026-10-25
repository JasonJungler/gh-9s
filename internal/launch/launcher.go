package launch

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

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
	// made from it. It returns a note to show the user, or an error.
	Fetch func(dir, base string) (string, error)
}

// Do launches r. For Orca it fetches the base first and creates nothing if
// that fails. A failure is a *StepError naming the stage; whatever was already
// created is left in place and nothing is retried.
func (l Launcher) Do(r Request) (Result, error) {
	switch r.Target {
	case TargetOrca:
		if l.Fetch == nil {
			return Result{}, &StepError{StepFetch, errors.New("no way to fetch the base was configured")}
		}
		note, err := l.Fetch(r.Dir, r.Base)
		if err != nil {
			return Result{}, &StepError{StepFetch, err}
		}
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
