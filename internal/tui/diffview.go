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
	fetchThreads = ghcli.FetchReviewThreads
	maxDiffLines = 50000 // lines kept; a bigger diff is cut and the browser is offered
)

// diffState is what the diff screen keeps in the model. The scroll position is m.top, and
// the plain text of every display line is in m.logs so the log viewer's search works unchanged.
type diffState struct {
	d        diff.Diff
	lines    []dline // the laid-out diff, built once when it loads
	starts   []int   // where each file starts in lines
	listMode bool    // showing the file list instead of the diff
	sel      int     // selected file in the list

	usrc      []int           // source key of every unified row, to keep the place when the layout changes
	sb        *sbLayout       // the side-by-side layout for the current width; nil while unified
	sbs       int             // 0 by width, 1 always side-by-side, 2 always unified
	focusList bool            // side-by-side: j/k move through the file list
	threads   []ghcli.Thread  // review comments other people left
	viewed    map[string]bool // files marked viewed, by path
	head      string          // the PR head commit the marks belong to
}

// diffPayload is what the diff fetch delivers: the diff and, if they could be had, the threads.
type diffPayload struct {
	d         diff.Diff
	threads   []ghcli.Thread
	threadErr error
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
		name:     "diff",
		identity: func(m model) string { return m.itemID(m.pr) },
		fetch: func(m model) tea.Cmd {
			tg, number, limit, id := m.target, m.pr, maxDiffLines, m.itemID(m.pr)
			return func() tea.Msg {
				type thr struct {
					ts  []ghcli.Thread
					err error
				}
				ch := make(chan thr, 1)
				go func() {
					ts, err := fetchThreads(tg, number)
					ch <- thr{ts, err}
				}()
				text, err := fetchDiff(tg, number)
				th := <-ch
				if err != nil {
					return screenMsg{view: viewDiff, id: id, err: err}
				}
				return screenMsg{view: viewDiff, id: id, data: diffPayload{d: diff.Parse(text, limit), threads: th.ts, threadErr: th.err}}
			}
		},
		apply: func(m model, msg screenMsg) model {
			p := msg.data.(diffPayload)
			old, _ := m.screenData(viewDiff).(diffState)
			st := diffState{d: p.d, threads: p.threads, sbs: old.sbs}
			if pd, ok := m.screenData(viewPRDetail).(prDetailState); ok {
				st.head = pd.d.HeadSHA
			}
			if st.head != "" && st.head == old.head {
				st.viewed = old.viewed
			}
			lines, starts, src := diffDisplaySrc(st.d)
			st.lines, st.starts, st.usrc = lines, starts, src
			m.logs = make([]string, len(lines))
			for i, l := range lines {
				m.logs[i] = l.text
			}
			if m.query != "" {
				m.matches = searchMatches(m.logs, m.query)
				m.mi = min(m.mi, max(0, len(m.matches)-1))
			}
			m = m.withScreenData(viewDiff, st)
			if p.threadErr != nil {
				m.status = styErr.Render("couldn't load review comments: " + p.threadErr.Error())
			}
			return m.syncDiffLayout()
		},
		key:    diffKey,
		body:   diffBody,
		footer: "j/k scroll  ]/[ file  }/{ hunk  tab files  v viewed  s side-by-side  / search  n/N  o browser",
	})
}

func plusMinus(f diff.File) string { return fmt.Sprintf("+%d −%d", f.Added, f.Removed) }

// srcKey identifies a source line of the diff (idx 0 is the file's header) in a way that grows
// with the position, so rows of the two layouts can be matched up.
func srcKey(file, idx int) int { return file<<20 | idx }

// diffDisplay lays the diff out as display lines and reports where each file starts.
func diffDisplay(d diff.Diff) (lines []dline, starts []int) {
	lines, starts, _ = diffDisplaySrc(d)
	return
}

