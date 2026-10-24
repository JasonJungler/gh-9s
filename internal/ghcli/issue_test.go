package ghcli

import (
	"reflect"
	"testing"
)

func TestIssueFilterArgs(t *testing.T) {
	cases := []struct {
		f    IssueFilter
		want []string
	}{
		{IssueFilter{}, []string{"--state", "open"}},
		{IssueFilter{State: "closed"}, []string{"--state", "closed"}},
		{IssueFilter{State: "all", AssignedToMe: true}, []string{"--state", "all", "--assignee", "@me"}},
		{IssueFilter{OpenedByMe: true}, []string{"--state", "open", "--author", "@me"}},
		{IssueFilter{State: "open", AssignedToMe: true, OpenedByMe: true}, []string{"--state", "open", "--assignee", "@me", "--author", "@me"}},
		{IssueFilter{State: "--web"}, []string{"--state", "open"}}, // only the three known states get through
	}
	for _, c := range cases {
		if got := IssueFilterArgs(c.f); !reflect.DeepEqual(got, c.want) {
			t.Errorf("IssueFilterArgs(%+v) = %q; want %q", c.f, got, c.want)
		}
	}
}

const issueFull = `{
 "number": 14812, "title": "auth login hangs", "body": "### Describe the bug\n\nIt hangs.",
 "author": {"login": "dana"}, "state": "OPEN", "url": "https://github.com/acme/api/issues/14812",
 "labels": [{"name": "bug"}, {"name": "needs triage 🚧"}],
 "assignees": [{"login": "erin"}, {"login": "frank"}],
 "comments": [{"author": {"login": "bot"}, "body": "Thanks for the report", "createdAt": "2026-10-23T10:00:00Z"}]
}`

func TestParseIssueDetailFull(t *testing.T) {
	d, err := ParseIssueDetail([]byte(issueFull))
	if err != nil {
		t.Fatal(err)
	}
	if d.Number != 14812 || d.Title != "auth login hangs" || d.Author != "dana" || d.State != "OPEN" || d.URL == "" {
		t.Errorf("detail = %+v", d)
	}
	if !reflect.DeepEqual(d.Labels, []string{"bug", "needs triage 🚧"}) || !reflect.DeepEqual(d.Assignees, []string{"erin", "frank"}) {
		t.Errorf("labels %q assignees %q", d.Labels, d.Assignees)
	}
	if len(d.Comments) != 1 || d.Comments[0] != (Comment{Author: "bot", Body: "Thanks for the report", CreatedAt: "2026-10-23T10:00:00Z"}) {
		t.Errorf("comments = %+v", d.Comments)
	}
}

func TestParseIssueDetailNullsAndEmpties(t *testing.T) {
	d, err := ParseIssueDetail([]byte(`{"number":3,"title":"t","body":"","author":null,"state":"CLOSED","labels":null,"assignees":null,"comments":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Author != "ghost" || len(d.Labels) != 0 || len(d.Assignees) != 0 || len(d.Comments) != 0 || d.State != "CLOSED" {
		t.Errorf("detail = %+v; want a deleted author as ghost and empty lists", d)
	}
	d, err = ParseIssueDetail([]byte(`{"number":4,"assignees":[{"login":""}],"comments":[{"author":null,"body":"x"}]}`))
	if err != nil || d.Comments[0].Author != "ghost" {
		t.Errorf("a comment by a deleted author: %+v err %v", d, err)
	}
	if _, err := ParseIssueDetail([]byte("garbage")); err == nil {
		t.Error("garbage should be an error")
	}
}

func TestCommentIssueArgs(t *testing.T) {
	got := CommentIssueArgs(acmeAPI, "14812", "-1: same here")
	want := []string{"issue", "comment", "14812", "--body=-1: same here", "-R", "acme/api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}
