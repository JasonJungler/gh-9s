package graph

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Style is how a graph is drawn.
type Style int

const (
	StyleBoxes Style = iota // A: left-to-right boxes with arrows (default)
	StyleTree               // B: top-to-bottom tree with dependencies
	StyleLanes              // C: timeline lanes
)

// Next cycles boxes, tree, lanes.
func (s Style) Next() Style { return (s + 1) % 3 }

// String names the style.
func (s Style) String() string {
	return [...]string{"boxes", "tree", "lanes"}[s]
}

// Render draws g in the requested style, at most `width` columns wide.
// Boxes that do not fit are drawn as a tree; used says what was drawn.
// sel is the selected node index (out of range = none).
func Render(g Graph, style Style, sel, width int, now time.Time) (lines []string, used Style) {
	if width <= 0 {
		width = 80
	}
	if len(g.Nodes) == 0 {
		return []string{ui.Dim.Render("no jobs yet")}, style
	}
	switch style {
	case StyleBoxes:
		if l, ok := renderBoxes(g, sel, width, now); ok {
			return l, StyleBoxes
		}
		return renderTree(g, sel, width, now), StyleTree
	case StyleLanes:
		return renderLanes(g, sel, width, now), StyleLanes
	}
	return renderTree(g, sel, width, now), StyleTree
}

// ---- shared helpers ----

