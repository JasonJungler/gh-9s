package diff

import (
	"regexp"
	"strconv"
)

// NumLine is a Line with its line numbers on each side (0 where it has none).
type NumLine struct {
	Line
	Old, New int
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// Numbered returns the file's lines with their numbers, taken from the hunk headers.
func (f File) Numbered() []NumLine {
	out := make([]NumLine, 0, len(f.Lines))
	var old, new int
	for _, l := range f.Lines {
		n := NumLine{Line: l}
		switch l.Kind {
		case Hunk:
			if m := hunkHeader.FindStringSubmatch(l.Text); m != nil {
				old, _ = strconv.Atoi(m[1])
				new, _ = strconv.Atoi(m[2])
			}
		case Context:
			n.Old, n.New = old, new
			old, new = old+1, new+1
		case Del:
			n.Old = old
			old++
		case Add:
			n.New = new
			new++
		}
		out = append(out, n)
	}
	return out
}

// Row is one row of a side-by-side view. A nil side is empty. Hunk headers and notes sit on both
// sides as the same line.
type Row struct{ Old, New *NumLine }

// Rows pairs the lines for a side-by-side view: a run of deletions followed by a run of
// additions is paired line by line, the shorter run leaving blanks.
func (f File) Rows() []Row {
	lines := f.Numbered()
	var rows []Row
	for i := 0; i < len(lines); {
		l := &lines[i]
		switch l.Kind {
		case Del:
			j := i
			for j < len(lines) && lines[j].Kind == Del {
				j++
			}
			k := j
			for k < len(lines) && lines[k].Kind == Add {
				k++
			}
			dels, adds := lines[i:j], lines[j:k]
			for n := 0; n < max(len(dels), len(adds)); n++ {
				var r Row
				if n < len(dels) {
					r.Old = &dels[n]
				}
				if n < len(adds) {
					r.New = &adds[n]
				}
				rows = append(rows, r)
			}
			i = k
		case Add:
			rows = append(rows, Row{New: l})
			i++
		case Context:
			rows = append(rows, Row{Old: l, New: l})
			i++
		default: // Hunk, Meta
			rows = append(rows, Row{Old: l, New: l})
			i++
		}
	}
	return rows
}
