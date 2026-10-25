package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ProjectItem is one card of a GitHub Project (v2) board.
type ProjectItem struct {
	ID                string // the card's node id in the Project
	Status            string // "" when the item has no Status
	Type              string // "Issue", "PullRequest", "DraftIssue", or "" when the item is not readable
	Number            int
	Body              string // a draft card's text
	Title, URL, Repo  string // Repo is OWNER/NAME
	State             string // OPEN, CLOSED or MERGED
	Draft             bool   // a draft pull request, or a draft issue that lives only in the Project
	Updated           string
	Assignees, Labels []string
	Dates             map[string]string // date fields by field name, as YYYY-MM-DD
	DraftID           string            // a draft card's own id, which editing it needs
	Values            map[string]string // the other fields as text, by field id
}

// FieldOption is one choice of a single-select or iteration field.
type FieldOption struct{ ID, Name string }

// ProjectField is a field of a Project that can be changed from the board.
type ProjectField struct {
	ID, Name string
	Type     string // TEXT, NUMBER, DATE, SINGLE_SELECT or ITERATION
	Options  []FieldOption
}

// ProjectView is a saved view of a Project: its layout, filter text and first sort.
type ProjectView struct {
	Name, Layout, Filter string // Layout is BOARD_LAYOUT, TABLE_LAYOUT or ROADMAP_LAYOUT
	SortField            string
	SortDesc             bool
	DateFields           []string // the date fields the view shows, in order (a roadmap is drawn by the first)
}

