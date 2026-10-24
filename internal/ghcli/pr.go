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
	return a.Login
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
		Number: raw.Number, Title: raw.Title, Body: raw.Body, Author: loginOrGhost(raw.Author), State: raw.State,
		BaseRef: raw.BaseRefName, HeadRef: raw.HeadRefName, URL: raw.URL, ReviewDecision: raw.ReviewDecision,
		Mergeable: raw.Mergeable, MergeState: raw.MergeStateStatus, IsDraft: raw.IsDraft,
		Additions: raw.Additions, Deletions: raw.Deletions, ChangedFiles: raw.ChangedFiles,
	}
	for _, l := range raw.Labels {
		d.Labels = append(d.Labels, l.Name)
	}
	for _, c := range raw.Comments {
		d.Comments = append(d.Comments, Comment{loginOrGhost(c.Author), c.Body, c.CreatedAt})
	}
	for _, r := range raw.Reviews {
		d.Reviews = append(d.Reviews, Review{loginOrGhost(r.Author), r.State, r.Body, r.SubmittedAt})
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
		"number,title,body,author,state,isDraft,baseRefName,headRefName,url,reviewDecision,mergeable,"+
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
