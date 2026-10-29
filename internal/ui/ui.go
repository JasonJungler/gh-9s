// Package ui holds the presentation primitives shared by the terminal UI and the graph
// renderer: run states, colours, and status/time formatting.
package ui

import (
	"fmt"
	"time"

	"github.com/MurabitoB/gh-9s/internal/icons"

	"github.com/charmbracelet/lipgloss"
)

// Age renders an RFC3339 timestamp as a short age such as 5m or 3d.
func Age(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil || t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// RunState maps a run or job status and conclusion to a label and a State.
func RunState(status, conclusion string) (string, State) {
	if status != "completed" {
		return icons.Get(icons.RunActive) + " " + status, StRun
	}
	switch conclusion {
	case "success":
		return icons.Get(icons.RunOK) + " success", StOK
	case "failure", "timed_out", "startup_failure":
		return icons.Get(icons.RunFail) + " " + conclusion, StFail
	default:
		return icons.Get(icons.RunOther) + " " + conclusion, StOther
	}
}

// Duration renders the time between two RFC3339 timestamps (end empty = now).
func Duration(start, end string) string {
	s, err := time.Parse(time.RFC3339, start)
	if err != nil || s.IsZero() {
		return "-"
	}
	e, err := time.Parse(time.RFC3339, end)
	switch {
	case err != nil || e.IsZero():
		e = time.Now() // no end yet: still running
	case e.Before(s):
		e = s // skipped jobs report an end a second before their start; that is zero time, not "still running"
	}
	d := e.Sub(s).Round(time.Second)
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// State is the coarse health of a run, job, check or workflow, used for colour and symbols.
type State int

const (
	StOther State = iota
	StOK
	StFail
	StRun
)

// Symbol is the one-character marker for each state, from the icon set in use.
func Symbol(s State) string {
	switch s {
	case StOK:
		return icons.Get(icons.RunOK)
	case StFail:
		return icons.Get(icons.RunFail)
	case StRun:
		return icons.Get(icons.RunActive)
	}
	return icons.Get(icons.RunOther)
}

var (
	Header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
	Cols   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	Sel    = lipgloss.NewStyle().Reverse(true).Bold(true)
	OK     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Fail   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Run    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	Err    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	Key    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	NumKey = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
)

// StateStyle colours text by state.
func StateStyle(s State) lipgloss.Style {
	switch s {
	case StOK:
		return OK
	case StFail:
		return Fail
	case StRun:
		return Run
	}
	return Dim
}
