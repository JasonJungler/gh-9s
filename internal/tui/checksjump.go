package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

// sameRepo compares repos ignoring case and the two spellings of the default host.
func sameRepo(a, b ghcli.RepoRef) bool {
	return favorites.HostOrDefault(a.Host) == favorites.HostOrDefault(b.Host) &&
		strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}

// openCheck handles enter on a PR check: a GitHub Actions run opens in the graph view (in the
// run's own repo, which may differ from the PR's); anything else opens in the browser.
func (m model) openCheck(r row) (tea.Model, tea.Cmd) {
	if r.url == "" {
		return m, nil
	}
	link, ok := ghcli.ParseActionsURL(r.url)
	if !ok {
		return m.open(r.url), nil
	}
	if !sameRepo(link.Repo, m.curTarget()) {
		m.target = link.Repo
	}
	m.runID = link.RunID
	m.graph, m.gsel, m.top, m.expanded, m.runStatus = graph.Graph{}, 0, 0, nil, ""
	return m, m.push(viewGraph)
}
