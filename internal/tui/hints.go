package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var functionBarItems = []struct{ key, label string }{
	{"1", "pulls"}, {"2", "issues"}, {"3", "runs"}, {"4", "workflows"},
	{"5", "disc"}, {"6", "wiki"}, {"0", "inbox"}, {":fav", "favs"},
	{":repo", "repo"}, {"P", "current-PR"}, {":", "cmd"}, {"/", "filter"},
}

func functionBar(width int) string {
	var items []string
	for _, item := range functionBarItems {
		keyStyle := styKey
		if len(item.key) == 1 && item.key[0] >= '0' && item.key[0] <= '9' {
			keyStyle = styNumKey
		}
		rendered := keyStyle.Render("<"+item.key+">") + " " + styDim.Render(item.label)
		candidate := strings.Join(append(items, rendered), " ")
		if width > 0 && ansi.StringWidth(candidate) > width {
			break
		}
		items = append(items, rendered)
	}
	return strings.Join(items, " ")
}

// labelValue highlights a short application-owned label while leaving its value's
// presentation alone. The colon belongs to the label, as it does in k9s.
func labelValue(label, value string) string {
	return styKey.Render(label+":") + " " + value
}

// shortcutHints renders the compact footer notation used throughout the TUI.
// Entries are separated by two or more spaces; the first word is the key spec.
func shortcutHints(s string) string {
	var rendered []string
	for _, entry := range hintEntries(s) {
		key, what, ok := strings.Cut(entry, " ")
		if !ok {
			rendered = append(rendered, styDim.Render(entry))
			continue
		}
		rendered = append(rendered, styKey.Render(key+":")+" "+styDim.Render(strings.TrimSpace(what)))
	}
	return strings.Join(rendered, "  ")
}

func hintEntries(s string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool { return r == '\n' })
	var entries []string
	for _, line := range fields {
		start := 0
		for i := 0; i < len(line); {
			if line[i] != ' ' {
				i++
				continue
			}
			j := i
			for j < len(line) && line[j] == ' ' {
				j++
			}
			if j-i >= 2 {
				if entry := strings.TrimSpace(line[start:i]); entry != "" {
					entries = append(entries, entry)
				}
				start = j
			}
			i = j
		}
		if entry := strings.TrimSpace(line[start:]); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}
