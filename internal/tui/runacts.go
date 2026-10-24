package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

// rerunPolls is how many polls (5 s each) to wait for a re-run's new attempt to show up before
// believing a "completed" status again.
const rerunPolls = 12

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
		switch {
		case !finished:
			return false, "this run is still in progress"
		case conclusion == "action_required":
			return false, "this run is waiting for approval on GitHub"
		}
		return true, ""
	case ghcli.RunRerunFailed:
		switch {
		case !finished:
			return false, "this run is still in progress"
		case conclusion == "failure" || conclusion == "timed_out" || conclusion == "cancelled":
			return true, ""
		}
		return false, "nothing failed to re-run"
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
			m.status = styDim.Render("the screen changed while you were answering: nothing was done")
			return m, nil
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
	if t.Owner == "" || t.Name == "" {
		m.status = styErr.Render("don't know which repo this run is in yet: try again in a moment")
		return m, nil
	}
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
	act := msg.act
	if msg.err != nil {
		m.status = styErr.Render("✗ " + msg.err.Error())
		return m, nil
	}
	// the user may have moved on while gh was working: only act where the run still is
	here := m.curTarget() == act.target && (m.view == act.view || (act.view == viewGraph && m.view == viewJobs) || (act.view == viewJobs && m.view == viewGraph))
	onRunScreen := (act.view == viewGraph || act.view == viewJobs) && here && m.runID == act.id
	switch act.action {
	case ghcli.RunRerun, ghcli.RunRerunFailed:
		m.status = styOK.Render("✓ re-running run " + act.id)
		if !here || (act.view != viewRuns && !onRunScreen) {
			return m, nil
		}
		m.runID, m.target = act.id, act.target
		m.graph, m.gsel, m.top, m.expanded, m.runStatus, m.runMeta = graph.Graph{}, 0, 0, nil, "", ghcli.RunMeta{}
		m.rerunWait = rerunPolls
		if act.view == viewRuns {
			return m, m.push(viewGraph)
		}
		return m, m.switchTo(viewGraph) // already on this run's screen: refresh it in place, no second copy on the back stack
	case ghcli.RunCancel:
		m.status = styOK.Render("✓ cancelled run " + act.id)
	default:
		m.status = styOK.Render("✓ deleted run " + act.id)
		if onRunScreen { // the run being looked at is gone: back to where we came from
			status := m.status
			if next, cmd, ok := m.popBack(); ok {
				next.status = status
				return next, cmd
			}
		}
		if act.view != viewRuns || !here {
			return m, nil
		}
	}
	if !here {
		return m, nil
	}
	m.loading = true
	return m, m.fetch()
}

// runActOnShown is a run action on the run the graph or job list is showing.
func (m model) runActOnShown(a ghcli.RunAction) (model, tea.Cmd) {
	if m.runMeta.Status == "" {
		m.status = styDim.Render("still loading this run…")
		return m, nil
	}
	return m.startRunAct(a, m.runID, m.runMeta.Status, m.runMeta.Conclusion)
}

var errorWord = regexp.MustCompile(`(?i)\b(error|failed|failure)\b`)

// firstFailureLine is where a failed job's log should open: GitHub's own ##[error] annotation, else
// the first line with an error word, else the top.
func firstFailureLine(lines []string) int {
	for i, l := range lines {
		if strings.Contains(l, "##[error]") {
			return i
		}
	}
	for i, l := range lines {
		if errorWord.MatchString(l) {
			return i
		}
	}
	return 0
}

// jumpToFailure opens the log of the first failed job of the graph, at its first failure.
func (m model) jumpToFailure() (model, tea.Cmd) {
	for _, n := range m.graph.Nodes {
		jobs := n.Members
		if len(jobs) == 0 {
			jobs = []graph.Job{n.Job}
		}
		for _, j := range jobs {
			if _, st := runState(j.Status, j.Conclusion); st == stFail && j.ID != "" {
				m.jobID, m.jobName, m.jobURL = j.ID, j.Name, j.URL
				m.logJump = true
				cmd := m.push(viewLogs)
				return m, cmd
			}
		}
	}
	m.status = styDim.Render("no failed job in this run")
	return m, nil
}