// diffDisplaySrc is diffDisplay that also gives each display row's source key.
func diffDisplaySrc(d diff.Diff) (lines []dline, starts []int, src []int) {
	if len(d.Files) == 0 {
		return []dline{{"(no changes)", dNote}}, nil, []int{0}
	}
	emit := func(l dline, key int) {
		lines = append(lines, l)
		src = append(src, key)
	}
	for i, f := range d.Files {
		if i > 0 {
			emit(dline{"", dBlank}, srcKey(i, 0))
		}
		starts = append(starts, len(lines))
		name := f.Path
		if f.Renamed {
			name = f.OldPath + " → " + f.Path
		}
		if f.Binary {
			name += " (binary)"
		}
		emit(dline{fmt.Sprintf("── %s  %s ──", name, plusMinus(f)), dFile}, srcKey(i, 0))
		crMark, crNote := lineEndingPairs(f.Lines)
		for li, l := range f.Lines {
			key := srcKey(i, li+1)
			text := strings.ReplaceAll(l.Text, "\t", "    ")
			if crMark[li] {
				text += " ␍"
			}
			switch l.Kind {
			case diff.Add:
				emit(dline{"+ " + text, dAdd}, key)
				if crNote[li] {
					emit(dline{"    ↳ only the line ending changed (CRLF ↔ LF)", dMeta}, key)
				}
			case diff.Del:
				emit(dline{"- " + text, dDel}, key)
			case diff.Context:
				emit(dline{"  " + text, dCtx}, key)
			case diff.Hunk:
				emit(dline{text, dHunk}, key)
			default:
				emit(dline{"  " + text, dMeta}, key)
			}
		}
	}
	if d.Truncated {
		end := srcKey(len(d.Files), 0)
		emit(dline{"", dBlank}, end)
		emit(dline{fmt.Sprintf("… diff truncated at %d of %d lines; press o to see the whole PR in the browser", maxDiffLines, d.TotalLines), dNote}, end)
	}
	return lines, starts, src
}

func diffKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	if next, cmd, ok := reviewKey(m, k); ok {
		return next, cmd, true
	}
	st, _ := m.screenData(viewDiff).(diffState)
	starts := st.starts
	if st.sb != nil {
		starts = st.sb.starts
	}
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
			m = m.open(pd.d.URL)
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
	if st.sb != nil {
		return sideBody(m, st)
	}
	h := m.logHeight()
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.Err.Render(" "+m.err.Error()) + "\n")
		h--
		if diffTooLarge(m.err) {
			b.WriteString(ui.Dim.Render(" The diff is too large for GitHub to send. Press o to see the PR in the browser.") + "\n")
			h--
		}
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
		emit(ui.Cols.Render(fmt.Sprintf(" files (%d)", len(st.d.Files))))
		for i, f := range st.d.Files {
			line := fmt.Sprintf("%s  %s", f.Path, plusMinus(f))
			if i == st.sel {
				emit(ui.Sel.Render(" ▸ " + strings.TrimRight(trunc(line, w-3), " ")))
			} else {
				emit("   " + strings.TrimRight(trunc(line, w-3), " "))
			}
		}
	default:
		for i := m.top; i < len(st.lines) && n < h; i++ {
			emit(m.styledDiffLine(i, st.lines[i], w))
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

// lineEndingPairs finds removed/added line pairs that differ only in a trailing carriage return,
// which would otherwise show as two identical lines. crMark marks the line that has the CR;
// crNote marks the added line after which an explanation is shown.
func lineEndingPairs(ls []diff.Line) (crMark, crNote map[int]bool) {
	crMark, crNote = map[int]bool{}, map[int]bool{}
	for i := 0; i < len(ls); {
		if ls[i].Kind != diff.Del {
			i++
			continue
		}
		var dels, adds []int
		for ; i < len(ls) && ls[i].Kind == diff.Del; i++ {
			dels = append(dels, i)
		}
		for ; i < len(ls) && ls[i].Kind == diff.Add; i++ {
			adds = append(adds, i)
		}
		for k := 0; k < len(dels) && k < len(adds); k++ {
			d, a := ls[dels[k]], ls[adds[k]]
			if d.Text == a.Text && d.CR != a.CR {
				crMark[dels[k]], crMark[adds[k]] = d.CR, a.CR
				crNote[adds[k]] = true
			}
		}
	}
	return crMark, crNote
}

// diffTooLarge reports whether GitHub refused the diff because of its size.
func diffTooLarge(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "406") || strings.Contains(s, "too large") || strings.Contains(s, "exceeded the maximum")
}
