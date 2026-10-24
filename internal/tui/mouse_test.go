package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func wheelMsg(b tea.MouseButton, x int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: 5, Button: b, Action: tea.MouseActionPress, Type: tea.MouseWheelDown}
}

func send(m model, msg tea.Msg) model {
	next, _ := m.Update(msg)
	return next.(model)
}

func listOf(n int) model {
	m := model{view: viewPRs, width: 100, height: 30, target: acme, previewOff: true}
	var prs []prListItem
	for i := 1; i <= n; i++ {
		prs = append(prs, prListItem{Number: i, Title: "t"})
	}
	m.tbl = prRows(prs, "")
	return m
}

func TestWheelScrollsThreeRowsPerNotchAndIsOnByDefault(t *testing.T) {
	m := listOf(40)
	if !m.cfg().Bool("mouse") {
		t.Fatal("the mouse setting is on by default")
	}
	m = send(m, wheelMsg(tea.MouseButtonWheelDown, 10))
	if m.cursor != 3 {
		t.Errorf("one notch down moves 3 rows, cursor = %d", m.cursor)
	}
	m = send(m, wheelMsg(tea.MouseButtonWheelUp, 10))
	if m.cursor != 0 {
		t.Errorf("and one notch up moves back, cursor = %d", m.cursor)
	}
}

func TestWheelIgnoresClicksPromptsAndTheSetting(t *testing.T) {
	m := listOf(40)
	click := tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	if got := send(m, click); got.cursor != 0 {
		t.Error("a click is ignored")
	}
	release := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease}
	if got := send(m, release); got.cursor != 0 {
		t.Error("only the press counts")
	}
	typing := m
	typing.mode, typing.input = modeFilter, "ab"
	if got := send(typing, wheelMsg(tea.MouseButtonWheelDown, 1)); got.cursor != 0 || got.input != "ab" {
		t.Error("the wheel does nothing while a prompt is open")
	}
	off, _ := settingsModel(t, `{"mouse": false}`, nil)
	off.view, off.width, off.height, off.previewOff = viewPRs, 100, 30, true
	off.tbl = m.tbl
	if got := send(off, wheelMsg(tea.MouseButtonWheelDown, 1)); got.cursor != 0 {
		t.Error("the setting turns the wheel off")
	}
}

func TestWheelOverThePaneScrollsThePane(t *testing.T) {
	f := newPreviewFixture(t, 10)
	p := fullPR()
	p.Body = "line\n\n"
	for i := 0; i < 80; i++ {
		p.Body += "more\n\n"
	}
	f.settle(f.move(1))
	f.m.preview.put(f.m.preview.sel, previewEntry{pr: &p, at: previewNow(), rowUpdated: "9"})
	f.m.previewContent(60, 10)
	lw, _ := f.m.paneWidths()
	m := send(f.m, wheelMsg(tea.MouseButtonWheelDown, lw+5))
	if m.preview.top != wheelLines || m.cursor != 1 {
		t.Errorf("over the pane the pane scrolls (top=%d) and the list stays (cursor=%d)", m.preview.top, m.cursor)
	}
	m = send(m, wheelMsg(tea.MouseButtonWheelDown, 2))
	if m.cursor != 4 {
		t.Errorf("over the list the list moves, cursor = %d", m.cursor)
	}
}

func TestWheelScrollsTheHelpAndTheSettingTogglesTheMouse(t *testing.T) {
	m := helpModel(viewDiff)
	m.height = 14
	m = keys(m, "?")
	m = send(m, wheelMsg(tea.MouseButtonWheelDown, 5))
	if m.helpTop != wheelLines {
		t.Errorf("the help scrolls, top = %d", m.helpTop)
	}
	if mouseCmd("mouse", true) == nil || mouseCmd("mouse", false) == nil || mouseCmd("icons", true) != nil {
		t.Error("changing the mouse setting turns reporting on or off, other settings do not")
	}
}
