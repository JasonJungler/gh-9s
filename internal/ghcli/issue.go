package ghcli

import "encoding/json"

// IssueFilter narrows the issue list. State is "open", "closed" or "all" ("" means open).
type IssueFilter struct {
	State                    string
	AssignedToMe, OpenedByMe bool
}

// IssueFilterArgs are the extra `gh issue list` flags for f. Only the three known states get
// through, so a stray value can never turn into another flag.
func IssueFilterArgs(f IssueFilter) []string {
	state := "open"
	if f.State == "closed" || f.State == "all" {
		state = f.State
	}
	a := []string{"--state", state}
	if f.AssignedToMe {
		a = append(a, "--assignee", "@me")
	}
	if f.OpenedByMe {
		a = append(a, "--author", "@me")
	}
	return a
}

// IssueDetail is what the issue screen shows.
type IssueDetail struct {
	Number                          int
	Title, Body, Author, State, URL string
	Labels, Assignees               []string
	Comments                        []Comment
}

// ParseIssueDetail reads the JSON of `gh issue view --json ...` (fields listed in FetchIssue).
func ParseIssueDetail(b []byte) (IssueDetail, error) {
	var raw struct {
		Number int       `json:"number"`
		Title  string    `json:"title"`
		Body   string    `json:"body"`
		Author *rawLogin `json:"author"`
		State  string    `json:"state"`
		URL    string    `json:"url"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Assignees []rawLogin `json:"assignees"`
		Comments  []struct {
			Author    *rawLogin `json:"author"`
			Body      string    `json:"body"`
			CreatedAt string    `json:"createdAt"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return IssueDetail{}, err
	}
	d := IssueDetail{Number: raw.Number, Title: raw.Title, Body: raw.Body, Author: loginOrGhost(raw.Author),
		State: raw.State, URL: raw.URL}
	for _, l := range raw.Labels {
		d.Labels = append(d.Labels, l.Name)
	}
	for _, a := range raw.Assignees {
		if a.Login != "" {
			d.Assignees = append(d.Assignees, a.Login)
		}
	}
	for _, c := range raw.Comments {
		d.Comments = append(d.Comments, Comment{loginOrGhost(c.Author), c.Body, c.CreatedAt})
	}
	return d, nil
}

// FetchIssue loads one issue with its comments.
func FetchIssue(r RepoRef, number string) (IssueDetail, error) {
	out, err := Run(Scoped(r, "issue", "view", number, "--json",
		"number,title,body,author,state,url,labels,assignees,comments")...)
	if err != nil {
		return IssueDetail{}, err
	}
	return ParseIssueDetail(out)
}

// CommentIssueArgs is the `gh issue comment` command; the body is one --body=… argument.
func CommentIssueArgs(r RepoRef, number, body string) []string {
	return Scoped(r, "issue", "comment", number, "--body="+body)
}

// CommentIssue adds a comment to the issue.
func CommentIssue(r RepoRef, number, body string) error {
	_, err := Run(CommentIssueArgs(r, number, body)...)
	return err
}
