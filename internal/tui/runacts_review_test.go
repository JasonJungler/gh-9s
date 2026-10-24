package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

func TestOpeningAnotherRunForgetsTheLastRunsState(t *testing.T) {
	m := runsModel(runRow("222", "in_progress", ""))
	m.runMeta = ghcli.RunMeta{Status: "completed", Conclusion: "failure"} // left over from run 111
	m = keys(m, "enter")
	if m.view != viewGraph || m.runMeta != (ghcli.RunMeta{}) {
		t.Errorf("view=%v runMeta=%+v; the new run's state is unknown until it loads", m.view, m.runMeta)
	}
	f := installRunActs(t)
	m = keyAll(m, "d")
	if m.dlg != nil || len(f.calls) != 0 || !strings.Contains(m.status, "loading") {
		t.Errorf("dialog=%v status=%q; a delete must wait for the run's real state", m.dlg != nil, m.status)
	}
}

func TestADataMessageOfAnotherRunIsIgnored(t *testing.T) {
	m := failedGraphModel()
	m.runID = "222"
	m.runMeta = ghcli.RunMeta{}
	other := graph.Build([]graph.Job{ok("elsewhere")}, nil)
	next, _ := m.Update(dataMsg{view: viewGraph, runID: "111", graph: &other, run: ghcli.RunMeta{Status: "completed", Conclusion: "success"}})
	got := next.(model)
	if got.runMeta != (ghcli.RunMeta{}) || len(got.graph.Nodes) != len(m.graph.Nodes) {
		t.Errorf("a late poll of run 111 changed run 222's screen: meta=%+v nodes=%d", got.runMeta, len(got.graph.Nodes))
	}
}

func TestDeletingLeavesEvenWithAFilterOrAnOpenCommandLine(t *testing.T) {
	installRunActs(t)
	m := failedGraphModel()
	m.filter = "x"
	m = keyAll(m, "d", "y")
	if m.view != viewRuns {
		t.Errorf("view = %v; a filter must not keep the deleted run's graph on screen", m.view)
	}
	m = failedGraphModel()
	m = keyAll(m, "d")
	m = keyAll(m, "y")
	m.mode, m.input = modeCommand, "x" // the user typed ':' while gh was working
	next, _ := m.Update(runActDoneMsg{act: runAct{action: ghcli.RunDelete, target: acme, id: "777", view: viewGraph}})
	if nm := next.(model); nm.view != viewRuns {
		t.Errorf("view = %v; leaving must not depend on a key press reaching the right handler", nm.view)
	}
}

func TestAnOutcomeActsOnlyWhereTheRunStillIs(t *testing.T) {
	// delete finished while the user is looking at another run
	m := failedGraphModel()
	m.runID = "999"
	next, _ := m.Update(runActDoneMsg{act: runAct{action: ghcli.RunDelete, target: acme, id: "123", view: viewGraph}})
	if nm := next.(model); nm.view != viewGraph || nm.runID != "999" || !strings.Contains(nm.status, "✓") {
		t.Errorf("view=%v runID=%q status=%q; run 999's graph must stay", nm.view, nm.runID, nm.status)
	}
	// re-run accepted after the user switched repo or screen
	m = failedGraphModel()
	m.view = viewWorkflows
	next, _ = m.Update(runActDoneMsg{act: runAct{action: ghcli.RunRerun, target: acme, id: "777", view: viewGraph}})
	if nm := next.(model); nm.view != viewWorkflows || !strings.Contains(nm.status, "re-running") {
		t.Errorf("view=%v status=%q; the user is not dragged to the old run", nm.view, nm.status)
	}
}

