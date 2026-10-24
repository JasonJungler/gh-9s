package main

import (
	"fmt"
	"strings"
	"time"
)

// runJob is one job of a workflow run as reported by GitHub.
type runJob struct {
	ID, Name, Status, Conclusion, Step, URL string
	Start, End                              time.Time
}

// gNode is one node of the job graph: a single job, or a collapsed group of
// matrix / reusable-workflow jobs (Members non-empty).
type gNode struct {
	ID, Name, Label, Step string
	State                 state
	Start, End            time.Time
	Needs                 []int // indices into graph.nodes
	Members               []runJob
	Job                   runJob // the job itself, for single-job nodes
	Group                 string // ID of the group this node was expanded from
}

// graph is the dependency graph of a run. When flat is set the dependencies
// could not be determined: nodes are listed with no edges and reason says why.
type graph struct {
	nodes  []gNode
	flat   bool
	reason string
}

func (j runJob) state() (string, state) { return runState(j.Status, j.Conclusion) }

// specBase is the part of a spec's display name that run job names start with.
func specBase(s jobSpec) (prefix string, expr bool) {
	if i := strings.Index(s.Name, "${{"); i >= 0 {
		return s.Name[:i], true
	}
	return s.Name, false
}

// matchSpec returns the spec a run job belongs to (longest matching name wins).
func matchSpec(name string, specs []jobSpec) int {
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

func aggregate(jobs []runJob) (st state, done int) {
	st = stOK
	running := false
	for _, j := range jobs {
		_, s := j.state()
		switch s {
		case stOK:
			done++
		case stFail:
			st = stFail
		case stRun:
			running = true
		default:
			if st == stOK {
				st = stOther
			}
		}
	}
	if st != stFail && running {
		st = stRun
	}
	return st, done
}

var stateSymbol = map[state]string{stOK: "✓", stFail: "✗", stRun: "●", stOther: "○"}

func newNode(id, name string, jobs []runJob) gNode {
	n := gNode{ID: id, Name: name}
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
		n.Label, n.State = jobs[0].state()
		return n
	}
	var done int
	n.State, done = aggregate(jobs)
	n.Members = jobs
	n.Label = fmt.Sprintf("%d/%d %s", done, len(jobs), stateSymbol[n.State])
	return n
}

func flatGraph(jobs []runJob, reason string) graph {
	g := graph{flat: true, reason: reason}
	for _, j := range jobs {
		g.nodes = append(g.nodes, newNode(j.ID, j.Name, []runJob{j}))
	}
	return g
}

// buildGraph matches a run's jobs to the workflow file's job specs and links
// them by `needs`. Anything it cannot place or order falls back to a flat graph.
func buildGraph(jobs []runJob, specs []jobSpec) graph {
	if len(jobs) == 0 {
		return graph{}
	}
	if len(specs) == 0 {
		return flatGraph(jobs, "dependencies unavailable (workflow file could not be read)")
	}
	groups := make([][]runJob, len(specs))
	for _, j := range jobs {
		i := matchSpec(j.Name, specs)
		if i < 0 {
			return flatGraph(jobs, fmt.Sprintf("job %q was not found in the workflow file", j.Name))
		}
		groups[i] = append(groups[i], j)
	}
	index := map[string]int{} // spec ID -> node index
	known := map[string]bool{}
	for _, s := range specs {
		known[s.ID] = true
	}
	g := graph{}
	for i, s := range specs {
		if len(groups[i]) == 0 {
			continue // skipped job: nothing to draw
		}
		index[s.ID] = len(g.nodes)
		g.nodes = append(g.nodes, newNode(s.ID, s.Name, groups[i]))
	}
	for i, s := range specs {
		if len(groups[i]) == 0 {
			continue
		}
		for _, need := range s.Needs {
			if !known[need] {
				return flatGraph(jobs, fmt.Sprintf("job %q needs unknown job %q", s.ID, need))
			}
			if n, ok := index[need]; ok {
				g.nodes[index[s.ID]].Needs = append(g.nodes[index[s.ID]].Needs, n)
			}
		}
	}
	if _, ok := g.depths(); !ok {
		return flatGraph(jobs, "the workflow's needs form a cycle")
	}
	return g
}

// depths gives each node its longest-path depth; ok is false on a cycle.
func (g graph) depths() ([]int, bool) {
	depth := make([]int, len(g.nodes))
	indeg := make([]int, len(g.nodes))
	users := make([][]int, len(g.nodes))
	for i, n := range g.nodes {
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
	return depth, seen == len(g.nodes)
}

// stages groups node indices into dependency layers, stable within a layer.
func (g graph) stages() [][]int {
	if len(g.nodes) == 0 {
		return nil
	}
	if g.flat {
		all := make([]int, len(g.nodes))
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
