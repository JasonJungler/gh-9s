package graph

import (
	"reflect"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/workflow"
)

func TestMoveSelAcrossStages(t *testing.T) {
	g := fixture() // stages: [lint test] [build] [deploy e2e] [release]
	steps := []struct {
		dir  rune
		from int
		want int
	}{
		{'l', 0, 2}, // lint -> build (only node in next stage)
		{'l', 2, 3}, // build -> deploy-staging
		{'j', 3, 4}, // deploy-staging -> e2e
		{'l', 4, 5}, // e2e -> release
		{'h', 5, 3}, // release -> first node of previous stage (position clamped)
		{'k', 4, 3}, // e2e -> deploy-staging
		{'j', 0, 1}, // lint -> test
		{'h', 2, 0}, // build -> lint (position 0)
	}
	for _, s := range steps {
		if got := MoveSel(g, s.from, s.dir); got != s.want {
			t.Errorf("MoveSel(%c from %d) = %d; want %d", s.dir, s.from, got, s.want)
		}
	}
}

func TestMoveSelClamps(t *testing.T) {
	g := fixture()
	if got := MoveSel(g, 5, 'l'); got != 5 {
		t.Errorf("right at last stage = %d; want 5", got)
	}
	if got := MoveSel(g, 0, 'h'); got != 0 {
		t.Errorf("left at first stage = %d; want 0", got)
	}
	if got := MoveSel(g, 0, 'k'); got != 0 {
		t.Errorf("up at top = %d; want 0", got)
	}
	if got := MoveSel(g, 1, 'j'); got != 1 {
		t.Errorf("down at bottom = %d; want 1", got)
	}
}

func TestMoveSelEmptyAndOutOfRange(t *testing.T) {
	if got := MoveSel(Graph{}, 3, 'l'); got != 3 {
		t.Errorf("empty Graph changed sel to %d", got)
	}
	if got := MoveSel(fixture(), 99, 'l'); got < 0 || got >= 6 {
		t.Errorf("out of range sel gave %d", got)
	}
}

func TestExpandGraph(t *testing.T) {
	g := Build([]Job{ok("build (a)"), ok("build (b)"), ok("ship")}, []workflow.JobSpec{
		{ID: "build", Name: "build"}, {ID: "ship", Name: "ship", Needs: []string{"build"}}})
	if len(g.Nodes) != 2 || len(g.Nodes[0].Members) != 2 {
		t.Fatalf("collapsed Graph = %+v", g)
	}
	if same := Expand(g, nil); !reflect.DeepEqual(same, g) {
		t.Error("nothing expanded should return the Graph unchanged")
	}
	e := Expand(g, map[string]bool{"build": true})
	if !reflect.DeepEqual(nodeNames(e), []string{"build (a)", "build (b)", "ship"}) {
		t.Fatalf("names = %v", nodeNames(e))
	}
	if e.Nodes[0].Group != "build" || e.Nodes[1].Group != "build" || e.Nodes[0].Job.ID != "build (a)" {
		t.Errorf("members not marked: %+v", e.Nodes[:2])
	}
	if !reflect.DeepEqual(e.Nodes[2].Needs, []int{0, 1}) {
		t.Errorf("ship needs = %v; want both members", e.Nodes[2].Needs)
	}
}
