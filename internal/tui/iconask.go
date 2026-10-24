package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/MurabitoB/gh-9s/internal/icons"
)

// isTerminal says whether someone can answer a question; tests replace it.
var isTerminal = func() bool { return term.IsTerminal(os.Stdin.Fd()) }

// applyIcons makes the icon set in use match the `icons` setting ("ask" shows plain symbols until
// the question is answered).
func (m model) applyIcons() model {
	mode, ok := icons.Parse(m.cfg().String("icons"))
	if !ok {
		mode = icons.Plain
	}
	icons.Use(mode)
	return m
}

// startIcons decides at start whether to ask: only when the answer is not known yet, the settings
// can be saved, the environment has not decided, and someone is there to answer.
func (m model) startIcons() model {
	s := m.cfg()
	m = m.applyIcons()
	m.askIcons = s.String("icons") == "ask" && s.ReadOnly() == nil && !s.Overridden("icons") && isTerminal()
	return m
}

// afterIconsChanged redraws what was built with the old set.
func (m model) afterIconsChanged() (model, tea.Cmd) {
	m = m.applyIcons()
	if st, ok := m.screenData(viewInbox).(inboxState); ok && st.loaded && m.view == viewInbox { // rows are built once
		keep := ""
		if r, ok := m.selected(); ok {
			keep = r.id
		}
		m = m.rebuildInbox(st, keep)
	}
	return m, m.fetch()
}

// iconsCommand is ":icons": ask again.
func (m model) iconsCommand() (tea.Model, tea.Cmd) {
	if m.cfg().Overridden("icons") {
		m.status = styDim.Render("icons are set by GH9S_ICONS, which decides: nothing to ask")
		return m, nil
	}
	m.askIcons, m.iconHelp = true, false
	return m, nil
}

func (m model) iconAskKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y":
		return m.answerIcons("nerd")
	case "n", "N":
		return m.answerIcons("plain")
	case "?":
		m.iconHelp = !m.iconHelp
	}
	return m, nil
}

func (m model) answerIcons(value string) (tea.Model, tea.Cmd) {
	m.askIcons, m.iconHelp = false, false
	_, err := m.cfg().Set("icons", value)
	if err != nil {
		m.status = styErr.Render("icons = " + value + " for this run, but not saved: " + strings.TrimPrefix(err.Error(), "not saved: "))
	}
	return m.afterIconsChanged()
}

func (m model) iconAskView() string {
	w := max(m.width, 10)
	var glyphs []string
	for _, n := range icons.Sample {
		g := icons.GetIn(icons.Nerd, n)
		if c := icons.Color(n); c != "" {
			g = lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(g)
		}
		glyphs = append(glyphs, g)
	}
	var lines []string
	lines = append(lines, styKey.Render("Nerd Font icons"), "", " "+strings.Join(glyphs, "  "), "")
	lines = append(lines, strings.Split(ansi.Wrap("Can you see icons in the row above (not boxes or question marks)?", w-2, ""), "\n")...)
	lines = append(lines, "", styDim.Render("y  yes, use icons"), styDim.Render("n  no, use plain symbols"), styDim.Render("?  help"))
	if m.iconHelp {
		lines = append(lines, "")
		lines = append(lines, strings.Split(ansi.Wrap("Install a Nerd Font (https://www.nerdfonts.com) and set it as your terminal's font, then answer y.", w-2, ""), "\n")...)
	}
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}
