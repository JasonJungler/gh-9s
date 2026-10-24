package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func sbFile(t *testing.T, patch string) diff.File {
	t.Helper()
	return diff.Parse("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n"+patch, 100000).Files[0]
}

func plainSB(l sbLine) string { return ansi.Strip(l.left) + "│" + ansi.Strip(l.right) }

func TestSideBySideShowsOldLeftAndNewRightWithNumbers(t *testing.T) {
	f := sbFile(t, "@@ -10,2 +10,2 @@\n ctx\n-old line\n+new line\n")
	var rows []string
	for _, l := range sideBySide(f, nil, 80) {
		rows = append(rows, plainSB(l))
	}
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"10   ctx", "11 - old line", "11 + new line"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rows lack %q:\n%s", want, joined)
		}
	}
	for _, r := range rows {
		if strings.Contains(r, "old line") && !strings.Contains(r, "new line") {
			t.Errorf("a changed pair must share a row: %q", r)
		}
	}
}

func TestSideBySideAddedOnlyLeavesTheLeftPaneEmpty(t *testing.T) {
	f := sbFile(t, "@@ -0,0 +1,2 @@\n+first\n+second\n")
	for _, l := range sideBySide(f, nil, 80) {
		if strings.Contains(plainSB(l), "first") && strings.TrimSpace(ansi.Strip(l.left)) != "" {
			t.Errorf("left pane should be blank for an added line: %q", ansi.Strip(l.left))
		}
	}
}

func TestSideBySidePanesHaveExactWidthsAcrossContent(t *testing.T) {
	patches := []string{
		"@@ -1,2 +1,2 @@\n-" + strings.Repeat("x", 10000) + "\n+short\n ctx\n",
		"@@ -1,2 +1,2 @@\n-你好世界你好世界你好世界\n+emoji 😀 and\ttab\n ctx\n",
		"@@ -1,3 +1,1 @@\n-a\n-b\n-c\n+X\n",
		"@@ -0,0 +1,3 @@\n+a\n+b\n+c\n",
	}
	for _, w := range []int{120, 121, 200} {
		pw := (w - 1) / 2
		for _, p := range patches {
			for _, l := range sideBySide(sbFile(t, p), nil, w) {
				if lw, rw := ansi.StringWidth(l.left), ansi.StringWidth(l.right); lw != pw || rw != pw {
					t.Errorf("width %d: left %d right %d; want %d each (%q)", w, lw, rw, pw, plainSB(l))
				}
			}
		}
	}
	long := sideBySide(sbFile(t, patches[0]), nil, 120)
	found := false
	for _, l := range long {
		found = found || strings.Contains(ansi.Strip(l.left), "…")
	}
	if !found {
		t.Error("a cut line should end in …")
	}
}

func TestSideBySideThreadsSitUnderTheirLine(t *testing.T) {
	f := sbFile(t, "@@ -1,2 +1,2 @@\n ctx\n-old\n+new\n")
	threads := []ghcli.Thread{
		{Path: "f.go", Line: 2, Side: "RIGHT", Comments: []ghcli.ThreadComment{{Author: "bob", Body: "why **this**?"}}},
		{Path: "f.go", Line: 2, Side: "LEFT", Comments: []ghcli.ThreadComment{{Author: "amy", Body: "removed on purpose"}}},
		{Path: "other.go", Line: 2, Side: "RIGHT", Comments: []ghcli.ThreadComment{{Author: "zed", Body: "not this file"}}},
	}
	rows := sideBySide(f, threads, 120)
	var texts []string
	for _, l := range rows {
		texts = append(texts, plainSB(l))
	}
	all := strings.Join(texts, "\n")
	if strings.Contains(all, "not this file") {
		t.Error("a thread on another file leaked in")
	}
	pair := indexOfRow(texts, "new")
	right := indexOfRow(texts, "why this?")
	left := indexOfRow(texts, "removed on purpose")
	if pair < 0 || right <= pair || left <= pair {
		t.Fatalf("pair=%d right=%d left=%d; threads belong below the changed pair:\n%s", pair, right, left, all)
	}
	if !rows[right].thread || strings.TrimSpace(ansi.Strip(rows[right].left)) != "" {
		t.Errorf("a RIGHT thread goes in the right pane only: %q", texts[right])
	}
	if strings.TrimSpace(ansi.Strip(rows[left].right)) != "" {
		t.Errorf("a LEFT thread goes in the left pane only: %q", texts[left])
	}
}

func TestSideBySideOutdatedThreadsGoLastAndSaySo(t *testing.T) {
	f := sbFile(t, "@@ -1,1 +1,1 @@\n ctx\n")
	rows := sideBySide(f, []ghcli.Thread{{Path: "f.go", Line: 99, Side: "RIGHT", Outdated: true,
		Comments: []ghcli.ThreadComment{{Author: "bob", Body: "stale remark"}}}}, 120)
	last := plainSB(rows[len(rows)-1])
	if !strings.Contains(strings.Join(plainRows(rows), "\n"), "(outdated)") || !strings.Contains(strings.Join(plainRows(rows)[len(rows)-3:], "\n"), "stale remark") {
		t.Errorf("outdated thread not at the end: %q", last)
	}
}

func plainRows(ls []sbLine) []string {
	var out []string
	for _, l := range ls {
		out = append(out, plainSB(l))
	}
	return out
}

func indexOfRow(rows []string, sub string) int {
	for i, r := range rows {
		if strings.Contains(r, sub) {
			return i
		}
	}
	return -1
}

func TestSideBySideBinaryAndHunkRows(t *testing.T) {
	bin := sideBySide(sbFile(t, "Binary files a/f.go and b/f.go differ\n"), nil, 120)
	if len(bin) != 1 || !strings.Contains(plainSB(bin[0]), "Binary files") {
		t.Errorf("binary file rows: %q", plainRows(bin))
	}
	rows := sideBySide(sbFile(t, "@@ -1,1 +1,1 @@ func x() {\n-a\n+b\n"), nil, 120)
	if !rows[0].hunk || !strings.Contains(plainSB(rows[0]), "@@ -1,1 +1,1 @@") {
		t.Errorf("hunk header row: %+v", rows[0])
	}
}

func TestSideBySideHighlightsOnlyTheChangedWords(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	f := sbFile(t, "@@ -1,1 +1,1 @@\n-for i := 0; i < 3; i++ {\n+for i := 0; i < n; i++ {\n")
	rows := sideBySide(f, nil, 120)
	var pair sbLine
	for _, l := range rows {
		if strings.Contains(ansi.Strip(l.left), "for i") {
			pair = l
		}
	}
	if !strings.Contains(pair.left, hlDel("3")) || !strings.Contains(pair.right, hlAdd("n")) {
		t.Errorf("changed words are not marked: %q / %q", pair.left, pair.right)
	}
	if strings.Contains(pair.left, hlDel("for")) {
		t.Errorf("unchanged words must not be marked: %q", pair.left)
	}
}
