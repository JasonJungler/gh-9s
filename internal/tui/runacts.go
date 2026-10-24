package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

// doRunAction does an action on a run; tests replace it.
var doRunAction = ghcli.DoRunAction

// runAct is a run action being confirmed: what, where, and which run, fixed when the key was pressed.
type runAct struct {
	action ghcli.RunAction
	target ghcli.RepoRef
	id     string
	view   viewKind // the screen the question was asked on
	label  string   // "acme/api", shown in the confirmation
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

// runActionOf is the action a key stands for on the run screens.
func runActionOf(key string) ghcli.RunAction {
	switch key {
	case "r":
		return ghcli.RunRerun
	case "F":
		return ghcli.RunRerunFailed
	case "c":
		return ghcli.RunCancel
	}
	return ghcli.RunDelete
}

// runActDoneMsg reports how a run action ended.
type runActDoneMsg struct {
	act runAct
	err error
}

func init() {
	dialogHandlers["run.confirm"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pendingRun
		m.pendingRun = nil
		if !r.ok || p == nil {
			return m, nil
		}
		if m.view != p.view || m.curTarget() != p.target {
			return m, nil // the screen moved on while the question was open
		}
		return m, func() tea.Msg {
			return runActDoneMsg{act: *p, err: doRunAction(p.target, p.id, p.action)}
		}
	}
}

// startRunAct asks before doing an action on a run, or says in a line why it does not apply.
// Run, repo and screen are fixed now, not when the answer comes.
func (m model) startRunAct(a ghcli.RunAction, id, status, conclusion string) (model, tea.Cmd) {
	if ok, why := runRule(a, status, conclusion); !ok {
		m.status = styDim.Render(why)
		return m, nil
	}
	t := m.curTarget()
	p := &runAct{action: a, target: t, id: id, view: m.view, label: displayRef(t)}
	m.pendingRun = p
	var title, detail string
	switch a {
	case ghcli.RunRerun:
		title = fmt.Sprintf("Re-run run %s in %s?", id, p.label)
	case ghcli.RunRerunFailed:
		title = fmt.Sprintf("Re-run the failed jobs of run %s in %s?", id, p.label)
	case ghcli.RunCancel:
		title = fmt.Sprintf("Cancel run %s in %s?", id, p.label)
	default:
		title, detail = fmt.Sprintf("Delete run %s in %s?", id, p.label), "This can't be undone."
	}
	return m.withDialog(newConfirmDialog("run.confirm", title, detail)), nil
}

// runActDone shows the outcome. A re-run that GitHub accepted goes to that run's graph and watches
// it again; cancel and delete reload what is on screen.
func (m model) runActDone(msg runActDoneMsg) (model, tea.Cmd) {
	if msg.err != nil {
		m.status = styErr.Render("✗ " + msg.err.Error())
		return m, nil
	}
	switch msg.act.action {
	case ghcli.RunRerun, ghcli.RunRerunFailed:
		m.status = styOK.Render("✓ re-running run " + msg.act.id)
		m.runID, m.target = msg.act.id, msg.act.target
		m.graph, m.gsel, m.top, m.expanded, m.runStatus, m.runMeta = graph.Graph{}, 0, 0, nil, "", ghcli.RunMeta{}
		return m, m.push(viewGraph)
	case ghcli.RunCancel:
		m.status = styOK.Render("✓ cancelled run " + msg.act.id)
	default:
		m.status = styOK.Render("✓ deleted run " + msg.act.id)
	}
	m.loading = true
	return m, m.fetch()
}
