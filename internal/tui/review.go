package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/MurabitoB/gh-9s/internal/icons"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/highlight"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// sideMinWidth is the narrowest terminal that gets the side-by-side view on its own.
const (
	sideMinWidth  = 120
	sideMinForced = 60 // below this the list column and two panes cannot fit
)

// sbLayout is the side-by-side layout of the whole diff for one terminal width.
type sbLayout struct {
	width  int
	lw     int // width of the file list column
	lines  []sbLine
	starts []int    // row where each file starts
	src    []int    // source key of every row
	src2   []int    // the higher source key of a paired row (same as src for the others)
	logs   []string // plain text of every row, for search
}

func listWidth(width int) int {
	if width >= 160 {
		return 40
	}
	return 32
}

// sideMode says whether the diff is shown side by side at the current width.
func (m model) sideMode(st diffState) bool {
	if len(st.d.Files) == 0 || m.width < sideMinForced {
		return false
	}
	return st.sbs == 1 || (st.sbs == 0 && m.width >= sideMinWidth)
}

// What may be coloured by language; the rest of a diff stays plain.
var (
	maxHighlightBytes = 2 << 20
	highlightTime     = 3 * time.Second
)

// highlightFiles colours the files of the diff within a byte and time budget. It runs while the diff
// is fetched, never on the UI goroutine: lexers are third-party code and can be slow on odd text.
func highlightFiles(d diff.Diff) []*highlight.File {
	out := make([]*highlight.File, len(d.Files))
	bytes, deadline := maxHighlightBytes, time.Now().Add(highlightTime)
	for i, f := range d.Files {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		if size := highlight.Size(f); size > bytes {
			continue
		} else if out[i] = highlight.ForFileWithin(f, min(left, time.Second)); out[i] != nil {
			bytes -= size
		}
	}
	return out
}

// syntaxDiffLine draws a unified-diff row in syntax colours; ok is false when the row has none.
func (m model) syntaxDiffLine(i int, l dline, w int) (string, bool) {
	if l.kind != dAdd && l.kind != dDel && l.kind != dCtx {
		return "", false
	}
	st, _ := m.screenData(viewDiff).(diffState)
	if i >= len(st.usrc) {
		return "", false
	}
	file, idx := st.usrc[i]>>20, st.usrc[i]&(1<<20-1)-1
	if file >= len(st.hl) || st.hl[file] == nil || idx < 0 {
		return "", false
	}
	var segs []highlight.Seg
	kind, sign, signSty := diff.Context, "  ", lipgloss.NewStyle()
	switch l.kind {
	case dAdd:
		kind, sign, signSty = diff.Add, "+ ", ui.OK
		segs = st.hl[file].Right(idx)
	case dDel:
		kind, sign, signSty = diff.Del, "- ", ui.Fail
		segs = st.hl[file].Left(idx)
	default:
		segs = st.hl[file].Right(idx)
	}
	var text strings.Builder
	for _, s := range segs {
		text.WriteString(s.Text)
	}
	if segs == nil || sign+text.String() != l.text { // a line-ending mark or anything else odd: leave it plain
		return "", false
	}
	return " " + signSty.Render(sign) + syntaxText(segs, nil, kind, max(w-3, 1)), true
}

func buildSB(st diffState, width int) *sbLayout {
	lw := listWidth(width)
	rw := width - lw - 1
	pw := max((rw-1)/2, 10)
	l := &sbLayout{width: width, lw: lw}
	add2 := func(row sbLine, key, key2 int) {
		l.lines = append(l.lines, row)
		l.src = append(l.src, key)
		l.src2 = append(l.src2, key2)
		l.logs = append(l.logs, row.plain())
	}
	add := func(row sbLine, key int) { add2(row, key, key) }
	for fi, f := range st.d.Files {
		if fi > 0 {
			add(sbLine{left: fit("", pw), right: fit("", pw), kind: dBlank}, srcKey(fi, 0))
		}
		l.starts = append(l.starts, len(l.lines))
		name := f.Path
		if f.Renamed {
			name = f.OldPath + " → " + f.Path
		}
		if f.Binary {
			name += " (binary)"
		}
		add(sbLine{left: fit(ui.Key.Render(fmt.Sprintf("── %s  %s ──", name, plusMinus(f))), pw), right: fit("", pw), kind: dFile}, srcKey(fi, 0))
		var hf *highlight.File
		if fi < len(st.hl) {
			hf = st.hl[fi]
		}
		for _, row := range sideBySideHL(f, allThreads(st), rw, hf) {
			add2(row, srcKey(fi, row.idx), srcKey(fi, row.idx2))
		}
	}
	if st.d.Truncated {
		end := srcKey(len(st.d.Files), 0)
		add(sbLine{left: fit("", pw), right: fit("", pw), kind: dBlank}, end)
		add(sbLine{left: fit(styDim.Render(fmt.Sprintf("… diff truncated at %d of %d lines; press o for the whole PR in the browser", maxDiffLines, st.d.TotalLines)), pw), right: fit("", pw), kind: dNote}, end)
	}
	return l
}

