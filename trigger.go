package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type runInfo struct {
	ID           int64
	CreatedAt    time.Time
	Actor, Event string
}

type runLister func(r repoRef, workflow, branch string) ([]runInfo, error)

var errRunNotFound = errors.New("triggered run not found")

// clockSkew lets a run created by GitHub a moment "before" our local clock
// still count as ours.
const clockSkew = 5 * time.Second

// findTriggeredRun polls for the run a workflow_dispatch just created: the
// newest run of this workflow on this branch, by this user, created since
// `since`. Polling stops after timeout; it never returns a run it cannot match.
func findTriggeredRun(list runLister, r repoRef, workflow, branch, user string, since time.Time,
	timeout, interval time.Duration, sleep func(time.Duration)) (int64, error) {
	var lastErr error
	for waited := time.Duration(0); ; waited += interval {
		runs, err := list(r, workflow, branch)
		if err != nil {
			lastErr = err
		}
		var best *runInfo
		for i := range runs {
			x := &runs[i]
			if x.Event != "workflow_dispatch" || x.Actor != user || !x.CreatedAt.After(since.Add(-clockSkew)) {
				continue
			}
			if best == nil || x.CreatedAt.After(best.CreatedAt) {
				best = x
			}
		}
		if best != nil {
			return best.ID, nil
		}
		if waited >= timeout {
			if lastErr != nil {
				return 0, fmt.Errorf("%w: %w", errRunNotFound, lastErr)
			}
			return 0, errRunNotFound
		}
		sleep(interval)
	}
}

// currentUser is the login `gh` uses on host ("" = default host).
func currentUser(host string) (string, error) {
	args := []string{"api", "user", "-q", ".login"}
	if host != "" {
		args = append(args, "--hostname", host)
	}
	out, err := gh(args...)
	return strings.TrimSpace(string(out)), err
}

// ghRunLister lists recent workflow_dispatch runs started by user.
func ghRunLister(user string) runLister {
	return func(r repoRef, workflow, branch string) ([]runInfo, error) {
		var raw []struct {
			ID        int64     `json:"databaseId"`
			CreatedAt time.Time `json:"createdAt"`
			Event     string    `json:"event"`
		}
		args := scoped(r, "run", "list", "-w", workflow, "-b", branch, "-e", "workflow_dispatch",
			"-u", user, "-L", "10", "--json", "databaseId,createdAt,event")
		if err := ghJSON(&raw, args...); err != nil {
			return nil, err
		}
		out := make([]runInfo, len(raw))
		for i, x := range raw {
			out[i] = runInfo{ID: x.ID, CreatedAt: x.CreatedAt, Actor: user, Event: x.Event}
		}
		return out, nil
	}
}

// triggerDeps are the side effects of "trigger then watch"; tests replace them.
var triggerDeps = struct {
	dispatch          func(args []string) error
	user              func(host string) (string, error)
	lister            func(user string) runLister
	sleep             func(time.Duration)
	now               func() time.Time
	timeout, interval time.Duration
}{
	dispatch: func(a []string) error { _, err := gh(a...); return err },
	user:     currentUser,
	lister:   ghRunLister,
	sleep:    time.Sleep,
	now:      time.Now,
	timeout:  30 * time.Second,
	interval: 2 * time.Second,
}

// triggeredMsg reports the outcome of a dispatch and the search for its run.
type triggeredMsg struct {
	workflow   string
	target     repoRef
	runID      string
	dispatched bool
	err        error
}

// triggerAndFind dispatches the form's workflow, then looks for the run it created.
// The dispatch always happens; failing to identify the run is reported, not fatal.
func triggerAndFind(f triggerForm) func() triggeredMsg {
	return func() triggeredMsg {
		d := triggerDeps
		msg := triggeredMsg{workflow: f.workflow, target: f.target}
		since := d.now()
		user, userErr := d.user(f.target.Host)
		if err := d.dispatch(f.args()); err != nil {
			msg.err = err
			return msg
		}
		msg.dispatched = true
		if userErr != nil {
			msg.err = fmt.Errorf("cannot look up your login: %w", userErr)
			return msg
		}
		id, err := findTriggeredRun(d.lister(user), f.target, f.workflow, f.ref, user, since, d.timeout, d.interval, d.sleep)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.runID = fmt.Sprint(id)
		return msg
	}
}
