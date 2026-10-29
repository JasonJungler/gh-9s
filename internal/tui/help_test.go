package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func helpModel(v viewKind) model {
	return model{view: v, width: 120, height: 40, target: acme}
}

func TestQuestionMarkOpensAndClosesTheHelp(t *testing.T) {
	m := helpModel(viewRuns)
	m = keys(m, "?")
	if !m.helpOpen {
		t.Fatal("? opens the help")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Runs", "re-run", "Everywhere", "this help"} {
		if !strings.Contains(v, want) {
			t.Errorf("help lacks %q:\n%s", want, v)
		}
	}
	m = keys(m, "x") // other keys do nothing while it is open
	if !m.helpOpen {
		t.Error("an unrelated key must not close it")
	}
	m = keys(m, "esc")
	if m.helpOpen || m.view != viewRuns {
		t.Errorf("esc closes the help and stays on the screen (open=%v view=%v)", m.helpOpen, m.view)
	}
	m = keys(keys(m, "?"), "?")
	if m.helpOpen {
		t.Error("? closes it again")
	}
}

func TestQuestionMarkIsTextInAPrompt(t *testing.T) {
	m := helpModel(viewRuns)
	m = keyAll(m, "/")
	m = typeAll(m, "a?b")
	if m.helpOpen || m.input != "a?b" {
		t.Errorf("in the filter ? is a character (open=%v input=%q)", m.helpOpen, m.input)
	}
}

func TestHelpFitsAndScrolls(t *testing.T) {
	m := helpModel(viewDiff)
	m.height = 14
	m = keys(m, "?")
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) > m.height {
		t.Errorf("the help is %d lines on a %d line screen", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > m.width {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
	before := ansi.Strip(m.View())
	m = keys(m, "G")
	after := ansi.Strip(m.View())
	if before == after || !strings.Contains(after, "this help") {
		t.Error("G scrolls to the end, where the keys of every screen are")
	}
}

// Every key the footer of a screen mentions is explained in its help.
func TestHelpExplainsEveryKeyOfTheFooters(t *testing.T) {
	views := []viewKind{viewRuns, viewWorkflows, viewPRs, viewIssues, viewInbox, viewPRDetail, viewIssue, viewDiff, viewChecks, viewJobs, viewGraph, viewLogs, viewFavs, viewSettings}
	for _, v := range views {
		m := helpModel(v)
		help := ""
		for _, g := range m.helpGroups() {
			for _, r := range g.rows {
				help += " " + r.keys + " "
			}
		}
		footer := ansi.Strip(m.footer())
		footer = footer[strings.Index(footer, "\n")+1:]
		for _, item := range strings.Split(footer, "  ") {
			spec := strings.Fields(strings.TrimSpace(item))
			if len(spec) == 0 {
				continue
			}
			for _, key := range splitKeySpec(strings.TrimSuffix(spec[0], ":")) {
				key = strings.Replace(key, "ctrl-", "ctrl+", 1)
				if key == "refresh" || key == "quit" {
					continue
				}
				if !strings.Contains(help, key) {
					t.Errorf("%v: the footer mentions %q (%q) but the help does not", v, key, item)
				}
			}
		}
	}
}

// splitKeySpec turns "c/C" into c and C, and leaves "/" and "]/[" sensible.
func splitKeySpec(s string) []string {
	if s == "/" {
		return []string{"/"}
	}
	switch s {
	case "tab/h/l":
		return []string{"tab", "h", "l"}
	case "hjkl/arrows":
		return []string{"h", "j", "k", "l"}
	}
	return strings.Split(s, "/")
}
