package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Short local names for the shared presentation primitives in package ui.
type state = ui.State

const (
	stOther = ui.StOther
	stOK    = ui.StOK
	stFail  = ui.StFail
	stRun   = ui.StRun
)

var (
	styHeader = ui.Header
	styCols   = ui.Cols
	stySel    = ui.Sel
	styOK     = ui.OK
	styFail   = ui.Fail
	styRun    = ui.Run
	styDim    = ui.Dim
	styErr    = ui.Err
	styKey    = ui.Key
)

func stateStyle(s state) lipgloss.Style { return ui.StateStyle(s) }

func runState(status, conclusion string) (string, state) { return ui.RunState(status, conclusion) }

func age(ts string) string { return ui.Age(ts) }

func duration(start, end string) string { return ui.Duration(start, end) }
