package main

import (
	"path/filepath"
	"sync"
	"time"
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
func pinFavorites(rows []row, favs []favorite) []row {
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

func fetchLatest(r repoRef, workflow string) (string, state, string, error) {
	var runs []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		CreatedAt  string `json:"createdAt"`
	}
	if err := ghJSON(&runs, scoped(r, "run", "list", "-L", "1", "-w", workflow,
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
func fetchFavorites(s *favStore, cache map[string]latestEntry, now time.Time) (table, map[string]latestEntry) {
	next := map[string]latestEntry{}
	var stale []favorite
	for _, f := range s.items {
		if e, ok := cache[f.key()]; ok && now.Sub(e.at) < latestTTL {
			next[f.key()] = e
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
		go func(f favorite) {
			defer wg.Done()
			defer func() { <-sem }()
			label, st, a, err := latestFetcher(f.target(), f.Workflow)
			mu.Lock()
			next[f.key()] = latestEntry{label, st, a, err, now}
			mu.Unlock()
		}(f)
	}
	wg.Wait()

	t := table{cols: []string{"ALIAS", "REPO", "WORKFLOW", "LATEST", "AGE"}}
	for _, f := range s.items {
		e := next[f.key()]
		alias := f.Alias
		if alias == "" {
			alias = "-"
		}
		label, st, a := e.label, e.st, e.age
		if e.err != nil {
			label, st, a = "unavailable", stOther, "-"
		}
		t.rows = append(t.rows, row{id: f.key(), extra: f.key(), state: st,
			cells: []string{alias, f.Repo, f.Workflow, label, a}})
	}
	return t, next
}