func clip(s string, w int) string {
	n := utf8.RuneCountInString(s)
	switch {
	case w <= 0:
		return ""
	case n <= w:
		return s
	case w == 1:
		return "…"
	}
	return string([]rune(s)[:w-1]) + "…"
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func nodeDur(n Node, now time.Time) string {
	if n.Start.IsZero() {
		return ""
	}
	end := n.End
	if end.IsZero() || end.Before(n.Start) {
		end = now
	}
	if end.Before(n.Start) {
		return ""
	}
	return fmtDur(end.Sub(n.Start))
}

// nodeParts splits a node's text into a status symbol and the rest.
func nodeParts(n Node, now time.Time) (sym, rest string) {
	sym = ui.Symbol[n.State]
	if len(n.Members) == 0 {
		if n.Start.IsZero() && n.State == ui.StRun {
			sym = "○" // queued or waiting on dependencies
		}
		rest = n.Name
	} else {
		rest = n.Name + " " + n.Label
	}
	if d := nodeDur(n, now); d != "" {
		rest += " " + d
	}
	return sym, rest
}

func marker(i, sel int) string {
	if i == sel {
		return "▸ "
	}
	return "  "
}

// colorRune styles the rune at idx of an already-clipped plain line.
func colorRune(line string, idx int, st ui.State) string {
	r := []rune(line)
	if idx < 0 || idx >= len(r) {
		return line
	}
	return string(r[:idx]) + ui.StateStyle(st).Render(string(r[idx])) + string(r[idx+1:])
}

// ---- B: tree ----

func renderTree(g Graph, sel, width int, now time.Time) []string {
	depth, _ := g.depths()
	children := make([][]int, len(g.Nodes))
	var roots []int
	for i, n := range g.Nodes {
		if len(n.Needs) == 0 {
			roots = append(roots, i)
			continue
		}
		p := n.Needs[0]
		for _, d := range n.Needs[1:] {
			if depth[d] > depth[p] || (depth[d] == depth[p] && d > p) {
				p = d
			}
		}
		children[p] = append(children[p], i)
	}
	var out []string
	var walk func(i int, prefix, connector string, last, root bool)
	walk = func(i int, prefix, connector string, last, root bool) {
		n := g.Nodes[i]
		sym, rest := nodeParts(n, now)
		if n.Step != "" && (n.State == ui.StRun || n.State == ui.StFail) {
			rest += " ▸ " + n.Step
		}
		if len(n.Needs) > 1 {
			var names []string
			for _, d := range n.Needs {
				names = append(names, g.Nodes[d].Name)
			}
			rest += " (needs " + strings.Join(names, ", ") + ")"
		}
		head := marker(i, sel) + prefix + connector
		line := clip(head+sym+" "+rest, width)
		out = append(out, colorRune(line, utf8.RuneCountInString(head), n.State))
		childPrefix := prefix
		if !root {
			if last {
				childPrefix += "   "
			} else {
				childPrefix += "│  "
			}
		}
		for k, c := range children[i] {
			conn := "├─ "
			if k == len(children[i])-1 {
				conn = "└─ "
			}
			walk(c, childPrefix, conn, k == len(children[i])-1, false)
		}
	}
	for _, r := range roots {
		walk(r, "", "", true, true)
	}
	return out
}

// ---- C: timeline lanes ----

func renderLanes(g Graph, sel, width int, now time.Time) []string {
	nameW := 4
	for _, n := range g.Nodes {
		nameW = max(nameW, min(utf8.RuneCountInString(n.Name), 20))
	}
	barW := max(8, width-13-nameW)
	var t0, t1 time.Time
	for _, n := range g.Nodes {
		if n.Start.IsZero() {
			continue
		}
		if t0.IsZero() || n.Start.Before(t0) {
			t0 = n.Start
		}
		end := n.End
		if end.IsZero() {
			end = now
		}
		if end.After(t1) {
			t1 = end
		}
	}
	span := t1.Sub(t0)
	if t0.IsZero() || span <= 0 {
		span = time.Second
	}
	col := func(t time.Time) int {
		c := int(float64(t.Sub(t0)) / float64(span) * float64(barW-1))
		return min(max(c, 0), barW-1)
	}
	var out []string
	for i, n := range g.Nodes {
		sym, _ := nodeParts(n, now)
		bar := []rune(strings.Repeat(" ", barW))
		if n.Start.IsZero() {
			for c := max(0, barW-5); c < barW; c++ {
				bar[c] = '·'
			}
		} else {
			s, e := col(n.Start), col(n.End)
			if n.End.IsZero() {
				e = col(now)
			}
			e = max(e, s)
			for c := s; c <= e; c++ {
				bar[c] = '█'
			}
			if n.End.IsZero() {
				bar[e] = '▌'
			}
		}
		name := clip(n.Name, nameW)
		line := fmt.Sprintf("%s%s %-*s %s %s", marker(i, sel), sym, nameW, name, string(bar), nodeDur(n, now))
		line = clip(strings.TrimRight(line, " "), width)
		out = append(out, colorRune(line, 2, n.State))
	}
	total := fmtDur(span)
	pad := max(1, barW-2-utf8.RuneCountInString(total))
	axis := strings.Repeat(" ", 4+nameW+1) + "0s" + strings.Repeat(" ", pad) + total
	return append(out, ui.Dim.Render(clip(axis, width)))
}

// ---- A: boxes on a canvas ----

const (
	mN uint8 = 1 << iota
	mE
	mS
	mW
)

var junction = map[uint8]rune{
	mE: '─', mW: '─', mE | mW: '─', mN: '│', mS: '│', mN | mS: '│',
	mE | mS: '┌', mS | mW: '┐', mN | mE: '└', mN | mW: '┘',
	mN | mE | mS: '├', mN | mS | mW: '┤', mE | mS | mW: '┬', mN | mE | mW: '┴',
	mN | mE | mS | mW: '┼',
}

type cell struct {
	r    rune  // text/box character; 0 = none
	mask uint8 // connector directions when r == 0
	st   ui.State
	col  bool
}

type canvas [][]cell

func newCanvas(w, h int) canvas {
	c := make(canvas, h)
	for y := range c {
		c[y] = make([]cell, w)
	}
	return c
}

func (c canvas) put(x, y int, r rune, st ui.State, col bool) {
	if y >= 0 && y < len(c) && x >= 0 && x < len(c[y]) {
		c[y][x] = cell{r: r, st: st, col: col}
	}
}

func (c canvas) hline(x0, x1, y int, st ui.State) {
	for x := x0; x <= x1; x++ {
		if y < 0 || y >= len(c) || x < 0 || x >= len(c[y]) || c[y][x].r != 0 {
			continue
		}
		if x > x0 {
			c[y][x].mask |= mW
		}
		if x < x1 {
			c[y][x].mask |= mE
		}
		c[y][x].st, c[y][x].col = st, true
	}
}

func (c canvas) vline(x, y0, y1 int, st ui.State) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		if y < 0 || y >= len(c) || x < 0 || x >= len(c[y]) || c[y][x].r != 0 {
			continue
		}
		if y > y0 {
			c[y][x].mask |= mN
		}
		if y < y1 {
			c[y][x].mask |= mS
		}
		c[y][x].st, c[y][x].col = st, true
	}
}

func (c canvas) lines() []string {
	out := make([]string, len(c))
	for y, row := range c {
		var b strings.Builder
		var run []rune
		var cur cell
		flush := func() {
			if len(run) == 0 {
				return
			}
			s := string(run)
			if cur.col {
				s = ui.StateStyle(cur.st).Render(s)
			}
			b.WriteString(s)
			run = run[:0]
		}
		for _, ce := range row {
			r := ce.r
			if r == 0 && ce.mask != 0 {
				r = junction[ce.mask]
			}
			if r == 0 {
				r, ce.col = ' ', false
			}
			if len(run) > 0 && (ce.col != cur.col || (ce.col && ce.st != cur.st)) {
				flush()
			}
			cur = ce
			run = append(run, r)
		}
		flush()
		out[y] = strings.TrimRight(b.String(), " ")
	}
	return out
}

