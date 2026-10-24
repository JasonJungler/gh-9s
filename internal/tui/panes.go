package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The PR list, the issue list and the inbox can show a pane on the right with the selected item, like
// gh-dash. The list is drawn by its own body function with a narrower model; this file only splits the
// screen and frames the two halves.

const (
	paneMinList = 50 // the list never gets narrower than this
	paneMinPane = 30
)

// previewShown says whether the current screen has a preview pane: the setting is on, the session has
// not switched it off, the screen is one of the lists and the terminal is wide enough.
func (m model) previewShown() bool {
	switch m.view {
	case viewPRs, viewIssues, viewInbox, viewBoard:
	default:
		return false
	}
	if m.view == viewBoard && !m.boardSt().loaded {
		return false // nothing to list yet, or an error to read in full
	}
	if m.previewOff || !m.cfg().Bool("preview") || m.width < m.cfg().Int("preview_min_columns") {
		return false
	}
	lw, _ := m.paneWidths()
	return lw > 0
}

// paneWidths splits the terminal width into the list column and the pane column (borders included);
// both are 0 when the list cannot keep paneMinList columns beside a pane of at least paneMinPane.
func (m model) paneWidths() (list, pane int) {
	pane = max(m.width*m.cfg().Int("preview_width")/100, paneMinPane)
	if m.width-pane < paneMinList {
		pane = m.width - paneMinList
	}
	if pane < paneMinPane {
		return 0, 0
	}
	return m.width - pane, pane
}

var paneBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

// framed puts s in a rounded box of exactly w columns and h rows (borders included); lines that are
// too long are cut, missing ones are blank.
func framed(s string, w, h int) string {
	iw, ih := max(w-2, 1), max(h-2, 1)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := make([]string, ih)
	for i := range out {
		l := ""
		if i < len(lines) {
			l = strings.ReplaceAll(lines[i], "\t", " ")
		}
		out[i] = fit(l, iw)
	}
	return paneBorder.Render(strings.Join(out, "\n"))
}

// joinPanes lays two boxes side by side: exactly lw+rw columns and h rows.
func joinPanes(left, right string, lw, rw, h int) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, framed(left, lw, h), framed(right, rw, h))
}

// withPane draws a list screen with the preview pane beside it. body is the screen's own body
// function; it is called with a model that is as narrow and as short as the list's box.
func (m model) withPane(body func(model) string) string {
	lw, rw := m.paneWidths()
	inner := m
	inner.width, inner.height, inner.paneList = lw-2, m.height-2, true
	list := strings.TrimSuffix(body(inner), "\n")
	h := strings.Count(list, "\n") + 3 // the list's lines plus the two border rows
	return joinPanes(list, m.previewContent(rw-2, h-2), lw, rw, h) + "\n"
}

// padTo pads s with spaces to w columns (never cuts).
func padTo(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
