// Package pfilter applies the filter text of a GitHub Project view to the items of a Project. It supports
// the common terms and reports the ones it does not, so a view is never silently shown wrong.
package pfilter

import (
	"sort"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type term struct {
	neg   bool
	key   string   // "" for a bare word
	words []string // alternatives (comma separated), lower case
}

// Filter is a parsed filter: all terms must match.
type Filter struct {
	terms       []term
	Unsupported []string // terms that were left out, as written
}

var supportedKeys = map[string]bool{"assignee": true, "label": true, "status": true, "is": true, "repo": true, "no": true}

// tokens splits on spaces outside double quotes; ok is false when a quote is not closed.
func tokens(s string) (out []string, ok bool) {
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
			cur.WriteRune(r)
		case r == ' ' && !inQ:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, !inQ
}

func unquote(s string) string { return strings.ReplaceAll(s, `"`, "") }

// splitValues splits on commas outside quotes.
func splitValues(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ',' && !inQ:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, cur.String())
}

// knownValues checks the values of the keys with a fixed set (is:, no:); other keys take free text.
func knownValues(key, v string) bool {
	var ok []string
	switch key {
	case "is":
		ok = []string{"open", "closed", "merged", "issue", "pr", "pull-request", "draft"}
	case "no":
		ok = []string{"assignee", "label", "status"}
	default:
		return true
	}
	for _, w := range splitValues(v) {
		found := false
		for _, o := range ok {
			if strings.EqualFold(unquote(w), o) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Parse reads a view's filter text. An empty filter matches everything.
func Parse(s string) Filter {
	var f Filter
	toks, ok := tokens(s)
	if !ok {
		f.Unsupported = append(f.Unsupported, s)
		return f
	}
	for _, tk := range toks {
		t := term{}
		body := tk
		if strings.HasPrefix(body, "-") && len(body) > 1 {
			t.neg, body = true, body[1:]
		}
		if k, v, has := strings.Cut(body, ":"); has && !strings.HasPrefix(body, `"`) {
			t.key = strings.ToLower(k)
			if !supportedKeys[t.key] || v == "" {
				f.Unsupported = append(f.Unsupported, tk)
				continue
			}
			body = v
			if !knownValues(t.key, v) {
				f.Unsupported = append(f.Unsupported, tk)
				continue
			}
		}
		for _, w := range splitValues(body) {
			if w = unquote(w); w != "" {
				t.words = append(t.words, strings.ToLower(w))
			}
		}
		if len(t.words) == 0 {
			f.Unsupported = append(f.Unsupported, tk)
			continue
		}
		f.terms = append(f.terms, t)
	}
	return f
}

// NeedsMe says whether the filter mentions @me.
func (f Filter) NeedsMe() bool {
	for _, t := range f.terms {
		for _, w := range t.words {
			if w == "@me" {
				return true
			}
		}
	}
	return false
}

func has(list []string, w string) bool {
	for _, x := range list {
		if strings.EqualFold(x, w) {
			return true
		}
	}
	return false
}

func (t term) matches(it ghcli.ProjectItem, me string) bool {
	any := false
	for _, w := range t.words {
		if t.one(it, me, w) {
			any = true
			break
		}
	}
	return any != t.neg
}

func (t term) one(it ghcli.ProjectItem, me, w string) bool {
	switch t.key {
	case "":
		return strings.Contains(strings.ToLower(it.Title), w)
	case "assignee":
		if w == "@me" {
			return me != "" && has(it.Assignees, me)
		}
		if w == "none" {
			return len(it.Assignees) == 0
		}
		return has(it.Assignees, w)
	case "label":
		return has(it.Labels, w)
	case "status":
		return strings.EqualFold(it.Status, w)
	case "repo":
		return strings.EqualFold(it.Repo, w)
	case "no":
		switch w {
		case "assignee":
			return len(it.Assignees) == 0
		case "label":
			return len(it.Labels) == 0
		case "status":
			return it.Status == ""
		}
		return false
	case "is":
		switch w {
		case "open":
			return it.State == "OPEN"
		case "closed":
			return it.State == "CLOSED" || it.State == "MERGED"
		case "merged":
			return it.State == "MERGED"
		case "issue":
			return it.Type == "Issue"
		case "pr", "pull-request":
			return it.Type == "PullRequest"
		case "draft":
			return it.Draft
		}
	}
	return false
}

// Match reports whether the item passes every supported term. me is the logged-in user's login.
func (f Filter) Match(it ghcli.ProjectItem, me string) bool {
	for _, t := range f.terms {
		if !t.matches(it, me) {
			return false
		}
	}
	return true
}

// Sort orders the item indices by a view's sort field; ok is false (and the order is untouched) for a
// field it does not know.
func Sort(idx []int, items []ghcli.ProjectItem, statusOrder []string, field string, desc bool) (ok bool) {
	rank := func(s string) int {
		for i, o := range statusOrder {
			if o == s {
				return i
			}
		}
		return len(statusOrder)
	}
	var less func(a, b ghcli.ProjectItem) int
	switch strings.ToLower(field) {
	case "title":
		less = func(a, b ghcli.ProjectItem) int {
			return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		}
	case "status":
		less = func(a, b ghcli.ProjectItem) int { return rank(a.Status) - rank(b.Status) }
	case "updated at", "updated":
		less = func(a, b ghcli.ProjectItem) int { return strings.Compare(a.Updated, b.Updated) }
	case "assignees", "labels":
		first := func(it ghcli.ProjectItem) string {
			l := it.Assignees
			if strings.EqualFold(field, "labels") {
				l = it.Labels
			}
			if len(l) == 0 {
				return ""
			}
			return strings.ToLower(l[0])
		}
		sort.SliceStable(idx, func(i, j int) bool { // empty last in both directions
			a, b := first(items[idx[i]]), first(items[idx[j]])
			if (a == "") != (b == "") {
				return a != ""
			}
			if desc {
				return a > b
			}
			return a < b
		})
		return true
	default:
		return false
	}
	sort.SliceStable(idx, func(i, j int) bool {
		c := less(items[idx[i]], items[idx[j]])
		if desc {
			return c > 0
		}
		return c < 0
	})
	return true
}
