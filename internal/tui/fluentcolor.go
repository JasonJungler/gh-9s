package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// trueColor says whether exact colours can be drawn; tests replace it.
var trueColor = func() bool { return lipgloss.ColorProfile() == termenv.TrueColor }

// fluentSel draws a selected row or card: a blue block on truecolor terminals, the plain reverse video elsewhere.
func fluentSel(s string) string {
	if !trueColor() {
		return stySel.Render(s)
	}
	bg, fg := "#0f3a5f", "#ffffff"
	if !lipgloss.HasDarkBackground() {
		bg, fg = "#cfe4fa", "#0b2a4a"
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Bold(true).Render(s)
}

// fluentCard draws a card that is not selected: a block a little off the page colour. Without truecolor it stays plain.
func fluentCard(s string) string {
	if !trueColor() {
		return s
	}
	bg, fg := "#292929", "#cfcfcf"
	if !lipgloss.HasDarkBackground() {
		bg, fg = "#f3f3f3", "#242424"
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Render(s)
}
