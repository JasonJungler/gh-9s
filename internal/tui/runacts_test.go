package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

func TestRunRule(t *testing.T) {
	type c struct {
		action             ghcli.RunAction
		status, conclusion string
		ok                 bool
		why                string
	}
	cases := []c{
		{ghcli.RunRerun, "completed", "success", true, ""},
		{ghcli.RunRerun, "completed", "failure", true, ""},
		{ghcli.RunRerun, "in_progress", "", false, "in progress"},
		{ghcli.RunRerun, "queued", "", false, "in progress"},
		{ghcli.RunRerunFailed, "completed", "failure", true, ""},
		{ghcli.RunRerunFailed, "completed", "timed_out", true, ""},
		{ghcli.RunRerunFailed, "completed", "cancelled", true, ""},
		{ghcli.RunRerunFailed, "completed", "success", false, "nothing failed"},
		{ghcli.RunRerunFailed, "IN_PROGRESS", "", false, "in progress"},
		{ghcli.RunCancel, "in_progress", "", true, ""},
		{ghcli.RunCancel, "queued", "", true, ""},
		{ghcli.RunCancel, "waiting", "", true, ""},
		{ghcli.RunCancel, "completed", "failure", false, "already finished"},
		{ghcli.RunDelete, "completed", "failure", true, ""},
		{ghcli.RunDelete, "completed", "success", true, ""},
		{ghcli.RunDelete, "in_progress", "", false, "cancel it first"},
	}
	for _, x := range cases {
		ok, why := runRule(x.action, x.status, x.conclusion)
		if ok != x.ok || (x.why == "") != (why == "") || !strings.Contains(why, x.why) {
			t.Errorf("%s %s/%s: ok=%v why=%q; want ok=%v why~%q", x.action, x.status, x.conclusion, ok, why, x.ok, x.why)
		}
	}
	if ok, _ := runRule(ghcli.RunAction("x"), "completed", "failure"); ok {
		t.Error("an unknown action never applies")
	}
}

type runCall struct {
	target ghcli.RepoRef
	id     string
	action ghcli.RunAction
}

type runFakes struct {
	calls []runCall
	err   error
}

func installRunActs(t *testing.T) *runFakes {
	t.Helper()
	f := &runFakes{}
	old := doRunAction
	doRunAction = func(r ghcli.RepoRef, id string, a ghcli.RunAction) error {
		f.calls = append(f.calls, runCall{r, id, a})
		return f.err
	}
	t.Cleanup(func() { doRunAction = old })
	return f
}

func runRow(id, status, conclusion string) row {
	return row{id: id, status: status, conclusion: conclusion, cells: []string{id, status + "/" + conclusion, "CI"}}
}

func runsModel(rows ...row) model {
	return model{view: viewRuns, width: 120, height: 30, target: acme,
		tbl: table{cols: []string{"ID", "STATUS", "WORKFLOW"}, rows: rows}}
}

func TestRunKeysAskFirstNamingTheRepoAndTheRun(t *testing.T) {
	for key, want := range map[string]string{
		"r": "Re-run run 123", "F": "failed jobs of run 123", "c": "Cancel run 123", "d": "Delete run 123",
	} {
		f := installRunActs(t)
		status, conclusion := "completed", "failure"
		if key == "c" {
			status, conclusion = "in_progress", ""
		}
		m := keyAll(runsModel(runRow("123", status, conclusion)), key)
		if m.dlg == nil || m.dlg.kind != dialogConfirm || !strings.Contains(m.dlg.title, want) || !strings.Contains(m.dlg.title, "acme/api") {
			t.Errorf("%s: dialog = %+v; want a confirmation containing %q and acme/api", key, m.dlg, want)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: nothing may run before the answer", key)
		}
		m = keyAll(m, "n")
		if len(f.calls) != 0 || m.dlg != nil {
			t.Errorf("%s: declining must run nothing", key)
		}
	}
}

func TestAConfirmedRunActionTouchesExactlyTheRunItNamed(t *testing.T) {
	f := installRunActs(t)
	m := runsModel(runRow("123", "completed", "failure"), runRow("456", "completed", "failure"))
	m = keyAll(m, "F")
	m.cursor = 1                                                                        // the cursor moved
	m.tbl = table{cols: m.tbl.cols, rows: []row{runRow("789", "completed", "failure")}} // the list refreshed
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"}
	m = keyAll(m, "y")
	if len(f.calls) != 1 || f.calls[0] != (runCall{acme, "123", ghcli.RunRerunFailed}) {
		t.Errorf("calls = %+v; want run 123 of acme/api only", f.calls)
	}
}

