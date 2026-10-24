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

// CloseIssueArgs is the `gh issue close` command.
func CloseIssueArgs(r RepoRef, number string) []string {
	return Scoped(r, "issue", "close", number)
}

// ReopenIssueArgs is the `gh issue reopen` command.
func ReopenIssueArgs(r RepoRef, number string) []string {
	return Scoped(r, "issue", "reopen", number)
}

// EditIssueArgs is the `gh issue edit` command for label and assignee changes. Every name is
// its own --flag=value argument, so a name with spaces, an emoji, or a leading "-" stays one value.
func EditIssueArgs(r RepoRef, number string, addLabels, removeLabels, addAssignees, removeAssignees []string) []string {
	a := []string{"issue", "edit", number}
	for _, group := range []struct {
		flag  string
		names []string
	}{
		{"--add-label=", addLabels}, {"--remove-label=", removeLabels},
		{"--add-assignee=", addAssignees}, {"--remove-assignee=", removeAssignees},
	} {
		for _, n := range group.names {
			a = append(a, group.flag+n)
		}
	}
	return Scoped(r, a...)
}

// ParseLabelNames reads the JSON of `gh label list --json name`.
func ParseLabelNames(b []byte) ([]string, error) {
	var raw []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var out []string
	for _, l := range raw {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}
	return out, nil
}

// ParseLogins reads a list of users ([{"login": ...}]).
func ParseLogins(b []byte) ([]string, error) {
	var raw []rawLogin
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var out []string
	for _, u := range raw {
		if u.Login != "" {
			out = append(out, u.Login)
		}
	}
	return out, nil
}

// ListLabels lists the repo's labels (up to 200).
func ListLabels(r RepoRef) ([]string, error) {
	out, err := Run(Scoped(r, "label", "list", "--limit", "200", "--json", "name")...)
	if err != nil {
		return nil, err
	}
	return ParseLabelNames(out)
}

// ListAssignees lists the users who can be assigned in the repo (up to 100).
func ListAssignees(r RepoRef) ([]string, error) {
	endpoint, flags := r.APIEndpoint("/assignees?per_page=100")
	out, err := Run(append(append([]string{"api"}, flags...), endpoint)...)
	if err != nil {
		return nil, err
	}
	return ParseLogins(out)
}

// CloseIssue closes the issue.
func CloseIssue(r RepoRef, number string) error {
	_, err := Run(CloseIssueArgs(r, number)...)
	return err
}

// ReopenIssue reopens the issue.
func ReopenIssue(r RepoRef, number string) error {
	_, err := Run(ReopenIssueArgs(r, number)...)
	return err
}

// EditIssue changes the issue's labels and assignees.
func EditIssue(r RepoRef, number string, addLabels, removeLabels, addAssignees, removeAssignees []string) error {
	_, err := Run(EditIssueArgs(r, number, addLabels, removeLabels, addAssignees, removeAssignees)...)
	return err
}