// Project is a GitHub Project (v2) as read from one board.
type Project struct {
	ID          string            // the Project's node id, which the write calls need
	StatusField string            // the id of its Status field
	StatusIDs   map[string]string // Status option name to option id
	Title       string
	Closed      bool
	URL         string   // the Project on the web
	StatusOrder []string // the options of the Status field, in the Project's order
	Total       int      // how many items the Project has, which can be more than Items
	Items       []ProjectItem
	Views       []ProjectView  // the saved views, in the Project's order
	Note        string         // what could not be read, for the screen to say
	Fields      []ProjectField // the fields besides Status that can be changed
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

const (
	projectPageSize = 100
	maxViews        = 100
	maxProjectPages = 20 // 2000 cards
)

const itemFields = `number title state url updatedAt repository{nameWithOwner} assignees(first:20){nodes{login}} labels(first:20){nodes{name}}`

// projectQuery only reads. Owner, number and cursor travel as variables, never inside the text.
const projectQuery = `query($owner:String!,$number:Int!,$after:String){repositoryOwner(login:$owner){... on ProjectV2Owner{projectV2(number:$number){id title closed url views(first:100){nodes{name layout filter fields(first:30){nodes{... on ProjectV2Field{name dataType}}} sortByFields(first:1){nodes{field{... on ProjectV2FieldCommon{name}} direction}}}} field(name:"Status"){... on ProjectV2SingleSelectField{id options{id name}}} fields(first:50){nodes{... on ProjectV2Field{id name dataType} ... on ProjectV2SingleSelectField{id name dataType options{id name}} ... on ProjectV2IterationField{id name dataType configuration{iterations{id title}}}}} items(first:100,after:$after){totalCount pageInfo{hasNextPage endCursor} nodes{id fieldValues(first:30){nodes{__typename ... on ProjectV2ItemFieldDateValue{date field{... on ProjectV2FieldCommon{id name}}} ... on ProjectV2ItemFieldTextValue{text field{... on ProjectV2FieldCommon{id name}}} ... on ProjectV2ItemFieldNumberValue{number field{... on ProjectV2FieldCommon{id name}}} ... on ProjectV2ItemFieldSingleSelectValue{name field{... on ProjectV2FieldCommon{id name}}} ... on ProjectV2ItemFieldIterationValue{title field{... on ProjectV2FieldCommon{id name}}}}} fieldValueByName(name:"Status"){... on ProjectV2ItemFieldSingleSelectValue{name}} content{__typename ... on Issue{` + itemFields + `} ... on PullRequest{isDraft merged ` + itemFields + `} ... on DraftIssue{id title body}}}}}}}}`

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
					ID     string `json:"id"`
					Title  string `json:"title"`
					Closed bool   `json:"closed"`
					URL    string `json:"url"`
					Views  struct {
						Nodes []struct {
							Name, Layout string
							Filter       *string
							Fields       struct {
								Nodes []struct {
									Name     string `json:"name"`
									DataType string `json:"dataType"`
								} `json:"nodes"`
							} `json:"fields"`
							Sort struct {
								Nodes []struct {
									Field     *struct{ Name string } `json:"field"`
									Direction string                 `json:"direction"`
								} `json:"nodes"`
							} `json:"sortByFields"`
						} `json:"nodes"`
					} `json:"views"`
					Fields struct {
						Nodes []struct {
							ID       string `json:"id"`
							Name     string `json:"name"`
							DataType string `json:"dataType"`
							Options  []struct {
								ID   string `json:"id"`
								Name string `json:"name"`
							} `json:"options"`
							Config *struct {
								Iterations []struct {
									ID    string `json:"id"`
									Title string `json:"title"`
								} `json:"iterations"`
							} `json:"configuration"`
						} `json:"nodes"`
					} `json:"fields"`
					Field *struct {
						ID      string `json:"id"`
						Options []struct {
							ID   string `json:"id"`
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
							ID     string `json:"id"`
							Values struct {
								Nodes []struct {
									Type   string   `json:"__typename"`
									Date   string   `json:"date"`
									Text   string   `json:"text"`
									Number *float64 `json:"number"`
									Name   string   `json:"name"`
									Title  string   `json:"title"`
									Field  *struct {
										ID   string `json:"id"`
										Name string `json:"name"`
									} `json:"field"`
								} `json:"nodes"`
							} `json:"fieldValues"`
							Status *struct {
								Name string `json:"name"`
							} `json:"fieldValueByName"`
							Content *struct {
								Type    string `json:"__typename"`
								Number  int    `json:"number"`
								Title   string `json:"title"`
								Body    string `json:"body"`
								DraftID string `json:"id"`
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
	p = Project{ID: pr.ID, Title: oneLine(CleanText(pr.Title)), Closed: pr.Closed, URL: CleanText(pr.URL), Total: pr.Items.TotalCount}
	if pr.Field != nil {
		p.StatusField, p.StatusIDs = pr.Field.ID, map[string]string{}
		for _, o := range pr.Field.Options {
			name := oneLine(CleanText(o.Name))
			for n := 2; ; n++ { // options that read the same stay apart, so each writes its own id
				if _, dup := p.StatusIDs[name]; !dup {
					break
				}
				name = fmt.Sprintf("%s (%d)", oneLine(CleanText(o.Name)), n)
			}
			p.StatusOrder = append(p.StatusOrder, name)
			p.StatusIDs[name] = o.ID
		}
	}
	for _, f := range pr.Fields.Nodes {
		if f.ID == "" || f.ID == p.StatusField {
			continue
		}
		pf := ProjectField{ID: f.ID, Name: oneLine(CleanText(f.Name))}
		switch f.DataType {
		case "TEXT", "NUMBER", "DATE":
			pf.Type = f.DataType
		case "SINGLE_SELECT":
			pf.Type = f.DataType
			for _, o := range f.Options {
				pf.Options = append(pf.Options, FieldOption{o.ID, oneLine(CleanText(o.Name))})
			}
		case "ITERATION":
			pf.Type = f.DataType
			if f.Config != nil {
				for _, o := range f.Config.Iterations {
					pf.Options = append(pf.Options, FieldOption{o.ID, oneLine(CleanText(o.Title))})
				}
			}
		default:
			continue
		}
		p.Fields = append(p.Fields, pf)
	}
	if len(pr.Views.Nodes) >= maxViews {
		p.Note = fmt.Sprintf("only the first %d views are read", maxViews)
	}
	for _, v := range pr.Views.Nodes {
		pv := ProjectView{Name: oneLine(CleanText(v.Name)), Layout: v.Layout}
		if v.Filter != nil {
			pv.Filter = oneLine(CleanText(*v.Filter))
		}
		if len(v.Sort.Nodes) > 0 && v.Sort.Nodes[0].Field != nil {
			pv.SortField, pv.SortDesc = CleanText(v.Sort.Nodes[0].Field.Name), v.Sort.Nodes[0].Direction == "DESC"
		}
		for _, f := range v.Fields.Nodes {
			if f.DataType == "DATE" && f.Name != "" {
				pv.DateFields = append(pv.DateFields, oneLine(CleanText(f.Name)))
			}
		}
		p.Views = append(p.Views, pv)
	}
	for _, n := range pr.Items.Nodes {
		if n == nil {
			continue
		}
		it := ProjectItem{ID: n.ID}
		for _, v := range n.Values.Nodes {
			if v.Field == nil {
				continue
			}
			val := ""
			switch v.Type {
			case "ProjectV2ItemFieldDateValue":
				if len(v.Date) >= 10 {
					if it.Dates == nil {
						it.Dates = map[string]string{}
					}
					it.Dates[oneLine(CleanText(v.Field.Name))] = CleanText(v.Date[:10])
					val = CleanText(v.Date[:10])
				}
			case "ProjectV2ItemFieldTextValue":
				val = oneLine(CleanText(v.Text))
			case "ProjectV2ItemFieldNumberValue":
				if v.Number != nil {
					val = strconv.FormatFloat(*v.Number, 'f', -1, 64)
				}
			case "ProjectV2ItemFieldSingleSelectValue":
				val = oneLine(CleanText(v.Name))
			case "ProjectV2ItemFieldIterationValue":
				val = oneLine(CleanText(v.Title))
			}
			if val != "" && v.Field.ID != "" {
				if it.Values == nil {
					it.Values = map[string]string{}
				}
				it.Values[v.Field.ID] = val
			}
		}
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
			it.Draft, it.Body, it.DraftID = true, CleanText(c.Body), c.DraftID
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
	return strings.Contains(s, "read:project") || (strings.Contains(s, "required scopes") && strings.Contains(s, "project"))
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

// FetchProject reads a Project's items, up to maxProjectPages pages of 100; Project.Total says how many there are.
// When a later page cannot be read the cards read so far are kept and Project.Note says so.
func FetchProject(host, owner string, number int) (Project, error) {
	var all Project
	after := ""
	for page := 0; page < maxProjectPages; page++ {
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
			if page == 0 {
				return Project{}, err
			}
			note := fmt.Sprintf("page %d of the cards could not be read (%s); showing the first %d", page+1, oneLine(CleanText(err.Error())), len(all.Items))
			all.Note = joinNote(all.Note, note)
			return all, nil
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

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// ProjectRef is a Project found by ListProjects.
type ProjectRef struct {
	Host, Owner string
	Number      int
	Title, URL  string
	Closed      bool
}

const listProjectsQuery = `{viewer{login projectsV2(first:50){nodes{number title closed url}} organizations(first:50){nodes{login projectsV2(first:50){nodes{number title closed url}}}}}}`

type projectNodes struct {
	Nodes []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Closed bool   `json:"closed"`
		URL    string `json:"url"`
	} `json:"nodes"`
}

// ParseProjectList reads the answer to listProjectsQuery: the viewer's own Projects, then each organisation's.
func ParseProjectList(host string, b []byte) ([]ProjectRef, error) {
	var resp struct {
		Data *struct {
			Viewer struct {
				Login    string       `json:"login"`
				Projects projectNodes `json:"projectsV2"`
				Orgs     struct {
					Nodes []struct {
						Login    string       `json:"login"`
						Projects projectNodes `json:"projectsV2"`
					} `json:"nodes"`
				} `json:"organizations"`
			} `json:"viewer"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		if len(resp.Errors) > 0 {
			msg := CleanText(resp.Errors[0].Message)
			if missingScope(msg) {
				return nil, fmt.Errorf("%w: %s", ErrMissingScope, msg)
			}
			return nil, errors.New(msg)
		}
		return nil, errors.New("no data in the response")
	}
	if host == "github.com" {
		host = ""
	}
	var out []ProjectRef
	add := func(owner string, n projectNodes) {
		for _, p := range n.Nodes {
			if p.Number > 0 && owner != "" {
				out = append(out, ProjectRef{Host: host, Owner: CleanText(owner), Number: p.Number,
					Title: oneLine(CleanText(p.Title)), URL: CleanText(p.URL), Closed: p.Closed})
			}
		}
	}
	add(resp.Data.Viewer.Login, resp.Data.Viewer.Projects)
	for _, o := range resp.Data.Viewer.Orgs.Nodes {
		add(o.Login, o.Projects)
	}
	return out, nil
}

// ListProjects lists the Projects the user can see on one host (their own and their organisations').
func ListProjects(host string) ([]ProjectRef, error) {
	args := append(graphqlArgs(host), "-f", "query="+listProjectsQuery)
	var out []byte
	err := retryOnce(func() error {
		var e error
		out, e = projectRun(inboxTimeout, args...)
		if e != nil && len(out) == 0 {
			if missingScope(e.Error()) {
				return fmt.Errorf("%w: %s", ErrMissingScope, CleanText(e.Error()))
			}
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ParseProjectList(host, out)
}
