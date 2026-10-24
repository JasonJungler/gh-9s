package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func tf(paths ...string) []diff.File {
	var fs []diff.File
	for _, p := range paths {
		fs = append(fs, diff.File{Path: p, Added: 2, Removed: 1})
	}
	return fs
}

// shape renders rows as "<indent>name[/]" lines.
func shape(rows []treeRow) string {
	var out []string
	for _, r := range rows {
		s := strings.Repeat(" ", r.depth) + r.name
		if r.dir {
			s += "/"
		}
		out = append(out, s)
	}
	return strings.Join(out, "|")
}

func TestTreeOrderAndMerge(t *testing.T) {
	rows := buildTree(tf("z.go", "internal/tui/a.go", "internal/tui/b.go", "README.md", "cmd/x/y/main.go", "Internal2/q.go"), nil, nil, nil, nil)
	want := "cmd/x/y/| main.go|internal/tui/| a.go| b.go|Internal2/| q.go|README.md|z.go"
	if got := shape(rows); got != want {
		t.Errorf("tree:\n got %s\nwant %s", got, want)
	}
	// a sibling file splits the chain
	rows = buildTree(tf("a/b/c/f.go", "a/g.go"), nil, nil, nil, nil)
	if got, want := shape(rows), "a/| b/c/|  f.go| g.go"; got != want {
		t.Errorf("split chain:\n got %s\nwant %s", got, want)
	}
	if got := shape(buildTree(tf("a/b/c/f.go"), nil, nil, nil, nil)); got != "a/b/c/| f.go" {
		t.Errorf("single chain = %s", got)
	}
}

func TestTreeSameNames(t *testing.T) {
	rows := buildTree(tf("a/x.go", "a.go", "a-b/y.go"), nil, nil, nil, nil)
	if got, want := shape(rows), "a/| x.go|a-b/| y.go|a.go"; got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestTreeAggregatesAndFold(t *testing.T) {
	files := tf("d/a.go", "d/s/b.go", "d/s/c.go", "top.go")
	viewed := map[string]bool{"d/a.go": true, "d/s/b.go": true}
	th := []ghcli.Thread{{Path: "d/s/c.go"}, {Path: "d/s/c.go"}}
	dr := []drafts.Draft{{Path: "d/a.go"}}
	rows := buildTree(files, viewed, th, dr, nil)
	d := rows[0]
	if !d.dir || d.path != "d" || d.files != 3 || d.viewed != 2 || d.added != 6 || d.removed != 3 || d.threads != 2 || d.drafts != 1 {
		t.Errorf("dir row = %+v", d)
	}
	rows = buildTree(files, viewed, th, dr, map[string]bool{"d": true})
	if got, want := shape(rows), "d/|top.go"; got != want {
		t.Errorf("folded = %s", got)
	}
	if rows[0].files != 3 {
		t.Error("a folded directory keeps its aggregates")
	}
	rows = buildTree(files, nil, nil, nil, map[string]bool{"d/s": true})
	if got, want := shape(rows), "d/| s/| a.go|top.go"; got != want {
		t.Errorf("inner fold = %s", got)
	}
	// a merged chain folds by its full path
	rows = buildTree(tf("a/b/c/f.go"), nil, nil, nil, map[string]bool{"a/b/c": true})
	if got := shape(rows); got != "a/b/c/" {
		t.Errorf("merged fold = %s", got)
	}
}

func TestTreeRenameAndFileIndex(t *testing.T) {
	files := []diff.File{
		{Path: "new/dir/b.go", OldPath: "old/b.go", Renamed: true},
		{Path: "x/c.go", OldPath: "x/d.go", Renamed: true},
	}
	rows := buildTree(files, nil, nil, nil, nil)
	for _, r := range rows {
		switch r.path {
		case "new/dir/b.go":
			if r.name != "old/b.go → b.go" || r.file != 0 {
				t.Errorf("moved rename row = %+v", r)
			}
		case "x/c.go":
			if r.name != "d.go → c.go" || r.file != 1 {
				t.Errorf("same-dir rename row = %+v", r)
			}
		}
	}
}

func TestTreeEdgeCases(t *testing.T) {
	if rows := buildTree(nil, nil, nil, nil, nil); len(rows) != 0 {
		t.Error("empty diff has rows")
	}
	rows := buildTree(tf("only.go"), nil, nil, nil, nil)
	if len(rows) != 1 || rows[0].dir || rows[0].depth != 0 {
		t.Errorf("single file = %+v", rows)
	}
	rows = buildTree(tf(".github/workflows/ci.yml", "文件/中 文.md", "a b/c d.txt"), nil, nil, nil, nil)
	if got, want := shape(rows), ".github/workflows/| ci.yml|a b/| c d.txt|文件/| 中 文.md"; got != want {
		t.Errorf("odd paths = %s", got)
	}
}

func TestTreeBigIsFast(t *testing.T) {
	var ps []string
	for i := 0; i < 3000; i++ {
		ps = append(ps, fmt.Sprintf("pkg%d/sub%d/file%d.go", i%40, i%7, i))
	}
	start := time.Now()
	rows := buildTree(tf(ps...), nil, nil, nil, nil)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("3000 files took %s", d)
	}
	n := 0
	for _, r := range rows {
		if !r.dir {
			n++
		}
	}
	if n != 3000 {
		t.Errorf("%d file rows", n)
	}
}
