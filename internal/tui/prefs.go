package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// prefs are small remembered choices, kept next to favorites.json.
type prefs struct {
	DeleteBranchAfterMerge bool `json:"delete_branch_after_merge"`
}

func prefsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "gh-9s", "prefs.json")
}

// loadPrefs reads the file; a missing, empty or unreadable file is the zero value and is never rewritten.
func loadPrefs(path string) prefs {
	var p prefs
	if path == "" {
		return p
	}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

// save writes the file atomically (temp file, then rename).
func (p prefs) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
