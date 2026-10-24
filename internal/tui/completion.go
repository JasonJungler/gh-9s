package tui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// builtinCommands are the commands the command bar suggests, in the order they are offered.
// (Short forms such as r, wf and pr keep working; they are just not suggested.)
var builtinCommands = []string{"runs", "workflows", "pulls", "issues", "inbox", "icons", "settings", "set", "fav", "repo", "cpr", "quit"}

const maxCompletions = 8

// completions lists what the command bar could complete input to, best first: built-in commands,
// then favorite aliases; after "repo " the repos used recently, then the ones I can access.
// Matching ignores case. Nothing is offered for empty input or for something already complete.
func (m model) completions(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	if i := strings.IndexByte(input, ' '); i >= 0 {
		switch strings.ToLower(input[:i]) {
		case "repo":
			return m.repoCompletions(input[:i+1], strings.TrimLeft(input[i:], " "))
		case "set":
			return settingCompletions("set" + input[i:])
		}
		return nil
	}
	low := strings.ToLower(input)
	var out []string
	for _, c := range builtinCommands {
		if c != low && strings.HasPrefix(c, low) {
			out = append(out, c)
		}
	}
	if m.favs != nil {
		var aliases []string
		for _, f := range m.favs.Items {
			a := strings.ToLower(f.Alias)
			if f.Alias != "" && a != low && strings.HasPrefix(a, low) {
				aliases = append(aliases, f.Alias)
			}
		}
		sort.Strings(aliases)
		out = append(out, aliases...)
	}
	return out
}

// repoCompletions completes the argument of "repo": a name matches if the whole OWNER/NAME or just
// the NAME starts with what was typed.
func (m model) repoCompletions(cmd, arg string) []string {
	low := strings.ToLower(arg)
	seen := map[string]bool{}
	var out []string
	add := func(r ghcli.RepoRef) {
		full := displayRef(r)
		l := strings.ToLower(full)
		if seen[l] || l == low {
			return
		}
		if low == "" || strings.HasPrefix(l, low) || strings.HasPrefix(strings.ToLower(r.Name), low) {
			seen[l] = true
			out = append(out, cmd+full)
		}
	}
	for _, r := range m.recentList() {
		add(r)
	}
	if e, ok := m.repoCache[m.curTarget().Host]; ok {
		for _, i := range e.list {
			add(i.Ref)
		}
	}
	if len(out) > maxCompletions {
		out = out[:maxCompletions]
	}
	return out
}

// ghost is the part of the best completion that has not been typed yet.
func (m model) ghost() string {
	if m.mode != modeCommand {
		return ""
	}
	c := m.completions(m.input)
	if len(c) == 0 {
		return ""
	}
	return string([]rune(c[0])[utf8.RuneCountInString(m.input):])
}

// resolveCommand turns a unique prefix into the command it stands for ("pul" -> "pulls"); anything
// else is returned as typed.
func (m model) resolveCommand(c string) string {
	if comps := m.completions(c); len(comps) == 1 && !m.knownCommand(c) {
		return comps[0]
	}
	return c
}

// knownCommand reports whether the command bar already understands c as typed.
func (m model) knownCommand(c string) bool {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "runs", "run", "r", "workflows", "workflow", "wf", "w", "pulls", "pull", "prs", "pr",
		"issues", "issue", "is", "inbox", "in", "icons", "settings", "set", "fav", "favs", "favorites", "cpr", "checks", "q", "quit", "exit", "repo":
		return true
	}
	if m.favs != nil {
		if _, ok := m.favs.ByAlias(c); ok {
			return true
		}
	}
	return false
}
