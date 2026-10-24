// Package diff parses the unified diff that `gh pr diff` prints into files and lines, so the
// terminal UI can colour it, jump between files and cap very large diffs.
package diff

import "strings"

// clean drops control characters (escape sequences and the like) from a line of someone else's
// diff; tabs stay. A diff is untrusted text and must never be able to drive the terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// Kind says what a line of a diff is.
type Kind int

const (
	Context Kind = iota
	Add
	Del
	Hunk // an "@@ -a,b +c,d @@" header
	Meta // "Binary files … differ", "\ No newline at end of file"
)

// Line is one line of a file's diff, without its leading +, -, or space.
type Line struct {
	Kind Kind
	Text string
}

// File is the diff of one file. Lines may be cut short by the cap; Added and Removed never are.
type File struct {
	Path, OldPath  string
	Binary         bool
	Renamed        bool
	Added, Removed int
	Lines          []Line
}

// Diff is a whole parsed diff.
type Diff struct {
	Files      []File
	Truncated  bool // some lines were dropped to stay within the cap
	TotalLines int  // lines in the diff, including dropped ones
}

// Parse reads a unified diff, keeping at most maxLines lines in total (files that start after
// the cap are still listed, with their counts). Text before the first "diff --git" is ignored.
func Parse(text string, maxLines int) Diff {
	var d Diff
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1] // the newline that ends the last line
	}
	ci := -1 // index of the current file
	stored := 0
	inHunk, skipPatch := false, false
	add := func(l Line) {
		d.TotalLines++
		if stored >= maxLines {
			d.Truncated = true
			return
		}
		d.Files[ci].Lines = append(d.Files[ci].Lines, l)
		stored++
	}
	for _, line := range lines {
		line = clean(strings.TrimSuffix(line, "\r"))
		if strings.HasPrefix(line, "diff --git ") {
			d.Files = append(d.Files, File{Path: headerPath(line)})
			ci, inHunk, skipPatch = len(d.Files)-1, false, false
			continue
		}
		if ci < 0 || skipPatch {
			continue // stray text, or the body of a binary patch
		}
		f := &d.Files[ci]
		if inHunk {
			switch {
			case strings.HasPrefix(line, "@@"):
				add(Line{Hunk, line})
			case strings.HasPrefix(line, "+"):
				f.Added++
				add(Line{Add, line[1:]})
			case strings.HasPrefix(line, "-"):
				f.Removed++
				add(Line{Del, line[1:]})
			case strings.HasPrefix(line, `\`):
				add(Line{Meta, line})
			case strings.HasPrefix(line, " "):
				add(Line{Context, line[1:]})
			case line == "": // an empty context line whose single space was stripped
				add(Line{Context, ""})
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			add(Line{Hunk, line})
		case strings.HasPrefix(line, "rename from "):
			f.OldPath, f.Renamed = strings.TrimPrefix(line, "rename from "), true
		case strings.HasPrefix(line, "rename to "):
			f.Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "):
			f.Binary = true
			add(Line{Meta, line})
		case strings.HasPrefix(line, "GIT binary patch"):
			f.Binary, skipPatch = true, true
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				f.Path = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "--- "):
			// the new path wins; this only matters for a deleted file (+++ /dev/null)
			if p := strings.TrimPrefix(line, "--- "); p != "/dev/null" && !f.Renamed {
				f.Path = strings.TrimPrefix(p, "a/")
			}
		}
	}
	return d
}

// headerPath takes the path out of "diff --git a/X b/Y". A path may itself contain " b/", so it
// prefers the split where both sides agree (no rename), else the last one.
func headerPath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	last := ""
	for i := 0; i+3 <= len(rest); i++ {
		if rest[i:i+3] != " b/" {
			continue
		}
		left, right := strings.TrimPrefix(rest[:i], "a/"), rest[i+3:]
		if left == right {
			return right
		}
		last = right
	}
	if last != "" {
		return last
	}
	return strings.TrimPrefix(rest, "a/")
}
