package tui

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/launch"
)

// launchInput is what the launch dialog starts from.
type launchInput struct {
	Issue      ghcli.IssueDetail
	Target     launch.Target
	AutoTarget bool // Target was detected, not chosen; the summary marks it with ◆
	Agent      string
	Model      string // empty means the agent's default
	Models     []launch.Model
	Base, Name string // Orca only: the ref to branch from, and the workspace name
	Place      launch.Place
	Split      launch.Split
	ContextLen int // size of the text that will be sent, in characters (runes)
}

// launchAct tells the caller what a key did.
type launchAct int

// What a key press asks the caller to do.
const (
	actNone         launchAct = iota
	actCancel                 // close the dialog
	actSubmit                 // start the launch
	actAgentChanged           // the agent changed: load its models and call withModels
)

// launchField is one row of the expanded dialog.
type launchField int

// The rows of the expanded dialog, in focus order.
const (
	fWhere launchField = iota
	fAgent
	fModel
	fBase
	fName
	fPlace
	fSplit
	fieldCount
)

// launchFieldName labels the rows.
var launchFieldName = [fieldCount]string{"Where", "Agent", "Model", "Base", "Branch", "Place", "Split"}

// launchTargets is the order the Where field cycles through.
var launchTargets = []string{string(launch.TargetOrca), string(launch.TargetHerdr), string(launch.TargetTmux)}

// launchDialog confirms sending an issue to an agent. It starts as one line
// the user can accept with enter; e opens the form to change anything. It does
// no I/O: the caller starts the launch and reports back with start and failed.
type launchDialog struct {
	in         launchInput
	models     []launch.Model
	typing     bool // the model is free text, not a pick from models
	baseEdited bool // the user changed the base, so withBase leaves it alone
	expanded   bool
	focus      launchField
	run        bool
	err        string // why enter was refused
	failure    string // the last launch failure, shown until the next start
	retryID    string // operation id of a launch whose reply was lost
}

// newLaunchDialog opens the compact confirmation.
func newLaunchDialog(in launchInput) launchDialog {
	if in.Place == "" {
		in.Place = launch.PlacePane
	}
	if in.Split == "" {
		in.Split = launch.SplitHorizontal
	}
	return launchDialog{in: in, models: in.Models}
}

func (d launchDialog) focused() launchField { return d.focus }

func (d launchDialog) running() bool { return d.run }

// start marks the launch as running; enter and esc are ignored until failed.
func (d launchDialog) start() launchDialog {
	d.run = true
	d.err, d.failure = "", ""
	return d
}

// failed ends a running launch and keeps the dialog open to show why. When Orca
// lost its reply, the next request carries the operation id so the same launch
// is asked again instead of started afresh.
func (d launchDialog) failed(err error) launchDialog {
	d.run, d.err, d.retryID = false, "", ""
	var lost *launch.LostReplyError
	var se *launch.StepError
	_, _ = errors.As(err, &lost), errors.As(err, &se)
	msg := "no details"
	switch {
	case lost != nil:
		d.retryID = lost.OperationID
		msg = lost.Cause + "; the launch may already have happened"
	case se != nil && se.Err != nil:
		msg = se.Err.Error()
	case se == nil && err != nil && err.Error() != "":
		msg = err.Error()
	}
	d.failure = "failed: " + msg
	if se != nil {
		d.failure = "failed at " + se.Step + ": " + msg
	}
	return d
}

// withModels replaces the model list, after the agent changed or the real list arrived.
func (d launchDialog) withModels(m []launch.Model) launchDialog {
	d.models = m
	return d
}

// withBase sets the base once it is known, unless the user already typed one.
func (d launchDialog) withBase(b string) launchDialog {
	if !d.baseEdited {
		d.in.Base = b
	}
	return d
}

