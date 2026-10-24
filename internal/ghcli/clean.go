package ghcli

import "github.com/MurabitoB/gh-9s/internal/safe"

// CleanText makes someone else's text safe to show in a terminal: escape sequences (colours,
// cursor movement, window titles, the clipboard) and other control characters are removed.
// Anyone can open an issue or push a PR on a public repo, so every piece of remote text goes
// through this. See package safe for the exact rules.
func CleanText(s string) string { return safe.NoBidi(safe.Text(s)) }
