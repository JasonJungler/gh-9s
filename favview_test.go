package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkflowFile(t *testing.T) {
	if got := workflowFile(".github/workflows/deploy.yml"); got != "deploy.yml" {
		t.Errorf("got %q", got)
	}
	if got := workflowFile("dynamic/dependabot/update-graph"); got != "update-graph" {
		t.Errorf("got %q", got)
	}
}

func TestPinFavorites(t *testing.T) {
	rows := []row{
		{id: "1", cells: []string{"1", "CI", "active", ".github/workflows/ci.yml"}},
		{id: "2", cells: []string{"2", "Deploy", "active", ".github/workflows/deploy.yml"}},
		{id: "3", cells: []string{"3", "Lint", "active", ".github/workflows/lint.yml"}},
	}
	favs := []favorite{{Workflow: "deploy.yml"}, {Workflow: "lint.yml"}}
	got := pinFavorites(rows, favs)
	order := []string{got[0].id, got[1].id, got[2].id}
	if order[0] != "2" || order[1] != "3" || order[2] != "1" {
		t.Errorf("order = %v; want [2 3 1]", order)
	}
	if got[0].cells[1] != "★ Deploy" || got[2].cells[1] != "CI" {
		t.Errorf("star prefix wrong: %q / %q", got[0].cells[1], got[2].cells[1])
	}
	if rows[1].cells[1] != "Deploy" {
		t.Error("input rows were mutated")
	}
}

func TestFetchFavoritesUsesCache(t *testing.T) {
	var calls int32
	fail := false
	old := latestFetcher
	defer func() { latestFetcher = old }()
	latestFetcher = func(r repoRef, wf string) (string, state, string, error) {
		atomic.AddInt32(&calls, 1)
		if fail {
			return "", stOther, "", errors.New("404")
		}
		return "✓ success", stOK, "3m", nil
	}
	s := &favStore{items: []favorite{
		{Host: "github.com", Repo: "acme/api", Workflow: "deploy.yml", Alias: "prod"},
		{Host: "github.com", Repo: "acme/web", Workflow: "ci.yml"},
	}}
	now := time.Now()
	tbl, cache := fetchFavorites(s, nil, now)
	if calls != 2 || len(tbl.rows) != 2 {
		t.Fatalf("calls=%d rows=%d", calls, len(tbl.rows))
	}
	if tbl.rows[0].cells[0] != "prod" || tbl.rows[1].cells[0] != "-" {
		t.Errorf("alias cells = %q, %q", tbl.rows[0].cells[0], tbl.rows[1].cells[0])
	}
	if _, _ = fetchFavorites(s, cache, now.Add(10*time.Second)); calls != 2 {
		t.Errorf("cache miss within 30s: calls=%d", calls)
	}
	fail = true
	tbl, _ = fetchFavorites(s, cache, now.Add(40*time.Second))
	if calls != 4 {
		t.Errorf("stale entries should refetch: calls=%d", calls)
	}
	if tbl.rows[0].cells[3] != "unavailable" || tbl.rows[0].state != stOther {
		t.Errorf("failed fetch row = %+v", tbl.rows[0])
	}
}