func firstAtLeast(src []int, key int) int {
	for i, k := range src {
		if k >= key {
			return i
		}
	}
	return max(len(src)-1, 0)
}

// rowFor finds the side-by-side row that shows the source line key: a row holding it as either
// of its two lines, else the last row before it.
func (l *sbLayout) rowFor(key int) int {
	last := 0
	for i := range l.src {
		if l.src[i] == key || l.src2[i] == key {
			return i
		}
		if l.src[i] < key {
			last = i
		}
	}
	return last
}

// syncDiffLayout makes the layout match the terminal width and the user's choice, keeping the same
// line of the diff at the top of the screen.
func (m model) syncDiffLayout() model {
	st, _ := m.screenData(viewDiff).(diffState)
	if len(st.d.Files) == 0 {
		return m
	}
	want := m.sideMode(st)
	if want == (st.sb != nil) && (!want || st.sb.width == m.width) {
		return m
	}
	key := 0
	switch {
	case st.sb != nil && m.top < len(st.sb.src):
		key = st.sb.src[m.top]
	case st.sb == nil && m.top < len(st.usrc):
		key = st.usrc[m.top]
	}
	if want {
		st.sb = buildSB(st, m.width)
		st.listMode = false // the unified file list has no place in this layout
		m.logs = st.sb.logs
		m.top = st.sb.rowFor(key)
		st.cur = st.sb.rowFor(st.curKey)
	} else {
		st.sb = nil
		m.logs = make([]string, len(st.lines))
		for i, l := range st.lines {
			m.logs[i] = l.text
		}
		m.top = firstAtLeast(st.usrc, key)
		st.cur = firstAtLeast(st.usrc, st.curKey)
	}
	m.top = min(m.top, m.maxTop())
	if h := m.logHeight(); st.cur < m.top || st.cur >= m.top+h { // the same lines take more rows now: the view stays, the cursor is pulled in
		st = setCursor(st, min(max(st.cur, m.top), min(m.top+h-1, len(m.logs)-1)))
	}
	if m.query != "" {
		m.matches = searchMatches(m.logs, m.query)
		m.mi = min(m.mi, max(0, len(m.matches)-1))
	}
	return m.withScreenData(viewDiff, st)
}

func fileAt(starts []int, row int) int {
	at := 0
	for i, s := range starts {
		if s <= row {
			at = i
		}
	}
	return at
}

