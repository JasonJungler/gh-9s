package tui

import (
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// doRunAction does an action on a run; tests replace it.
var doRunAction = ghcli.DoRunAction

// runAct is a run action being confirmed: what, where, and which run, fixed when the key was pressed.
type runAct struct {
	action ghcli.RunAction
	target ghcli.RepoRef
	id     string
	label  string // "acme/api run 123", shown in the confirmation
}

// runRule says whether an action applies to a run in this state, and if not, why in a few words.
func runRule(a ghcli.RunAction, status, conclusion string) (ok bool, why string) {
	status, conclusion = strings.ToLower(status), strings.ToLower(conclusion)
	finished := status == "completed"
	switch a {
	case ghcli.RunRerun:
		if !finished {
			return false, "this run is still in progress"
		}
		return true, ""
	case ghcli.RunRerunFailed:
		switch {
		case !finished:
			return false, "this run is still in progress"
		case conclusion == "success":
			return false, "nothing failed to re-run"
		}
		return true, ""
	case ghcli.RunCancel:
		if finished {
			return false, "this run already finished"
		}
		return true, ""
	case ghcli.RunDelete:
		if !finished {
			return false, "this run is in progress: cancel it first"
		}
		return true, ""
	}
	return false, "unknown action"
}
