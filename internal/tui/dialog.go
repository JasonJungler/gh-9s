package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ui"
)

type dialogKind int

const (
	dialogText dialogKind = iota
	dialogChoice
	dialogConfirm
)

// choice is one row of a choice dialog.
type choice struct {
	label, value string
	disabled     bool   // shown but cannot be picked
	note         string // why it is disabled, or any hint
}

// dialog is a small overlay that asks one question: for text, for one or several choices, or yes/no.
// Whoever opens it gives it an id; the answer comes back as a dialogResultMsg with that id and is
// routed to the handler registered under it in dialogHandlers.
type dialog struct {
	id, title, prompt, detail string
	kind                      dialogKind
	text                      string          // dialogText: what has been typed
	choices                   []choice        // dialogChoice
	multi                     bool            // dialogChoice: several can be ticked
	checked                   map[string]bool // dialogChoice multi: ticked values
	filter                    string          // dialogChoice: typed filter
	sel                       int             // dialogChoice: position among the visible choices
	err                       string
	needBang                  bool   // dialogChoice: enter is refused; only "!" accepts (a deliberate override)
	bangHint                  string // what enter says when needBang
	toggleLabel, toggleKey    string // dialogConfirm: an optional yes/no setting flipped with toggleKey
	toggleOn                  bool
}

// dialogResultMsg is a dialog's answer. ok is false when it was cancelled.
type dialogResultMsg struct {
	id      string
	ok      bool
	text    string   // dialogText
	values  []string // dialogChoice: the picked values
	toggled bool     // dialogConfirm: the state of the toggle when answered
}

// dialogHandlers receive dialog answers by id. Features register theirs in init.
var dialogHandlers = map[string]func(m model, r dialogResultMsg) (model, tea.Cmd){}

func newTextDialog(id, title, prompt string) dialog {
	return dialog{id: id, title: title, prompt: prompt, kind: dialogText}
}

func newChoiceDialog(id, title string, choices []choice, multi bool, checked []string) dialog {
	d := dialog{id: id, title: title, kind: dialogChoice, choices: choices, multi: multi, checked: map[string]bool{}}
	for _, v := range checked {
		d.checked[v] = true
	}
	return d
}

func newConfirmDialog(id, title, detail string) dialog {
	return dialog{id: id, title: title, detail: detail, kind: dialogConfirm}
}

// withDialog opens d.
func (m model) withDialog(d dialog) model {
	m.dlg = &d
	return m
}

// editedMsg is the text written in $EDITOR.
type editedMsg struct {
	text string
	err  error
}

// editorCmd opens $EDITOR on a temporary file holding initial; tests replace it.
var editorCmd = realEditorCmd

func realEditorCmd(initial string) tea.Cmd {
	cmd, path, err := editorCommand(initial)
	if err != nil {
		return func() tea.Msg { return editedMsg{err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		b, rerr := os.ReadFile(path)
		_ = os.Remove(path)
		if err == nil {
			err = rerr
		}
		return editedMsg{text: string(b), err: err}
	})
}

// editorCommand writes initial to a temporary file and builds the command that edits it. The
// editor setting may carry arguments ("code --wait"), so it goes through the shell; the file
// name is passed as an argument, never pasted into the script.
func editorCommand(initial string) (cmd *exec.Cmd, path string, err error) {
	f, err := os.CreateTemp("", "gh9s-*.md")
	if err != nil {
		return nil, "", err
	}
	_, _ = f.WriteString(initial)
	_ = f.Close()
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	return exec.Command("sh", "-c", editor+` "$1"`, "sh", f.Name()), f.Name(), nil
}

// visible lists the indices of the choices matching the filter.
func (d dialog) visible() []int {
	var out []int
	f := strings.ToLower(d.filter)
	for i, c := range d.choices {
		if f == "" || strings.Contains(strings.ToLower(c.label), f) {
			out = append(out, i)
		}
	}
	return out
}

func (m model) dialogKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := *m.dlg
	finish := func(ok bool, text string, values []string) (tea.Model, tea.Cmd) {
		m.dlg = nil
		id := d.id
		toggled := d.toggleOn
		return m, func() tea.Msg { return dialogResultMsg{id: id, ok: ok, text: text, values: values, toggled: toggled} }
	}
	keep := func() (tea.Model, tea.Cmd) { m.dlg = &d; return m, nil }
	if k.Type == tea.KeyEsc {
		return finish(false, "", nil)
	}
	switch d.kind {
	case dialogConfirm:
		if d.toggleKey != "" && k.String() == d.toggleKey {
			d.toggleOn = !d.toggleOn
			return keep()
		}
		return finish(k.String() == "y" || k.String() == "Y", "", nil)
	case dialogText:
		d.err = ""
		switch k.Type {
		case tea.KeyEnter:
			if strings.TrimSpace(d.text) == "" {
				d.err = "type something first"
				return keep()
			}
			return finish(true, d.text, nil)
		case tea.KeyCtrlE:
			m.dlg = &d
			return m, editorCmd(d.text)
		case tea.KeyBackspace:
			if _, sz := utf8.DecodeLastRuneInString(d.text); sz > 0 {
				d.text = d.text[:len(d.text)-sz]
			}
		case tea.KeySpace:
			d.text += " "
		case tea.KeyRunes:
			d.text += string(k.Runes)
		}
		return keep()
	}
	// dialogChoice
	vis := d.visible()
	d.err = ""
	switch k.Type {
	case tea.KeyUp:
		d.sel = max(d.sel-1, 0)
	case tea.KeyDown:
		d.sel = min(d.sel+1, max(len(vis)-1, 0))
	case tea.KeySpace:
		if d.multi && d.sel < len(vis) {
			if c := d.choices[vis[d.sel]]; !c.disabled {
				d.checked[c.value] = !d.checked[c.value]
			}
		}
	case tea.KeyBackspace:
		if _, sz := utf8.DecodeLastRuneInString(d.filter); sz > 0 {
			d.filter = d.filter[:len(d.filter)-sz]
			d.sel = 0
		}
	case tea.KeyRunes:
		if d.needBang && string(k.Runes) == "!" {
			if d.sel < len(vis) && !d.choices[vis[d.sel]].disabled {
				return finish(true, "", []string{d.choices[vis[d.sel]].value})
			}
			return keep()
		}
		d.filter += string(k.Runes)
		d.sel = 0
	case tea.KeyEnter:
		if d.needBang {
			d.err = d.bangHint
			return keep()
		}
		if d.multi {
			var values []string
			for _, c := range d.choices {
				if d.checked[c.value] && !c.disabled {
					values = append(values, c.value)
				}
			}
			return finish(true, "", values)
		}
		if d.sel >= len(vis) {
			return keep()
		}
		c := d.choices[vis[d.sel]]
		if c.disabled {
			d.err = c.label + ": " + c.note
			return keep()
		}
		return finish(true, "", []string{c.value})
	}
	return keep()
}

