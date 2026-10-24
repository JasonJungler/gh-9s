package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The only writes gh-9s makes to a Project. Everything else in this package reads.

// ErrNeedsProjectScope is returned when the token lacks the project (write) permission.
var ErrNeedsProjectScope = errors.New("missing project permission")

// WriteScopeHint tells how to get the permission to change a Project.
func WriteScopeHint(host string) string {
	s := "Changing a Project needs the project permission. Run: gh auth refresh -s project"
	if host != "" && host != "github.com" {
		s += " --hostname " + host
	}
	return s
}

const setStatusMutation = `mutation($project:ID!,$item:ID!,$field:ID!,$option:String!){updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field,value:{singleSelectOptionId:$option}}){projectV2Item{id}}}`

const addItemMutation = `mutation($project:ID!,$content:ID!){addProjectV2ItemById(input:{projectId:$project,contentId:$content}){item{id}}}`

const contentQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){issueOrPullRequest(number:$number){__typename ... on Issue{id title projectItems(first:50){nodes{project{id}}}} ... on PullRequest{id title projectItems(first:50){nodes{project{id}}}}}}}`

// projectInput is the seam for tests: gh with a JSON body on stdin.
var projectInput = RunInput

// graphqlBody builds the request body; variables travel as JSON, never inside the query text.
func graphqlBody(query string, vars map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	return b
}

func inputArgs(host string) []string { return append(graphqlArgs(host), "--input", "-") }

func writeErr(host string, err error) error {
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "scopes") && strings.Contains(s, "project") {
		return fmt.Errorf("%w: %s", ErrNeedsProjectScope, WriteScopeHint(host))
	}
	return err
}

// runWrite sends one request and returns its data; GraphQL errors become errors.
func runWrite(host, query string, vars map[string]any) (json.RawMessage, error) {
	out, err := projectInput(graphqlBody(query, vars), inputArgs(host)...)
	if err != nil && len(out) == 0 {
		return nil, writeErr(host, err)
	}
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors gqlErrors       `json:"errors"`
	}
	if jerr := json.Unmarshal(out, &resp); jerr != nil {
		return nil, jerr
	}
	if len(resp.Errors) > 0 {
		return nil, writeErr(host, errors.New(CleanText(resp.Errors[0].Message)))
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, errors.New("no data in the response")
	}
	return resp.Data, nil
}

// SetProjectStatus moves a card to the Status option with the given id.
func SetProjectStatus(host, projectID, itemID, fieldID, optionID string) error {
	if projectID == "" || itemID == "" || fieldID == "" || optionID == "" {
		return errors.New("this Project was read without the ids needed to change it; press R and try again")
	}
	_, err := runWrite(host, setStatusMutation, map[string]any{"project": projectID, "item": itemID, "field": fieldID, "option": optionID})
	return err
}

// Content is an issue or pull request that can be added to a Project.
type Content struct {
	ID, Title, Kind string   // Kind is "Issue" or "PullRequest"
	OnProjects      []string // ids of the Projects it is already on
}

// ResolveContent finds an issue or PR by repo and number, to add it to a Project.
func ResolveContent(host, owner, name string, number int) (Content, error) {
	data, err := runWrite(host, contentQuery, map[string]any{"owner": owner, "name": name, "number": number})
	if err != nil {
		return Content{}, err
	}
	var d struct {
		Repository *struct {
			Item *struct {
				Type  string `json:"__typename"`
				ID    string `json:"id"`
				Title string `json:"title"`
				Items struct {
					Nodes []struct {
						Project struct {
							ID string `json:"id"`
						} `json:"project"`
					} `json:"nodes"`
				} `json:"projectItems"`
			} `json:"issueOrPullRequest"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return Content{}, err
	}
	if d.Repository == nil || d.Repository.Item == nil || d.Repository.Item.ID == "" {
		return Content{}, fmt.Errorf("%s/%s#%d was not found", owner, name, number)
	}
	it := d.Repository.Item
	c := Content{ID: it.ID, Title: oneLine(CleanText(it.Title)), Kind: it.Type}
	for _, n := range it.Items.Nodes {
		c.OnProjects = append(c.OnProjects, n.Project.ID)
	}
	return c, nil
}

// AddProjectItem puts an issue or PR (by node id) into the Project and returns the new card's id.
func AddProjectItem(host, projectID, contentID string) (string, error) {
	if projectID == "" || contentID == "" {
		return "", errors.New("missing ids to add the item")
	}
	data, err := runWrite(host, addItemMutation, map[string]any{"project": projectID, "content": contentID})
	if err != nil {
		return "", err
	}
	var d struct {
		Add struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		} `json:"addProjectV2ItemById"`
	}
	if err := json.Unmarshal(data, &d); err != nil || d.Add.Item.ID == "" {
		return "", errors.New("GitHub did not return the new card")
	}
	return d.Add.Item.ID, nil
}

// EditPRAssigneesArgs is the gh call that changes the assignees of a pull request.
func EditPRAssigneesArgs(r RepoRef, number string, add, remove []string) []string {
	a := []string{"pr", "edit", number}
	for _, n := range add {
		a = append(a, "--add-assignee="+n)
	}
	for _, n := range remove {
		a = append(a, "--remove-assignee="+n)
	}
	return Scoped(r, a...)
}

// EditPRAssignees changes the assignees of a pull request.
func EditPRAssignees(r RepoRef, number string, add, remove []string) error {
	_, err := Run(EditPRAssigneesArgs(r, number, add, remove)...)
	return err
}
