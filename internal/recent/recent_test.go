package recent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func ref(s string) ghcli.RepoRef {
	r, err := ghcli.ParseRepoRef(s)
	if err != nil {
		panic(err)
	}
	return r
}

func tmp(t *testing.T) string { return filepath.Join(t.TempDir(), "sub", "recent.json") }

func names(s *Store) []string {
	var out []string
	for _, r := range s.List() {
		out = append(out, r.String())
	}
	return out
}

func TestAddMovesToFrontAndDedupes(t *testing.T) {
	s, _ := Load(tmp(t))
	s.Add(ref("a/one"))
	s.Add(ref("a/two"))
	s.Add(ref("a/three"))
	s.Add(ref("a/one")) // used again: moves to the front, not duplicated
	got := fmt.Sprint(names(s))
	if got != "[a/one a/three a/two]" {
		t.Errorf("List = %s; want [a/one a/three a/two]", got)
	}
}

func TestKeepsAtMostTwenty(t *testing.T) {
	s, _ := Load(tmp(t))
	for i := 0; i < 30; i++ {
		s.Add(ref(fmt.Sprintf("a/r%d", i)))
	}
	l := names(s)
	if len(l) != 20 || l[0] != "a/r29" || l[19] != "a/r10" {
		t.Errorf("len=%d first=%s last=%s; want the newest 20", len(l), l[0], l[len(l)-1])
	}
}

func TestAddIgnoresTheZeroRepo(t *testing.T) {
	s, _ := Load(tmp(t))
	s.Add(ghcli.RepoRef{}) // "the current directory" is not a repo worth remembering
	if len(s.List()) != 0 {
		t.Errorf("List = %v; want empty", names(s))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := tmp(t)
	s, _ := Load(p)
	s.Add(ref("a/one"))
	s.Add(ref("ghe.corp.com/acme/api"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(names(again)) != "[ghe.corp.com/acme/api a/one]" {
		t.Errorf("round trip = %v", names(again))
	}
}

func TestLoadMissingAndEmpty(t *testing.T) {
	p := tmp(t)
	if s, err := Load(p); err != nil || len(s.List()) != 0 {
		t.Fatalf("missing: %v %v", s, err)
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("  \n"), 0o644)
	if s, err := Load(p); err != nil || len(s.List()) != 0 {
		t.Fatalf("empty: %v %v", s, err)
	}
}

func TestLoadCorruptLeavesFile(t *testing.T) {
	p := tmp(t)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{nope"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected an error")
	}
	if b, _ := os.ReadFile(p); string(b) != "{nope" {
		t.Errorf("file modified: %q", b)
	}
}

func TestLoadRejectsHostileEntries(t *testing.T) {
	for _, src := range []string{
		`["-x/y"]`, `["a/b;rm -rf ~"]`, `["gh.com/evil/host/a/b"]`, `["nonsense"]`, `["a/b c"]`, `[""]`,
	} {
		p := tmp(t)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(src), 0o644)
		if _, err := Load(p); err == nil {
			t.Errorf("accepted %s", src)
		}
		if b, _ := os.ReadFile(p); string(b) != src {
			t.Errorf("file was modified for %s", src)
		}
	}
}