func (m model) dialogView() string {
	d := *m.dlg
	var b strings.Builder
	b.WriteString(ui.Cols.Render(" "+d.title) + "\n")
	if d.prompt != "" {
		b.WriteString(ui.Dim.Render(" "+d.prompt) + "\n")
	}
	b.WriteString("\n")
	hint := ""
	switch d.kind {
	case dialogText:
		lines := strings.Split(d.text, "\n")
		if len(lines) > 6 {
			b.WriteString(ui.Dim.Render(fmt.Sprintf(" … %d earlier lines", len(lines)-6)) + "\n")
			lines = lines[len(lines)-6:]
		}
		for i, l := range lines {
			cursor := ""
			if i == len(lines)-1 {
				cursor = "█"
			}
			b.WriteString(" > " + l + cursor + "\n")
		}
		hint = "enter send   ctrl+e write in $EDITOR   esc cancel"
	case dialogConfirm:
		if d.detail != "" {
			b.WriteString(" " + d.detail + "\n\n")
		}
		hint = "y confirm   any other key cancels"
		if d.toggleKey != "" {
			box := "[ ] "
			if d.toggleOn {
				box = "[x] "
			}
			b.WriteString(" " + box + d.toggleLabel + ui.Dim.Render("   ("+d.toggleKey+" to change)") + "\n\n")
			hint = "y confirm   " + d.toggleKey + " toggle   any other key cancels"
		}
	default:
		b.WriteString(" filter: " + d.filter + "█\n")
		vis := d.visible()
		if len(vis) == 0 {
			b.WriteString(ui.Dim.Render(" (nothing to choose)") + "\n")
		}
		for pos, i := range vis {
			c := d.choices[i]
			mark := "  "
			if d.multi {
				mark = "[ ] "
				if d.checked[c.value] {
					mark = "[x] "
				}
			} else if pos == d.sel {
				mark = "▸ "
			}
			line := mark + c.label
			if c.note != "" {
				line += "   " + c.note
			}
			switch {
			case c.disabled:
				line = ui.Dim.Render(line)
			case pos == d.sel:
				line = ui.Sel.Render(line)
			}
			b.WriteString(" " + line + "\n")
		}
		hint = "↑↓ move   enter accept   esc cancel"
		if d.multi {
			hint = "↑↓ move   space tick   enter accept   esc cancel"
		}
		if d.needBang {
			hint = "↑↓ move   ! accept anyway   esc cancel"
		}
	}
	if d.err != "" {
		b.WriteString("\n" + ui.Err.Render(" "+d.err) + "\n")
	}
	b.WriteString("\n" + ui.Dim.Render(" "+hint) + "\n")
	return b.String()
}
