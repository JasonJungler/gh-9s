package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ProjectItem is one card of a GitHub Project (v2) board.
type ProjectItem struct {
	Status            string // "" when the item has no Status
	Type              string // "Issue", "PullRequest", "DraftIssue", or "" when the item is not readable
	Number            int
	Title, URL, Repo  string // Repo is OWNER/NAME
	State             string // OPEN, CLOSED or MERGED
	Draft             bool   // a draft pull request, or a draft issue that lives only in the Project
	Updated           string
	Assignees, Labels []string
}

// Project is a GitHub Project (v2) as read from one board.
type Project struct {
	Title       string
	Closed      bool
	StatusOrder []string // the options of the Status field, in the Project's order
	Total       int      // how many items the Project has, which can be more than Items
	Items       []ProjectItem
}

// ErrMissingScope is returned when gh's token lacks the read:project scope.
var ErrMissingScope = errors.New("missing read:project permission")

// ScopeHint is the sentence that tells the user how to get the permission.
func ScopeHint(host string) string {
	s := "Projects need the read:project permission. Run: gh auth refresh -s read:project"
	if host != "" && host != "github.com" {
		s += " --hostname " + host
	}
	return s
}

const projectPageSize = 100

const itemFields = `number title state url updatedAt repository{nameWithOwner} assignees(first:5){nodes{login}} labels(first:5){nodes{name}}`

// projectQuery only reads. Owner, number and cursor travel as variables, never inside the text.
const projectQuery = `query($owner:String!,$number:Int!,$after:String){repositoryOwner(login:$owner){... on ProjectV2Owner{projectV2(number:$number){title closed field(name:"Status"){... on ProjectV2SingleSelectField{options{name}}} items(first:100,after:$after){totalCount pageInfo{hasNextPage endCursor} nodes{fieldValueByName(name:"Status"){... on ProjectV2ItemFieldSingleSelectValue{name}} content{__typename ... on Issue{` + itemFields + `} ... on PullRequest{isDraft merged ` + itemFields + `} ... on DraftIssue{title}}}}}}}}`

type nameNodes struct {
	Nodes []struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	} `json:"nodes"`
}

func (n *nameNodes) logins() []string {
	var out []string
	if n == nil {
		return out
	}
	for _, x := range n.Nodes {
		if x.Login != "" {
			out = append(out, CleanText(x.Login))
		}
	}
	return out
}

func (n *nameNodes) names() []string {
	var out []string
	if n == nil {
		return out
	}
	for _, x := range n.Nodes {
		if x.Name != "" {
			out = append(out, oneLine(CleanText(x.Name)))
		}
	}
	return out
}

