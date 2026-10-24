// Package diff parses the unified diff that `gh pr diff` prints into files and lines, so the
// terminal UI can colour it, jump between files and cap very large diffs.
package diff

import (
	"strings"

	"github.com/MurabitoB/gh-9s/internal/safe"
)

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
	CR   bool // the line ended with a carriage return (CRLF); only set on content lines
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
	lines := strings.Split(text, "\n")
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
		cr := strings.HasSuffix(line, "\r")
		line = safe.Text(strings.TrimSuffix(line, "\r"))
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
				add(Line{Kind: Hunk, Text: line})
			case strings.HasPrefix(line, "+"):
				f.Added++
				add(Line{Add, line[1:], cr})
			case strings.HasPrefix(line, "-"):
				f.Removed++
				add(Line{Del, line[1:], cr})
			case strings.HasPrefix(line, `\`):
				add(Line{Kind: Meta, Text: line})
			case strings.HasPrefix(line, " "):
				add(Line{Context, line[1:], cr})
			case line == "": // an empty context line whose single space was stripped
				add(Line{Kind: Context})
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			add(Line{Kind: Hunk, Text: line})
		case strings.HasPrefix(line, "rename from "):
			f.OldPath, f.Renamed = unquoteGit(strings.TrimPrefix(line, "rename from ")), true
		case strings.HasPrefix(line, "rename to "):
			f.Path = unquoteGit(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "Binary files "):
			f.Binary = true
			add(Line{Kind: Meta, Text: line})
		case strings.HasPrefix(line, "GIT binary patch"):
			f.Binary, skipPatch = true, true
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				f.Path = strings.TrimPrefix(unquoteGit(p), "b/")
			}
		case strings.HasPrefix(line, "--- "):
			// the new path wins; this only matters for a deleted file (+++ /dev/null)
			if p := strings.TrimPrefix(line, "--- "); p != "/dev/null" && !f.Renamed {
				f.Path = strings.TrimPrefix(unquoteGit(p), "a/")
			}
		}
	}
	return d
}

// headerPath takes the path out of "diff --git a/X b/Y". A path may itself contain " b/", so it
// prefers the split where both sides agree (no rename), else the last one.
func headerPath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if strings.Contains(rest, `"`) { // git quotes names with special characters
		if t := quotedTokens(rest); len(t) == 2 {
			return strings.TrimPrefix(unquoteGit(t[1]), "b/")
		}
	}
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

// quotedTokens splits "a b" style text into at most two space-separated tokens, keeping the
// quotes on quoted ones (a quote inside a quoted token is escaped with a backslash).
func quotedTokens(s string) []string {
	var out []string
	for i := 0; i < len(s) && len(out) < 2; {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		start := i
		if i < len(s) && s[i] == '"' {
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
			i++
		} else {
			for i < len(s) && s[i] != ' ' {
				i++
			}
		}
		if start < len(s) {
			out = append(out, s[start:min(i, len(s))])
		}
	}
	return out
}

// unquoteGit undoes git's C-style quoting of a path ("f\303\251.txt" is "fé.txt"); an unquoted
// path is returned as it is.
func unquoteGit(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	in := s[1 : len(s)-1]
	var out []byte
	for i := 0; i < len(in); i++ {
		if in[i] != '\\' || i+1 >= len(in) {
			out = append(out, in[i])
			continue
		}
		i++
		switch c := in[i]; {
		case c >= '0' && c <= '7': // up to three octal digits are one byte
			v, n := 0, 0
			for n < 3 && i < len(in) && in[i] >= '0' && in[i] <= '7' {
				v = v*8 + int(in[i]-'0')
				i++
				n++
			}
			i--
			out = append(out, byte(v))
		case c == 'n':
			out = append(out, '\n')
		case c == 't':
			out = append(out, '\t')
		case c == 'r':
			out = append(out, '\r')
		case c == 'a':
			out = append(out, 7)
		case c == 'b':
			out = append(out, 8)
		case c == 'f':
			out = append(out, 12)
		case c == 'v':
			out = append(out, 11)
		default: // \\ and \" and anything else stand for the character itself
			out = append(out, c)
		}
	}
	return string(out)
}