// reviewKey handles the keys of the review screen; ok is false for everything else.
func reviewKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewDiff).(diffState)
	if len(st.d.Files) == 0 || st.listMode || m.err != nil || (m.loading && len(m.logs) == 0) {
		return m, nil, false
	}
	side := st.sb != nil
	starts := st.starts
	if side {
		starts = st.sb.starts
	}
	if nm, cmd, ok := draftKeys(m, st, k.String()); ok {
		return nm, cmd, true
	}
	if !(side && st.focusList) {
		if nm, nst, ok := moveCursor(m, st, k.String()); ok {
			return nm.withScreenData(viewDiff, nst), nil, true
		}
	}
	switch key := k.String(); {
	case key == "s":
		if m.width < sideMinForced {
			m.status = styDim.Render(fmt.Sprintf("side-by-side needs %d+ columns", sideMinForced))
			return m, nil, true
		}
		if side {
			st.sbs = 2
		} else {
			st.sbs = 1
		}
		return m.withScreenData(viewDiff, st).syncDiffLayout(), nil, true
	case key == "tab" && side, key == "f" && side:
		st.focusList = !st.focusList
		st.sel = fileAt(starts, m.top)
		return m.withScreenData(viewDiff, st), nil, true
	case key == "v":
		fi := fileAt(starts, m.top)
		if side && st.focusList {
			fi = st.sel
		}
		path := st.d.Files[fi].Path
		nv := map[string]bool{}
		for p, v := range st.viewed {
			nv[p] = v
		}
		if nv[path] {
			delete(nv, path)
		} else {
			nv[path] = true
		}
		st.viewed = nv
		return m.withScreenData(viewDiff, st), nil, true
	case key == "}" || key == "{":
		isHunk := func(i int) bool {
			if side {
				return st.sb.lines[i].hunk
			}
			return st.lines[i].kind == dHunk
		}
		n := len(m.logs)
		if key == "}" {
			for i := m.top + 1; i < n; i++ {
				if isHunk(i) {
					m.top = min(i, m.maxTop())
					break
				}
			}
		} else {
			for i := m.top - 1; i >= 0; i-- {
				if isHunk(i) {
					m.top = i
					break
				}
			}
		}
		return m, nil, true
	case side && st.focusList:
		switch key {
		case "j", "down", "k", "up", "g", "G", "home", "end":
			switch key {
			case "j", "down":
				st.sel = min(st.sel+1, len(starts)-1)
			case "k", "up":
				st.sel = max(st.sel-1, 0)
			case "g", "home":
				st.sel = 0
			default:
				st.sel = len(starts) - 1
			}
			m.top = min(starts[st.sel], m.maxTop())
			return m.withScreenData(viewDiff, st), nil, true
		case "enter", "esc":
			st.focusList = false
			return m.withScreenData(viewDiff, st), nil, true
		}
	}
	return m, nil, false
}

// listLine draws one entry of the file list column.
func listLine(f diff.File, threads []ghcli.Thread, nDrafts int, selected, viewed bool, w int) string {
	mark := " "
	if selected {
		mark = icons.Get(icons.Selected)
	}
	seen := " "
	if viewed {
		seen = icons.Get(icons.Viewed)
	}
	stats := " " + plusMinus(f)
	if n := threadCount(threads, f.Path); n > 0 {
		stats += fmt.Sprintf(" ◆%d", n)
	}
	if nDrafts > 0 {
		stats += fmt.Sprintf(" ✎%d", nDrafts)
	}
	name := f.Path
	avail := w - 3 - ansi.StringWidth(stats)
	if ansi.StringWidth(name) > avail && avail > 1 {
		r := []rune(name)
		for ansi.StringWidth("…"+string(r)) > avail && len(r) > 0 {
			r = r[1:]
		}
		name = "…" + string(r)
	}
	line := mark + seen + " " + fit(name, max(avail, 1)) + stats
	if selected {
		return ui.Sel.Render(fit(line, w))
	}
	if viewed {
		return styDim.Render(fit(line, w))
	}
	return fit(line, w)
}

func threadCount(ts []ghcli.Thread, path string) int {
	n := 0
	for _, t := range ts {
		if t.Path == path {
			n++
		}
	}
	return n
}

// sideBody draws the file list and the side-by-side diff.
func sideBody(m model, st diffState) string {
	h := m.logHeight()
	l := st.sb
	cur := fileAt(l.starts, m.top)
	sel := cur
	if st.focusList {
		sel = st.sel
	}
	listTop := 0
	if len(st.d.Files) > h {
		listTop = max(0, min(sel-h/2, len(st.d.Files)-h))
	}
	var b strings.Builder
	for n := 0; n < h; n++ {
		list := strings.Repeat(" ", l.lw)
		if fi := listTop + n; fi < len(st.d.Files) {
			f := st.d.Files[fi]
			nd := 0
			for _, d := range st.drafts {
				if d.Path == f.Path {
					nd++
				}
			}
			list = listLine(f, st.threads, nd, fi == sel, st.viewed[f.Path], l.lw)
		}
		row := ""
		if i := m.top + n; i < len(l.lines) {
			r := l.lines[i]
			sep := styDim.Render("│")
			switch {
			case i == st.cur:
				sep = ui.Key.Render("▶")
			case inRange(st, i):
				sep = ui.Key.Render("▌")
			}
			row = r.left + sep + r.right
			switch {
			case m.isMatch(i) && m.matches[m.mi] == i:
				row = ui.Sel.Render(ansi.Strip(row))
			case m.isMatch(i):
				row = ui.Key.Render(ansi.Strip(row))
			}
		}
		b.WriteString(list + styDim.Render("│") + row + "\n")
	}
	return b.String()
}
