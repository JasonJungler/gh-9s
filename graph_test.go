package main

import (
	"reflect"
	"strings"
	"testing"
)

func job(name, status, conclusion string) runJob {
	return runJob{ID: name, Name: name, Status: status, Conclusion: conclusion}
}

func ok(name string) runJob { return job(name, "completed", "success") }

func nodeNames(g graph) []string {
	var out []string
	for _, n := range g.nodes {
		out = append(out, n.Name)
	}
	return out
}

func TestBuildGraphLinear(t *testing.T) {
	g := buildGraph([]runJob{ok("lint"), ok("build")}, []jobSpec{
		{ID: "lint", Name: "lint"}, {ID: "build", Name: "build", Needs: []string{"lint"}}})
	if g.flat || len(g.nodes) != 2 || !reflect.DeepEqual(g.nodes[1].Needs, []int{0}) {
		t.Fatalf("graph = %+v", g)
	}
	if g.nodes[0].State != stOK || g.nodes[0].Label != "✓ success" {
		t.Errorf("node 0 = %+v", g.nodes[0])
	}
}

func TestBuildGraphMatrixCollapses(t *testing.T) {
	specs := []jobSpec{{ID: "build", Name: "build"}}
	g := buildGraph([]runJob{ok("build (ubuntu, 1.22)"), ok("build (macos, 1.22)")}, specs)
	if g.flat || len(g.nodes) != 1 || len(g.nodes[0].Members) != 2 || g.nodes[0].Label != "2/2 ✓" {
		t.Fatalf("graph = %+v", g)
	}
	g = buildGraph([]runJob{ok("build (ubuntu, 1.22)"), job("build (macos, 1.22)", "completed", "failure")}, specs)
	if g.nodes[0].State != stFail || g.nodes[0].Label != "1/2 ✗" {
		t.Errorf("failing group = %+v", g.nodes[0])
	}
	g = buildGraph([]runJob{ok("build (a)"), job("build (b)", "in_progress", "")}, specs)
	if g.nodes[0].State != stRun || g.nodes[0].Label != "1/2 ●" {
		t.Errorf("running group = %+v", g.nodes[0])
	}
}

func TestBuildGraphExpressionNames(t *testing.T) {
	g := buildGraph([]runJob{ok("Test 1.22"), ok("Test 1.23")},
		[]jobSpec{{ID: "test", Name: "Test ${{ matrix.go }}"}})
	if g.flat || len(g.nodes) != 1 || len(g.nodes[0].Members) != 2 {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphReusableGroup(t *testing.T) {
	g := buildGraph([]runJob{ok("call / inner-a"), ok("call / inner-b"), ok("after")}, []jobSpec{
		{ID: "call", Name: "call"}, {ID: "after", Name: "after", Needs: []string{"call"}}})
	if g.flat || len(g.nodes) != 2 || len(g.nodes[0].Members) != 2 || !reflect.DeepEqual(g.nodes[1].Needs, []int{0}) {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphDisplayNameDiffersFromID(t *testing.T) {
	g := buildGraph([]runJob{ok("Unit tests"), ok("ship")}, []jobSpec{
		{ID: "t", Name: "Unit tests"}, {ID: "ship", Name: "ship", Needs: []string{"t"}}})
	if g.flat || !reflect.DeepEqual(nodeNames(g), []string{"Unit tests", "ship"}) || !reflect.DeepEqual(g.nodes[1].Needs, []int{0}) {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphUnknownNeedsIsFlat(t *testing.T) {
	g := buildGraph([]runJob{ok("a"), ok("b")}, []jobSpec{
		{ID: "a", Name: "a"}, {ID: "b", Name: "b", Needs: []string{"ghost"}}})
	if !g.flat || len(g.nodes) != 2 || g.reason == "" || len(g.nodes[1].Needs) != 0 {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphCycleIsFlat(t *testing.T) {
	g := buildGraph([]runJob{ok("a"), ok("b")}, []jobSpec{
		{ID: "a", Name: "a", Needs: []string{"b"}}, {ID: "b", Name: "b", Needs: []string{"a"}}})
	if !g.flat || !strings.Contains(g.reason, "cycle") {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphUnmatchedJobIsFlat(t *testing.T) {
	g := buildGraph([]runJob{ok("a"), ok("mystery")}, []jobSpec{{ID: "a", Name: "a"}})
	if !g.flat || !strings.Contains(g.reason, "mystery") || len(g.nodes) != 2 {
		t.Errorf("graph = %+v", g)
	}
}

func TestBuildGraphEmptyAndNoSpecs(t *testing.T) {
	if g := buildGraph(nil, nil); g.flat || len(g.nodes) != 0 || len(g.stages()) != 0 {
		t.Errorf("empty graph = %+v", g)
	}
	g := buildGraph([]runJob{ok("a")}, nil)
	if !g.flat || len(g.nodes) != 1 || g.reason == "" {
		t.Errorf("no specs = %+v", g)
	}
}

func TestBuildGraphAbsentSpecIgnored(t *testing.T) {
	g := buildGraph([]runJob{ok("build")}, []jobSpec{
		{ID: "deploy", Name: "deploy"}, {ID: "build", Name: "build", Needs: []string{"deploy"}}})
	if g.flat || len(g.nodes) != 1 || len(g.nodes[0].Needs) != 0 {
		t.Errorf("graph = %+v", g)
	}
}

func TestStagesDiamond(t *testing.T) {
	g := buildGraph([]runJob{ok("lint"), ok("test"), ok("build"), ok("deploy"), ok("e2e"), ok("release")}, []jobSpec{
		{ID: "lint", Name: "lint"}, {ID: "test", Name: "test"},
		{ID: "build", Name: "build", Needs: []string{"lint", "test"}},
		{ID: "deploy", Name: "deploy", Needs: []string{"build"}},
		{ID: "e2e", Name: "e2e", Needs: []string{"build"}},
		{ID: "release", Name: "release", Needs: []string{"deploy", "e2e"}},
	})
	want := [][]int{{0, 1}, {2}, {3, 4}, {5}}
	if got := g.stages(); g.flat || !reflect.DeepEqual(got, want) {
		t.Errorf("stages = %v; want %v (flat=%v %s)", got, want, g.flat, g.reason)
	}
}
