package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/icons"
)

// treeDiff is a diff that adds one line to each of paths.
func treeDiff(paths ...string) string {
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,1 +1,2 @@\n x\n+y\n", p, p, p, p)
	}
	return b.String()
}

// listCol is the file list column of the side-by-side screen as plain text, one entry per row.
func listCol(m model) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n")[2:] {
		if r := []rune(l); len(r) >= listWidth(m.width) {
			out = append(out, strings.TrimRight(string(r[:listWidth(m.width)]), " "))
		}
	}
	return out
}

func hasRow(rows []string, sub string) bool {
	for _, r := range rows {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}

func TestSideListIsANestedTree(t *testing.T) {
	m := reviewModel(t, 140, treeDiff("pkg/a/x.go", "pkg/a/y.go", "pkg/b/z.go", "top.go"), nil)
	rows := listCol(m)
	for _, want := range []string{"▾ pkg/", "▾ a/", "    x.go", "    y.go", "▾ b/", "  top.go"} {
		if !hasRow(rows, want) {
			t.Errorf("list lacks %q:\n%s", want, strings.Join(rows, "\n"))
		}
	}
	if hasRow(rows, "pkg/a/x.go") {
		t.Error("files must show their name, not their path")
	}
}

func TestNerdListHasFolderAndLanguageIcons(t *testing.T) {
	useNerd(t)
	m := reviewModel(t, 140, treeDiff("src/app.go", "src/Dockerfile", "src/notes.md"), nil)
	v := ansi.Strip(m.View())
	for _, p := range []string{"src/", "app.go", "Dockerfile", "notes.md"} {
		_ = p
	}
	for _, f := range []string{"src/app.go", "src/Dockerfile", "src/notes.md"} {
		glyph, _ := icons.FileIcon(f)
		if !strings.Contains(v, glyph+" "+f[strings.LastIndex(f, "/")+1:]) {
			t.Errorf("no %q icon before %s:\n%s", glyph, f, v)
		}
	}
	open, _ := icons.FolderIcon(true)
	if !strings.Contains(v, open+" src") {
		t.Errorf("no open folder icon:\n%s", v)
	}
}

func TestEnterFoldsADirectoryButTheDiffKeepsEveryFile(t *testing.T) {
	m := reviewModel(t, 140, treeDiff("pkg/a.go", "pkg/b.go", "top.go"), nil)
	m = keys(m, "tab", "g", "enter") // focus the list, select pkg/, fold it
	rows := listCol(m)
	if hasRow(rows, "a.go") || hasRow(rows, "b.go") || !hasRow(rows, "▸ pkg/") {
		t.Errorf("pkg should be folded:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.Contains(ansi.Strip(m.View()), "pkg/b.go") && !strings.Contains(strings.Join(m.logs, "\n"), "pkg/b.go") {
		t.Error("the diff pane must still hold every file")
	}
	m = keys(m, "l") // unfold
	if !hasRow(listCol(m), "a.go") {
		t.Error("l should unfold")
	}
	m = keys(m, "j", "h") // on a file: h selects the parent directory
	if st := m.screenData(viewDiff).(diffState); st.treeRows()[st.sel].path != "pkg" {
		t.Errorf("h on a file should select its directory, sel=%d", st.sel)
	}
}

func TestVOnADirectoryMarksEverythingUnderItAndAgainClearsIt(t *testing.T) {
	m := reviewModel(t, 140, treeDiff("pkg/a.go", "pkg/b.go", "top.go"), nil)
	m = keys(m, "v") // a file first: only pkg/a.go... the top file is pkg/a.go
	st := m.screenData(viewDiff).(diffState)
	if !st.viewed["pkg/a.go"] || st.viewed["pkg/b.go"] {
		t.Fatalf("setup: %v", st.viewed)
	}
	m = keys(m, "tab", "g", "v") // directory with mixed marks: mark all
	st = m.screenData(viewDiff).(diffState)
	if !st.viewed["pkg/a.go"] || !st.viewed["pkg/b.go"] || st.viewed["top.go"] {
		t.Errorf("v on a mixed directory marks all of it: %v", st.viewed)
	}
	m = keys(m, "v")
	st = m.screenData(viewDiff).(diffState)
	if len(st.viewed) != 0 {
		t.Errorf("v on an all-viewed directory clears it: %v", st.viewed)
	}
	m = keys(m, "enter", "v") // folded: still reaches the files under it
	if st = m.screenData(viewDiff).(diffState); !st.viewed["pkg/a.go"] || !st.viewed["pkg/b.go"] {
		t.Errorf("v on a folded directory: %v", st.viewed)
	}
}

func TestTreeLineIsExactlyWideAndKeepsItsIcon(t *testing.T) {
	useNerd(t)
	long := "very/long/" + strings.Repeat("文件名", 12) + ".go"
	rows := buildTree(tf(long, "a/b.go", "t\tab.go"), map[string]bool{"a/b.go": true}, nil, nil, nil)
	for _, w := range []int{20, 32, 40, 60, 140} {
		for i, r := range rows {
			for _, sel := range []bool{false, true} {
				line := treeLine(r, false, sel, w)
				if got := ansi.StringWidth(line); got != w {
					t.Errorf("w=%d row %d (%q) sel=%v is %d wide", w, i, r.name, sel, got)
				}
			}
			if !r.dir && w >= 32 {
				glyph, _ := icons.FileIcon(r.path)
				if !strings.Contains(ansi.Strip(treeLine(r, false, false, w)), glyph) {
					t.Errorf("w=%d row %q lost its icon when truncated", w, r.name)
				}
			}
		}
	}
}

func TestUnifiedFileListUsesTheTree(t *testing.T) {
	m := reviewModel(t, 100, treeDiff("pkg/a.go", "pkg/b.go", "top.go"), nil)
	m = keys(m, "f")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "▾ pkg/") || !strings.Contains(v, "a.go") {
		t.Errorf("the unified list should be a tree:\n%s", v)
	}
	m = keys(m, "g", "enter") // fold pkg
	if strings.Contains(ansi.Strip(m.View()), "a.go") {
		t.Error("enter on a directory folds it in the unified list too")
	}
	m = keys(m, "j", "enter") // top.go: jump and leave the list
	st := m.screenData(viewDiff).(diffState)
	if st.listMode || m.top != st.starts[2] {
		t.Errorf("enter on a file jumps to it (listMode=%v top=%d starts=%v)", st.listMode, m.top, st.starts)
	}
}

func TestBigTreeScrollsAndKeepsTheSelectionVisible(t *testing.T) {
	var ps []string
	for i := 0; i < 3000; i++ {
		ps = append(ps, fmt.Sprintf("d%03d/f%d.go", i%50, i))
	}
	m := reviewModel(t, 140, treeDiff(ps...), nil)
	m = keys(m, "tab", "G")
	rows := listCol(m)
	if len(rows) == 0 || !strings.Contains(rows[len(rows)-1], "▸") && !hasRow(rows, "▸") {
		t.Errorf("the last row should be selected and visible:\n%s", strings.Join(rows, "\n"))
	}
}
