package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/settings"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

type settingsState struct{ sel, top int }

func init() {
	register(viewSettings, screen{
		name:     "settings",
		identity: func(m model) string { return "settings" },
		fetch: func(m model) tea.Cmd { // nothing to load; this just ends the "loading…" state
			return func() tea.Msg { return screenMsg{view: viewSettings, id: "settings"} }
		},
		apply:  func(m model, msg screenMsg) model { return m },
		key:    settingsKey,
		body:   settingsBody,
		footer: "j/k move  enter or → next value  ← previous  :set key value  esc back",
	})
}

// settingValue is a setting's current value as it is shown.
func (m model) settingValue(d settings.Def) string {
	s := m.cfg()
	switch d.Kind {
	case settings.Bool:
		return settings.Display(d, s.Bool(d.Key))
	case settings.Int:
		return settings.Display(d, s.Int(d.Key))
	}
	return s.String(d.Key)
}

func sourceTag(s *settings.Store, key string) string {
	switch s.Source(key) {
	case settings.FromFile:
		return "file"
	case settings.FromEnv:
		return "env"
	}
	return ""
}

// settingsRows is how many setting rows fit under the title, the reason line and the description.
func (m model) settingsRows() int {
	s := m.cfg()
	extra := 2 // title, description of the selected setting
	if s.ReadOnly() != nil {
		extra++
	}
	extra += min(len(s.Notes()), 2)
	return max(1, m.logHeight()-extra)
}

func settingsBody(m model) string {
	st, _ := m.screenData(viewSettings).(settingsState)
	s := m.cfg()
	w := max(m.width-1, 20)
	var b strings.Builder
	where := s.Path()
	if where == "" {
		where = "not saved anywhere"
	}
	b.WriteString(" " + ui.Cols.Render(trunc("Settings — "+where, w-1)) + "\n")
	if err := s.ReadOnly(); err != nil {
		b.WriteString(" " + styErr.Render(trunc("not saved: "+err.Error(), w-1)) + "\n")
	}
	rows := m.settingsRows()
	if st.sel >= st.top+rows { // the terminal shrank since the last key: keep the selection in view
		st.top = st.sel - rows + 1
	}
	lines := 0
	for i := st.top; i < len(settings.Registry) && lines < rows; i++ {
		d := settings.Registry[i]
		label := trunc(d.Label, 28)
		line := fmt.Sprintf("%s%-28s %-8s %s", map[bool]string{true: "▸ ", false: "  "}[i == st.sel], label, m.settingValue(d), sourceTag(s, d.Key))
		line = trunc(strings.TrimRight(line, " "), w)
		if i == st.sel {
			line = ui.Sel.Render(line)
		}
		b.WriteString(" " + line + "\n")
		lines++
	}
	for ; lines < rows; lines++ {
		b.WriteString("\n")
	}
	if st.sel < len(settings.Registry) {
		b.WriteString(" " + styDim.Render(trunc(settings.Registry[st.sel].Desc, w-1)) + "\n")
	}
	for i, n := range s.Notes() {
		if i >= 2 {
			break
		}
		b.WriteString(" " + styErr.Render(trunc(n, w-1)) + "\n")
	}
	return b.String()
}

func settingsKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewSettings).(settingsState)
	n := len(settings.Registry)
	rows := m.settingsRows()
	switch k.String() {
	case "j", "down":
		st.sel = min(st.sel+1, n-1)
	case "k", "up":
		st.sel = max(st.sel-1, 0)
	case "g", "home":
		st.sel = 0
	case "G", "end":
		st.sel = n - 1
	case "enter", " ", "space", "right", "l":
		return m.cycleSetting(st, 1), nil, true
	case "left", "h":
		return m.cycleSetting(st, -1), nil, true
	default:
		return m, nil, false
	}
	if st.sel < st.top {
		st.top = st.sel
	}
	if st.sel >= st.top+rows {
		st.top = st.sel - rows + 1
	}
	return m.withScreenData(viewSettings, st), nil, true
}

func (m model) cycleSetting(st settingsState, dir int) model {
	if st.sel >= len(settings.Registry) {
		return m
	}
	d := settings.Registry[st.sel]
	applied, err := m.cfg().Cycle(d.Key, dir)
	m.status = settingStatus(m.cfg(), d.Key, applied, err)
	return m.withScreenData(viewSettings, st)
}

// settingStatus words the outcome of a change for the status line.
func settingStatus(s *settings.Store, key, applied string, err error) string {
	switch {
	case err != nil && errors.Is(err, settings.ErrNotSaved):
		return styErr.Render(fmt.Sprintf("%s = %s for this session, but not saved: %s", key, applied, strings.TrimPrefix(err.Error(), "not saved: ")))
	case err != nil:
		return styErr.Render(err.Error())
	case s.Overridden(key):
		d, _ := settings.Lookup(key)
		return styDim.Render(fmt.Sprintf("%s = %s saved, but %s is set: no effect for now", key, applied, d.Env))
	}
	return styOK.Render(fmt.Sprintf("✓ %s = %s", key, applied))
}

// setCommand is ":set key value" (or ":set key" to see a value).
func (m model) setCommand(args []string) (tea.Model, tea.Cmd) {
	switch len(args) {
	case 0:
		m.status = styDim.Render("usage: :set key value   (keys: " + strings.Join(settingKeys(), ", ") + ")")
	case 1:
		d, ok := settings.Lookup(args[0])
		if !ok {
			m.status = styErr.Render("unknown setting " + fmt.Sprintf("%q", ghcli.CleanText(args[0])))
			break
		}
		m.status = styDim.Render(fmt.Sprintf("%s = %s", d.Key, m.settingValue(d)))
	case 2:
		applied, err := m.cfg().Set(args[0], args[1])
		m.status = settingStatus(m.cfg(), args[0], applied, err)
	default:
		m.status = styErr.Render("usage: :set key value")
	}
	return m, nil
}

func settingKeys() []string {
	var out []string
	for _, d := range settings.Registry {
		out = append(out, d.Key)
	}
	return out
}

// settingCompletions completes the argument of ":set": the keys, then the values of the key.
func settingCompletions(input string) []string {
	rest := strings.TrimLeft(input[len("set"):], " ")
	key, val, hasVal := strings.Cut(rest, " ")
	var out []string
	if !hasVal {
		for _, d := range settings.Registry {
			if d.Key != key && strings.HasPrefix(d.Key, strings.ToLower(key)) {
				out = append(out, "set "+d.Key)
			}
		}
		return out
	}
	d, ok := settings.Lookup(key)
	if !ok {
		return nil
	}
	val = strings.ToLower(strings.TrimLeft(val, " "))
	var options []string
	switch d.Kind {
	case settings.Bool:
		options = []string{"on", "off"}
	case settings.Enum:
		options = d.Options
	}
	for _, o := range options {
		if o != val && strings.HasPrefix(o, val) {
			out = append(out, "set "+d.Key+" "+o)
		}
	}
	return out
}
