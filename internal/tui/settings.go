package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/settings"
)

// Where the settings live; tests replace them so no test touches the real configuration.
var (
	settingsPath = defaultSettingsPath
	legacyPrefs  = func() string { return filepath.Join(configDir(), "prefs.json") }
	envGet       = os.Getenv
)

func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "gh-9s")
}

func defaultSettingsPath() string {
	if p := os.Getenv("GH9S_SETTINGS"); p != "" {
		return p
	}
	if d := configDir(); d != "" {
		return filepath.Join(d, "settings.json")
	}
	return ""
}

// loadSettings reads the settings and words any problem for the banner.
func loadSettings() (*settings.Store, string) {
	legacy := ""
	if d := configDir(); d != "" {
		legacy = legacyPrefs()
	}
	s, err := settings.Load(settingsPath(), envGet, legacy)
	var problems []string
	if err != nil {
		problems = append(problems, "settings are not saved: "+err.Error())
	}
	problems = append(problems, s.Notes()...)
	return s, strings.Join(problems, "; ")
}

// cfg is the settings in use; a model built without any (tests, a failed start) gets the in-memory ones.
func (m model) cfg() *settings.Store {
	if m.settings == nil {
		return settings.Memory()
	}
	return m.settings
}
