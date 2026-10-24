package pfilter

import (
	"reflect"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

var items = []ghcli.ProjectItem{
	{Type: "Issue", Title: "Fix Login", Status: "Todo", State: "OPEN", Repo: "acme/app", Assignees: []string{"ann"}, Labels: []string{"bug"}, Updated: "2026-10-03"},
	{Type: "PullRequest", Title: "Add retry", Status: "In Progress", State: "OPEN", Repo: "acme/web", Assignees: []string{"bob"}, Labels: []string{"feature", "ui"}, Updated: "2026-10-01"},
	{Type: "PullRequest", Title: "Old", Status: "Done", State: "MERGED", Repo: "acme/app", Updated: "2026-10-02"},
	{Type: "DraftIssue", Title: "Idea", Draft: true},
}

func match(t *testing.T, filter, me string) []int {
	t.Helper()
	f := Parse(filter)
	if len(f.Unsupported) != 0 {
		t.Fatalf("%q unsupported: %v", filter, f.Unsupported)
	}
	var got []int
	for i, it := range items {
		if f.Match(it, me) {
			got = append(got, i)
		}
	}
	return got
}

func TestTerms(t *testing.T) {
	cases := []struct {
		filter, me string
		want       []int
	}{
		{"", "", []int{0, 1, 2, 3}},
		{"assignee:@me", "ann", []int{0}},
		{"assignee:@me", "", nil},
		{"assignee:bob,ann", "", []int{0, 1}},
		{"assignee:none", "", []int{2, 3}},
		{"label:bug", "", []int{0}},
		{"-label:bug", "", []int{1, 2, 3}},
		{`status:"In Progress"`, "", []int{1}},
		{"-status:done", "", []int{0, 1, 3}},
		{"is:pr is:open", "", []int{1}},
		{"is:closed", "", []int{2}},
		{"is:draft", "", []int{3}},
		{"is:issue", "", []int{0}},
		{"repo:acme/app", "", []int{0, 2}},
		{"no:assignee", "", []int{2, 3}},
		{"no:status", "", []int{3}},
		{"login", "", []int{0}},
		{"is:open label:ui", "", []int{1}},
	}
	for _, c := range cases {
		if got := match(t, c.filter, c.me); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q (me %q) = %v, want %v", c.filter, c.me, got, c.want)
		}
	}
}

func TestUnsupportedTermsAreReportedNotGuessed(t *testing.T) {
	f := Parse(`iteration:@current assignee:@me milestone:v1 label:`)
	if !reflect.DeepEqual(f.Unsupported, []string{"iteration:@current", "milestone:v1", "label:"}) {
		t.Errorf("%v", f.Unsupported)
	}
	if !f.NeedsMe() {
		t.Error("NeedsMe")
	}
	for _, bad := range []string{`label:"open`, `"`} {
		f := Parse(bad)
		if len(f.Unsupported) != 1 {
			t.Errorf("%q: %v", bad, f.Unsupported)
		}
		if !f.Match(items[0], "") { // nothing usable left: everything passes, and the note says why
			t.Errorf("%q hides items", bad)
		}
	}
}

func TestSort(t *testing.T) {
	idx := []int{0, 1, 2}
	if !Sort(idx, items, []string{"Todo", "In Progress", "Done"}, "Status", false) || !reflect.DeepEqual(idx, []int{0, 1, 2}) {
		t.Errorf("status %v", idx)
	}
	Sort(idx, items, nil, "Title", false)
	if !reflect.DeepEqual(idx, []int{1, 0, 2}) { // Add retry, Fix Login, Old
		t.Errorf("title %v", idx)
	}
	Sort(idx, items, nil, "Updated at", true)
	if !reflect.DeepEqual(idx, []int{0, 2, 1}) {
		t.Errorf("updated desc %v", idx)
	}
	before := append([]int(nil), idx...)
	if Sort(idx, items, nil, "Priority", false) || !reflect.DeepEqual(idx, before) {
		t.Error("an unknown field must leave the order alone and say so")
	}
}

func TestHostileFilterDoesNotPanic(t *testing.T) {
	for _, s := range []string{"-", "--", ":", "::a", `"":""`, ",,,", "a:,", "-:x", string(make([]byte, 5000))} {
		f := Parse(s)
		for _, it := range items {
			f.Match(it, "")
		}
	}
}
