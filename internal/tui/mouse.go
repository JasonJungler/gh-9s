package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// wheelLines is how many lines one notch of the wheel scrolls.
const wheelLines = 3

// wheel turns the mouse wheel into the keys that already scroll every screen: a notch is three presses
// of down or up, so lists, diffs, logs, details, dialogs and the help all follow with no code of their
// own. Over the preview pane it scrolls the pane. Clicks and drags are ignored.
func (m model) wheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || !m.cfg().Bool("mouse") || m.askIcons {
		return m, nil
	}
	var key tea.KeyType
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		key = tea.KeyUp
	case tea.MouseButtonWheelDown:
		key = tea.KeyDown
	default:
		return m, nil
	}
	if m.mode != modeNormal { // a prompt is being typed into
		return m, nil
	}
	if m.previewShown() && m.dlg == nil && m.picker == nil && m.form == nil && m.launchDlg == nil && !m.helpOpen {
		if lw, _ := m.paneWidths(); msg.X >= lw {
			step := wheelLines
			if key == tea.KeyUp {
				step = -step
			}
			return m.previewScroll(step), nil
		}
	}
	var cur tea.Model = m
	var cmds []tea.Cmd
	for i := 0; i < wheelLines; i++ {
		var c tea.Cmd
		cur, c = cur.Update(tea.KeyMsg{Type: key})
		cmds = append(cmds, c)
	}
	return cur, tea.Batch(cmds...)
}
