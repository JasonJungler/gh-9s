package graph

import (
	"os"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/workflow"
)

// These tests read the drawn lines back the way a person does: follow a line from a job
// and see which jobs it reaches. A crossing (a cell with all four arms) lets lines go
// straight through; anything else joins them.

var arms = []struct {
	bit, opp uint8
	dx, dy   int
}{{mN, mS, 0, -1}, {mE, mW, 1, 0}, {mS, mN, 0, 1}, {mW, mE, -1, 0}}

func maskAt(cv canvas, x, y int) uint8 {
	if y < 0 || y >= len(cv) || x < 0 || x >= len(cv[y]) || cv[y][x].r != 0 {
		return 0
	}
	return cv[y][x].mask
}

// reach lists the line cells connected to (sx, sy).
func reach(cv canvas, sx, sy int) map[[2]int]bool {
	type state struct{ x, y, dir int } // dir: index into arms of the direction of travel, -1 at the start
	seen := map[state]bool{}
	cells := map[[2]int]bool{}
	stack := []state{{sx, sy, -1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[s] {
			continue
		}
		seen[s] = true
		m := maskAt(cv, s.x, s.y)
		if m == 0 {
			continue
		}
		cells[[2]int{s.x, s.y}] = true
		for d, a := range arms {
			if m&a.bit == 0 {
				continue
			}
			if s.dir >= 0 {
				if m == mN|mE|mS|mW && d != s.dir {
					continue // a crossing: lines pass straight through
				}
				if a.bit == arms[s.dir].opp {
					continue // do not walk back
				}
			}
			nx, ny := s.x+a.dx, s.y+a.dy
			if maskAt(cv, nx, ny)&a.opp != 0 {
				stack = append(stack, state{nx, ny, d})
			}
		}
	}
	return cells
}

// checkNoFalseOrMissingLinks: between neighbouring stages a line connects exactly the jobs
// that depend on each other. A dependency that skips a stage has no line at all; it is
// written in the dependent's box instead ("← lint").
func checkNoFalseOrMissingLinks(t *testing.T, g Graph) {
	t.Helper()
	l := layoutBoxes(g, at(60))
	for v, nv := range g.Nodes {
		for u := range g.Nodes {
			if l.stageOf[u] >= l.stageOf[v] {
				continue
			}
			want := false
			for _, d := range nv.Needs {
				want = want || d == u
			}
			out := [2]int{l.x[l.stageOf[u]] + l.bw[l.stageOf[u]], l.y[u] + 1}
			in := [2]int{l.x[l.stageOf[v]] - 2, l.y[v] + 1}
			connected := reach(l.cv, out[0], out[1])[in]
			adjacent := l.stageOf[v]-l.stageOf[u] == 1
			named := strings.Contains(l.texts[v], "← ") && strings.Contains(l.texts[v][strings.Index(l.texts[v], "← "):], g.Nodes[u].Name)
			switch {
			case connected && !want:
				t.Errorf("false link: following the line from %q reaches %q, which does not depend on it", g.Nodes[u].Name, nv.Name)
			case connected && !adjacent:
				t.Errorf("a line skips stages from %q to %q; it should be written in the box instead", g.Nodes[u].Name, nv.Name)
			case want && adjacent && !connected:
				t.Errorf("missing link: %q depends on %q but no line connects them", nv.Name, g.Nodes[u].Name)
			case want && !adjacent && !named:
				t.Errorf("%q depends on %q (skipping stages) but its box does not say so: %q", nv.Name, g.Nodes[u].Name, l.texts[v])
			case !want && named:
				t.Errorf("%q's box names %q, which it does not depend on: %q", nv.Name, g.Nodes[u].Name, l.texts[v])
			}
		}
	}
}

func TestBoxLinesOfTheSpecExample(t *testing.T) {
	checkNoFalseOrMissingLinks(t, fixture())
}

func TestBoxLinesOfTheDemoWorkflow(t *testing.T) {
	src, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	specs, _ := workflow.ParseJobSpecs(src)
	checkNoFalseOrMissingLinks(t, Build(demoRunJobs(false), specs))
}

// The case that looked wrong in the demo: lint only feeds build, but its line used to
// run into the row of Docs, which Prepare feeds.
func TestBoxLinesDoNotJoinSiblingsOfDifferentParents(t *testing.T) {
	g := Build([]Job{ok("prepare"), ok("lint"), ok("unit"), ok("docs"), ok("build")}, []workflow.JobSpec{
		{ID: "prepare", Name: "prepare"}, {ID: "lint", Name: "lint"},
		{ID: "unit", Name: "unit", Needs: []string{"prepare"}},
		{ID: "docs", Name: "docs", Needs: []string{"prepare"}},
		{ID: "build", Name: "build", Needs: []string{"lint", "unit"}},
	})
	checkNoFalseOrMissingLinks(t, g)
}
