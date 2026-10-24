package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// A screen is a view that lives in its own file. It registers itself with register, and the
// shared code (fetch, Update, handleKey, View, footer, shouldPoll) asks the registry before
// falling back to the built-in views, so adding a screen does not touch those switches.
type screen struct {
	name     string                                             // shown in the title as <name>
	fetch    func(m model) tea.Cmd                              // loads the screen's data; yields a screenMsg
	apply    func(m model, msg screenMsg) model                 // stores a loaded result in the model
	key      func(m model, k tea.KeyMsg) (model, tea.Cmd, bool) // handled=false falls through to the shared keys
	body     func(m model) string                               // everything between the header and the footer
	footer   string                                             // key hints
	identity func(m model) string                               // what the screen shows; a result for anything else is dropped
	poll     bool                                               // refetch on every tick
}

// screenMsg carries a screen's loaded data back to Update.
type screenMsg struct {
	view viewKind
	id   string // the identity the data was fetched for
	data any
	err  error
}

var screens = map[viewKind]screen{}

// register adds a screen. Registering a view twice is a programming error.
func register(v viewKind, s screen) {
	if _, dup := screens[v]; dup {
		panic(fmt.Sprintf("tui: screen for view %d registered twice", v))
	}
	screens[v] = s
	viewNames[v] = s.name
}

// screenData returns what a screen stored with withScreenData (nil if nothing).
func (m model) screenData(v viewKind) any { return m.sdata[v] }

// withScreenData returns a copy of m with the screen's data replaced. The map is copied, so
// fetch goroutines that still hold the old model never see a write.
func (m model) withScreenData(v viewKind, d any) model {
	next := make(map[viewKind]any, len(m.sdata)+1)
	for k, x := range m.sdata {
		next[k] = x
	}
	next[v] = d
	m.sdata = next
	return m
}

// itemID names "this repo's PR or issue number": the identity of a detail screen. Every fetch
// records it and Update drops a result whose identity is no longer the one on screen, so a slow
// answer for the previous item can never be shown (or acted on) as the current one.
func (m model) itemID(number string) string { return m.target.String() + "#" + number }
