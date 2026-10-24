package ghcli

import (
	"encoding/json"
	"fmt"
	"slices"
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
	Reviewers                                         []Reviewer // who was asked and who answered, like the sidebar of the PR page
	Assignees, Projects, LinkedIssues                 []string
	Milestone                                         string
	FailingChecks                                     []string // names of the checks that failed, without repeats, at most maxFailingChecks
	Comments                                          []Comment
	Reviews                                           []Review
}

// Reviewer is a person or team asked to review, with where they stand: "REQUESTED" (asked, no answer
// yet, or asked again) or the state of their latest review (APPROVED, CHANGES_REQUESTED, COMMENTED,
// DISMISSED).
type Reviewer struct{ Name, State string }

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
		ReviewRequests []struct {
			Login string `json:"login"`
			Slug  string `json:"slug"`
			Name  string `json:"name"`
		} `json:"reviewRequests"`
		LatestReviews []struct {
			Author *rawLogin `json:"author"`
			State  string    `json:"state"`
		} `json:"latestReviews"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
		ProjectItems []struct {
			Title  string `json:"title"`
			Status *struct {
				Name string `json:"name"`
			} `json:"status"`
			Project *struct {
				Title string `json:"title"`
			} `json:"project"`
		} `json:"projectItems"`
		Milestone *struct {
			Title string `json:"title"`
		} `json:"milestone"`
		Closing []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		} `json:"closingIssuesReferences"`
		Rollup []struct {
			Typename   string `json:"__typename"`
			Name       string `json:"name"`    // a check run
			Context    string `json:"context"` // a commit status
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
	asked := map[string]bool{}
	for _, r := range raw.ReviewRequests {
		name := r.Login
		if name == "" {
			name = r.Slug
		}
		if name = CleanText(name); name != "" && !asked[name] {
			asked[name] = true
			d.Reviewers = append(d.Reviewers, Reviewer{name, "REQUESTED"})
		}
	}
	for _, r := range raw.LatestReviews {
		if name := loginOrGhost(r.Author); !asked[name] { // asked again: the request is what the page shows
			asked[name] = true
			d.Reviewers = append(d.Reviewers, Reviewer{name, r.State})
		}
	}
	for _, a := range raw.Assignees {
		if l := CleanText(a.Login); l != "" {
			d.Assignees = append(d.Assignees, l)
		}
	}
	for _, it := range raw.ProjectItems {
		name := it.Title
		if it.Project != nil && it.Project.Title != "" {
			name = it.Project.Title
		}
		if name = CleanText(name); name != "" {
			if it.Status != nil && it.Status.Name != "" {
				name += " (" + CleanText(it.Status.Name) + ")"
			}
			d.Projects = append(d.Projects, name)
		}
	}
	if raw.Milestone != nil {
		d.Milestone = CleanText(raw.Milestone.Title)
	}
	for _, c := range raw.Closing {
		d.LinkedIssues = append(d.LinkedIssues, fmt.Sprintf("#%d %s", c.Number, CleanText(c.Title)))
	}
	for _, c := range raw.Rollup {
		switch checkOutcome(c.Typename, c.Status, c.Conclusion, c.State) {
		case "pass":
			d.ChecksPassed++
		case "fail":
			d.ChecksFailed++
			name := CleanText(c.Name)
			if name == "" {
				name = CleanText(c.Context)
			}
			if name != "" && len(d.FailingChecks) < maxFailingChecks && !slices.Contains(d.FailingChecks, name) {
				d.FailingChecks = append(d.FailingChecks, name)
			}
		default:
			d.ChecksPending++
		}
	}
	return d, nil
}

// maxFailingChecks caps how many failing checks are named.
const maxFailingChecks = 20

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
			"mergeStateStatus,statusCheckRollup,comments,reviews,additions,deletions,changedFiles,labels,"+
			"reviewRequests,latestReviews,assignees,projectItems,milestone,closingIssuesReferences")...)
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
