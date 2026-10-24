package tui

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// treeSig is what the rows of the tree depend on; two equal signatures mean the same rows.
type treeSig struct {
	files, viewed, folded uintptr
	nFiles, nThreads      int
	nDrafts               int
	threadHash, draftHash uint32
}

var treeMemo struct {
	sync.Mutex
	sig  treeSig
	rows []treeRow
	ok   bool
}

func hashStrings(n int, at func(int) string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < n; i++ {
		for _, c := range []byte(at(i)) {
			h = (h ^ uint32(c)) * 16777619
		}
		h = (h ^ 0xff) * 16777619
	}
	return h
}

// treeRows is the visible rows of the file tree, rebuilt only when the files, the viewed marks, the
// comments, the drafts or the folds changed.
func (st diffState) treeRows() []treeRow {
	sig := treeSig{
		files: reflect.ValueOf(st.d.Files).Pointer(), viewed: reflect.ValueOf(st.viewed).Pointer(),
		folded: reflect.ValueOf(st.folded).Pointer(), nFiles: len(st.d.Files), nThreads: len(st.threads), nDrafts: len(st.drafts),
		threadHash: hashStrings(len(st.threads), func(i int) string { return st.threads[i].Path }),
		draftHash:  hashStrings(len(st.drafts), func(i int) string { return st.drafts[i].Path }),
	}
	treeMemo.Lock()
	defer treeMemo.Unlock()
	if treeMemo.ok && treeMemo.sig == sig {
		return treeMemo.rows
	}
	rows := buildTree(st.d.Files, st.viewed, st.threads, st.drafts, st.folded)
	treeMemo.sig, treeMemo.rows, treeMemo.ok = sig, rows, true
	return rows
}

// treeRowOf is the row that stands for file fi: its own row, or the folded directory that hides it.
func treeRowOf(rows []treeRow, st diffState, fi int) int {
	if fi < 0 || fi >= len(st.d.Files) {
		return 0
	}
	p := st.d.Files[fi].Path
	at := 0
	for i, r := range rows {
		if r.file == fi {
			return i
		}
		if r.dir && strings.HasPrefix(p, r.path+"/") {
			at = i // the deepest directory seen so far that holds the file
		}
	}
	return at
}

func copyBools(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (st diffState) toggleFold(path string) diffState {
	nf := copyBools(st.folded)
	if nf[path] {
		delete(nf, path)
	} else {
		nf[path] = true
	}
	st.folded = nf
	return st
}

// toggleViewed marks a file, or every file under a directory, viewed; if all of them already are,
// the marks are cleared.
func (st diffState) toggleViewed(r treeRow) diffState {
	var paths []string
	for _, f := range st.d.Files {
		if r.dir && strings.HasPrefix(f.Path, r.path+"/") || !r.dir && f.Path == r.path {
			paths = append(paths, f.Path)
		}
	}
	all := len(paths) > 0
	for _, p := range paths {
		all = all && st.viewed[p]
	}
	nv := copyBools(st.viewed)
	for _, p := range paths {
		if all {
			delete(nv, p)
		} else {
			nv[p] = true
		}
	}
	st.viewed = nv
	return st
}

// parentRow is the directory row that holds row i (i itself when it is at the top).
func parentRow(rows []treeRow, i int) int {
	for j := i - 1; j >= 0; j-- {
		if rows[j].dir && rows[j].depth < rows[i].depth {
			return j
		}
	}
	return i
}

// treeNav moves through and folds the tree. file is the diff file the selection landed on (-1 if it is
// a directory or nothing changed); ok is false for keys that are not the tree's.
func treeNav(st diffState, key string) (out diffState, file int, ok bool) {
	rows := st.treeRows()
	if len(rows) == 0 {
		return st, -1, false
	}
	st.sel = min(max(st.sel, 0), len(rows)-1)
	r := rows[st.sel]
	switch key {
	case "j", "down":
		st.sel = min(st.sel+1, len(rows)-1)
	case "k", "up":
		st.sel = max(st.sel-1, 0)
	case "g", "home":
		st.sel = 0
	case "G", "end":
		st.sel = len(rows) - 1
	case "h", "left":
		switch {
		case r.dir && !st.folded[r.path]:
			return st.toggleFold(r.path), -1, true
		default:
			st.sel = parentRow(rows, st.sel)
		}
	case "l", "right":
		if r.dir && st.folded[r.path] {
			return st.toggleFold(r.path), -1, true
		}
		return st, -1, true
	case " ", "enter":
		if r.dir {
			return st.toggleFold(r.path), -1, true
		}
		return st, -1, false
	case "v":
		return st.toggleViewed(r), -1, true
	default:
		return st, -1, false
	}
	if nr := rows[st.sel]; !nr.dir {
		return st, nr.file, true
	}
	return st, -1, true
}

// treeLine draws one row of the file tree exactly w columns wide.
func treeLine(r treeRow, folded, selected bool, w int) string {
	mark := " "
	if selected {
		mark = icons.Get(icons.Selected)
	}
	seen := " "
	if r.viewed == r.files && r.files > 0 {
		seen = icons.Get(icons.Viewed)
	}
	stats := fmt.Sprintf(" +%d −%d", r.added, r.removed)
	if r.dir && r.viewed > 0 && r.viewed < r.files {
		stats += fmt.Sprintf(" %d/%d", r.viewed, r.files)
	}
	if r.threads > 0 {
		stats += fmt.Sprintf(" ◆%d", r.threads)
	}
	if r.drafts > 0 {
		stats += fmt.Sprintf(" ✎%d", r.drafts)
	}
	indent := strings.Repeat("  ", r.depth)
	glyph, color := "", ""
	name := strings.ReplaceAll(r.name, "\t", " ") // a tab would be widened by the styles
	switch {
	case r.dir && icons.Current() == icons.Nerd:
		glyph, color = icons.FolderIcon(!folded)
	case r.dir:
		name += "/"
		glyph = "▾"
		if folded {
			glyph = "▸"
		}
	default:
		glyph, color = icons.FileIcon(r.path)
	}
	iconCell := ""
	if glyph != "" {
		iconCell = glyph + " "
	}
	head := mark + seen + " " + indent + iconCell
	avail := w - ansi.StringWidth(head) - ansi.StringWidth(stats)
	if ansi.StringWidth(name) > avail && avail > 1 {
		rs := []rune(name)
		for ansi.StringWidth("…"+string(rs)) > avail && len(rs) > 0 {
			rs = rs[1:]
		}
		name = "…" + string(rs)
	}
	if avail < 1 { // too narrow for anything but the head
		return fit(head, w)
	}
	plain := head + fit(name, avail) + stats
	switch {
	case selected:
		return ui.Sel.Render(fit(plain, w))
	case r.viewed == r.files && r.files > 0:
		return styDim.Render(fit(plain, w))
	}
	if color != "" && iconCell != "" {
		coloured := mark + seen + " " + indent + lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(glyph) + " "
		return fit(coloured+fit(name, avail)+stats, w)
	}
	return fit(plain, w)
}

// treeWindow is the first row to draw so the selected row is in a window of h rows.
func treeWindow(n, sel, h int) int {
	if n <= h {
		return 0
	}
	return max(0, min(sel-h/2, n-h))
}