// ParseProjectPage reads one page of the answer: the Project's header fields and its items.
func ParseProjectPage(b []byte) (p Project, hasNext bool, cursor string, err error) {
	var resp struct {
		Data *struct {
			Owner *struct {
				Project *struct {
					Title  string `json:"title"`
					Closed bool   `json:"closed"`
					Field  *struct {
						Options []struct {
							Name string `json:"name"`
						} `json:"options"`
					} `json:"field"`
					Items struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNext bool   `json:"hasNextPage"`
							End     string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []*struct {
							Status *struct {
								Name string `json:"name"`
							} `json:"fieldValueByName"`
							Content *struct {
								Type    string `json:"__typename"`
								Number  int    `json:"number"`
								Title   string `json:"title"`
								State   string `json:"state"`
								URL     string `json:"url"`
								Updated string `json:"updatedAt"`
								IsDraft bool   `json:"isDraft"`
								Merged  bool   `json:"merged"`
								Repo    *struct {
									Name string `json:"nameWithOwner"`
								} `json:"repository"`
								Assignees *nameNodes `json:"assignees"`
								Labels    *nameNodes `json:"labels"`
							} `json:"content"`
						} `json:"nodes"`
					} `json:"items"`
				} `json:"projectV2"`
			} `json:"repositoryOwner"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return Project{}, false, "", err
	}
	if resp.Data == nil || resp.Data.Owner == nil || resp.Data.Owner.Project == nil {
		if len(resp.Errors) > 0 {
			msg := CleanText(resp.Errors[0].Message)
			if missingScope(msg) {
				return Project{}, false, "", fmt.Errorf("%w: %s", ErrMissingScope, msg)
			}
			return Project{}, false, "", errors.New(msg)
		}
		return Project{}, false, "", errors.New("project not found or no access")
	}
	pr := resp.Data.Owner.Project
	p = Project{Title: oneLine(CleanText(pr.Title)), Closed: pr.Closed, Total: pr.Items.TotalCount}
	if pr.Field != nil {
		for _, o := range pr.Field.Options {
			p.StatusOrder = append(p.StatusOrder, oneLine(CleanText(o.Name)))
		}
	}
	for _, n := range pr.Items.Nodes {
		if n == nil {
			continue
		}
		it := ProjectItem{}
		if n.Status != nil {
			it.Status = oneLine(CleanText(n.Status.Name))
		}
		c := n.Content
		if c == nil || (c.Type != "Issue" && c.Type != "PullRequest" && c.Type != "DraftIssue") {
			it.Title = "(no access)"
			p.Items = append(p.Items, it)
			continue
		}
		it.Type, it.Title = c.Type, oneLine(CleanText(c.Title))
		switch c.Type {
		case "DraftIssue":
			it.Draft = true
		default:
			it.Number, it.State, it.URL, it.Updated = c.Number, c.State, CleanText(c.URL), c.Updated
			if c.Repo != nil {
				it.Repo = CleanText(c.Repo.Name)
			}
			it.Assignees, it.Labels = c.Assignees.logins(), c.Labels.names()
			if c.Type == "PullRequest" {
				it.Draft = c.IsDraft
				if c.Merged {
					it.State = "MERGED"
				}
			}
		}
		p.Items = append(p.Items, it)
	}
	return p, pr.Items.PageInfo.HasNext, pr.Items.PageInfo.End, nil
}

func missingScope(msg string) bool {
	s := strings.ToLower(msg)
	return strings.Contains(s, "required scopes") || strings.Contains(s, "read:project")
}

// projectRun is the seam for tests.
var projectRun = RunWithTimeout

func projectArgs(host, owner string, number int, after string) []string {
	args := graphqlArgs(host)
	args = append(args, "-f", "query="+projectQuery, "-f", "owner="+owner, "-F", fmt.Sprintf("number=%d", number))
	if after != "" {
		args = append(args, "-f", "after="+after)
	}
	return args
}

func retryOnce(fn func() error) error {
	err := fn()
	if err != nil {
		s := strings.ToLower(err.Error())
		for _, w := range []string{"http 502", "http 503", "http 504", "timeout", "timed out", "did not answer"} {
			if strings.Contains(s, w) {
				return fn()
			}
		}
	}
	return err
}

// FetchProject reads a Project's first 200 items (two pages); Project.Total says how many there are.
func FetchProject(host, owner string, number int) (Project, error) {
	var all Project
	after := ""
	for page := 0; page < 2; page++ {
		var (
			p      Project
			more   bool
			cursor string
		)
		err := retryOnce(func() error {
			out, err := projectRun(inboxTimeout, projectArgs(host, owner, number, after)...)
			if err != nil && len(out) == 0 {
				if missingScope(err.Error()) {
					return fmt.Errorf("%w: %s", ErrMissingScope, CleanText(err.Error()))
				}
				return err
			}
			p, more, cursor, err = ParseProjectPage(out)
			return err
		})
		if err != nil {
			return Project{}, err
		}
		if page == 0 {
			all = p
		} else {
			all.Items = append(all.Items, p.Items...)
		}
		if !more || cursor == "" {
			break
		}
		after = cursor
	}
	return all, nil
}
