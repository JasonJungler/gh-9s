package drafts

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func tmp(t *testing.T) string { return filepath.Join(t.TempDir(), "gh-9s", "drafts.json") }

var one = []Draft{
	{Path: "main.go", Line: 12, Side: "RIGHT", Body: "why?"},
	{Path: "a b/c.go", Line: 9, StartLine: 7, Side: "LEFT", Body: "range\nwith \"quotes\" 😀"},
}

func TestRoundTrip(t *testing.T) {
	path := tmp(t)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if head, got := s.Get("github.com/acme/api#7"); head != "" || got != nil {
		t.Errorf("an empty store has nothing: %q %v", head, got)
	}
	if err := s.Put("github.com/acme/api#7", "abc1234", one); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	head, got := s2.Get("github.com/acme/api#7")
	if head != "abc1234" || !reflect.DeepEqual(got, one) {
		t.Errorf("after a restart: %q %+v", head, got)
	}
	if _, other := s2.Get("github.com/acme/api#8"); other != nil {
		t.Error("another PR has its own list")
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("the file is private: %v %v", err, st)
	}
}

func TestPutEmptyOrDeleteRemovesTheEntry(t *testing.T) {
	path := tmp(t)
	s, _ := Load(path)
	_ = s.Put("github.com/acme/api#7", "abc1234", one)
	_ = s.Put("github.com/acme/api#8", "abc1234", one[:1])
	if err := s.Put("github.com/acme/api#7", "abc1234", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("github.com/acme/api#8"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	s2, _ := Load(path)
	if _, g := s2.Get("github.com/acme/api#7"); g != nil {
		t.Errorf("empty list left behind: %s", b)
	}
	if _, g := s2.Get("github.com/acme/api#8"); g != nil {
		t.Errorf("deleted entry left behind: %s", b)
	}
}

func TestLoadRefusesCorruptOrHostileFilesAndLeavesThemAlone(t *testing.T) {
	good := `{"github.com/acme/api#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"side":"RIGHT","body":"x"}]}}`
	cases := map[string]string{
		"not json":         `{nope`,
		"bad key":          `{"../etc#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"side":"RIGHT","body":"x"}]}}`,
		"key with newline": "{\"github.com/a/b#7\\nx\":{\"head\":\"abc\",\"drafts\":[]}}",
		"bad head":         `{"github.com/a/b#7":{"head":"; rm -rf","drafts":[{"path":"a.go","line":3,"side":"RIGHT","body":"x"}]}}`,
		"empty body":       `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"side":"RIGHT","body":""}]}}`,
		"line zero":        `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"a.go","line":0,"side":"RIGHT","body":"x"}]}}`,
		"bad side":         `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"side":"MIDDLE","body":"x"}]}}`,
		"range backwards":  `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"start_line":5,"side":"RIGHT","body":"x"}]}}`,
		"empty path":       `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"","line":3,"side":"RIGHT","body":"x"}]}}`,
		"huge body":        `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"side":"RIGHT","body":"` + strings.Repeat("x", 70000) + `"}]}}`,
		"escape in body":   `{"github.com/a/b#7":{"head":"abc1234","drafts":[{"path":"a.go","line":3,"side":"RIGHT","body":"hi \u001b[2J"}]}}`,
	}
	for name, content := range cases {
		path := tmp(t)
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if b, _ := os.ReadFile(path); string(b) != content {
			t.Errorf("%s: the file must be left untouched", name)
		}
	}
	path := tmp(t)
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(good), 0o600)
	if _, err := Load(path); err != nil {
		t.Errorf("a good file loads: %v", err)
	}
}

func TestPutRefusesInvalidDraftsAndKeys(t *testing.T) {
	s, _ := Load(tmp(t))
	if err := s.Put("../x#1", "abc1234", one); err == nil {
		t.Error("bad key")
	}
	if err := s.Put("github.com/a/b#1", "abc1234", []Draft{{Path: "a.go", Line: 0, Side: "RIGHT", Body: "x"}}); err == nil {
		t.Error("bad draft")
	}
}

func TestNilStoreIsAnEmptyInMemoryStore(t *testing.T) {
	var s *Store
	if _, g := s.Get("github.com/a/b#1"); g != nil {
		t.Error("nil store has nothing")
	}
	if err := s.Put("github.com/a/b#1", "abc1234", one); err != nil {
		t.Errorf("a nil store accepts and forgets: %v", err)
	}
}

func TestKey(t *testing.T) {
	if got := Key("github.com", "acme", "api", "482"); got != "github.com/acme/api#482" {
		t.Errorf("Key = %q", got)
	}
	if got := Key("", "acme", "api", "482"); got != "github.com/acme/api#482" {
		t.Errorf("an empty host means github.com: %q", got)
	}
}
