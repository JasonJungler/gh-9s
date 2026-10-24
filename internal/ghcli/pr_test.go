package ghcli

import (
	"reflect"
	"testing"
)

const prFull = `{
 "number": 482, "title": "Fix flaky retry in deploy", "body": "Retries the deploy step.\n\nCloses #12",
 "author": {"login": "alice", "name": "Alice"}, "state": "OPEN", "isDraft": false,
 "baseRefName": "main", "headRefName": "fix/retry", "url": "https://github.com/acme/api/pull/482",
 "reviewDecision": "REVIEW_REQUIRED", "mergeable": "MERGEABLE", "mergeStateStatus": "BLOCKED",
 "additions": 40, "deletions": 7, "changedFiles": 3,
 "labels": [{"name": "bug"}, {"name": "needs review"}],
 "comments": [{"author": {"login": "bob"}, "body": "Looks good to me", "createdAt": "2026-10-20T10:00:00Z"}],
 "reviews": [{"author": {"login": "carol"}, "state": "CHANGES_REQUESTED", "body": "Please add a test", "submittedAt": "2026-10-21T09:00:00Z"}],
 "statusCheckRollup": [
  {"__typename": "CheckRun", "name": "build", "status": "COMPLETED", "conclusion": "SUCCESS"},
  {"__typename": "CheckRun", "name": "lint", "status": "COMPLETED", "conclusion": "FAILURE"},
  {"__typename": "CheckRun", "name": "e2e", "status": "IN_PROGRESS", "conclusion": ""},
  {"__typename": "CheckRun", "name": "docs", "status": "COMPLETED", "conclusion": "SKIPPED"},
  {"__typename": "StatusContext", "context": "ci/legacy", "state": "SUCCESS"},
  {"__typename": "StatusContext", "context": "ci/other", "state": "PENDING"}
 ]
}`

func TestParsePRDetailFull(t *testing.T) {
	d, err := ParsePRDetail([]byte(prFull))
	if err != nil {
		t.Fatal(err)
	}
	if d.Number != 482 || d.Title != "Fix flaky retry in deploy" || d.Author != "alice" || d.State != "OPEN" ||
		d.BaseRef != "main" || d.HeadRef != "fix/retry" || d.ReviewDecision != "REVIEW_REQUIRED" ||
		d.Mergeable != "MERGEABLE" || d.MergeState != "BLOCKED" || d.Additions != 40 || d.Deletions != 7 || d.ChangedFiles != 3 {
		t.Errorf("detail = %+v", d)
	}
	if !reflect.DeepEqual(d.Labels, []string{"bug", "needs review"}) {
		t.Errorf("labels = %v", d.Labels)
	}
	if len(d.Comments) != 1 || d.Comments[0] != (Comment{Author: "bob", Body: "Looks good to me", CreatedAt: "2026-10-20T10:00:00Z"}) {
		t.Errorf("comments = %+v", d.Comments)
	}
	if len(d.Reviews) != 1 || d.Reviews[0] != (Review{Author: "carol", State: "CHANGES_REQUESTED", Body: "Please add a test", CreatedAt: "2026-10-21T09:00:00Z"}) {
		t.Errorf("reviews = %+v", d.Reviews)
	}
}

func TestParsePRDetailEmptyAndNulls(t *testing.T) {
	d, err := ParsePRDetail([]byte(`{"number":1,"title":"t","body":"","author":null,"state":"OPEN",
		"labels":null,"comments":null,"reviews":null,"statusCheckRollup":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Author != "ghost" || d.Body != "" || len(d.Comments) != 0 || len(d.Reviews) != 0 || len(d.Labels) != 0 {
		t.Errorf("detail = %+v; want a deleted author shown as ghost and empty lists", d)
	}
	if d.ChecksPassed+d.ChecksFailed+d.ChecksPending != 0 {
		t.Errorf("no checks expected: %+v", d)
	}
	// a comment or review whose author was deleted
	d, err = ParsePRDetail([]byte(`{"number":2,"comments":[{"author":null,"body":"x","createdAt":""}],"reviews":[{"author":null,"state":"COMMENTED"}]}`))
	if err != nil || d.Comments[0].Author != "ghost" || d.Reviews[0].Author != "ghost" {
		t.Errorf("deleted comment/review authors: %+v err %v", d, err)
	}
	if _, err := ParsePRDetail([]byte(`not json`)); err == nil {
		t.Error("garbage should be an error")
	}
}

func TestParsePRDetailCountsChecks(t *testing.T) {
	d, _ := ParsePRDetail([]byte(prFull))
	// passed: build, docs (skipped), ci/legacy. failed: lint. pending: e2e, ci/other
	if d.ChecksPassed != 3 || d.ChecksFailed != 1 || d.ChecksPending != 2 {
		t.Errorf("passed/failed/pending = %d/%d/%d; want 3/1/2", d.ChecksPassed, d.ChecksFailed, d.ChecksPending)
	}
}

func TestParsePRDetailDraftAndUnknownMergeState(t *testing.T) {
	d, err := ParsePRDetail([]byte(`{"number":3,"isDraft":true,"mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","state":"OPEN"}`))
	if err != nil || !d.IsDraft || d.Mergeable != "UNKNOWN" || d.MergeState != "UNKNOWN" {
		t.Errorf("detail = %+v err %v", d, err)
	}
}

func TestPRFilterArgs(t *testing.T) {
	cases := []struct {
		f    PRFilter
		want []string
	}{
		{PRFilter{}, nil},
		{PRFilter{Mine: true}, []string{"--author", "@me"}},
		{PRFilter{ReviewRequested: true}, []string{"--search", "review-requested:@me"}},
		{PRFilter{Mine: true, ReviewRequested: true}, []string{"--author", "@me", "--search", "review-requested:@me"}},
	}
	for _, c := range cases {
		if got := PRFilterArgs(c.f); !reflect.DeepEqual(got, c.want) {
			t.Errorf("PRFilterArgs(%+v) = %v; want %v", c.f, got, c.want)
		}
	}
}
