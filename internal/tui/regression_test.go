package tui

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

func TestShouldPoll(t *testing.T) {
	// a failed matrix group with a member still running must not stop the graph
	g := graph.Build([]graph.Job{job("build (a)", "completed", "failure"), job("build (b)", "in_progress", "")},
		[]workflow.JobSpec{{ID: "build", Name: "build"}})
	if g.Nodes[0].State != stFail {
		t.Fatalf("setup: group state = %v", g.Nodes[0].State)
	}
	m := model{view: viewGraph, graph: g, runStatus: "in_progress"}
	if !m.shouldPoll() {
		t.Error("graph of an in-progress run must keep polling even if a group shows failed")
	}
	m.runStatus = "completed"
	if m.shouldPoll() {
		t.Error("a completed run needs no polling")
	}
	m.runStatus = ""
	if !m.shouldPoll() {
		t.Error("unknown status (still loading) must poll")
	}
	if (model{view: viewFavs}).shouldPoll() {
		t.Error(":fav must not poll continuously")
	}
	if (model{view: viewLogs}).shouldPoll() || !(model{view: viewLogs, follow: true}).shouldPoll() {
		t.Error("logs poll only while following")
	}
	if !(model{view: viewRuns}).shouldPoll() {
		t.Error("runs list polls")
	}
}

func TestSpecsMapIsNotMutatedInPlace(t *testing.T) {
	orig := map[string][]workflow.JobSpec{}
	m := graphModel()
	m.specs = orig
	g := fixture()
	next, _ := m.Update(dataMsg{view: viewGraph, graph: &g, jobs: nil,
		run: ghcli.RunMeta{Status: "in_progress", Conclusion: ""}, specs: map[string][]workflow.JobSpec{"42": {{ID: "x", Name: "x"}}}})
	if len(orig) != 0 {
		t.Error("Update wrote into the map shared with in-flight fetch goroutines")
	}
	if got := next.(model).specs["42"]; len(got) != 1 {
		t.Errorf("new model lost the spec: %v", got)
	}
}

func TestFallbackReasonSurvivesKeypress(t *testing.T) {
	m := graphModel()
	flat := graph.FlatGraph([]graph.Job{ok("a")}, "dependencies unavailable")
	next, _ := m.Update(dataMsg{view: viewGraph, graph: &flat, jobs: []graph.Job{ok("a")}, run: ghcli.RunMeta{Status: "completed", Conclusion: "success"}})
	m = keys(next, "j")
	if !strings.Contains(m.View(), "dependencies unavailable") {
		t.Errorf("reason disappeared after a keypress:\n%s", m.View())
	}
}

func TestFavoritesDisabledMessageIsSticky(t *testing.T) {
	m := favModel(t)
	m.banner = "favorites disabled: bad json"
	m = keys(m, "j", "j")
	if !strings.Contains(m.View(), "favorites disabled") {
		t.Errorf("startup problem should stay visible:\n%s", m.View())
	}
}

func TestPagerCommandKeepsValuesOutOfTheShell(t *testing.T) {
	evil := ghcli.RepoRef{Owner: "a", Name: "b;touch /tmp/pwned"}
	cmd := pagerCommand(evil, "123", false)
	script := cmd.Args[2]
	if strings.Contains(script, "pwned") || strings.Contains(script, "123") {
		t.Errorf("values must be passed as arguments, not pasted into the script: %q", script)
	}
	if !reflect.DeepEqual(cmd.Args[4:], []string{"run", "view", "123", "--log", "-R", "a/b;touch /tmp/pwned"}) {
		t.Errorf("args = %q", cmd.Args[4:])
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
}