func TestRunActionsGiveWayWhenTheyDoNotApply(t *testing.T) {
	cases := []struct{ key, status, conclusion, why string }{
		{"F", "completed", "success", "nothing failed"},
		{"r", "in_progress", "", "in progress"},
		{"F", "queued", "", "in progress"},
		{"c", "completed", "failure", "already finished"},
		{"d", "in_progress", "", "cancel it first"},
	}
	for _, c := range cases {
		f := installRunActs(t)
		m := keyAll(runsModel(runRow("123", c.status, c.conclusion)), c.key)
		if m.dlg != nil || len(f.calls) != 0 || !strings.Contains(m.status, c.why) {
			t.Errorf("%s on %s/%s: dialog=%v calls=%d status=%q; want only %q", c.key, c.status, c.conclusion, m.dlg != nil, len(f.calls), m.status, c.why)
		}
	}
}

func TestAnAnswerAfterLeavingTheScreenDoesNothing(t *testing.T) {
	f := installRunActs(t)
	m := keyAll(runsModel(runRow("123", "completed", "failure")), "r")
	m.view = viewWorkflows
	m = keyAll(m, "y")
	if len(f.calls) != 0 {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestAcceptedReRunLandsOnTheRunsGraphAndWatchesAgain(t *testing.T) {
	f := installRunActs(t)
	m := runsModel(runRow("123", "completed", "failure"))
	m.runStatus = "completed"
	m.gsel = 3
	m = keyAll(m, "r", "y")
	if len(f.calls) != 1 || f.calls[0].action != ghcli.RunRerun {
		t.Fatalf("calls = %+v", f.calls)
	}
	if m.view != viewGraph || m.runID != "123" || m.target != acme || m.runStatus != "" || m.gsel != 0 {
		t.Errorf("view=%v runID=%q target=%+v runStatus=%q gsel=%d; want a fresh graph of run 123", m.view, m.runID, m.target, m.runStatus, m.gsel)
	}
	if !strings.Contains(m.status, "✓") {
		t.Errorf("status = %q", m.status)
	}
}

func TestCancelAndDeleteStayOnTheListAndRefresh(t *testing.T) {
	f := installRunActs(t)
	m := keyAll(runsModel(runRow("123", "in_progress", "")), "c", "y")
	if m.view != viewRuns || len(f.calls) != 1 || f.calls[0].action != ghcli.RunCancel || !strings.Contains(m.status, "✓") {
		t.Errorf("view=%v calls=%+v status=%q", m.view, f.calls, m.status)
	}
}

func TestAGitHubErrorIsShownAsItIsAndDoesNotLeave(t *testing.T) {
	f := installRunActs(t)
	f.err = errors.New("run 123 cannot be rerun; its workflow file may be broken")
	m := keyAll(runsModel(runRow("123", "completed", "failure")), "r", "y")
	if m.view != viewRuns || !strings.Contains(m.status, "cannot be rerun") || strings.Contains(m.status, "✓") {
		t.Errorf("view=%v status=%q", m.view, m.status)
	}
}

func TestPressingYTwiceRunsOnce(t *testing.T) {
	f := installRunActs(t)
	keyAll(runsModel(runRow("123", "completed", "failure")), "r", "y", "y")
	if len(f.calls) != 1 {
		t.Errorf("%d calls", len(f.calls))
	}
}

func failedGraphModel() model {
	jobs := []graph.Job{ok("lint"), job("test", "completed", "failure"), job("e2e", "completed", "failure"), ok("docs")}
	specs := []workflow.JobSpec{{ID: "lint", Name: "lint"}, {ID: "test", Name: "test", Needs: []string{"lint"}},
		{ID: "e2e", Name: "e2e", Needs: []string{"lint"}}, {ID: "docs", Name: "docs"}}
	return model{view: viewGraph, runID: "777", target: acme, width: 120, height: 30,
		graph: graph.Build(jobs, specs), runStatus: "completed", runMeta: ghcli.RunMeta{Status: "completed", Conclusion: "failure"},
		back: []viewKind{viewRuns}}
}

func TestTheGraphHasTheSameRunKeys(t *testing.T) {
	f := installRunActs(t)
	m := keyAll(failedGraphModel(), "F")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "run 777") || !strings.Contains(m.dlg.title, "acme/api") {
		t.Fatalf("dialog = %+v", m.dlg)
	}
	m = keyAll(m, "y")
	if len(f.calls) != 1 || f.calls[0] != (runCall{acme, "777", ghcli.RunRerunFailed}) {
		t.Errorf("calls = %+v", f.calls)
	}
	if m.view != viewGraph || m.runID != "777" || m.runStatus != "" {
		t.Errorf("a re-run restarts the watch of the same run: view=%v runStatus=%q", m.view, m.runStatus)
	}
	collapse := failedGraphModel()
	collapse = keyAll(collapse, "x") // x is still the graph's own key
	if collapse.dlg != nil {
		t.Error("x must not open a run action dialog")
	}
}

func TestTheJobListHasTheSameRunKeys(t *testing.T) {
	f := installRunActs(t)
	m := model{view: viewJobs, runID: "888", target: acme, width: 120, height: 30,
		runMeta: ghcli.RunMeta{Status: "in_progress"},
		tbl:     table{cols: []string{"JOB"}, rows: []row{{id: "1", cells: []string{"build"}}}}}
	m = keyAll(m, "c")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Cancel run 888") {
		t.Fatalf("dialog = %+v", m.dlg)
	}
	keyAll(m, "y")
	if len(f.calls) != 1 || f.calls[0] != (runCall{acme, "888", ghcli.RunCancel}) {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestRunKeysWaitForTheRunToLoad(t *testing.T) {
	f := installRunActs(t)
	m := failedGraphModel()
	m.runMeta = ghcli.RunMeta{}
	m = keyAll(m, "r")
	if m.dlg != nil || len(f.calls) != 0 || !strings.Contains(m.status, "loading") {
		t.Errorf("dialog=%v status=%q; the rules need the run's state", m.dlg != nil, m.status)
	}
}

func TestAReRunUsesTheRunsOwnRepoEvenWhenThereIsNoTarget(t *testing.T) {
	f := installRunActs(t)
	ghe := ghcli.RepoRef{Host: "ghe.corp", Owner: "acme", Name: "api"}
	m := failedGraphModel()
	m.target, m.curRepo = ghcli.RepoRef{}, ghe
	m = keyAll(m, "r", "y")
	if len(f.calls) != 1 || f.calls[0].target != ghe || m.target != ghe {
		t.Errorf("calls = %+v target = %+v; want the Enterprise repo, kept for the new graph", f.calls, m.target)
	}
}

func TestDeletingTheRunYouAreLookingAtLeavesTheScreen(t *testing.T) {
	installRunActs(t)
	m := keyAll(failedGraphModel(), "d", "y")
	if m.view != viewRuns {
		t.Errorf("view = %v; a deleted run's graph is gone, so go back", m.view)
	}
}

func TestFJumpsToTheFirstFailedJobsLog(t *testing.T) {
	m := keys(failedGraphModel(), "f")
	if m.view != viewLogs || m.jobID != "test" || m.target != acme {
		t.Fatalf("view=%v jobID=%q; want the log of the first failed job (test)", m.view, m.jobID)
	}
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	lines[120] = "##[error]Process completed with exit code 1."
	next, _ := m.Update(dataMsg{view: viewLogs, logs: lines})
	m = next.(model)
	if m.top < 100 || m.top > 120 {
		t.Errorf("top = %d; the log should open around the error line (120)", m.top)
	}
	next, _ = m.Update(dataMsg{view: viewLogs, logs: lines}) // a refresh does not jump again
	m.top = 5
	next, _ = m.Update(dataMsg{view: viewLogs, logs: lines})
	if next.(model).top != 5 {
		t.Error("the jump happens once, on the first load")
	}
}

func TestFWithNothingFailedSaysSo(t *testing.T) {
	m := failedGraphModel()
	m.graph = graph.Build([]graph.Job{ok("lint")}, []workflow.JobSpec{{ID: "lint", Name: "lint"}})
	m = keys(m, "f")
	if m.view != viewGraph || !strings.Contains(m.status, "no failed job") {
		t.Errorf("view=%v status=%q", m.view, m.status)
	}
}

func TestFirstFailureLine(t *testing.T) {
	cases := []struct {
		lines []string
		want  int
	}{
		{[]string{"a", "b", "##[error]boom", "##[error]again"}, 2},
		{[]string{"a", "Some error: x", "b", "FAILED y"}, 1},
		{[]string{"a", "b"}, 0},
		{nil, 0},
		{[]string{"terror of the deep", "an error occurred"}, 1}, // whole words only
	}
	for _, c := range cases {
		if got := firstFailureLine(c.lines); got != c.want {
			t.Errorf("%q: %d; want %d", c.lines, got, c.want)
		}
	}
}

func TestFOpensTheJobByItsIDNotItsName(t *testing.T) {
	failed := job("test", "completed", "failure")
	failed.ID = "9876543210"
	m := failedGraphModel()
	m.graph = graph.Build([]graph.Job{ok("lint"), failed}, []workflow.JobSpec{{ID: "lint", Name: "lint"}, {ID: "test", Name: "test", Needs: []string{"lint"}}})
	m = keys(m, "f")
	if m.jobID != "9876543210" || m.jobName != "test" {
		t.Errorf("jobID=%q jobName=%q; the log is fetched by the job's id", m.jobID, m.jobName)
	}
}
