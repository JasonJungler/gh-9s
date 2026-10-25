package launch

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// WarnChars is the context size above which the caller should warn the user.
const WarnChars = 100000

// maxBranchLen caps the whole branch name.
const maxBranchLen = 50

// BuildContext renders an issue (title, body and all comments) as the prompt
// text handed to the agent. Control characters other than newline and tab are
// removed, so issue text cannot inject terminal escape sequences (such as the
// end of a bracketed paste) into the agent's terminal.
func BuildContext(d ghcli.IssueDetail) string {
	var b strings.Builder
	b.WriteString("Please solve this issue\n\n")
	fmt.Fprintf(&b, "# %s (#%d)\n%s\n\n", d.Title, d.Number, d.URL)
	if strings.TrimSpace(d.Body) == "" {
		b.WriteString("(no description)\n")
	} else {
		b.WriteString(strings.TrimRight(d.Body, "\n") + "\n")
	}
	if len(d.Comments) > 0 {
		b.WriteString("\n## Comments\n")
		for i, c := range d.Comments {
			author := c.Author
			if author == "" {
				author = "ghost"
			}
			if i > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "### %s (%d/%d)\n%s\n", author, i+1, len(d.Comments), strings.TrimRight(c.Body, "\n"))
		}
	}
	return stripControl(b.String())
}

// CleanContext makes text that did not come from BuildContext (a prompt the user edited) as safe to send as
// generated text: CRLF becomes LF first, so an editor that writes Windows line breaks keeps its lines, then
// control characters other than newline and tab are removed (stripControl: a lone CR, ESC and so the end of a
// bracketed paste can never reach the agent's terminal), invalid UTF-8 becomes the replacement character, and
// trailing whitespace and newlines are trimmed. A result of "" means there is nothing to send.
func CleanContext(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRightFunc(stripControl(s), unicode.IsSpace)
}

// stripControl removes control characters except newline and tab: runes below
// 0x20, DEL (0x7f) and the C1 range (0x80-0x9f).
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// BranchName returns "issue-<number>-<slug>" where the slug is the title in
// lowercase ASCII letters and digits, capped so the whole name is at most 50
// characters. A title with no usable characters gives "issue-<number>".
func BranchName(number int, title string) string {
	prefix := fmt.Sprintf("issue-%d", number)
	var slug strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			dash = false
			slug.WriteRune(r)
		} else {
			dash = true
		}
	}
	if slug.Len() == 0 {
		return prefix
	}
	name := prefix + "-" + slug.String()
	if len(name) > maxBranchLen {
		name = name[:maxBranchLen]
	}
	return strings.TrimRight(name, "-")
}
