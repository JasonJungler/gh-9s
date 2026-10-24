package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/pins"
)

// projectsTable lists the pinned Projects.
func projectsTable(st *pins.Store) table {
	t := table{cols: []string{"PROJECT", "WHERE"}}
	for _, p := range st.Projects {
		t.rows = append(t.rows, row{id: projectKey(p), extra: projectKey(p), cells: []string{p.Title, projectKey(p)}})
	}
	return t
}

func projectKey(p pins.Project) string {
	s := p.Owner + "/#" + strconv.Itoa(p.Number)
	if p.Host != "" {
		s = p.Host + "/" + s
	}
	return s
}

func (m model) projectTabKey(k tea.KeyMsg) (model, tea.Cmd, bool) { return m, nil, false }