// request builds the launch request from the fields. The caller adds Context and Dir.
func (d launchDialog) request() launch.Request {
	a, _ := launch.LookupAgent(d.in.Agent)
	return launch.Request{
		Target:  d.in.Target,
		Agent:   a,
		Model:   strings.TrimSpace(d.in.Model),
		Place:   d.in.Place,
		Split:   d.in.Split,
		Base:    strings.TrimSpace(d.in.Base),
		Name:    strings.TrimSpace(d.in.Name),
		Issue:   d.in.Issue.Number,
		RetryID: d.retryID,
	}
}

func (d launchDialog) orca() bool { return d.in.Target == launch.TargetOrca }

// applies reports whether the field is used for the current target and place.
func (d launchDialog) applies(f launchField) bool {
	switch f {
	case fBase, fName:
		return d.orca()
	case fPlace:
		return !d.orca()
	case fSplit:
		return !d.orca() && d.in.Place == launch.PlacePane
	}
	return true
}

// invalid says what stops the launch, or "".
func (d launchDialog) invalid() string {
	if _, ok := launch.LookupAgent(d.in.Agent); !ok {
		return "unknown agent " + d.in.Agent
	}
	if d.orca() {
		if strings.TrimSpace(d.in.Base) == "" {
			return "base is required"
		}
		if strings.TrimSpace(d.in.Name) == "" {
			return "branch is required"
		}
	}
	return ""
}

// move shifts the focus by dir, skipping rows that do not apply.
func (d *launchDialog) move(dir int) {
	for i := 0; i < int(fieldCount); i++ {
		d.focus = launchField((int(d.focus) + dir + int(fieldCount)) % int(fieldCount))
		if d.applies(d.focus) {
			return
		}
	}
}

// update handles one key and says what the caller should do.
func (d launchDialog) update(k tea.KeyMsg) (launchDialog, launchAct) {
	if d.run {
		return d, actNone
	}
	switch k.Type {
	case tea.KeyEsc:
		return d, actCancel
	case tea.KeyEnter:
		if msg := d.invalid(); msg != "" {
			if !d.expanded {
				msg += ", press e to edit"
			}
			d.err = msg
			return d, actNone
		}
		d.err = ""
		return d, actSubmit
	}
	if !d.expanded {
		if k.Type == tea.KeyRunes && string(k.Runes) == "e" && !k.Alt && !k.Paste {
			d.expanded = true
			d.err = ""
		}
		return d, actNone
	}
	if k.Alt && (k.Type == tea.KeyRunes || k.Type == tea.KeySpace || k.Type == tea.KeyBackspace) {
		return d, actNone // Alt chords are not text
	}
	switch k.Type {
	case tea.KeyTab, tea.KeyDown:
		d.move(1)
	case tea.KeyShiftTab, tea.KeyUp:
		d.move(-1)
	case tea.KeyLeft:
		return d.cycle(-1)
	case tea.KeyRight:
		return d.cycle(1)
	case tea.KeyBackspace:
		d.editText(func(s string) string { return dropLast(s) })
	case tea.KeySpace:
		d.editText(func(s string) string { return s + " " })
	case tea.KeyRunes:
		// A paste arrives as one message: keep what can be shown, drop newlines,
		// tabs and escape sequences instead of letting them reach the layout.
		r := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, string(k.Runes))
		if r != "" {
			d.editText(func(s string) string { return s + r })
		}
	}
	return d, actNone
}

// editText changes the focused row if it is a text row. The model row turns
// into free text: typing replaces the picked model.
func (d *launchDialog) editText(f func(string) string) {
	switch d.focus {
	case fModel:
		if !d.typing {
			d.typing = true
			// a typed character replaces the selection; backspace trims it
			if f("") != "" {
				d.in.Model = ""
			}
		}
		d.in.Model = f(d.in.Model)
	case fBase:
		d.in.Base = f(d.in.Base)
		d.baseEdited = true
	case fName:
		d.in.Name = f(d.in.Name)
	default:
		return
	}
	d.retryID, d.err = "", "" // an edit: a lost-reply retry no longer describes the fields
}

