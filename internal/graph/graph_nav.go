package graph

// MoveSel moves the selected node of g. dir is one of h l j k: h/l go to the
// previous/next stage (landing on the node at the same position, or the last
// one if that stage is shorter), j/k move within a stage. Moves past an edge
// are ignored.
func MoveSel(g Graph, sel int, dir rune) int {
	stages := g.stages()
	if len(stages) == 0 {
		return sel
	}
	s, k := 0, 0
	for si, st := range stages {
		for ki, i := range st {
			if i == sel {
				s, k = si, ki
			}
		}
	}
	switch dir {
	case 'l':
		if s+1 < len(stages) {
			s, k = s+1, min(k, len(stages[s+1])-1)
		}
	case 'h':
		if s > 0 {
			s, k = s-1, min(k, len(stages[s-1])-1)
		}
	case 'j':
		k = min(k+1, len(stages[s])-1)
	case 'k':
		k = max(k-1, 0)
	}
	return stages[s][k]
}

// Expand replaces each expanded group node by one node per member job.
// Dependents of the group then depend on every member.
func Expand(g Graph, expanded map[string]bool) Graph {
	if g.Flat || len(expanded) == 0 {
		return g
	}
	var nodes []Node
	newIdx := make([][]int, len(g.Nodes))
	for i, n := range g.Nodes {
		if len(n.Members) > 0 && expanded[n.ID] {
			for _, m := range n.Members {
				mn := newNode(m.ID, m.Name, []Job{m})
				mn.Group = n.ID
				newIdx[i] = append(newIdx[i], len(nodes))
				nodes = append(nodes, mn)
			}
			continue
		}
		newIdx[i] = []int{len(nodes)}
		nodes = append(nodes, n)
	}
	for i, n := range g.Nodes {
		var needs []int
		for _, d := range n.Needs {
			needs = append(needs, newIdx[d]...)
		}
		for _, ni := range newIdx[i] {
			nodes[ni].Needs = append([]int(nil), needs...)
		}
	}
	g.Nodes = nodes
	return g
}
