package graph

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ui"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

func job(name, status, conclusion string) Job {
	return Job{ID: name, Name: name, Status: status, Conclusion: conclusion}
}

func ok(name string) Job { return job(name, "completed", "success") }

func nodeNames(g Graph) []string {
	var out []string
	for _, n := range g.Nodes {
		out = append(out, n.Name)
	}
	return out
}

func TestBuildGraphLinear(t *testing.T) {
	g := Build([]Job{ok("lint"), ok("build")}, []workflow.JobSpec{
		{ID: "lint", Name: "lint"}, {ID: "build", Name: "build", Needs: []string{"lint"}}})
	if g.Flat || len(g.Nodes) != 2 || !reflect.DeepEqual(g.Nodes[1].Needs, []int{0}) {
		t.Fatalf("graph = %+v", g)
	}
	if g.Nodes[0].State != ui.StOK || g.Nodes[0].Label != "✓ success" {
		t.Errorf("node 0 = %+v", g.Nodes[0])
	}
}

func TestBuildGraphMatrixCollapses(t *testing.T) {
	specs := []workflow.JobSpec{{ID: "build", Name: "build"}}
	g := Build([]Job{ok("build (ubuntu, 1.22)"), ok("build (macos, 1.22)")}, specs)
	if g.Flat || len(g.Nodes) != 1 || len(g.Nodes[0].Members) != 2 || g.Nodes[0].Label != "2/2 ✓" {
		t.Fatalf("graph = %+v", g)
	}
	g = Build([]Job{ok("build (ubuntu, 1.22)"), job("build (macos, 1.22)", "completed", "failure")}, specs)
	if g.Nodes[0].State != ui.StFail || g.Nodes[0].Label != "1/2 ✗" {
		t.Errorf("failing group = %+v", g.Nodes[0])
	}
	g = Build([]Job{ok("build (a)"), job("build (b)", "in_progress", "")}, specs)
	if g.Nodes[0].State != ui.StRun || g.Nodes[0].Label != "1/2 ●" {
		t.Errorf("running group = %+v", g.Nodes[0])
	}
}

func TestBuildGraphExpressionNames(t *testing.T) {
	g := Build([]Job{ok("Test 1.22"), ok("Test 1.23")},
		[]workflow.JobSpec{{ID: "test", Name: "Test ${{ matrix.go }}"}})
	if g.Flat || len(g.Nodes) != 1 || len(g.Nodes[0].Members) != 2 {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphReusableGroup(t *testing.T) {
	g := Build([]Job{ok("call / inner-a"), ok("call / inner-b"), ok("after")}, []workflow.JobSpec{
		{ID: "call", Name: "call"}, {ID: "after", Name: "after", Needs: []string{"call"}}})
	if g.Flat || len(g.Nodes) != 2 || len(g.Nodes[0].Members) != 2 || !reflect.DeepEqual(g.Nodes[1].Needs, []int{0}) {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphDisplayNameDiffersFromID(t *testing.T) {
	g := Build([]Job{ok("Unit tests"), ok("ship")}, []workflow.JobSpec{
		{ID: "t", Name: "Unit tests"}, {ID: "ship", Name: "ship", Needs: []string{"t"}}})
	if g.Flat || !reflect.DeepEqual(nodeNames(g), []string{"Unit tests", "ship"}) || !reflect.DeepEqual(g.Nodes[1].Needs, []int{0}) {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphUnknownNeedsIsFlat(t *testing.T) {
	g := Build([]Job{ok("a"), ok("b")}, []workflow.JobSpec{
		{ID: "a", Name: "a"}, {ID: "b", Name: "b", Needs: []string{"ghost"}}})
	if !g.Flat || len(g.Nodes) != 2 || g.Reason == "" || len(g.Nodes[1].Needs) != 0 {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphCycleIsFlat(t *testing.T) {
	g := Build([]Job{ok("a"), ok("b")}, []workflow.JobSpec{
		{ID: "a", Name: "a", Needs: []string{"b"}}, {ID: "b", Name: "b", Needs: []string{"a"}}})
	if !g.Flat || !strings.Contains(g.Reason, "cycle") {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphUnmatchedJobIsFlat(t *testing.T) {
	g := Build([]Job{ok("a"), ok("mystery")}, []workflow.JobSpec{{ID: "a", Name: "a"}})
	if !g.Flat || !strings.Contains(g.Reason, "mystery") || len(g.Nodes) != 2 {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphEmptyAndNoSpecs(t *testing.T) {
	if g := Build(nil, nil); g.Flat || len(g.Nodes) != 0 || len(g.stages()) != 0 {
		t.Errorf("empty Graph = %+v", g)
	}
	g := Build([]Job{ok("a")}, nil)
	if !g.Flat || len(g.Nodes) != 1 || g.Reason == "" {
		t.Errorf("no specs = %+v", g)
	}
}

func TestBuildGraphAbsentSpecIgnored(t *testing.T) {
	g := Build([]Job{ok("build")}, []workflow.JobSpec{
		{ID: "deploy", Name: "deploy"}, {ID: "build", Name: "build", Needs: []string{"deploy"}}})
	if g.Flat || len(g.Nodes) != 1 || len(g.Nodes[0].Needs) != 0 {
		t.Errorf("graph = %+v", g)
	}
}

func TestStagesDiamond(t *testing.T) {
	g := Build([]Job{ok("lint"), ok("test"), ok("build"), ok("deploy"), ok("e2e"), ok("release")}, []workflow.JobSpec{
		{ID: "lint", Name: "lint"}, {ID: "test", Name: "test"},
		{ID: "build", Name: "build", Needs: []string{"lint", "test"}},
		{ID: "deploy", Name: "deploy", Needs: []string{"build"}},
		{ID: "e2e", Name: "e2e", Needs: []string{"build"}},
		{ID: "release", Name: "release", Needs: []string{"deploy", "e2e"}},
	})
	want := [][]int{{0, 1}, {2}, {3, 4}, {5}}
	if got := g.stages(); g.Flat || !reflect.DeepEqual(got, want) {
		t.Errorf("stages = %v; want %v (flat=%v %s)", got, want, g.Flat, g.Reason)
	}
}
