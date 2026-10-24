package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/settings"
)

func settingsFileOf(m model) string { return m.settings.Path() }

// useSettingsFiles points the loader at temp files for the test.
func useSettingsFiles(t *testing.T, content, legacy string) (settingsFile string) {
	t.Helper()
	dir := t.TempDir()
	settingsFile = filepath.Join(dir, "settings.json")
	legacyFile := filepath.Join(dir, "prefs.json")
	if content != "" {
		if err := os.WriteFile(settingsFile, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if legacy != "" {
		if err := os.WriteFile(legacyFile, []byte(legacy), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldP, oldL, oldE := settingsPath, legacyPrefs, envGet
	settingsPath, legacyPrefs, envGet = func() string { return settingsFile }, func() string { return legacyFile }, func(string) string { return "" }
	t.Cleanup(func() { settingsPath, legacyPrefs, envGet = oldP, oldL, oldE })
	return settingsFile
}

func TestLoadSettingsWordsProblemsForTheBanner(t *testing.T) {
	useSettingsFiles(t, `{"icons": `, "")
	s, banner := loadSettings()
	if s == nil || s.ReadOnly() == nil || !strings.Contains(banner, "not valid JSON") || !strings.Contains(banner, "not saved") {
		t.Errorf("store=%v banner=%q; a broken file is a banner, not a crash", s, banner)
	}
	useSettingsFiles(t, `{"preview_width": 5}`, "")
	if _, banner := loadSettings(); !strings.Contains(banner, "preview_width") {
		t.Errorf("banner = %q; an invalid value is named", banner)
	}
	useSettingsFiles(t, `{"preview": false}`, "")
	if _, banner := loadSettings(); banner != "" {
		t.Errorf("a good file gives no banner: %q", banner)
	}
}

func TestAnOldPrefsFileIsCarriedOver(t *testing.T) {
	useSettingsFiles(t, "", `{"delete_branch_after_merge": true}`)
	s, banner := loadSettings()
	if !s.Bool("delete_branch_after_merge") || banner != "" {
		t.Errorf("delete=%v banner=%q", s.Bool("delete_branch_after_merge"), banner)
	}
}

func TestMergingStillWorksWhenTheSettingsFileIsBroken(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	path := useSettingsFiles(t, `{nope`, "")
	m := mergeStart(t, f)
	m.settings, _ = settings.Load(path, nil, "")
	m = keyAll(m, "m", "down", "enter")
	m = keyAll(m, "d", "y")
	if len(f.calls) != 1 || !f.calls[0].del {
		t.Fatalf("calls = %+v; the merge goes ahead with the choice made", f.calls)
	}
	if !strings.Contains(m.status, "remember") {
		t.Errorf("status = %q; say the choice could not be kept", m.status)
	}
	if b, _ := os.ReadFile(path); string(b) != `{nope` {
		t.Errorf("the broken file must be left alone: %q", b)
	}
}

func TestMergeNeverWritesPrefsJSONAnyMore(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	useSettingsFiles(t, "", `{"delete_branch_after_merge": false}`)
	m := mergeStart(t, f)
	m = keyAll(m, "m", "down", "enter", "d", "y")
	b, _ := os.ReadFile(legacyPrefs())
	if !strings.Contains(string(b), "false") {
		t.Errorf("prefs.json = %q; it is left as it was", b)
	}
	_ = ghcli.RepoRef{}
}

func TestAModelWithoutSettingsUsesInMemoryDefaults(t *testing.T) {
	m := model{}
	if m.cfg().Bool("delete_branch_after_merge") || !m.cfg().Bool("preview") {
		t.Error("defaults")
	}
}
