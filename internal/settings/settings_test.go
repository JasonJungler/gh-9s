package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }

func tmpPath(t *testing.T) string { return filepath.Join(t.TempDir(), "gh-9s", "settings.json") }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, path string, vars map[string]string) *Store {
	t.Helper()
	s, err := Load(path, env(vars), "")
	if s == nil {
		t.Fatalf("Load returned no store (%v)", err)
	}
	return s
}

func TestDefaults(t *testing.T) {
	s := load(t, tmpPath(t), nil)
	if s.String("icons") != "ask" || !s.Bool("preview") || s.Int("preview_width") != 40 || s.Int("preview_min_columns") != 120 || s.Bool("delete_branch_after_merge") {
		t.Errorf("defaults wrong: icons=%q preview=%v width=%d min=%d", s.String("icons"), s.Bool("preview"), s.Int("preview_width"), s.Int("preview_min_columns"))
	}
	if s.Source("icons") != FromDefault || s.ReadOnly() != nil || len(s.Notes()) != 0 {
		t.Errorf("source=%v readOnly=%v notes=%v", s.Source("icons"), s.ReadOnly(), s.Notes())
	}
	if s.String("nope") != "" || s.Int("nope") != 0 || s.Bool("nope") {
		t.Error("unknown keys read as zero values")
	}
}

func TestFileEnvAndSources(t *testing.T) {
	path := tmpPath(t)
	write(t, path, `{"icons":"plain","preview":false,"preview_width":55,"extra_key":{"keep":"me"}}`)
	s := load(t, path, nil)
	if s.String("icons") != "plain" || s.Bool("preview") || s.Int("preview_width") != 55 || s.Source("icons") != FromFile {
		t.Errorf("file values not read: %q %v %d", s.String("icons"), s.Bool("preview"), s.Int("preview_width"))
	}
	s = load(t, path, map[string]string{"GH9S_ICONS": "nerd"})
	if s.String("icons") != "nerd" || s.Source("icons") != FromEnv {
		t.Errorf("env must win: %q %v", s.String("icons"), s.Source("icons"))
	}
}

func TestBadEnvAndBadFileValuesFallBackWithANote(t *testing.T) {
	path := tmpPath(t)
	write(t, path, `{"preview":"yes","preview_width":999,"preview_min_columns":1.5,"icons":"banana","delete_branch_after_merge":true}`)
	s := load(t, path, map[string]string{"GH9S_ICONS": "banana"})
	if s.String("icons") != "ask" || !s.Bool("preview") || s.Int("preview_width") != 40 || s.Int("preview_min_columns") != 120 {
		t.Errorf("bad values must fall back to defaults")
	}
	if !s.Bool("delete_branch_after_merge") {
		t.Error("a good value next to bad ones is kept")
	}
	if len(s.Notes()) < 5 {
		t.Errorf("each problem is reported once: %q", s.Notes())
	}
	for _, n := range s.Notes() {
		if strings.ContainsAny(n, "\x1b\n") {
			t.Errorf("notes are one clean line: %q", n)
		}
	}
	if s2 := load(t, tmpPath(t), map[string]string{"GH9S_ICONS": "ask"}); s2.String("icons") != "ask" || s2.Source("icons") == FromEnv {
		t.Error("'ask' is not allowed from the environment")
	}
}

