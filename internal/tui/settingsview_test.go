package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/settings"
)

func settingsModel(t *testing.T, content string, env map[string]string) (model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := settings.Load(path, func(k string) string { return env[k] }, "")
	m := model{view: viewRuns, width: 120, height: 30, settings: s}
	return m, path
}

func openSettings(m model) model { return keyAll(cmdLine(m, "settings"), "enter") }

func cmdLine(m model, cmd string) model {
	m = keyAll(m, ":")
	return typeAll(m, cmd)
}

func fileText(path string) string { b, _ := os.ReadFile(path); return string(b) }

func TestSettingsListsEverySettingWithValueAndSource(t *testing.T) {
	m, _ := settingsModel(t, `{"preview": false}`, map[string]string{"GH9S_ICONS": "nerd"})
	m = openSettings(m)
	v := ansi.Strip(m.View())
	for _, want := range []string{"Icons", "Preview pane", "Preview width", "Preview minimum columns", "Delete branch after merge", "nerd", "off", "40", "120"} {
		if !strings.Contains(v, want) {
			t.Errorf("settings lacks %q:\n%s", want, v)
		}
	}
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "Icons") && !strings.Contains(l, "env") {
			t.Errorf("an environment value is tagged: %q", l)
		}
		if strings.Contains(l, "Preview pane") && !strings.Contains(l, "file") {
			t.Errorf("a file value is tagged: %q", l)
		}
	}
	if m.view != viewSettings {
		t.Errorf("view = %v", m.view)
	}
}

func TestEnterCyclesAndSavesEachChange(t *testing.T) {
	m, path := settingsModel(t, "", nil)
	m = openSettings(m)
	m = keyAll(m, "enter") // icons: ask -> nerd
	if !strings.Contains(fileText(path), `"icons": "nerd"`) || !strings.Contains(m.status, "icons = nerd") {
		t.Errorf("file=%s status=%q", fileText(path), m.status)
	}
	m = keyAll(m, "enter")
	if !strings.Contains(fileText(path), `"plain"`) {
		t.Errorf("file=%s", fileText(path))
	}
	m = keyAll(m, "left")
	if !strings.Contains(fileText(path), `"nerd"`) {
		t.Errorf("left cycles back: %s", fileText(path))
	}
	m = keyAll(m, "j", "space") // preview on -> off
	if !strings.Contains(fileText(path), `"preview": false`) {
		t.Errorf("a bool toggles: %s", fileText(path))
	}
	m = keyAll(m, "j", "enter", "enter") // preview_width 40 -> 45 -> 50
	if !strings.Contains(fileText(path), `"preview_width": 50`) {
		t.Errorf("an int steps: %s", fileText(path))
	}
	for i := 0; i < 6; i++ {
		m = keyAll(m, "right")
	}
	if !strings.Contains(fileText(path), `"preview_width": 60`) {
		t.Errorf("an int stops at its maximum: %s", fileText(path))
	}
}

func TestChangingAnOverriddenSettingSaysItHasNoEffect(t *testing.T) {
	m, path := settingsModel(t, "", map[string]string{"GH9S_ICONS": "nerd"})
	m = openSettings(m)
	m = keyAll(m, "enter")
	if !strings.Contains(m.status, "GH9S_ICONS") || !strings.Contains(m.status, "no effect") {
		t.Errorf("status = %q", m.status)
	}
	if !strings.Contains(fileText(path), `"icons"`) {
		t.Errorf("the change is still saved: %s", fileText(path))
	}
}

func TestABrokenFileShowsTheReasonAndChangesApplyButAreNotSaved(t *testing.T) {
	m, path := settingsModel(t, `{"icons": `, nil)
	m = openSettings(m)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "not valid JSON") {
		t.Errorf("the reason is at the top:\n%s", v)
	}
	m = keyAll(m, "j", "space")
	if !strings.Contains(m.status, "not saved") {
		t.Errorf("status = %q", m.status)
	}
	if m.cfg().Bool("preview") {
		t.Error("the change applies for this run")
	}
	if fileText(path) != `{"icons": ` {
		t.Errorf("the broken file is left alone: %q", fileText(path))
	}
}