// cycle moves the focused choice row one step.
func (d launchDialog) cycle(dir int) (launchDialog, launchAct) {
	switch d.focus {
	case fWhere:
		d.in.Target = launch.Target(cycleChoice(launchTargets, string(d.in.Target), dir))
		d.in.AutoTarget = false
	case fAgent:
		ids := make([]string, len(launch.Agents))
		for i, a := range launch.Agents {
			ids[i] = a.ID
		}
		next := cycleChoice(ids, d.in.Agent, dir)
		d.retryID, d.err = "", "" // an edit: a lost-reply retry no longer describes the fields
		if next == d.in.Agent {
			return d, actNone
		}
		d.in.Agent = next
		d.in.Model, d.typing = "", false
		d.models = []launch.Model{{}}
		return d, actAgentChanged
	case fModel:
		ids := make([]string, len(d.models))
		for i, m := range d.models {
			ids[i] = m.ID
		}
		cur := d.in.Model
		if d.typing {
			cur = "\x00" // not in the list: right goes to the first entry, left to the last
		}
		d.in.Model, d.typing = cycleChoice(ids, cur, dir), false
	case fPlace:
		d.in.Place = launch.Place(cycleChoice([]string{string(launch.PlacePane), string(launch.PlaceWindow)}, string(d.in.Place), dir))
	case fSplit:
		d.in.Split = launch.Split(cycleChoice([]string{string(launch.SplitHorizontal), string(launch.SplitVertical)}, string(d.in.Split), dir))
	default:
		return d, actNone
	}
	d.retryID, d.err = "", "" // an edit: a lost-reply retry no longer describes the fields
	return d, actNone
}

// cycleChoice steps through vals from cur. A cur that is not in vals starts
// before the first entry (right) or after the last (left).
func cycleChoice(vals []string, cur string, dir int) string {
	if len(vals) == 0 {
		return cur
	}
	i := -1
	for j, v := range vals {
		if v == cur {
			i = j
			break
		}
	}
	if i < 0 && dir < 0 {
		i = 0
	}
	return vals[((i+dir)%len(vals)+len(vals))%len(vals)]
}

// summary is the one line the user confirms.
func (d launchDialog) summary() string {
	mark := " ·"
	if d.in.AutoTarget {
		mark = " ◆"
	}
	model := d.in.Model
	if strings.TrimSpace(model) == "" {
		model = "default model"
	}
	var where string
	switch {
	case d.orca():
		where = "new workspace"
	case d.in.Place == launch.PlaceWindow:
		where = "new window"
	case d.in.Split == launch.SplitVertical:
		where = "new pane (below)"
	default:
		where = "new pane (right)"
	}
	parts := []string{string(d.in.Target) + mark + " " + where, d.in.Agent, model}
	if d.orca() {
		parts = append(parts, d.in.Base+" → "+d.in.Name)
	}
	s := strings.Join(parts, " · ")
	if d.in.ContextLen > launch.WarnChars {
		s += fmt.Sprintf(" · ⚠ large issue, %d characters", d.in.ContextLen)
	}
	return s
}

// minWrap is the narrowest width text is wrapped to (a wide character needs 2).
const minWrap = 4

// wrapLines indents text by one space and wraps it to width display columns
// (0 means no wrapping). Continuation lines are indented too.
func wrapLines(s string, width int) string {
	if width > 0 {
		s = ansi.Wrap(s, max(width-1, minWrap), "")
	}
	return " " + strings.ReplaceAll(s, "\n", "\n ")
}

// fitLines wraps any line of s wider than width display columns. It is the
// safety net for lines built from styled pieces.
func fitLines(s string, width int) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if ansi.StringWidth(l) > width {
			lines[i] = ansi.Wrap(l, max(width, minWrap), "")
		}
	}
	return strings.Join(lines, "\n")
}

