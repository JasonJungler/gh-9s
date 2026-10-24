package ghcli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("payload is not JSON: %v\n%s", err, b)
	}
	return m
}

func TestReviewPayloadApproveWithoutAnything(t *testing.T) {
	b, err := ReviewPayload("abc123", ReviewApprove, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, b)
	if m["commit_id"] != "abc123" || m["event"] != "APPROVE" {
		t.Errorf("payload = %v", m)
	}
	if _, ok := m["body"]; ok {
		t.Error("an empty body is left out")
	}
	if _, ok := m["comments"]; ok {
		t.Error("no comments, no comments key")
	}
}

func TestReviewPayloadKeepsTrickyTextExactly(t *testing.T) {
	big := strings.Repeat("é", 60000)
	body := "line1\nline2 \"quoted\" -flag 😀 \\ " + big
	cs := []InlineComment{
		{Path: "a b/c.go", Line: 12, Side: "RIGHT", Body: "-x \"y\"\n😀"},
		{Path: "d.go", Line: 9, Side: "LEFT", StartLine: 7, Body: "range"},
	}
	b, err := ReviewPayload("deadbeef", ReviewRequestChanges, body, cs)
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, b)
	if m["event"] != "REQUEST_CHANGES" || m["body"] != body {
		t.Errorf("event/body not preserved")
	}
	got := m["comments"].([]any)
	c0, c1 := got[0].(map[string]any), got[1].(map[string]any)
	if c0["path"] != "a b/c.go" || c0["line"] != float64(12) || c0["side"] != "RIGHT" || c0["body"] != "-x \"y\"\n😀" {
		t.Errorf("comment 0 = %v", c0)
	}
	if _, ok := c0["start_line"]; ok {
		t.Error("a one-line comment has no start_line")
	}
	if c1["start_line"] != float64(7) || c1["start_side"] != "LEFT" || c1["line"] != float64(9) {
		t.Errorf("range comment = %v", c1)
	}
}

func TestReviewPayloadRefusesWhatGitHubWouldRefuse(t *testing.T) {
	ok := InlineComment{Path: "a.go", Line: 3, Side: "RIGHT", Body: "x"}
	bad := []struct {
		name string
		kind ReviewKind
		body string
		cs   []InlineComment
	}{
		{"request changes without a body", ReviewRequestChanges, "", []InlineComment{ok}},
		{"comment without a body", ReviewComment, "  ", []InlineComment{ok}},
		{"empty comment body", ReviewApprove, "", []InlineComment{{Path: "a.go", Line: 3, Side: "RIGHT"}}},
		{"empty path", ReviewApprove, "", []InlineComment{{Line: 3, Side: "RIGHT", Body: "x"}}},
		{"line zero", ReviewApprove, "", []InlineComment{{Path: "a.go", Side: "RIGHT", Body: "x"}}},
		{"range that does not go forward", ReviewApprove, "", []InlineComment{{Path: "a.go", Line: 3, StartLine: 3, Side: "RIGHT", Body: "x"}}},
		{"side missing", ReviewApprove, "", []InlineComment{{Path: "a.go", Line: 3, Body: "x"}}},
	}
	for _, c := range bad {
		if _, err := ReviewPayload("h", c.kind, c.body, c.cs); err == nil {
			t.Errorf("%s: want an error", c.name)
		}
	}
	if _, err := ReviewPayload("", ReviewApprove, "", nil); err == nil {
		t.Error("a review without a commit is refused: it must be pinned")
	}
	if _, err := ReviewPayload("h", ReviewApprove, "", []InlineComment{ok}); err != nil {
		t.Errorf("approve with comments and no body is allowed: %v", err)
	}
}

func TestSubmitReviewArgs(t *testing.T) {
	got := SubmitReviewArgs(RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, "482")
	want := []string{"api", "--method", "POST", "repos/acme/api/pulls/482/reviews", "--input", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q; want %q", got, want)
	}
	got = SubmitReviewArgs(RepoRef{Host: "ghe.corp", Owner: "acme", Name: "api"}, "7")
	want = []string{"api", "--hostname", "ghe.corp", "--method", "POST", "repos/acme/api/pulls/7/reviews", "--input", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enterprise args = %q; want %q", got, want)
	}
}

func TestHeadArgsAndErrorText(t *testing.T) {
	if got := PRHeadArgs(RepoRef{Owner: "acme", Name: "api"}, "9"); !reflect.DeepEqual(got, []string{"pr", "view", "9", "--json", "headRefOid", "--jq", ".headRefOid", "-R", "acme/api"}) {
		t.Errorf("head args = %q", got)
	}
	if got := ReviewErrorText("gh: Unprocessable Entity (HTTP 422)", []byte(`{"message":"Unprocessable Entity","errors":["User can only have one pending review per pull request"]}`)); !strings.Contains(got, "one pending review") || !strings.Contains(got, "422") {
		t.Errorf("error text = %q; want GitHub's reason kept", got)
	}
	if got := ReviewErrorText("boom", nil); got != "boom" {
		t.Errorf("error text = %q", got)
	}
}
