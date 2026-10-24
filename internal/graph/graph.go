// Package graph models a workflow run as a job dependency graph and draws it
// as boxes, a tree, or timeline lanes.
package graph

import (
	"fmt"
	"strings"
	"time"

	"github.com/MurabitoB/gh-9s/internal/ui"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

// Job is one job of a workflow run as reported by GitHub.
type Job struct {
	ID, Name, Status, Conclusion, Step, URL string
	Start, End                              time.Time
}

// Node is one node of the job graph: a single job, or a collapsed group of
// matrix / reusable-workflow jobs (Members non-empty).
type Node struct {
	ID, Name, Label, Step string
	State                 ui.State
	Start, End            time.Time
	Needs                 []int // indices into graph.Nodes
	Members               []Job
	Job                   Job    // the job itself, for single-job nodes
	Group                 string // ID of the group this node was expanded from
}

// Graph is the dependency graph of a run. When Flat is set the dependencies
// could not be determined: Nodes are listed with no edges and Reason says why.
type Graph struct {
	Nodes  []Node
	Flat   bool
	Reason string
}

// State is the job's display label and coarse state.
func (j Job) State() (string, ui.State) { return ui.RunState(j.Status, j.Conclusion) }

// specBase is the part of a spec's display name that run job names start with.
func specBase(s workflow.JobSpec) (prefix string, expr bool) {
	if i := strings.Index(s.Name, "${{"); i >= 0 {
		return s.Name[:i], true
	}
	return s.Name, false
}

// matchSpec returns the spec a run job belongs to (longest matching name wins).
func matchSpec(name string, specs []workflow.JobSpec) int {
	best, bestLen := -1, -1
	for i, s := range specs {
		base, expr := specBase(s)
		match := false
		switch {
		case name == s.Name || name == s.ID:
			match, base = true, s.Name
		case expr:
			match = base != "" && strings.HasPrefix(name, base)
		default:
			match = strings.HasPrefix(name, base+" (") || strings.HasPrefix(name, base+" / ")
		}
		if match && len(base) > bestLen {
			best, bestLen = i, len(base)
		}
	}
	return best
}

// displayName is a spec's name without matrix expressions: "Build (${{ matrix.os }})"
// and "Unit (go ${{ matrix.go }} / ${{ matrix.os }})" are shown as "Build" and "Unit".
func displayName(s workflow.JobSpec) string {
	i := strings.Index(s.Name, "${{")
	if i < 0 {
		return s.Name
	}
	prefix := s.Name[:i]
	if j := strings.LastIndex(prefix, "("); j >= 0 && !strings.Contains(prefix[j:], ")") {
		prefix = prefix[:j] // the expression sits inside parentheses: drop the whole group
	}
	return strings.TrimRight(prefix, " ([-:/")
}

// countLabel summarises a group by status, e.g. "6✓ 1✗ 2●" (zero counts are left out).
// ● is a job in progress; ○ is one that has not started, was skipped, or was cancelled.
func countLabel(jobs []Job) string {
	var ok, fail, run, wait int
	for _, j := range jobs {
		_, s := j.State()
		switch {
		case s == ui.StOK:
			ok++
		case s == ui.StFail:
			fail++
		case s == ui.StRun && j.Status == "in_progress":
			run++
		default:
			wait++
		}
	}
	var parts []string
	for _, p := range []struct {
		n   int
		sym string
	}{{ok, "✓"}, {fail, "✗"}, {run, "●"}, {wait, "○"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", p.n, p.sym))
		}
	}
	return strings.Join(parts, " ")
}

func aggregate(jobs []Job) ui.State {
	st := ui.StOK
	running := false
	for _, j := range jobs {
		_, s := j.State()
		switch s {
		case ui.StOK:
		case ui.StFail:
			st = ui.StFail
		case ui.StRun:
			running = true
		default:
			if st == ui.StOK {
				st = ui.StOther
			}
		}
	}
	if st != ui.StFail && running {
		st = ui.StRun
	}
	return st
}

func newNode(id, name string, jobs []Job) Node {
	n := Node{ID: id, Name: name}
	running := false
	for _, j := range jobs {
		if !j.Start.IsZero() && (n.Start.IsZero() || j.Start.Before(n.Start)) {
			n.Start = j.Start
		}
		if j.End.IsZero() {
			running = true
		} else if j.End.After(n.End) {
			n.End = j.End
		}
		if n.Step == "" {
			n.Step = j.Step
		}
	}
	if running {
		n.End = time.Time{} // still going: renderers measure up to "now"
	}
	if len(jobs) == 1 {
		n.Job = jobs[0]
		if jobs[0].Name != "" {
			n.Name = jobs[0].Name // keeps a matrix suffix such as "build (ubuntu)"
		}
		n.Label, n.State = jobs[0].State()
		return n
	}
	n.State = aggregate(jobs)
	n.Members = jobs
	n.Label = countLabel(jobs)
	return n
}

// FlatGraph lists jobs with no edges, for when dependencies cannot be determined.
func FlatGraph(jobs []Job, reason string) Graph {
	g := Graph{Flat: true, Reason: reason}
	for _, j := range jobs {
		g.Nodes = append(g.Nodes, newNode(j.ID, j.Name, []Job{j}))
	}
	return g
}

// Build matches a run's jobs to the workflow file's job specs and links
// them by `needs`. Anything it cannot place or order falls back to a flat graph.
func Build(jobs []Job, specs []workflow.JobSpec) Graph {
	if len(jobs) == 0 {
		return Graph{}
	}
	if len(specs) == 0 {
		return FlatGraph(jobs, "dependencies unavailable (workflow file could not be read)")
	}
	groups := make([][]Job, len(specs))
	for _, j := range jobs {
		i := matchSpec(j.Name, specs)
		if i < 0 {
			return FlatGraph(jobs, fmt.Sprintf("job %q was not found in the workflow file", j.Name))
		}
		groups[i] = append(groups[i], j)
	}
	index := map[string]int{} // spec ID -> node index
	known := map[string]bool{}
	for _, s := range specs {
		known[s.ID] = true
	}
	g := Graph{}
	for i, s := range specs {
		if len(groups[i]) == 0 {
			continue // skipped job: nothing to draw
		}
		index[s.ID] = len(g.Nodes)
		g.Nodes = append(g.Nodes, newNode(s.ID, displayName(s), groups[i]))
	}
	for i, s := range specs {
		if len(groups[i]) == 0 {
			continue
		}
		for _, need := range s.Needs {
			if !known[need] {
				return FlatGraph(jobs, fmt.Sprintf("job %q needs unknown job %q", s.ID, need))
			}
			if n, ok := index[need]; ok {
				g.Nodes[index[s.ID]].Needs = append(g.Nodes[index[s.ID]].Needs, n)
			}
		}
	}
	if _, ok := g.depths(); !ok {
		return FlatGraph(jobs, "the workflow's needs form a cycle")
	}
	return g
}

// depths gives each node its longest-path depth; ok is false on a cycle.
func (g Graph) depths() ([]int, bool) {
	depth := make([]int, len(g.Nodes))
	indeg := make([]int, len(g.Nodes))
	users := make([][]int, len(g.Nodes))
	for i, n := range g.Nodes {
		indeg[i] = len(n.Needs)
		for _, d := range n.Needs {
			users[d] = append(users[d], i)
		}
	}
	var queue []int
	for i, d := range indeg {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	seen := 0
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		seen++
		for _, u := range users[i] {
			depth[u] = max(depth[u], depth[i]+1)
			if indeg[u]--; indeg[u] == 0 {
				queue = append(queue, u)
			}
		}
	}
	return depth, seen == len(g.Nodes)
}

// stages groups node indices into dependency layers, stable within a layer.
func (g Graph) stages() [][]int {
	if len(g.Nodes) == 0 {
		return nil
	}
	if g.Flat {
		all := make([]int, len(g.Nodes))
		for i := range all {
			all[i] = i
		}
		return [][]int{all}
	}
	depth, _ := g.depths()
	var out [][]int
	for i, d := range depth {
		for len(out) <= d {
			out = append(out, nil)
		}
		out[d] = append(out[d], i)
	}
	return out
}
