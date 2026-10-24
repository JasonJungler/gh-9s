package tui

import (
	"path"
	"sort"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// treeRow is one line of the file tree in the review screen.
type treeRow struct {
	dir     bool
	path    string // the full path; for a directory the merged chain ("internal/tui")
	name    string // what the row shows: "internal/tui", "file.go" or "old → new"
	depth   int
	file    int // index in diff.Files, -1 for a directory
	added   int
	removed int
	files   int // files under a directory (1 for a file)
	viewed  int // how many of them are viewed
	threads int
	drafts  int
}

type treeNode struct {
	name  string // one path segment, or a merged chain after compaction
	path  string
	dirs  []*treeNode
	files []int
}

func lessName(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

// buildTree turns the diff's files into the visible rows of a directory tree: directories first, then
// files, each alphabetical; a directory whose only child is a directory is merged with it; the
// directories in folded (by full path) show no descendants.
func buildTree(files []diff.File, viewed map[string]bool, threads []ghcli.Thread, ds []drafts.Draft, folded map[string]bool) []treeRow {
	root := &treeNode{}
	for i, f := range files {
		n := root
		parts := strings.Split(f.Path, "/")
		for k, seg := range parts[:len(parts)-1] {
			var next *treeNode
			for _, d := range n.dirs {
				if d.name == seg {
					next = d
					break
				}
			}
			if next == nil {
				next = &treeNode{name: seg, path: strings.Join(parts[:k+1], "/")}
				n.dirs = append(n.dirs, next)
			}
			n = next
		}
		n.files = append(n.files, i)
	}
	nThreads, nDrafts := map[string]int{}, map[string]int{}
	for _, t := range threads {
		nThreads[t.Path]++
	}
	for _, d := range ds {
		nDrafts[d.Path]++
	}
	var rows []treeRow
	var walk func(n *treeNode, depth int)
	// sum adds up the files under n.
	var sum func(n *treeNode) treeRow
	sum = func(n *treeNode) treeRow {
		var r treeRow
		for _, fi := range n.files {
			f := files[fi]
			r.added += f.Added
			r.removed += f.Removed
			r.files++
			if viewed[f.Path] {
				r.viewed++
			}
			r.threads += nThreads[f.Path]
			r.drafts += nDrafts[f.Path]
		}
		for _, d := range n.dirs {
			s := sum(d)
			r.added += s.added
			r.removed += s.removed
			r.files += s.files
			r.viewed += s.viewed
			r.threads += s.threads
			r.drafts += s.drafts
		}
		return r
	}
	walk = func(n *treeNode, depth int) {
		sort.Slice(n.dirs, func(i, j int) bool { return lessName(n.dirs[i].name, n.dirs[j].name) })
		for _, d := range n.dirs {
			for len(d.dirs) == 1 && len(d.files) == 0 { // merge a chain of single-child directories
				c := d.dirs[0]
				d = &treeNode{name: d.name + "/" + c.name, path: c.path, dirs: c.dirs, files: c.files}
			}
			r := sum(d)
			r.dir, r.path, r.name, r.depth, r.file = true, d.path, d.name, depth, -1
			rows = append(rows, r)
			if !folded[d.path] {
				walk(d, depth+1)
			}
		}
		sort.Slice(n.files, func(i, j int) bool {
			return lessName(path.Base(files[n.files[i]].Path), path.Base(files[n.files[j]].Path))
		})
		for _, fi := range n.files {
			f := files[fi]
			name := path.Base(f.Path)
			if f.Renamed && f.OldPath != "" && f.OldPath != f.Path {
				old := f.OldPath
				if path.Dir(old) == path.Dir(f.Path) {
					old = path.Base(old)
				}
				name = old + " → " + name
			}
			r := treeRow{path: f.Path, name: name, depth: depth, file: fi, added: f.Added, removed: f.Removed,
				files: 1, threads: nThreads[f.Path], drafts: nDrafts[f.Path]}
			if viewed[f.Path] {
				r.viewed = 1
			}
			rows = append(rows, r)
		}
	}
	walk(root, 0)
	return rows
}
