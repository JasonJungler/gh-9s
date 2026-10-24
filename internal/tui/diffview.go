package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls and limits of the diff screen; tests replace them.
var (
	fetchDiff    = ghcli.FetchPRDiff
	maxDiffLines = 5000 // lines kept; a bigger diff is cut and the browser is offered
)

// diffState is what the diff screen keeps in the model. The scroll position is m.top, and
// the plain text of every display line is in m.logs so the log viewer's search works unchanged.
type diffState struct {
	d        diff.Diff
	listMode bool // showing the file list instead of the diff
	sel      int  // selected file in the list
}

type dkind int

const (
	dFile dkind = iota
	dHunk
	dAdd
	dDel
	dCtx
	dMeta
	dNote
	dBlank
)

type dline struct {
	text string
	kind dkind
}

func init() {
	register(viewDiff, screen{
		name: "diff",
		fetch: func(m model) tea.Cmd {
			tg, number, limit := m.target, m.pr, maxDiffLines
			return func() tea.Msg {
				text, err := fetchDiff(tg, number)
				if err != nil {
					return screenMsg{view: viewDiff, err: err}
				}
				return screenMsg{view: viewDiff, data: diff.Parse(text, limit)}
			}
		},
		apply: func(m model, msg screenMsg) model {
			st := diffState{d: msg.data.(diff.Diff)}
			lines, _ := diffDisplay(st.d)
			m.logs = make([]string, len(lines))
			for i, l := range lines {
				m.logs[i] = l.text
			}
			if m.query != "" {
				m.matches = searchMatches(m.logs, m.query)
				m.mi = min(m.mi, max(0, len(m.matches)-1))
			}
			return m.withScreenData(viewDiff, st)
		},
		key:    diffKey,
		body:   diffBody,
		footer: "j/k scroll  ]/[ next/prev file  f files  / search  n/N  o browser",
	})
}

func plusMinus(f diff.File) string { return fmt.Sprintf("+%d −%d", f.Added, f.Removed) }

// diffDisplay lays the diff out as display lines and reports where each file starts.
func diffDisplay(d diff.Diff) (lines []dline, starts []int) {
	if len(d.Files) == 0 {
		return []dline{{"(no changes)", dNote}}, nil
	}
	for i, f := range d.Files {
		if i > 0 {
			lines = append(lines, dline{"", dBlank})
		}
		starts = append(starts, len(lines))
		name := f.Path
		if f.Renamed {
			name = f.OldPath + " → " + f.Path
		}
		if f.Binary {
			name += " (binary)"
		}
		lines = append(lines, dline{fmt.Sprintf("── %s  %s ──", name, plusMinus(f)), dFile})
		for _, l := range f.Lines {
			text := strings.ReplaceAll(l.Text, "\t", "    ")
			switch l.Kind {
			case diff.Add:
				lines = append(lines, dline{"+ " + text, dAdd})
			case diff.Del:
				lines = append(lines, dline{"- " + text, dDel})
			case diff.Context:
				lines = append(lines, dline{"  " + text, dCtx})
			case diff.Hunk:
				lines = append(lines, dline{text, dHunk})
			default:
				lines = append(lines, dline{"  " + text, dMeta})
			}
		}
	}
	if d.Truncated {
		lines = append(lines, dline{"", dBlank}, dline{fmt.Sprintf(
			"… diff truncated at %d of %d lines; press o to see the whole PR in the browser", maxDiffLines, d.TotalLines), dNote})
	}
	return lines, starts
}

func diffKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewDiff).(diffState)
	_, starts := diffDisplay(st.d)
	if st.listMode {
		switch k.String() {
		case "j", "down":
			st.sel = min(st.sel+1, max(len(starts)-1, 0))
		case "k", "up":
			st.sel = max(st.sel-1, 0)
		case "enter":
			if st.sel < len(starts) {
				m.top = starts[st.sel]
			}
			st.listMode = false
		case "esc", "f":
			st.listMode = false
		default:
			return m, nil, false
		}
		return m.withScreenData(viewDiff, st), nil, true
	}
	h := m.logHeight()
	switch k.String() {
	case "j", "down":
		m.top = min(m.top+1, m.maxTop())
	case "k", "up":
		m.top = max(m.top-1, 0)
	case "ctrl+d", "pgdown", " ":
		m.top = min(m.top+h/2, m.maxTop())
	case "ctrl+u", "pgup":
		m.top = max(m.top-h/2, 0)
	case "g", "home":
		m.top = 0
	case "G", "end":
		m.top = m.maxTop()
	case "]":
		for _, s := range starts {
			if s > m.top {
				m.top = s
				break
			}
		}
	case "[":
		prev := 0
		for _, s := range starts {
			if s < m.top {
				prev = s
			}
		}
		m.top = prev
	case "f":
		st.listMode, st.sel = true, 0
		for i, s := range starts {
			if s <= m.top {
				st.sel = i
			}
		}
	case "/":
		m.mode, m.input = modeSearch, m.query
	case "n", "N":
		if len(m.matches) > 0 {
			d := 1
			if k.String() == "N" {
				d = len(m.matches) - 1
			}
			m.gotoMatch((m.mi + d) % len(m.matches))
		}
	case "o":
		pd, _ := m.screenData(viewPRDetail).(prDetailState)
		if pd.d.URL != "" {
			_ = openURL(pd.d.URL)
		}
	case "esc":
		if m.query == "" {
			return m, nil, false
		}
		m.query, m.matches, m.mi = "", nil, 0
	default:
		return m, nil, false
	}
	return m.withScreenData(viewDiff, st), nil, true
}

func diffBody(m model) string {
	st, _ := m.screenData(viewDiff).(diffState)
	h := m.logHeight()
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.Err.Render(" "+m.err.Error()) + "\n")
		h--
	}
	n := 0
	emit := func(s string) {
		b.WriteString(s + "\n")
		n++
	}
	w := max(m.width-1, 20)
	switch {
	case m.loading && len(m.logs) == 0:
		emit(ui.Dim.Render(" loading diff…"))
	case st.listMode:
		_, starts := diffDisplay(st.d)
		emit(ui.Cols.Render(fmt.Sprintf(" files (%d)", len(st.d.Files))))
		for i, f := range st.d.Files {
			line := fmt.Sprintf("%s  %s", f.Path, plusMinus(f))
			if i == st.sel {
				emit(ui.Sel.Render(" ▸ " + strings.TrimRight(trunc(line, w-3), " ")))
			} else {
				emit("   " + strings.TrimRight(trunc(line, w-3), " "))
			}
		}
		_ = starts
	default:
		lines, _ := diffDisplay(st.d)
		for i := m.top; i < len(lines) && n < h; i++ {
			emit(m.styledDiffLine(i, lines[i], w))
		}
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) styledDiffLine(i int, l dline, w int) string {
	text := strings.TrimRight(trunc(" "+l.text, w), " ")
	switch {
	case m.isMatch(i) && m.matches[m.mi] == i:
		return ui.Sel.Render(text)
	case m.isMatch(i):
		return ui.Key.Render(text)
	}
	switch l.kind {
	case dFile:
		return ui.Key.Render(text)
	case dHunk:
		return ui.Cols.Render(text)
	case dAdd:
		return ui.OK.Render(text)
	case dDel:
		return ui.Fail.Render(text)
	case dMeta, dNote:
		return ui.Dim.Render(text)
	}
	return text
}
