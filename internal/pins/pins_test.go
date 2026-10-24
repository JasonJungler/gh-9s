package pins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRepo(t *testing.T) {
	r, err := ParseRepo("acme/app")
	if err != nil || r != (Repo{Repo: "acme/app"}) {
		t.Fatalf("got %+v %v", r, err)
	}
	r, err = ParseRepo("ghe.corp.com/acme/app")
	if err != nil || r.Host != "ghe.corp.com" || r.Target().Owner != "acme" || r.Target().Name != "app" {
		t.Fatalf("got %+v %v", r, err)
	}
	if r, _ := ParseRepo("github.com/acme/app"); r.Host != "" {
		t.Fatalf("github.com host kept: %+v", r)
	}
	for _, bad := range []string{"", "acme", "a/b/c/d", "acme/../x", "-x/y", "a b/c", "acme/app;rm"} {
		if _, err := ParseRepo(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseProject(t *testing.T) {
	h, o, n, err := ParseProject("acme/7")
	if err != nil || h != "" || o != "acme" || n != 7 {
		t.Fatalf("got %q %q %d %v", h, o, n, err)
	}
	h, o, n, err = ParseProject("ghe.corp.com/acme/12")
	if err != nil || h != "ghe.corp.com" || o != "acme" || n != 12 {
		t.Fatalf("got %q %q %d %v", h, o, n, err)
	}
	for _, bad := range []string{"", "acme", "acme/0", "acme/-1", "acme/x", "acme/07", "a/b/c/1/2", "../x/1"} {
		if _, _, _, err := ParseProject(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestToggleAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "pins.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if on, err := s.ToggleRepo(Repo{Repo: "acme/app"}); !on || err != nil {
		t.Fatal(on, err)
	}
	if on, _ := s.ToggleProject(Project{Owner: "acme", Number: 3, Title: "Roadmap"}); !on {
		t.Fatal("not pinned")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	s2, err := Load(path)
	if err != nil || !s2.HasRepo("", "ACME/app") || !s2.HasProject("github.com", "acme", 3) {
		t.Fatalf("%+v %v", s2, err)
	}
	if on, _ := s2.ToggleRepo(Repo{Repo: "acme/app"}); on {
		t.Fatal("second toggle should unpin")
	}
	if err := s2.Save(); err != nil {
		t.Fatal(err)
	}
	s3, _ := Load(path)
	if len(s3.Repos) != 0 || len(s3.Projects) != 1 {
		t.Fatalf("%+v", s3)
	}
}

func TestSaveKeepsPinsAddedElsewhere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	a, _ := Load(path)
	b, _ := Load(path)
	a.ToggleRepo(Repo{Repo: "acme/one"})
	a.Save()
	b.ToggleRepo(Repo{Repo: "acme/two"})
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(path)
	if !c.HasRepo("", "acme/one") || !c.HasRepo("", "acme/two") {
		t.Fatalf("lost a pin: %+v", c.Repos)
	}
	// an unpin is not undone by the re-read
	c.ToggleRepo(Repo{Repo: "acme/one"})
	c.Save()
	d, _ := Load(path)
	if d.HasRepo("", "acme/one") {
		t.Fatal("unpinned repo came back")
	}
}

func TestBrokenFileIsNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("broken file loaded")
	}
	s := Memory()
	s.ToggleRepo(Repo{Repo: "acme/app"})
	if err := s.Save(); err == nil {
		t.Fatal("memory store saved")
	}
	// a store loaded earlier must not clobber a file that went bad afterwards
	good := filepath.Join(t.TempDir(), "p.json")
	st, _ := Load(good)
	os.WriteFile(good, []byte("garbage"), 0o600)
	st.ToggleRepo(Repo{Repo: "acme/app"})
	if err := st.Save(); err == nil {
		t.Fatal("saved over garbage")
	}
	if b, _ := os.ReadFile(good); string(b) != "garbage" {
		t.Fatalf("file changed: %q", b)
	}
}

func TestHostileFileValues(t *testing.T) {
	for name, body := range map[string]string{
		"repo":   `{"repos":[{"host":"","repo":"acme/app --flag"}]}`,
		"host":   `{"repos":[{"host":"x y","repo":"a/b"}]}`,
		"number": `{"projects":[{"owner":"acme","number":0,"title":"t"}]}`,
		"owner":  `{"projects":[{"owner":"../x","number":1,"title":"t"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "pins.json")
		os.WriteFile(path, []byte(body), 0o600)
		if _, err := Load(path); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	p := filepath.Join(t.TempDir(), "e.json")
	os.WriteFile(p, []byte("  \n"), 0o600)
	if s, err := Load(p); err != nil || len(s.Repos) != 0 {
		t.Fatal(s, err)
	}
}

func TestDefaultPathEnv(t *testing.T) {
	t.Setenv("GH9S_PINS", "/x/y.json")
	if DefaultPath() != "/x/y.json" {
		t.Fatal(DefaultPath())
	}
	t.Setenv("GH9S_PINS", "")
	if !strings.HasSuffix(DefaultPath(), filepath.Join("gh-9s", "pins.json")) {
		t.Fatal(DefaultPath())
	}
}

func TestProjectURLMustBePlainHTTPS(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)", "http://github.com/orgs/a/projects/1", "https://u:p@github.com/x", "https://github.com/a b", "file:///etc/passwd"} {
		p := Project{Owner: "acme", Number: 1, URL: bad}
		if err := p.validate(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := (Project{Owner: "acme", Number: 1, URL: "https://github.com/orgs/acme/projects/1"}).validate(); err != nil {
		t.Error(err)
	}
}