// Layout rules that keep the drawn lines honest:
//   - every job that has dependents gets its own vertical channel in the gap after its stage,
//     so lines of different jobs never share a column and merge;
//   - lines are drawn only between neighbouring stages; a dependency that skips a stage is
//     written in the dependent's box ("← lint") instead of being routed behind other boxes;
//   - the rows of consecutive stages are staggered (stage s starts s%4 rows lower), so a line
//     leaving a job never runs along the row of an unrelated job in the next stage.
//
// Lines may cross each other (┼); crossing lines go straight through.
const boxLead = 4 // cells in a gap besides the channels: 1 lead-in, 2 lead-out, 1 arrow

// boxLayout is the geometry of the box style with its edges already drawn on cv
// (the boxes themselves are drawn afterwards, over the parts of edges that pass behind them).
type boxLayout struct {
	cv      canvas
	stages  [][]int
	stageOf []int
	texts   []string
	x, bw   []int // per stage: left column and box width
	y       []int // per node: top row of its box
	total   int   // width of the whole drawing
}

func layoutBoxes(g Graph, now time.Time) boxLayout {
	prevChannels := 0
	l := boxLayout{stages: g.stages(), stageOf: make([]int, len(g.Nodes)), texts: make([]string, len(g.Nodes)), y: make([]int, len(g.Nodes))}
	for s, st := range l.stages {
		for _, i := range st {
			l.stageOf[i] = s
		}
	}
	hasDependents := make([]bool, len(g.Nodes)) // has a dependent in the very next stage
	for i, n := range g.Nodes {
		sym, rest := nodeParts(n, now)
		l.texts[i] = sym + " " + rest
		var skipped []string
		for _, u := range n.Needs {
			if l.stageOf[i]-l.stageOf[u] == 1 {
				hasDependents[u] = true
			} else {
				skipped = append(skipped, g.Nodes[u].Name)
			}
		}
		if len(skipped) > 0 {
			l.texts[i] += " ← " + strings.Join(skipped, ", ")
		}
	}
	l.x = make([]int, len(l.stages))
	l.bw = make([]int, len(l.stages))
	channel := make([]int, len(g.Nodes)) // per node: its channel index within the gap after its stage
	height := 3
	for s, st := range l.stages {
		channels := 0
		for k, i := range st {
			l.bw[s] = max(l.bw[s], utf8.RuneCountInString(l.texts[i])+4)
			l.y[i] = k*4 + s%4
			height = max(height, l.y[i]+3)
			if hasDependents[i] {
				channel[i] = channels
				channels++
			}
		}
		if s > 0 {
			l.x[s] = l.x[s-1] + l.bw[s-1] + prevChannels + boxLead
		}
		prevChannels = channels
	}
	last := len(l.stages) - 1
	l.total = l.x[last] + l.bw[last]
	l.cv = newCanvas(l.total, height)
	for v, n := range g.Nodes {
		for _, u := range n.Needs {
			su, sv := l.stageOf[u], l.stageOf[v]
			if sv-su != 1 {
				continue // written in the box instead
			}
			xr := l.x[su] + l.bw[su] - 1
			xc := xr + 2 + channel[u]
			yu, yv := l.y[u]+1, l.y[v]+1
			st := g.Nodes[u].State
			if yu == yv {
				l.cv.hline(xr+1, l.x[sv]-2, yu, st)
			} else {
				l.cv.hline(xr+1, xc, yu, st)
				l.cv.vline(xc, yu, yv, st)
				l.cv.hline(xc, l.x[sv]-2, yv, st)
			}
			l.cv.put(l.x[sv]-1, yv, '▶', st, true)
		}
	}
	return l
}

func renderBoxes(g Graph, sel, width int, now time.Time) ([]string, bool) {
	l := layoutBoxes(g, now)
	if l.total > width {
		return nil, false
	}
	for s, st := range l.stages {
		for _, i := range st {
			tl, tr, bl, br, h, vb := '┌', '┐', '└', '┘', '─', '│'
			if i == sel {
				tl, tr, bl, br, h, vb = '╔', '╗', '╚', '╝', '═', '║'
			}
			n, x0, y0, w := g.Nodes[i], l.x[s], l.y[i], l.bw[s]
			for dx := 0; dx < w; dx++ {
				l.cv.put(x0+dx, y0, h, n.State, true)
				l.cv.put(x0+dx, y0+2, h, n.State, true)
				l.cv.put(x0+dx, y0+1, ' ', n.State, false)
			}
			l.cv.put(x0, y0, tl, n.State, true)
			l.cv.put(x0+w-1, y0, tr, n.State, true)
			l.cv.put(x0, y0+2, bl, n.State, true)
			l.cv.put(x0+w-1, y0+2, br, n.State, true)
			l.cv.put(x0, y0+1, vb, n.State, true)
			l.cv.put(x0+w-1, y0+1, vb, n.State, true)
			for k, r := range []rune(l.texts[i]) {
				l.cv.put(x0+2+k, y0+1, r, n.State, k == 0) // only the status symbol is coloured
			}
		}
	}
	return l.cv.lines(), true
}
