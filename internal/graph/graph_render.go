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
		done := 0
		for _, m := range n.Members {
			if _, s := m.State(); s == ui.StOK {
				done++
			}
		}
		rest = fmt.Sprintf("%s %d/%d", n.Name, done, len(n.Members))
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

const boxGap = 6

func renderBoxes(g Graph, sel, width int, now time.Time) ([]string, bool) {
	stages := g.stages()
	stageOf := make([]int, len(g.Nodes))
	texts := make([]string, len(g.Nodes))
	for s, st := range stages {
		for _, i := range st {
			stageOf[i] = s
		}
	}
	for i, n := range g.Nodes {
		sym, rest := nodeParts(n, now)
		texts[i] = sym + " " + rest
	}
	x := make([]int, len(stages))
	bw := make([]int, len(stages))
	rows := 1
	for s, st := range stages {
		for _, i := range st {
			bw[s] = max(bw[s], utf8.RuneCountInString(texts[i])+4)
		}
		rows = max(rows, len(st))
		if s > 0 {
			x[s] = x[s-1] + bw[s-1] + boxGap
		}
	}
	last := len(stages) - 1
	total := x[last] + bw[last]
	if total > width {
		return nil, false
	}
	cv := newCanvas(total, rows*4-1)
	y := make([]int, len(g.Nodes))
	for _, st := range stages {
		for k, i := range st {
			y[i] = k * 4
		}
	}
	// edges first, so boxes drawn afterwards hide the parts that pass behind them
	for v, n := range g.Nodes {
		for _, u := range n.Needs {
			su := stageOf[u]
			xr := x[su] + bw[su] - 1
			xc := xr + 3
			yu, yv := y[u]+1, y[v]+1
			st := g.Nodes[u].State
			if yu == yv {
				cv.hline(xr+1, x[stageOf[v]]-2, yu, st)
			} else {
				cv.hline(xr+1, xc, yu, st)
				cv.vline(xc, yu, yv, st)
				cv.hline(xc, x[stageOf[v]]-2, yv, st)
			}
			cv.put(x[stageOf[v]]-1, yv, '▶', st, true)
		}
	}
	for s, st := range stages {
		for _, i := range st {
			tl, tr, bl, br, h, vb := '┌', '┐', '└', '┘', '─', '│'
			if i == sel {
				tl, tr, bl, br, h, vb = '╔', '╗', '╚', '╝', '═', '║'
			}
			n, x0, y0, w := g.Nodes[i], x[s], y[i], bw[s]
			for dx := 0; dx < w; dx++ {
				cv.put(x0+dx, y0, h, n.State, true)
				cv.put(x0+dx, y0+2, h, n.State, true)
				cv.put(x0+dx, y0+1, ' ', n.State, false)
			}
			cv.put(x0, y0, tl, n.State, true)
			cv.put(x0+w-1, y0, tr, n.State, true)
			cv.put(x0, y0+2, bl, n.State, true)
			cv.put(x0+w-1, y0+2, br, n.State, true)
			cv.put(x0, y0+1, vb, n.State, true)
			cv.put(x0+w-1, y0+1, vb, n.State, true)
			for k, r := range []rune(texts[i]) {
				colored := k == 0 // the status symbol
				cv.put(x0+2+k, y0+1, r, n.State, colored)
			}
		}
	}
	return cv.lines(), true
}
