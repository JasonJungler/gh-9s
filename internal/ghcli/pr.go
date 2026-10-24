package ghcli

import (
	"encoding/json"
	"strings"
)

// Comment is a comment on a PR or an issue.
type Comment struct{ Author, Body, CreatedAt string }

// Review is a submitted review of a PR.
type Review struct{ Author, State, Body, CreatedAt string }

// PRDetail is what the PR detail screen shows.
type PRDetail struct {
	Number                                            int
	Title, Body, Author, State, BaseRef, HeadRef, URL string
	ReviewDecision, Mergeable, MergeState             string
	IsDraft                                           bool
	HeadSHA                                           string // the commit the PR was read at
	Additions, Deletions, ChangedFiles                int
	Labels                                            []string
	ChecksPassed, ChecksFailed, ChecksPending         int
	Comments                                          []Comment
	Reviews                                           []Review
}

// PRFilter narrows the PR list. The zero value lists everything.
type PRFilter struct{ Mine, ReviewRequested bool }

// PRFilterArgs are the extra `gh pr list` flags for f.
func PRFilterArgs(f PRFilter) []string {
	var a []string
	if f.Mine {
		a = append(a, "--author", "@me")
	}
	if f.ReviewRequested {
		a = append(a, "--search", "review-requested:@me")
	}
	return a
}

type rawLogin struct {
	Login string `json:"login"`
}

// loginOrGhost names the author; GitHub shows deleted accounts as "ghost".
func loginOrGhost(a *rawLogin) string {
	if a == nil || a.Login == "" {
		return "ghost"
	}
	return CleanText(a.Login)
}

// ParsePRDetail reads the JSON of `gh pr view --json ...` (fields listed in FetchPR).
func ParsePRDetail(b []byte) (PRDetail, error) {
	var raw struct {
		Number           int       `json:"number"`
		Title            string    `json:"title"`
		Body             string    `json:"body"`
		Author           *rawLogin `json:"author"`
		State            string    `json:"state"`
		IsDraft          bool      `json:"isDraft"`
		BaseRefName      string    `json:"baseRefName"`
		HeadRefName      string    `json:"headRefName"`
		HeadRefOid       string    `json:"headRefOid"`
		URL              string    `json:"url"`
		ReviewDecision   string    `json:"reviewDecision"`
		Mergeable        string    `json:"mergeable"`
		MergeStateStatus string    `json:"mergeStateStatus"`
		Additions        int       `json:"additions"`
		Deletions        int       `json:"deletions"`
		ChangedFiles     int       `json:"changedFiles"`
		Labels           []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Comments []struct {
			Author    *rawLogin `json:"author"`
			Body      string    `json:"body"`
			CreatedAt string    `json:"createdAt"`
		} `json:"comments"`
		Reviews []struct {
			Author      *rawLogin `json:"author"`
			State       string    `json:"state"`
			Body        string    `json:"body"`
			SubmittedAt string    `json:"submittedAt"`
		} `json:"reviews"`
		Rollup []struct {
			Typename   string `json:"__typename"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return PRDetail{}, err
	}
	d := PRDetail{
		Number: raw.Number, Title: CleanText(raw.Title), Body: CleanText(raw.Body), Author: loginOrGhost(raw.Author), State: raw.State,
		BaseRef: CleanText(raw.BaseRefName), HeadRef: CleanText(raw.HeadRefName), HeadSHA: raw.HeadRefOid, URL: raw.URL, ReviewDecision: raw.ReviewDecision,
		Mergeable: raw.Mergeable, MergeState: raw.MergeStateStatus, IsDraft: raw.IsDraft,
		Additions: raw.Additions, Deletions: raw.Deletions, ChangedFiles: raw.ChangedFiles,
	}
	for _, l := range raw.Labels {
		d.Labels = append(d.Labels, CleanText(l.Name))
	}
	for _, c := range raw.Comments {
		d.Comments = append(d.Comments, Comment{loginOrGhost(c.Author), CleanText(c.Body), c.CreatedAt})
	}
	for _, r := range raw.Reviews {
		d.Reviews = append(d.Reviews, Review{loginOrGhost(r.Author), r.State, CleanText(r.Body), r.SubmittedAt})
	}
	for _, c := range raw.Rollup {
		switch checkOutcome(c.Typename, c.Status, c.Conclusion, c.State) {
		case "pass":
			d.ChecksPassed++
		case "fail":
			d.ChecksFailed++
		default:
			d.ChecksPending++
		}
	}
	return d, nil
}

// checkOutcome folds a CheckRun (status + conclusion) or a StatusContext (state) into pass/fail/pending.
func checkOutcome(typename, status, conclusion, state string) string {
	if typename == "StatusContext" {
		switch strings.ToUpper(state) {
		case "SUCCESS":
			return "pass"
		case "FAILURE", "ERROR":
			return "fail"
		}
		return "pending"
	}
	if !strings.EqualFold(status, "COMPLETED") {
		return "pending"
	}
	switch strings.ToUpper(conclusion) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return "pass"
	case "":
		return "pending"
	}
	return "fail" // failure, cancelled, timed out, action required, startup failure
}

// FetchPR loads one PR with everything the detail screen needs.
func FetchPR(r RepoRef, number string) (PRDetail, error) {
	out, err := Run(Scoped(r, "pr", "view", number, "--json",
		"number,title,body,author,state,isDraft,baseRefName,headRefName,headRefOid,url,reviewDecision,mergeable,"+
			"mergeStateStatus,statusCheckRollup,comments,reviews,additions,deletions,changedFiles,labels")...)
	if err != nil {
		return PRDetail{}, err
	}
	return ParsePRDetail(out)
}

// FetchPRDiff returns the PR's unified diff as plain text.
func FetchPRDiff(r RepoRef, number string) (string, error) {
	out, err := Run(Scoped(r, "pr", "diff", number, "--color", "never")...)
	return string(out), err
}

// ReviewKind is what a submitted review says about the PR.
type ReviewKind string

const (
	ReviewApprove        ReviewKind = "approve"
	ReviewRequestChanges ReviewKind = "request-changes"
	ReviewComment        ReviewKind = "comment"
)

// ReviewArgs is the `gh pr review` command. The body is one --body=… argument, so text that
// starts with "-" or holds quotes and newlines can never be read as anything else. An approval
// may have no body.
func ReviewArgs(r RepoRef, number string, kind ReviewKind, body string) []string {
	a := []string{"pr", "review", number, "--" + string(kind)}
	if body != "" {
		a = append(a, "--body="+body)
	}
	return Scoped(r, a...)
}

// CommentArgs is the `gh pr comment` command.
func CommentArgs(r RepoRef, number, body string) []string {
	return Scoped(r, "pr", "comment", number, "--body="+body)
}

// ReviewPR submits a review.
func ReviewPR(r RepoRef, number string, kind ReviewKind, body string) error {
	_, err := Run(ReviewArgs(r, number, kind, body)...)
	return err
}

// CommentPR adds a comment to the PR.
func CommentPR(r RepoRef, number, body string) error {
	_, err := Run(CommentArgs(r, number, body)...)
	return err
}
