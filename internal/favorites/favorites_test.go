package favorites

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func tmpFavs(t *testing.T) string { return filepath.Join(t.TempDir(), "sub", "favorites.json") }

var fa = Favorite{Host: "github.com", Repo: "acme/api", Workflow: "deploy.yml"}
var fb = Favorite{Host: "github.com", Repo: "acme/web", Workflow: "ci.yml"}

func TestLoadMissingAndEmpty(t *testing.T) {
	p := tmpFavs(t)
	s, err := Load(p)
	if err != nil || len(s.Items) != 0 {
		t.Fatalf("missing: %v %v", s, err)
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, nil, 0o644)
	if s, err = Load(p); err != nil || len(s.Items) != 0 {
		t.Fatalf("empty: %v %v", s, err)
	}
}

func TestLoadCorruptLeavesFile(t *testing.T) {
	p := tmpFavs(t)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{not json"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(p); string(b) != "{not json" {
		t.Errorf("file modified: %q", b)
	}
}

func TestToggleAddRemove(t *testing.T) {
	s, _ := Load(tmpFavs(t))
	if !s.Toggle(fa) || len(s.Items) != 1 {
		t.Fatal("first toggle should add")
	}
	if s.Toggle(fa) || len(s.Items) != 0 {
		t.Fatal("second toggle should remove")
	}
}

func TestSetAliasRejects(t *testing.T) {
	s, _ := Load(tmpFavs(t))
	s.Toggle(fa)
	s.Toggle(fb)
	for _, bad := range []string{"runs", "FAV", "a b", ""} {
		if err := s.SetAlias(fa, bad); err == nil {
			t.Errorf("alias %q should be rejected", bad)
		}
	}
	if err := s.SetAlias(fa, "prod"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(fb, "PROD"); err == nil {
		t.Error("duplicate alias in different case should be rejected")
	}
	if err := s.SetAlias(fa, "prod"); err != nil {
		t.Errorf("re-setting own alias should work: %v", err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	p := tmpFavs(t)
	s, _ := Load(p)
	s.Toggle(fa)
	s.SetAlias(fa, "prod")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(p)
	if err != nil || len(s2.Items) != 1 || s2.Items[0].Alias != "prod" {
		t.Fatalf("round trip: %+v %v", s2, err)
	}
}

func TestByAliasCaseInsensitive(t *testing.T) {
	s, _ := Load(tmpFavs(t))
	s.Toggle(fa)
	s.SetAlias(fa, "Prod")
	if f, ok := s.ByAlias("prod"); !ok || f.Key() != fa.Key() {
		t.Errorf("byAlias = %+v %v", f, ok)
	}
}

func TestForRepo(t *testing.T) {
	s, _ := Load(tmpFavs(t))
	s.Toggle(fa)
	s.Toggle(fb)
	got := s.ForRepo(ghcli.RepoRef{Host: "github.com", Owner: "acme", Name: "api"})
	if len(got) != 1 || got[0].Workflow != "deploy.yml" {
		t.Errorf("forRepo = %+v", got)
	}
}
