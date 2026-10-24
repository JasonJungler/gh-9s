package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/settings"
)

// startAs builds the model the way Run does (settings loaded, icons decided) with a terminal.
func startAs(t *testing.T, content string, env map[string]string) (model, string) {
	t.Helper()
	old, oldTerm := icons.Current(), isTerminal
	isTerminal = func() bool { return true }
	t.Cleanup(func() { icons.Use(old); isTerminal = oldTerm })
	icons.Use(icons.Plain)
	m, path := settingsModel(t, content, env)
	m.width, m.height = 100, 30
	return m.startIcons(), path
}

func TestFirstRunAsksBeforeAnythingElse(t *testing.T) {
	m, _ := startAs(t, "", nil)
	if !m.askIcons {
		t.Fatal("icons=ask with a usable file and a terminal: ask")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "see") || !strings.Contains(v, "y") {
		t.Errorf("the question:\n%s", v)
	}
	for _, n := range icons.Sample {
		if !strings.Contains(v, icons.GetIn(icons.Nerd, n)) {
			t.Errorf("the sample row lacks %q:\n%s", icons.GetIn(icons.Nerd, n), v)
		}
	}
	if icons.Current() != icons.Plain {
		t.Error("until answered the plain set is used")
	}
}

func TestYSavesNerdAndAppliesIt(t *testing.T) {
	m, path := startAs(t, "", nil)
	next, _ := m.Update(k("y"))
	m = next.(model)
	if m.askIcons || icons.Current() != icons.Nerd || !strings.Contains(fileText(path), `"icons": "nerd"`) {
		t.Errorf("ask=%v mode=%v file=%s", m.askIcons, icons.Current(), fileText(path))
	}
	m.view = viewRuns
	if v := m.View(); strings.Contains(v, "see icons") {
		t.Errorf("the question is gone:\n%s", v)
	}
}

func TestNSavesPlain(t *testing.T) {
	m, path := startAs(t, "", nil)
	next, _ := m.Update(k("n"))
	m = next.(model)
	if m.askIcons || icons.Current() != icons.Plain || !strings.Contains(fileText(path), `"icons": "plain"`) {
		t.Errorf("ask=%v mode=%v file=%s", m.askIcons, icons.Current(), fileText(path))
	}
}

func TestQuestionMarkHelpsAndOtherKeysDoNothing(t *testing.T) {
	m, path := startAs(t, "", nil)
	next, _ := m.Update(k("?"))
	m = next.(model)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "nerdfonts.com") {
		t.Errorf("help should say where to get a Nerd Font:\n%s", v)
	}
	for _, key := range []string{"x", "enter", "esc", "1", "q", ":"} {
		next, _ = m.Update(k(key))
		m = next.(model)
	}
	if !m.askIcons || fileText(path) != "" {
		t.Errorf("ask=%v file=%q; nothing but y, n, ? and ctrl+c answers", m.askIcons, fileText(path))
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl+c must quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c must quit")
	}
}

func TestNothingIsAskedWhenTheAnswerIsAlreadyKnown(t *testing.T) {
	cases := map[string]struct {
		content string
		env     map[string]string
		want    icons.Mode
	}{
		"file nerd":      {`{"icons":"nerd"}`, nil, icons.Nerd},
		"file plain":     {`{"icons":"plain"}`, nil, icons.Plain},
		"env nerd":       {"", map[string]string{"GH9S_ICONS": "nerd"}, icons.Nerd},
		"env beats file": {`{"icons":"plain"}`, map[string]string{"GH9S_ICONS": "nerd"}, icons.Nerd},
	}
	for name, c := range cases {
		m, _ := startAs(t, c.content, c.env)
		if m.askIcons || icons.Current() != c.want {
			t.Errorf("%s: ask=%v mode=%v", name, m.askIcons, icons.Current())
		}
	}
}

