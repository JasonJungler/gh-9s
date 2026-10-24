// Package safe cleans text that comes from somewhere else (issue bodies, diffs, job logs) so it
// can be shown in a terminal without being able to drive it.
package safe

import "strings"

// maxSeq bounds how far an escape sequence is searched for its end; a sequence that never ends
// is not allowed to swallow the rest of the text.
const maxSeq = 4096

// Text removes terminal escape sequences whole (colours, cursor movement, window titles, the
// clipboard), every other control character and DEL. Newlines and tabs stay; CRLF becomes LF
// and a lone CR is dropped.
func Text(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	r := []rune(s)
	var b strings.Builder
	for i := 0; i < len(r); i++ {
		switch c := r[i]; {
		case c == 0x1b:
			i = skipEscape(r, i)
		case c == 0x9b: // C1 CSI
			i = skipCSI(r, i+1)
		case c == 0x9d: // C1 OSC
			i = skipString(r, i+1)
		case c == '\n' || c == '\t':
			b.WriteRune(c)
		case c < 0x20, c == 0x7f, c >= 0x80 && c <= 0x9f:
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// skipEscape returns the index of the last rune of the escape sequence that starts at r[i].
func skipEscape(r []rune, i int) int {
	if i+1 >= len(r) {
		return i
	}
	switch r[i+1] {
	case '[':
		return skipCSI(r, i+2)
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: strings ended by BEL or ST
		return skipString(r, i+2)
	}
	return i + 1 // a two-character escape such as ESC c
}

// skipCSI skips parameters and the final byte of a CSI sequence whose body starts at r[i].
func skipCSI(r []rune, i int) int {
	for j := i; j < len(r) && j < i+maxSeq; j++ {
		if r[j] >= 0x40 && r[j] <= 0x7e {
			return j
		}
		if r[j] < 0x20 || r[j] > 0x3f {
			return j - 1 // not a CSI after all: resume at this rune
		}
	}
	return i - 1 // unterminated: drop only the introducer
}

// skipString skips a BEL- or ST-terminated string whose body starts at r[i].
func skipString(r []rune, i int) int {
	for j := i; j < len(r) && j < i+maxSeq; j++ {
		if r[j] == 0x07 {
			return j
		}
		if r[j] == 0x1b && j+1 < len(r) && r[j+1] == '\\' {
			return j + 1
		}
	}
	return i - 1 // unterminated: drop only the introducer, keep the text
}
