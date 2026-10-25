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

func TestSavingThroughASymlinkWritesTheRealFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "settings.json")
	write(t, real, `{"icons":"plain"}`)
	if err := os.Chmod(real, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here")
	}
	s := load(t, link, nil)
	if _, err := s.Set("preview", "off"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link must still be a link")
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), `"preview": false`) {
		t.Errorf("the target file was not updated: %s", b)
	}
	if fi, _ := os.Stat(real); fi.Mode().Perm() != 0o644 {
		t.Errorf("an existing file keeps its permissions, got %v", fi.Mode().Perm())
	}
}

func TestAnOlderChangeIsNotWrittenAgain(t *testing.T) {
	path := tmpPath(t)
	a, b := load(t, path, nil), load(t, path, nil)
	if _, err := a.Set("icons", "nerd"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Set("icons", "plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Set("preview", "off"); err != nil { // a saves another key: icons must stay what b wrote
		t.Fatal(err)
	}
	if c := load(t, path, nil); c.String("icons") != "plain" || c.Bool("preview") {
		t.Errorf("icons=%q preview=%v; a's old icons value must not come back", c.String("icons"), c.Bool("preview"))
	}
}

func TestNullAndQuotedNumbersAreNotValues(t *testing.T) {
	path := tmpPath(t)
	write(t, path, `{"preview": null, "preview_width": "40", "icons": null}`)
	s := load(t, path, nil)
	if !s.Bool("preview") || s.Int("preview_width") != 40 || s.String("icons") != "ask" {
		t.Error("defaults expected")
	}
	if len(s.Notes()) != 3 {
		t.Errorf("each one is reported: %q", s.Notes())
	}
	if s.Source("preview") != FromDefault || s.Source("preview_width") != FromDefault {
		t.Error("they are not 'from the file'")
	}
}

func TestKeysAreMatchedIgnoringCase(t *testing.T) {
	s := load(t, tmpPath(t), nil)
	if got, err := s.Set("PREVIEW_Width", "50"); err != nil || got != "50" || s.Int("preview_width") != 50 {
		t.Errorf("Set = %q %v width=%d", got, err, s.Int("preview_width"))
	}
	if d, ok := Lookup("Icons"); !ok || d.Key != "icons" {
		t.Errorf("Lookup = %+v %v", d, ok)
	}
	if _, err := s.Set("zzz", "1"); err == nil || strings.Count(err.Error(), `"`) != 2 {
		t.Errorf("an unknown key is quoted once: %v", err)
	}
}

func TestLaunchSettingsDefaults(t *testing.T) {
	want := []struct {
		key, def string
		options  []string
	}{
		{"launch_target", "auto", []string{"auto", "orca", "herdr", "tmux"}},
		{"launch_agent", "claude", []string{"claude", "devin"}},
		{"launch_tmux_place", "pane", []string{"pane", "window"}},
		{"launch_split", "horizontal", []string{"horizontal", "vertical"}},
	}
	for _, w := range want {
		d, ok := Lookup(w.key)
		if !ok {
			t.Errorf("%s is not in the registry", w.key)
			continue
		}
		if d.Kind != Enum {
			t.Errorf("%s kind = %v, want Enum", w.key, d.Kind)
		}
		if d.Default != w.def {
			t.Errorf("%s default = %v, want %s", w.key, d.Default, w.def)
		}
		if strings.Join(d.Options, ",") != strings.Join(w.options, ",") {
			t.Errorf("%s options = %v, want %v", w.key, d.Options, w.options)
		}
		if got := Memory().String(w.key); got != w.def {
			t.Errorf("Memory().String(%s) = %q, want %q", w.key, got, w.def)
		}
	}
	for _, key := range []string{"launch_tmux_place", "launch_split"} {
		if d, _ := Lookup(key); !strings.Contains(d.Desc, "herdr") {
			t.Errorf("%s Desc %q should say herdr uses it too", key, d.Desc)
		}
	}
}

func TestLaunchSubmitSettingDefaultsToOn(t *testing.T) {
	d, ok := Lookup("launch_submit")
	if !ok {
		t.Fatal("launch_submit is not in the registry")
	}
	if d.Kind != Bool || d.Default != true {
		t.Errorf("launch_submit = kind %v default %v, want a Bool that defaults to true", d.Kind, d.Default)
	}
	if !Memory().Bool("launch_submit") {
		t.Error("launch_submit is not on by default")
	}
	s := Memory()
	if _, err := s.Set("launch_submit", "off"); !errors.Is(err, ErrNotSaved) || s.Bool("launch_submit") { // a memory store has no file
		t.Errorf("off: err = %v, value = %v", err, s.Bool("launch_submit"))
	}
}

// A description is one line in the settings list; launch_submit's was far longer than the others, and the
// detail belongs in the docs.
func TestLaunchSubmitDescIsAsShortAsTheOthers(t *testing.T) {
	d, _ := Lookup("launch_submit")
	longest := 0
	for _, o := range Registry {
		if o.Key != "launch_submit" {
			longest = max(longest, len(o.Desc))
		}
	}
	if len(d.Desc) > longest {
		t.Errorf("launch_submit Desc is %d bytes, longer than any other (%d): %q", len(d.Desc), longest, d.Desc)
	}
}