// clipTail shortens s to room display columns for showing, keeping the end
// (where typing happens) behind a leading ellipsis. The stored value is not touched.
func clipTail(s string, room int) string {
	w := ansi.StringWidth(s)
	if room <= 0 || w <= room {
		return s
	}
	if room < 2 {
		return "…"
	}
	return ansi.TruncateLeft(s, w-(room-1), "…")
}

// view renders the dialog body, every line at most width display columns
// wide (0 means unlimited); the caller draws the box around it.
func (d launchDialog) view(width int) string {
	var b strings.Builder
	b.WriteString(styCols.Render(fmt.Sprintf(" Send #%d to an agent", d.in.Issue.Number)) + "\n")
	b.WriteString(styDim.Render(wrapLines(d.in.Issue.Title, width)) + "\n\n")
	if d.expanded {
		d.formRows(&b, width)
		b.WriteString("\n" + wrapLines(d.summary(), width) + "\n")
	} else {
		b.WriteString(wrapLines(d.summary(), width) + "\n")
	}
	switch {
	case d.run:
		b.WriteString("\n " + styRun.Render("launching…") + "\n")
	case d.failure != "":
		b.WriteString("\n" + styErr.Render(wrapLines(d.failure, width)) + "\n")
		again := "enter to try again"
		if d.retryID != "" {
			again = "enter retries the same launch (it may already be running)"
		}
		b.WriteString(styDim.Render(wrapLines(again, width)) + "\n")
	}
	if d.err != "" {
		b.WriteString("\n" + styErr.Render(wrapLines(d.err, width)) + "\n")
	}
	keys := "enter launch   e edit   esc cancel"
	if d.expanded {
		keys = "tab/↑↓ move   ←/→ choose   type to edit   enter launch   esc cancel"
	}
	if d.run {
		keys = "wait for the launch to finish"
	}
	b.WriteString("\n" + styDim.Render(wrapLines(keys, width)) + "\n")
	return fitLines(b.String(), width)
}

// rowPrefix is the width of the focus mark, the label and the space after it.
const rowPrefix = 2 + 8 + 1

// formRows writes the rows that apply to the current target and place.
func (d launchDialog) formRows(b *strings.Builder, width int) {
	room := 0 // columns left for a value; 0 means unlimited
	if width > 0 {
		room = max(width-rowPrefix, 1)
	}
	for f := fWhere; f < fieldCount; f++ {
		if !d.applies(f) {
			continue
		}
		mark := "  "
		if d.focus == f {
			mark = styKey.Render("▸ ")
		}
		focused := d.focus == f
		var val, note string
		switch f {
		case fWhere:
			val = "‹ " + string(d.in.Target) + " ›"
		case fAgent:
			val = "‹ " + d.in.Agent + " ›"
		case fModel:
			switch {
			case d.typing:
				val = textValue(d.in.Model, focused, room)
				note = "a mistyped name only fails in the agent pane"
			default:
				name := d.in.Model
				if name == "" {
					name = "default"
				}
				val = "‹ " + clipTail(name, max(room-4, 0)) + " ›"
				for _, m := range d.models {
					if m.ID == d.in.Model && m.ID != "" {
						note = m.Hint
					}
				}
				if focused {
					note = "type to enter another name"
				}
			}
		case fBase:
			val = textValue(d.in.Base, focused, room)
		case fName:
			val = textValue(d.in.Name, focused, room)
			note = "Orca's workspace name; Orca adds its own prefix"
		case fPlace:
			val = "‹ " + string(d.in.Place) + " ›"
		case fSplit:
			val = "‹ " + string(d.in.Split) + " ›"
		}
		fmt.Fprintf(b, "%s%-8s %s", mark, launchFieldName[f], val)
		b.WriteString(hint(note != "", note))
		b.WriteString("\n")
	}
}

// textValue shows an editable value, with the cursor when focused, clipped to room columns.
func textValue(s string, focused bool, room int) string {
	if focused {
		return clipTail(s, max(room-1, 0)) + "█"
	}
	return clipTail(s, room)
}