func TestABrokenFileOrNoTerminalMeansPlainAndNoQuestion(t *testing.T) {
	m, path := startAs(t, `{"icons": `, nil)
	if m.askIcons || icons.Current() != icons.Plain || fileText(path) != `{"icons": ` {
		t.Errorf("ask=%v mode=%v file=%q", m.askIcons, icons.Current(), fileText(path))
	}
	old := isTerminal
	isTerminal = func() bool { return false }
	t.Cleanup(func() { isTerminal = old })
	m2, _ := settingsModel(t, "", nil)
	if m2 = m2.startIcons(); m2.askIcons {
		t.Error("without a terminal nobody can answer")
	}
}

func TestAnswerWhileTheFileIsReadOnlyStillApplies(t *testing.T) {
	m, path := startAs(t, "", nil)
	// the file breaks after the question was shown
	if err := os.WriteFile(path, []byte(`{nope`), 0o600); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(k("y"))
	m = next.(model)
	if icons.Current() != icons.Nerd || !strings.Contains(m.status, "not saved") && !strings.Contains(m.status, "couldn't") {
		t.Errorf("mode=%v status=%q; applied for this run and said it was not saved", icons.Current(), m.status)
	}
	if fileText(path) != `{nope` {
		t.Errorf("the broken file is left alone: %q", fileText(path))
	}
}

func TestIconsCommandAsksAgainUnlessTheEnvironmentDecides(t *testing.T) {
	m, _ := startAs(t, `{"icons":"nerd"}`, nil)
	m = keyAll(cmdLine(m, "icons"), "enter")
	if !m.askIcons {
		t.Error(":icons should ask again")
	}
	e, _ := startAs(t, "", map[string]string{"GH9S_ICONS": "plain"})
	e = keyAll(cmdLine(e, "icons"), "enter")
	if e.askIcons || !strings.Contains(e.status, "GH9S_ICONS") {
		t.Errorf("ask=%v status=%q", e.askIcons, e.status)
	}
}

func TestChangingIconsInSettingsSwitchesTheSet(t *testing.T) {
	m, _ := startAs(t, `{"icons":"plain"}`, nil)
	m = keyAll(cmdLine(m, "set icons nerd"), "enter")
	if icons.Current() != icons.Nerd {
		t.Errorf("mode = %v after :set icons nerd", icons.Current())
	}
	m = keyAll(cmdLine(m, "settings"), "enter")
	m = keyAll(m, "enter") // nerd -> plain
	if icons.Current() != icons.Plain {
		t.Errorf("mode = %v after cycling in :settings", icons.Current())
	}
	m = keyAll(m, "enter") // plain -> ask: back to plain symbols until answered
	if icons.Current() != icons.Plain {
		t.Errorf("mode = %v for ask", icons.Current())
	}
}

func TestTheInboxRowsFollowAnIconSwitch(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	m, _ := startAs(t, `{"icons":"plain"}`, nil)
	m = keyAll(m, "0")
	if v := ansi.Strip(m.View()); strings.Contains(v, icons.GetIn(icons.Nerd, icons.IssueOpen)+" #1") {
		t.Fatalf("setup: plain first:\n%s", v)
	}
	m = keyAll(cmdLine(m, "set icons nerd"), "enter")
	if m.view != viewInbox {
		m = keyAll(m, "0")
	}
	m = pump(m, m.fetch())
	if v := ansi.Strip(m.View()); !strings.Contains(v, icons.GetIn(icons.Nerd, icons.IssueOpen)+" #1") {
		t.Errorf("rows built before the switch must change with it:\n%s", v)
	}
}

func TestTheQuestionFitsASmallTerminalAndAResize(t *testing.T) {
	m, _ := startAs(t, "", nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m = next.(model)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) > 8 {
		t.Errorf("%d lines on an 8-line terminal", len(lines))
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 20 {
			t.Errorf("a %d-wide line on a 20-column terminal: %q", ansi.StringWidth(l), l)
		}
	}
}

func TestAnsweringTwiceSavesOnce(t *testing.T) {
	m, path := startAs(t, "", nil)
	next, _ := m.Update(k("y"))
	next, _ = next.(model).Update(k("n")) // a second key after the answer is just a key on the first screen
	m = next.(model)
	if !strings.Contains(fileText(path), `"nerd"`) || strings.Contains(fileText(path), `"plain"`) {
		t.Errorf("file = %s", fileText(path))
	}
	_ = filepath.Join
	_ = settings.Registry
}
