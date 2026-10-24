package main

import (
	"strings"
	"testing"
)

func graphModel() model {
	g := fixture()
	return model{view: viewGraph, runID: "42", width: 120, height: 30, graph: g,
		back: []viewKind{viewRuns}}
}

func TestGraphViewKeys(t *testing.T) {
	m := graphModel()
	m = keys(m, "l")
	if m.gsel != 2 {
		t.Errorf("gsel = %d after l; want 2 (build)", m.gsel)
	}
	if v := m.View(); !strings.Contains(v, "build") || !strings.Contains(v, "╔") {
		t.Errorf("boxes view should show the selected build box:\n%s", v)
	}
	m = keys(m, "v")
	if m.gstyle != styleTree {
		t.Errorf("style = %v after v; want tree", m.gstyle)
	}
	m = keys(m, "v", "v")
	if m.gstyle != styleBoxes {
		t.Errorf("style = %v after three v presses; want boxes again", m.gstyle)
	}
}

func TestGraphEnterOnSingleOpensLogs(t *testing.T) {
	m := graphModel()
	m.gsel = 2
	m = keys(m, "enter")
	if m.view != viewLogs || m.jobID != "build" || m.jobName != "build" {
		t.Errorf("view=%v jobID=%q jobName=%q", m.view, m.jobID, m.jobName)
	}
	m = keys(m, "esc")
	if m.view != viewGraph {
		t.Errorf("esc from logs should return to the graph, got %v", m.view)
	}
}

func matrixGraphModel() model {
	m := graphModel()
	m.graph = buildGraph([]runJob{ok("build (a)"), ok("build (b)"), ok("ship")}, []jobSpec{
		{ID: "build", Name: "build"}, {ID: "ship", Name: "ship", Needs: []string{"build"}}})
	return m
}

func TestGraphEnterExpandsGroupThenXCollapses(t *testing.T) {
	m := matrixGraphModel()
	m = keys(m, "enter") // group node selected: expand
	if m.view != viewGraph || !m.expanded["build"] {
		t.Fatalf("view=%v expanded=%v", m.view, m.expanded)
	}
	if got := m.shown().nodes[m.gsel].Name; got != "build (a)" {
		t.Errorf("selection after expand = %q; want first member", got)
	}
	m = keys(m, "x")
	if m.expanded["build"] {
		t.Error("x on a member should collapse its group")
	}
	if got := m.shown().nodes[m.gsel].Name; got != "build" {
		t.Errorf("selection after collapse = %q; want the group", got)
	}
}

func TestFlatGraphFallsBackToJobsTable(t *testing.T) {
	m := graphModel()
	m.graph = graph{}
	flat := flatGraph([]runJob{ok("a"), ok("b")}, "dependencies unavailable")
	next, _ := m.Update(dataMsg{view: viewGraph, graph: &flat, jobs: []runJob{ok("a"), ok("b")}})
	m = next.(model)
	if m.view != viewJobs || len(m.tbl.rows) != 2 || !strings.Contains(m.note, "dependencies unavailable") {
		t.Errorf("view=%v rows=%d status=%q", m.view, len(m.tbl.rows), m.status)
	}
	if m = keys(m, "esc"); m.view != viewRuns {
		t.Errorf("esc from fallback should return to runs, got %v", m.view)
	}
}

func TestEnterOnRunOpensGraph(t *testing.T) {
	m := model{view: viewRuns, width: 120, height: 30,
		tbl: table{rows: []row{{id: "77", cells: []string{"77", "✓ success", "CI", "t", "main", "push", "1m"}}}}}
	m = keys(m, "enter")
	if m.view != viewGraph || m.runID != "77" || len(m.back) != 1 || m.back[0] != viewRuns {
		t.Errorf("view=%v runID=%q back=%v", m.view, m.runID, m.back)
	}
}

func TestGraphTitleCountsNodes(t *testing.T) {
	if v := graphModel().View(); !strings.Contains(v, "[6]") {
		t.Errorf("title should count the 6 jobs:\n%s", v)
	}
}

func TestGraphScrollKeysClamp(t *testing.T) {
	m := graphModel()
	m = keys(m, "ctrl+u")
	if m.top != 0 {
		t.Errorf("top = %d; want 0", m.top)
	}
}
