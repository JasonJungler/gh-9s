package tui

import (
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

const latestTTL = 30 * time.Second

type latestEntry struct {
	label string
	st    state
	age   string
	err   error
	at    time.Time
}

// latestFetcher is a variable so tests can replace the network call.
var latestFetcher = fetchLatest

func workflowFile(path string) string { return filepath.Base(path) }

// pinFavorites moves favorite workflows (matched by file name) to the top,
// stars them, and leaves the input rows untouched.
func pinFavorites(rows []row, favs []favorites.Favorite) []row {
	isFav := map[string]bool{}
	for _, f := range favs {
		isFav[f.Workflow] = true
	}
	var top, rest []row
	for _, r := range rows {
		if len(r.cells) > 3 && isFav[workflowFile(r.cells[3])] {
			c := append([]string(nil), r.cells...)
			c[1] = "★ " + c[1]
			r.cells = c
			top = append(top, r)
		} else {
			rest = append(rest, r)
		}
	}
	return append(top, rest...)
}

func fetchLatest(r ghcli.RepoRef, workflow string) (string, state, string, error) {
	var runs []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		CreatedAt  string `json:"createdAt"`
	}
	if err := ghcli.JSON(&runs, ghcli.Scoped(r, "run", "list", "-L", "1", "-w", workflow,
		"--json", "status,conclusion,createdAt")...); err != nil {
		return "", stOther, "", err
	}
	if len(runs) == 0 {
		return "no runs", stOther, "-", nil
	}
	label, st := runState(runs[0].Status, runs[0].Conclusion)
	return label, st, age(runs[0].CreatedAt), nil
}

// fetchFavorites builds the :fav table. Entries newer than latestTTL come from
// cache; stale ones are fetched a few at a time. A failed fetch renders as an
// "unavailable" row instead of failing the view.
func fetchFavorites(s *favorites.Store, cache map[string]latestEntry, now time.Time) (table, map[string]latestEntry) {
	next := map[string]latestEntry{}
	var stale []favorites.Favorite
	for _, f := range s.Items {
		if e, ok := cache[f.Key()]; ok && now.Sub(e.at) < latestTTL {
			next[f.Key()] = e
		} else {
			stale = append(stale, f)
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, f := range stale {
		wg.Add(1)
		sem <- struct{}{}
		go func(f favorites.Favorite) {
			defer wg.Done()
			defer func() { <-sem }()
			label, st, a, err := latestFetcher(f.Target(), f.Workflow)
			mu.Lock()
			next[f.Key()] = latestEntry{label, st, a, err, now}
			mu.Unlock()
		}(f)
	}
	wg.Wait()

	t := table{cols: []string{"ALIAS", "REPO", "WORKFLOW", "LATEST", "AGE"}}
	for _, f := range s.Items {
		e := next[f.Key()]
		alias := f.Alias
		if alias == "" {
			alias = "-"
		}
		label, st, a := e.label, e.st, e.age
		if e.err != nil {
			label, st, a = "unavailable", stOther, "-"
		}
		t.rows = append(t.rows, row{id: f.Key(), extra: f.Key(), state: st,
			cells: []string{alias, f.Repo, f.Workflow, label, a}})
	}
	return t, next
}

func (m model) favForRow(r row) (favorites.Favorite, bool) {
	switch m.view {
	case viewFavs:
		if m.favs != nil {
			for _, f := range m.favs.Items {
				if f.Key() == r.extra {
					return f, true
				}
			}
		}
	case viewWorkflows:
		if t := m.curTarget(); t != (ghcli.RepoRef{}) && len(r.cells) > 3 {
			return favorites.Favorite{Host: favorites.HostOrDefault(t.Host), Repo: t.Owner + "/" + t.Name,
				Workflow: workflowFile(r.cells[3])}, true
		}
	}
	return favorites.Favorite{}, false
}

func (m model) toggleFavorite(r row) (tea.Model, tea.Cmd) {
	f, ok := m.favForRow(r)
	switch {
	case m.favs == nil:
		m.status = styErr.Render("favorites are disabled (see the startup message)")
	case !ok:
		m.status = styErr.Render("cannot tell which repo this is; run gh-9s inside a repo")
	default:
		added := m.favs.Toggle(f)
		if err := m.favs.Save(); err != nil {
			m.status = styErr.Render("✗ " + err.Error())
		} else if added {
			m.status = styOK.Render("★ added " + f.Workflow)
		} else {
			m.status = styDim.Render("removed " + f.Workflow)
		}
		return m, m.fetch()
	}
	return m, nil
}

func (m model) startAlias(r row) (tea.Model, tea.Cmd) {
	f, ok := m.favForRow(r)
	if m.favs == nil || !ok {
		m.status = styErr.Render("favorites are not available here")
		return m, nil
	}
	i := m.favs.Index(f)
	if i < 0 {
		m.status = styDim.Render("press f to make it a favorites.Favorite first")
		return m, nil
	}
	m.aliasFor, m.mode, m.input = f, modeAlias, m.favs.Items[i].Alias
	return m, nil
}

// openFavorite shows a favorite's runs, in its own repo.
func (m *model) openFavorite(f favorites.Favorite, push bool) tea.Cmd {
	m.target = f.Target()
	if m.target == m.curRepo {
		m.target = ghcli.RepoRef{}
	}
	m.wfID, m.wfName = f.Workflow, f.Alias
	if m.wfName == "" {
		m.wfName = f.Workflow
	}
	if push {
		return m.push(viewRuns)
	}
	m.back = nil
	return m.switchTo(viewRuns)
}
