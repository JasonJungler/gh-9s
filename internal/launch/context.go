package launch

import (
	"fmt"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// WarnChars is the context size above which the caller should warn the user.
const WarnChars = 100000

// maxBranchLen caps the whole branch name.
const maxBranchLen = 50

// BuildContext renders an issue (title, body and all comments) as the prompt
// text handed to the agent.
func BuildContext(d ghcli.IssueDetail) string {
	var b strings.Builder
	b.WriteString("請解決這個 issue\n\n")
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
	return b.String()
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
