package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/highlight"
	"github.com/MurabitoB/gh-9s/internal/markdown"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// sbLine is one row of the side-by-side view. left and right are styled and exactly one pane wide.
type sbLine struct {
	left, right string
	kind        dkind
	thread      bool // a comment from a review thread
	hunk        bool // a hunk header
	idx, idx2   int  // 1+index in File.Lines of the row's lines (the lower and the higher; thread rows share them); 0 none
}

func hlDel(s string) string { return ui.Fail.Reverse(true).Render(s) }
func hlAdd(s string) string { return ui.OK.Reverse(true).Render(s) }

// plain is the row's text without styling, for search.
func (l sbLine) plain() string {
	return strings.TrimRight(ansi.Strip(l.left), " ") + " │ " + strings.TrimRight(ansi.Strip(l.right), " ")
}

// fit cuts s to w columns (ending in … when it was longer) and pads it with spaces to exactly w.
func fit(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// sideBySide lays one file out in two panes of (width-1)/2 columns, old on the left and new on the
// right, with the review threads other people started shown under the lines they are about.
func sideBySide(f diff.File, threads []ghcli.Thread, width int) []sbLine {
	return sideBySideHL(f, threads, width, nil)
}

// sideBySideHL is sideBySide with the code coloured by language when hl is not nil.
func sideBySideHL(f diff.File, threads []ghcli.Thread, width int, hl *highlight.File) []sbLine {
	pw := max((width-1)/2, 10)
	rows := f.Rows()
	gw := 2
	for _, r := range rows {
		for _, n := range []*diff.NumLine{r.Old, r.New} {
			if n != nil {
				gw = max(gw, len(strconv.Itoa(max(n.Old, n.New))))
			}
		}
	}
	cw := max(pw-gw-3, 1)
	var mine, outdated []ghcli.Thread
	for _, t := range threads {
		switch {
		case t.Path != f.Path:
		case t.Outdated:
			outdated = append(outdated, t)
		default:
			mine = append(mine, t)
		}
	}
	var out []sbLine
	if f.Binary && len(f.Lines) == 0 {
		out = append(out, sbLine{left: fit(styDim.Render("(binary file)"), pw), right: fit("", pw), kind: dMeta})
	}
	for _, r := range rows {
		if r.Old == r.New && r.Old != nil && (r.Old.Kind == diff.Hunk || r.Old.Kind == diff.Meta) {
			k, st := dMeta, styDim
			if r.Old.Kind == diff.Hunk {
				k, st = dHunk, ui.Cols
			}
			out = append(out, sbLine{left: fit(st.Render(expandTabs(r.Old.Text)), pw), right: fit("", pw), kind: k, hunk: k == dHunk, idx: r.Old.Idx + 1, idx2: r.Old.Idx + 1})
			continue
		}
		var spOld, spNew []diff.Span
		if r.Old != nil && r.New != nil && r.Old.Kind == diff.Del && r.New.Kind == diff.Add {
			spOld, spNew = diff.WordDiff(expandTabs(r.Old.Text), expandTabs(r.New.Text))
		}
		kind := dCtx
		switch {
		case r.Old != nil && r.Old.Kind == diff.Del, r.New != nil && r.New.Kind == diff.Add:
			kind = dDel
			if r.Old == nil {
				kind = dAdd
			}
		}
		idx, idx2 := 0, 0
		for _, n := range []*diff.NumLine{r.Old, r.New} {
			if n != nil {
				if idx == 0 || n.Idx+1 < idx {
					idx = n.Idx + 1
				}
				idx2 = max(idx2, n.Idx+1)
			}
		}
		out = append(out, sbLine{left: cell(r.Old, true, spOld, gw, cw, hl), right: cell(r.New, false, spNew, gw, cw, hl), kind: kind, idx: idx, idx2: idx2})
		for _, t := range mine {
			if (t.Side == "RIGHT" && r.New != nil && r.New.New == t.Line) || (t.Side == "LEFT" && r.Old != nil && r.Old.Old == t.Line) {
				for _, tr := range threadRows(t, pw) {
					tr.idx, tr.idx2 = idx, idx2
					out = append(out, tr)
				}
			}
		}
	}
	for _, t := range outdated {
		for _, tr := range threadRows(t, pw) {
			tr.idx, tr.idx2 = len(f.Lines)+1, len(f.Lines)+1 // after every line of the file, so the keys stay in order
			out = append(out, tr)
		}
	}
	return out
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

// cell draws one pane of a row: line number, sign, text (with its changed words marked).
func cell(n *diff.NumLine, left bool, spans []diff.Span, gw, cw int, hf *highlight.File) string {
	if n == nil {
		return strings.Repeat(" ", gw+3+cw)
	}
	num := n.New
	if left {
		num = n.Old
	}
	sign, base, hl := " ", lipgloss.NewStyle(), func(s string) string { return s }
	switch n.Kind {
	case diff.Del:
		sign, base, hl = "-", ui.Fail, hlDel
	case diff.Add:
		sign, base, hl = "+", ui.OK, hlAdd
	}
	if segs := syntaxSegs(hf, n, left); segs != nil {
		return styDim.Render(fmt.Sprintf("%*d", gw, num)) + " " + base.Render(sign) + " " + syntaxText(segs, spans, n.Kind, cw)
	}
	text := []rune(expandTabs(n.Text))
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if sp.Start > pos {
			b.WriteString(base.Render(string(text[pos:sp.Start])))
		}
		b.WriteString(hl(string(text[sp.Start:min(sp.End, len(text))])))
		pos = min(sp.End, len(text))
	}
	if pos < len(text) {
		b.WriteString(base.Render(string(text[pos:])))
	}
	return styDim.Render(fmt.Sprintf("%*d", gw, num)) + " " + base.Render(sign) + " " + fit(b.String(), cw)
}

// threadRows draws a review thread in the pane of the side it is on, the other pane left blank.
func threadRows(t ghcli.Thread, pw int) []sbLine {
	var lines []string
	if t.Outdated {
		lines = append(lines, styDim.Render(fmt.Sprintf("(outdated) %s:%d", t.Path, t.Line)))
	}
	for _, c := range t.Comments {
		lines = append(lines, ui.Key.Render("┃ "+c.Author))
		for _, l := range markdown.Render(c.Body, max(pw-2, 1)).Lines {
			lines = append(lines, styDim.Render("┃ ")+l)
		}
	}
	var out []sbLine
	for _, l := range lines {
		row := sbLine{left: fit("", pw), right: fit("", pw), kind: dMeta, thread: true}
		if t.Side == "LEFT" {
			row.left = fit(l, pw)
		} else {
			row.right = fit(l, pw)
		}
		out = append(out, row)
	}
	return out
}

func syntaxSegs(hl *highlight.File, n *diff.NumLine, left bool) []highlight.Seg {
	if left {
		return hl.Left(n.Idx)
	}
	return hl.Right(n.Idx)
}

// tint is the background of added and deleted lines, and the stronger one under changed words.
func tint(kind diff.Kind, changed bool) lipgloss.Color {
	switch {
	case kind == diff.Add && changed:
		return "28"
	case kind == diff.Add:
		return "22"
	case kind == diff.Del && changed:
		return "88"
	case kind == diff.Del:
		return "52"
	}
	return ""
}

func segStyle(sg highlight.Seg, kind diff.Kind, changed bool) lipgloss.Style {
	st := lipgloss.NewStyle().Bold(sg.Bold).Italic(sg.Italic)
	if sg.FG != "" {
		st = st.Foreground(lipgloss.Color(sg.FG))
	}
	if bg := tint(kind, changed); bg != "" {
		st = st.Background(bg)
	}
	return st
}

// syntaxText draws a line from its coloured segments: syntax colour in front, the line's tint
// behind, a stronger tint under the changed words; cut or padded to exactly cw columns.
func syntaxText(segs []highlight.Seg, spans []diff.Span, kind diff.Kind, cw int) string {
	changedAt := func(i int) bool {
		for _, sp := range spans {
			if i >= sp.Start && i < sp.End {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	pos := 0
	for _, sg := range segs {
		r := []rune(sg.Text)
		for start := 0; start < len(r); {
			ch := changedAt(pos + start)
			end := start + 1
			for end < len(r) && changedAt(pos+end) == ch {
				end++
			}
			b.WriteString(segStyle(sg, kind, ch).Render(string(r[start:end])))
			start = end
		}
		pos += len(r)
	}
	s := b.String()
	if ansi.StringWidth(s) > cw {
		s = ansi.Truncate(s, cw, "…")
	}
	pad := strings.Repeat(" ", max(cw-ansi.StringWidth(s), 0))
	if bg := tint(kind, false); bg != "" {
		pad = lipgloss.NewStyle().Background(bg).Render(pad)
	}
	return s + pad
}