func TestSetCommand(t *testing.T) {
	m, path := settingsModel(t, "", nil)
	m = keyAll(cmdLine(m, "set preview off"), "enter")
	if m.cfg().Bool("preview") || !strings.Contains(fileText(path), `"preview": false`) || !strings.Contains(m.status, "preview = off") {
		t.Errorf("status=%q file=%s", m.status, fileText(path))
	}
	m = keyAll(cmdLine(m, "set preview maybe"), "enter")
	if !strings.Contains(m.status, "on or off") || m.cfg().Bool("preview") {
		t.Errorf("status = %q; say what is allowed and change nothing", m.status)
	}
	m = keyAll(cmdLine(m, "set nope 1"), "enter")
	if !strings.Contains(m.status, "unknown setting") {
		t.Errorf("status = %q", m.status)
	}
	m = keyAll(cmdLine(m, "set icons"), "enter")
	if !strings.Contains(m.status, "icons = ask") {
		t.Errorf("one word shows the value: %q", m.status)
	}
	m = keyAll(cmdLine(m, "set preview_width  55  "), "enter")
	if m.cfg().Int("preview_width") != 55 {
		t.Errorf("extra spaces are fine: %d", m.cfg().Int("preview_width"))
	}
	m = keyAll(cmdLine(m, "set"), "enter")
	if !strings.Contains(m.status, "usage") {
		t.Errorf("status = %q", m.status)
	}
	m = keyAll(cmdLine(m, "set preview on extra"), "enter")
	if !strings.Contains(m.status, "usage") && !strings.Contains(m.status, "on or off") {
		t.Errorf("status = %q; extra words are not silently dropped", m.status)
	}
}

func TestSetCompletesKeysAndValues(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	if got := m.completions("set pre"); len(got) != 3 || got[0] != "set preview" {
		t.Errorf("keys: %q", got)
	}
	if got := m.completions("set icons n"); len(got) != 1 || got[0] != "set icons nerd" {
		t.Errorf("enum values: %q", got)
	}
	if got := m.completions("set preview "); len(got) != 2 || got[0] != "set preview on" {
		t.Errorf("bool values: %q", got)
	}
	if got := m.completions("sett"); len(got) != 1 || got[0] != "settings" {
		t.Errorf("the command: %q", got)
	}
}

func TestEscLeavesSettings(t *testing.T) {
	m, _ := settingsModel(t, "", nil)
	m = openSettings(m)
	m = keyAll(m, "esc")
	if m.view == viewSettings {
		t.Error("esc should leave")
	}
}

func TestSettingsScrollAndFitANarrowTerminal(t *testing.T) {
	old := settings.Registry
	t.Cleanup(func() { settings.Registry = old })
	for i := 0; i < 100; i++ {
		settings.Registry = append(settings.Registry, settings.Def{Key: "extra_" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Label: "Extra setting number " + strings.Repeat("x", i%30), Desc: "an extra", Kind: settings.Int, Default: 5, Min: 1, Max: 9})
	}
	m, _ := settingsModel(t, "", nil)
	m.width, m.height = 40, 14
	m = openSettings(m)
	for i := 0; i < 60; i++ {
		m = keys(m, "j")
	}
	body := strings.Split(ansi.Strip(m.View()), "\n")
	if len(body) > 14+2 {
		t.Errorf("the screen grew to %d lines on a 14-line terminal", len(body))
	}
	for _, l := range body[2 : len(body)-3] {
		if ansi.StringWidth(l) > 40 {
			t.Errorf("a %d-wide row on a 40-column terminal: %q", ansi.StringWidth(l), l)
		}
	}
	if !strings.Contains(strings.Join(body, "\n"), "▸") {
		t.Error("the selected row must stay on screen after scrolling")
	}
	m = keys(m, "G")
	if !strings.Contains(ansi.Strip(m.View()), "▸") {
		t.Error("G keeps the selection visible")
	}
}
