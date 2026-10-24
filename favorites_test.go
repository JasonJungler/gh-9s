package main

import (
	"os"
	"path/filepath"
	"testing"
)

func tmpFavs(t *testing.T) string { return filepath.Join(t.TempDir(), "sub", "favorites.json") }

var fa = favorite{Host: "github.com", Repo: "acme/api", Workflow: "deploy.yml"}
var fb = favorite{Host: "github.com", Repo: "acme/web", Workflow: "ci.yml"}

func TestLoadMissingAndEmpty(t *testing.T) {
	p := tmpFavs(t)
	s, err := loadFavs(p)
	if err != nil || len(s.items) != 0 {
		t.Fatalf("missing: %v %v", s, err)
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, nil, 0o644)
	if s, err = loadFavs(p); err != nil || len(s.items) != 0 {
		t.Fatalf("empty: %v %v", s, err)
	}
}

func TestLoadCorruptLeavesFile(t *testing.T) {
	p := tmpFavs(t)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{not json"), 0o644)
	if _, err := loadFavs(p); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(p); string(b) != "{not json" {
		t.Errorf("file modified: %q", b)
	}
}

func TestToggleAddRemove(t *testing.T) {
	s, _ := loadFavs(tmpFavs(t))
	if !s.toggle(fa) || len(s.items) != 1 {
		t.Fatal("first toggle should add")
	}
	if s.toggle(fa) || len(s.items) != 0 {
		t.Fatal("second toggle should remove")
	}
}

func TestSetAliasRejects(t *testing.T) {
	s, _ := loadFavs(tmpFavs(t))
	s.toggle(fa)
	s.toggle(fb)
	for _, bad := range []string{"runs", "FAV", "a b", ""} {
		if err := s.setAlias(fa, bad); err == nil {
			t.Errorf("alias %q should be rejected", bad)
		}
	}
	if err := s.setAlias(fa, "prod"); err != nil {
		t.Fatal(err)
	}
	if err := s.setAlias(fb, "PROD"); err == nil {
		t.Error("duplicate alias in different case should be rejected")
	}
	if err := s.setAlias(fa, "prod"); err != nil {
		t.Errorf("re-setting own alias should work: %v", err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	p := tmpFavs(t)
	s, _ := loadFavs(p)
	s.toggle(fa)
	s.setAlias(fa, "prod")
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	s2, err := loadFavs(p)
	if err != nil || len(s2.items) != 1 || s2.items[0].Alias != "prod" {
		t.Fatalf("round trip: %+v %v", s2, err)
	}
}

func TestByAliasCaseInsensitive(t *testing.T) {
	s, _ := loadFavs(tmpFavs(t))
	s.toggle(fa)
	s.setAlias(fa, "Prod")
	if f, ok := s.byAlias("prod"); !ok || f.key() != fa.key() {
		t.Errorf("byAlias = %+v %v", f, ok)
	}
}

func TestForRepo(t *testing.T) {
	s, _ := loadFavs(tmpFavs(t))
	s.toggle(fa)
	s.toggle(fb)
	got := s.forRepo(repoRef{Host: "github.com", Owner: "acme", Name: "api"})
	if len(got) != 1 || got[0].Workflow != "deploy.yml" {
		t.Errorf("forRepo = %+v", got)
	}
}
