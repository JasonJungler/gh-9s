package ghcli

import "strings"

// CleanText removes what could drive the terminal instead of being shown on it: escape and other
// control characters (so no cursor movement, OSC title or clipboard sequences), DEL, and the C1
// controls. Newlines and tabs stay; CRLF becomes LF and a lone CR is dropped. Anyone can open an
// issue or push a PR on a public repo, so every piece of remote text goes through this.
func CleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		}
		return r
	}, s)
}
