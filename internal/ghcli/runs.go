package ghcli

import (
	"errors"
	"regexp"
)

var errBadRunAction = errors.New("not a valid run action")

// RunAction is something done to a workflow run.
type RunAction string

const (
	RunRerun       RunAction = "rerun"
	RunRerunFailed RunAction = "rerun-failed"
	RunCancel      RunAction = "cancel"
	RunDelete      RunAction = "delete"
)

var runID = regexp.MustCompile(`^[0-9]{1,20}$`)

// RunActionArgs is the gh command for an action on a run, scoped to the repo. It returns nil for an
// unknown action or a run id that is not plain digits (which could be read as a flag).
func RunActionArgs(r RepoRef, id string, a RunAction) []string {
	if !runID.MatchString(id) {
		return nil
	}
	switch a {
	case RunRerun:
		return Scoped(r, "run", "rerun", id)
	case RunRerunFailed:
		return Scoped(r, "run", "rerun", id, "--failed")
	case RunCancel:
		return Scoped(r, "run", "cancel", id)
	case RunDelete:
		return Scoped(r, "run", "delete", id)
	}
	return nil
}

// DoRunAction runs an action on a run; gh's own message is the error.
func DoRunAction(r RepoRef, id string, a RunAction) error {
	args := RunActionArgs(r, id, a)
	if args == nil {
		return errBadRunAction
	}
	_, err := Run(args...)
	return err
}