func TestSetValidatesAndSavesAndKeepsUnknownKeys(t *testing.T) {
	path := tmpPath(t)
	write(t, path, `{"extra_key":{"keep":"me"},"icons":"plain"}`)
	s := load(t, path, nil)
	cases := []struct {
		key, in, want string
		bad           bool
	}{
		{"preview", "OFF", "off", false}, {"preview", " yes ", "on", false}, {"preview", "0", "off", false}, {"preview", "maybe", "", true},
		{"icons", "NERD", "nerd", false}, {"icons", "ask", "ask", false}, {"icons", "emoji", "", true},
		{"preview_width", " 50 ", "50", false}, {"preview_width", "10", "", true}, {"preview_width", "61", "", true}, {"preview_width", "forty", "", true}, {"preview_width", "99999999999999999999", "", true},
		{"nope", "1", "", true}, {"", "1", "", true},
	}
	for _, c := range cases {
		got, err := s.Set(c.key, c.in)
		if c.bad != (err != nil) || (!c.bad && got != c.want) {
			t.Errorf("Set(%q,%q) = %q, %v; want %q bad=%v", c.key, c.in, got, err, c.want, c.bad)
		}
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"extra_key"`) || !strings.Contains(string(b), `"keep"`) {
		t.Errorf("unknown keys must survive a save: %s", b)
	}
	again := load(t, path, nil)
	if again.String("icons") != "ask" || again.Int("preview_width") != 50 || again.Bool("preview") != false { // the last accepted value was "0"
		t.Errorf("saved values come back: %q %d %v", again.String("icons"), again.Int("preview_width"), again.Bool("preview"))
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("file mode: %v %v", err, st)
	}
	dst, _ := os.Stat(filepath.Dir(path))
	if dst.Mode().Perm() != 0o700 {
		t.Errorf("dir mode: %v", dst.Mode())
	}
}

func TestInvalidJSONIsReadOnlyAndLeftAlone(t *testing.T) {
	path := tmpPath(t)
	write(t, path, `{"icons": nerd`)
	s, err := Load(path, env(nil), "")
	if s == nil || err == nil || s.ReadOnly() == nil {
		t.Fatalf("store=%v err=%v; a broken file gives a usable read-only store and says why", s, err)
	}
	got, serr := s.Set("preview", "off")
	if got != "off" || !errors.Is(serr, ErrNotSaved) || s.Bool("preview") {
		t.Errorf("Set = %q, %v preview=%v; applied in memory, not saved", got, serr, s.Bool("preview"))
	}
	if b, _ := os.ReadFile(path); string(b) != `{"icons": nerd` {
		t.Errorf("the broken file must be left untouched: %q", b)
	}
}

func TestOverriddenSettingsAreSavedButMarked(t *testing.T) {
	path := tmpPath(t)
	s := load(t, path, map[string]string{"GH9S_ICONS": "nerd"})
	if _, err := s.Set("icons", "plain"); err != nil {
		t.Fatal(err)
	}
	if !s.Overridden("icons") || s.String("icons") != "nerd" {
		t.Errorf("overridden=%v value=%q; the environment keeps winning", s.Overridden("icons"), s.String("icons"))
	}
	if again := load(t, path, nil); again.String("icons") != "plain" {
		t.Errorf("the change is in the file: %q", again.String("icons"))
	}
}

func TestTwoInstancesDoNotEraseEachOther(t *testing.T) {
	path := tmpPath(t)
	a, b := load(t, path, nil), load(t, path, nil)
	if _, err := a.Set("preview", "off"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Set("preview_width", "30"); err != nil {
		t.Fatal(err)
	}
	c := load(t, path, nil)
	if c.Bool("preview") || c.Int("preview_width") != 30 {
		t.Errorf("preview=%v width=%d; the second save must keep the first one's key", c.Bool("preview"), c.Int("preview_width"))
	}
}

func TestLegacyPrefsAreCarriedOverOnce(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "prefs.json")
	write(t, legacy, `{"delete_branch_after_merge": true}`)
	path := filepath.Join(dir, "settings.json")
	s, _ := Load(path, env(nil), legacy)
	if !s.Bool("delete_branch_after_merge") {
		t.Error("the old choice is the default of the new setting")
	}
	if _, err := s.Set("preview", "off"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(legacy); !strings.Contains(string(b), "true") {
		t.Error("prefs.json is left alone")
	}
	s2, _ := Load(path, env(nil), legacy)
	write(t, legacy, `{"delete_branch_after_merge": false}`)
	if !s2.Bool("delete_branch_after_merge") {
		t.Error("once carried into settings.json the legacy file is no longer read")
	}
	own := filepath.Join(dir, "own.json")
	write(t, own, `{"delete_branch_after_merge": false}`)
	write(t, legacy, `{"delete_branch_after_merge": true}`)
	s3, _ := Load(own, env(nil), legacy)
	if s3.Bool("delete_branch_after_merge") {
		t.Error("a value already in settings.json beats the legacy file")
	}
}

func TestHostileFiles(t *testing.T) {
	big := tmpPath(t)
	write(t, big, `{"x":"`+strings.Repeat("a", 11<<20)+`"}`)
	if s, err := Load(big, env(nil), ""); s == nil || err == nil || s.ReadOnly() == nil || !strings.Contains(err.Error(), "large") {
		t.Errorf("a 10 MB+ file is refused: %v", err)
	}
	dir := tmpPath(t)
	if err := os.MkdirAll(dir, 0o700); err != nil { // a directory where the file should be
		t.Fatal(err)
	}
	s, err := Load(dir, env(nil), "")
	if s == nil || err == nil {
		t.Fatalf("a directory is not a settings file: %v", err)
	}
	if _, serr := s.Set("preview", "off"); !errors.Is(serr, ErrNotSaved) {
		t.Errorf("Set on it: %v", serr)
	}
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	s2 := load(t, filepath.Join(ro, "sub", "settings.json"), nil)
	if _, serr := s2.Set("preview", "off"); !errors.Is(serr, ErrNotSaved) || s2.Bool("preview") {
		t.Errorf("an unwritable directory: err=%v preview=%v", serr, s2.Bool("preview"))
	}
	if s3, err := Load("", env(nil), ""); s3 == nil || err == nil {
		t.Error("no path: a store that cannot save")
	}
}

func TestCycle(t *testing.T) {
	s := load(t, tmpPath(t), nil)
	for _, want := range []string{"nerd", "plain", "ask"} {
		got, err := s.Cycle("icons", 1)
		if err != nil || got != want {
			t.Fatalf("Cycle icons = %q, %v; want %q", got, err, want)
		}
	}
	if got, _ := s.Cycle("icons", -1); got != "plain" {
		t.Errorf("backwards from ask: %q", got)
	}
	if got, _ := s.Cycle("preview", 1); got != "off" {
		t.Errorf("toggle: %q", got)
	}
	if got, _ := s.Cycle("preview_width", 1); got != "45" {
		t.Errorf("a wide range steps by 5: %q", got)
	}
	for i := 0; i < 10; i++ {
		_, _ = s.Cycle("preview_width", 1)
	}
	if s.Int("preview_width") != 60 {
		t.Errorf("an int stops at its maximum: %d", s.Int("preview_width"))
	}
	if _, err := s.Cycle("nope", 1); err == nil {
		t.Error("unknown key")
	}
}

func TestRegistryLookup(t *testing.T) {
	if d, ok := Lookup("preview_width"); !ok || d.Min != 25 || d.Max != 60 || d.Default.(int) != 40 {
		t.Errorf("Lookup = %+v %v", d, ok)
	}
	if _, ok := Lookup("zzz"); ok {
		t.Error("unknown")
	}
	seen := map[string]bool{}
	for _, d := range Registry {
		if d.Key == "" || d.Label == "" || d.Desc == "" || seen[d.Key] {
			t.Errorf("registry entry %+v is incomplete or duplicated", d)
		}
		seen[d.Key] = true
	}
}
