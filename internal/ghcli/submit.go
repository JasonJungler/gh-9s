package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// InlineComment is a comment on a line (or a run of lines) of the diff. Side is "LEFT" for the old
// file and "RIGHT" for the new one; StartLine is 0 for a single line.
type InlineComment struct {
	Path      string
	Line      int
	Side      string
	StartLine int
	Body      string
}

type payloadComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

type payload struct {
	CommitID string           `json:"commit_id"`
	Event    string           `json:"event"`
	Body     string           `json:"body,omitempty"`
	Comments []payloadComment `json:"comments,omitempty"`
}

var events = map[ReviewKind]string{
	ReviewApprove:        "APPROVE",
	ReviewRequestChanges: "REQUEST_CHANGES",
	ReviewComment:        "COMMENT",
}

// ReviewPayload is the JSON body of the "create a review" request. It is pinned to commit head, and
// it refuses what GitHub would: a request-changes or comment review needs a body, every comment a
// path, a body, a side and a line, and a range has to run forward.
func ReviewPayload(head string, kind ReviewKind, body string, comments []InlineComment) ([]byte, error) {
	event, ok := events[kind]
	if !ok {
		return nil, fmt.Errorf("unknown review kind %q", kind)
	}
	if head == "" {
		return nil, errors.New("no commit to pin the review to")
	}
	if kind != ReviewApprove && strings.TrimSpace(body) == "" {
		return nil, errors.New("this kind of review needs a body: GitHub requires one")
	}
	p := payload{CommitID: head, Event: event, Body: body}
	for i, c := range comments {
		switch {
		case c.Path == "":
			return nil, fmt.Errorf("comment %d has no file", i+1)
		case strings.TrimSpace(c.Body) == "":
			return nil, fmt.Errorf("comment %d on %s is empty", i+1, c.Path)
		case c.Line < 1:
			return nil, fmt.Errorf("comment %d on %s has no line", i+1, c.Path)
		case c.Side != "LEFT" && c.Side != "RIGHT":
			return nil, fmt.Errorf("comment %d on %s:%d has no side", i+1, c.Path, c.Line)
		case c.StartLine != 0 && c.StartLine >= c.Line:
			return nil, fmt.Errorf("comment %d on %s: the range %d–%d does not run forward", i+1, c.Path, c.StartLine, c.Line)
		}
		pc := payloadComment{Path: c.Path, Body: c.Body, Line: c.Line, Side: c.Side}
		if c.StartLine != 0 {
			pc.StartLine, pc.StartSide = c.StartLine, c.Side
		}
		p.Comments = append(p.Comments, pc)
	}
	return json.Marshal(p)
}

// SubmitReviewArgs is the `gh api` call that posts a review; the JSON body comes on stdin.
func SubmitReviewArgs(r RepoRef, number string) []string {
	if r.Owner == "" || r.Name == "" {
		return nil // never let gh guess where a review goes
	}
	args := []string{"api"}
	if r.Host != "" && r.Host != "github.com" {
		args = append(args, "--hostname", r.Host)
	}
	return append(args, "--method", "POST", "repos/"+r.Owner+"/"+r.Name+"/pulls/"+number+"/reviews", "--input", "-")
}

// SubmitReview posts one review with its inline comments, pinned to commit head. GitHub's own
// reason is kept in the error (for example when the user already has a pending review).
func SubmitReview(r RepoRef, number, head string, kind ReviewKind, body string, comments []InlineComment) error {
	args := SubmitReviewArgs(r, number)
	if args == nil {
		return errors.New("don't know which repo this review is for")
	}
	b, err := ReviewPayload(head, kind, body, comments)
	if err != nil {
		return err
	}
	out, err := RunInput(b, args...)
	if err != nil {
		return errors.New(ReviewErrorText(err.Error(), out))
	}
	return nil
}

// ReviewErrorText joins gh's one-line error with the reasons GitHub put in the response body.
func ReviewErrorText(stderr string, body []byte) string {
	var resp struct {
		Message string `json:"message"`
		Errors  []any  `json:"errors"`
	}
	if json.Unmarshal(body, &resp) != nil {
		return stderr
	}
	var parts []string
	if resp.Message != "" && !strings.Contains(stderr, resp.Message) {
		parts = append(parts, resp.Message)
	}
	for _, e := range resp.Errors {
		switch v := e.(type) {
		case string:
			parts = append(parts, v)
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				parts = append(parts, m)
			} else if b, err := json.Marshal(v); err == nil {
				parts = append(parts, string(b))
			}
		}
	}
	if len(parts) == 0 {
		return stderr
	}
	return CleanText(stderr + ": " + strings.Join(parts, "; "))
}

// PRHeadArgs reads the PR's current head commit.
func PRHeadArgs(r RepoRef, number string) []string {
	return Scoped(r, "pr", "view", number, "--json", "headRefOid", "--jq", ".headRefOid")
}

// PRHead is the commit the PR's branch points at now.
func PRHead(r RepoRef, number string) (string, error) {
	out, err := Run(PRHeadArgs(r, number)...)
	return strings.TrimSpace(string(out)), err
}
