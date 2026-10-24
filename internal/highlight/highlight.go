// Package highlight colours the code in a diff by language, with chroma. A hunk's old side
// (context and deleted lines) and new side (context and added lines) are each tokenised as one
// piece of text, so strings and comments that span lines inside a hunk come out right. Something
// that starts before the hunk can still be coloured wrongly: that is the price of not fetching
// whole files.
package highlight

import (
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/MurabitoB/gh-9s/internal/diff"
)

const (
	maxLineLen  = 400 // some lexers are far slower than linear on long lines
	maxFileLine = 20000
)

// Seg is a stretch of one line in one colour. FG is "#rrggbb", or empty for the terminal's own.
type Seg struct {
	Text         string
	FG           string
	Bold, Italic bool
}

// File holds the highlighted segments of a file's diff lines, by index in diff.File.Lines.
type File struct{ left, right map[int][]Seg }

// Left is a line as it reads in the old file (context and deleted lines); nil when not coloured.
func (f *File) Left(i int) []Seg {
	if f == nil {
		return nil
	}
	return f.left[i]
}

// Right is a line as it reads in the new file (context and added lines); nil when not coloured.
func (f *File) Right(i int) []Seg {
	if f == nil {
		return nil
	}
	return f.right[i]
}

// denied lists lexers that are known to loop forever on ordinary text (chroma v2.14.0).
var denied = map[string]bool{"Jungle": true}

// ForFile highlights a file's diff within a second; see ForFileWithin.
func ForFile(f diff.File) *File { return ForFileWithin(f, time.Second) }

// Size is how many bytes of text ForFile would hand to the lexer.
func Size(f diff.File) int {
	n := 0
	for _, l := range f.Lines {
		if l.Kind != diff.Hunk && l.Kind != diff.Meta {
			n += len(l.Text)
		}
	}
	return n
}

// ForFileWithin highlights a file's diff, or returns nil when its language is unknown, it is binary
// or too big, the lexer fails or panics, or it takes longer than limit. Lexers are third-party code
// run on hostile text: a lexer that does not finish is abandoned and the file stays plain.
func ForFileWithin(f diff.File, limit time.Duration) *File {
	if f.Binary || len(f.Lines) > maxFileLine {
		return nil
	}
	lexer := lexers.Match(f.Path)
	if lexer == nil || denied[lexer.Config().Name] {
		return nil
	}
	done := make(chan *File, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- nil
			}
		}()
		done <- build(f, chroma.Coalesce(lexer))
	}()
	select {
	case h := <-done:
		return h
	case <-time.After(limit):
		return nil
	}
}

func build(f diff.File, lexer chroma.Lexer) *File {
	style := styles.Get("monokai")
	out := &File{left: map[int][]Seg{}, right: map[int][]Seg{}}
	var oldIdx, newIdx []int
	flush := func() {
		side(f, lexer, style, oldIdx, out.left)
		side(f, lexer, style, newIdx, out.right)
		oldIdx, newIdx = nil, nil
	}
	for i, l := range f.Lines {
		switch l.Kind {
		case diff.Hunk:
			flush()
		case diff.Context:
			oldIdx, newIdx = append(oldIdx, i), append(newIdx, i)
		case diff.Del:
			oldIdx = append(oldIdx, i)
		case diff.Add:
			newIdx = append(newIdx, i)
		}
	}
	flush()
	return out
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

// side tokenises the lines idx of f as one text and stores each line's segments in into.
func side(f diff.File, lexer chroma.Lexer, style *chroma.Style, idx []int, into map[int][]Seg) {
	if len(idx) == 0 {
		return
	}
	texts := make([]string, len(idx))
	for n, i := range idx {
		if t := f.Lines[i].Text; len(t) <= maxLineLen {
			texts[n] = t
		}
	}
	plain := style.Get(chroma.Text)
	it, err := lexer.Tokenise(nil, strings.Join(texts, "\n")+"\n")
	if err != nil {
		return
	}
	lines := [][]Seg{nil}
	for _, tok := range it.Tokens() {
		entry := style.Get(tok.Type)
		fg := ""
		if entry.Colour.IsSet() && entry.Colour != plain.Colour { // plain text keeps the terminal's own colour
			fg = entry.Colour.String()
		}
		for k, piece := range strings.Split(tok.Value, "\n") {
			if k > 0 {
				lines = append(lines, nil)
			}
			if piece == "" {
				continue
			}
			lines[len(lines)-1] = append(lines[len(lines)-1], Seg{Text: expandTabs(piece), FG: fg, Bold: entry.Bold == chroma.Yes, Italic: entry.Italic == chroma.Yes})
		}
	}
	for n, i := range idx {
		if n >= len(lines) || len(f.Lines[i].Text) > maxLineLen {
			continue
		}
		var b strings.Builder
		for _, s := range lines[n] {
			b.WriteString(s.Text)
		}
		if b.String() == expandTabs(f.Lines[i].Text) && b.Len() > 0 { // anything odd: leave the line plain
			into[i] = lines[n]
		}
	}
}