func TestAReRunFromTheGraphDoesNotStackAnotherGraph(t *testing.T) {
	installRunActs(t)
	m := failedGraphModel()
	before := append([]viewKind(nil), m.back...)
	m = keyAll(m, "r", "y")
	if m.view != viewGraph || len(m.back) != len(before) {
		t.Errorf("view=%v back=%v; want the same stack %v", m.view, m.back, before)
	}
	jobs := model{view: viewJobs, runID: "888", target: acme, width: 120, height: 30, runMeta: ghcli.RunMeta{Status: "completed", Conclusion: "failure"}, back: []viewKind{viewRuns}}
	jobs = keyAll(jobs, "r", "y")
	if jobs.view == viewJobs && len(jobs.back) > 1 || len(jobs.back) != 1 {
		t.Errorf("view=%v back=%v; the job list must not be pushed twice", jobs.view, jobs.back)
	}
}

func TestPollingKeepsGoingUntilTheNewAttemptShowsUp(t *testing.T) {
	m := failedGraphModel()
	m.runStatus, m.rerunWait = "", rerunPolls
	next, _ := m.observeRun(ghcli.RunMeta{Status: "completed", Conclusion: "failure"}) // GitHub has not queued the new attempt yet
	m = next
	if m.runStatus == "completed" || !m.shouldPoll() {
		t.Fatalf("runStatus=%q; the stale 'completed' must not stop the polling", m.runStatus)
	}
	m, _ = m.observeRun(ghcli.RunMeta{Status: "in_progress"})
	if m.runStatus != "in_progress" || m.rerunWait != 0 {
		t.Errorf("runStatus=%q rerunWait=%d; the new attempt is seen", m.runStatus, m.rerunWait)
	}
	var cmdRan bool
	old := notifier
	notifier = func(string, string) { cmdRan = true }
	t.Cleanup(func() { notifier = old })
	var cmd tea.Cmd
	m, cmd = m.observeRun(ghcli.RunMeta{Status: "completed", Conclusion: "success"})
	if cmd != nil {
		cmd()
	}
	if !cmdRan {
		t.Error("the finished notification fires for the new attempt")
	}
	// but it gives up if the new attempt never appears
	g := failedGraphModel()
	g.runStatus, g.rerunWait = "", 2
	g, _ = g.observeRun(ghcli.RunMeta{Status: "completed"})
	g, _ = g.observeRun(ghcli.RunMeta{Status: "completed"})
	if g.runStatus != "completed" {
		t.Errorf("runStatus=%q; after a while 'completed' is believed", g.runStatus)
	}
}

func TestWithoutAKnownRepoNothingIsAsked(t *testing.T) {
	f := installRunActs(t)
	m := runsModel(runRow("123", "completed", "failure"))
	m.target = ghcli.RepoRef{}
	m = keyAll(m, "r")
	if m.dlg != nil || len(f.calls) != 0 || !strings.Contains(m.status, "repo") {
		t.Errorf("dialog=%v status=%q; a question that reads 'in /?' is no question", m.dlg != nil, m.status)
	}
}

func TestADroppedAnswerSaysSo(t *testing.T) {
	installRunActs(t)
	m := keyAll(runsModel(runRow("123", "completed", "failure")), "r")
	m.view = viewWorkflows
	m = keyAll(m, "y")
	if !strings.Contains(m.status, "dropped") && !strings.Contains(m.status, "changed") {
		t.Errorf("status = %q; a 'yes' that did nothing must say why", m.status)
	}
}

func TestRunRuleKnowsGitHubsOtherConclusions(t *testing.T) {
	for _, concl := range []string{"skipped", "neutral", "stale", "startup_failure", "action_required"} {
		if ok, why := runRule(ghcli.RunRerunFailed, "completed", concl); ok || why == "" {
			t.Errorf("F on %s: ok=%v why=%q; nothing there can be re-run as failed", concl, ok, why)
		}
	}
	if ok, _ := runRule(ghcli.RunRerun, "completed", "action_required"); ok {
		t.Error("r on a run awaiting approval is refused")
	}
	for _, concl := range []string{"failure", "timed_out", "cancelled"} {
		if ok, _ := runRule(ghcli.RunRerunFailed, "completed", concl); !ok {
			t.Errorf("F on %s should be allowed", concl)
		}
	}
}
