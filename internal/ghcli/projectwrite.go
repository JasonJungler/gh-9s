package ghcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
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

const contentQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){issueOrPullRequest(number:$number){__typename ... on Issue{id title projectItems(first:50){nodes{id project{id title}}}} ... on PullRequest{id title projectItems(first:50){nodes{id project{id title}}}}}}}`

const deleteItemMutation = `mutation($project:ID!,$item:ID!){deleteProjectV2Item(input:{projectId:$project,itemId:$item}){deletedItemId}}`

const archiveItemMutation = `mutation($project:ID!,$item:ID!){archiveProjectV2Item(input:{projectId:$project,itemId:$item}){item{id}}}`

const addDraftMutation = `mutation($project:ID!,$title:String!){addProjectV2DraftIssue(input:{projectId:$project,title:$title}){projectItem{id}}}`

const editDraftMutation = `mutation($draft:ID!,$title:String,$body:String){updateProjectV2DraftIssue(input:{draftIssueId:$draft,title:$title,body:$body}){draftIssue{id}}}`

const setFieldMutation = `mutation($project:ID!,$item:ID!,$field:ID!,$value:ProjectV2FieldValue!){updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field,value:$value}){projectV2Item{id}}}`

const clearFieldMutation = `mutation($project:ID!,$item:ID!,$field:ID!){clearProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field}){projectV2Item{id}}}`

const moveItemMutation = `mutation($project:ID!,$item:ID!,$after:ID){updateProjectV2ItemPosition(input:{projectId:$project,itemId:$item,afterId:$after}){items(first:1){nodes{id}}}}`

// projectInput is the seam for tests: gh with a JSON body on stdin.
var projectInput = RunInput

// graphqlBody builds the request body; variables travel as JSON, never inside the query text.
func graphqlBody(query string, vars map[string]any) ([]byte, error) {
	b, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, fmt.Errorf("the request could not be built: %w", err)
	}
	return b, nil
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
	return runWriteMode(host, query, vars, false)
}

// runWriteMode is runWrite; with partial set, data that came back with errors is used (a lookup that SSO
// blocked in part still answers for the rest).
func runWriteMode(host, query string, vars map[string]any, partial bool) (json.RawMessage, error) {
	body, berr := graphqlBody(query, vars)
	if berr != nil {
		return nil, berr
	}
	out, err := projectInput(body, inputArgs(host)...)
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
	if len(resp.Errors) > 0 && !(partial && len(resp.Data) > 0 && string(resp.Data) != "null") {
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
// Membership is a card of an issue or PR in a Project.
type Membership struct{ ItemID, ProjectID, Title string }

type Content struct {
	ID, Title, Kind string   // Kind is "Issue" or "PullRequest"
	OnProjects      []string // ids of the Projects it is already on
	Memberships     []Membership
}

// ResolveContent finds an issue or PR by repo and number, to add it to a Project.
func ResolveContent(host, owner, name string, number int) (Content, error) {
	data, err := runWriteMode(host, contentQuery, map[string]any{"owner": owner, "name": name, "number": number}, true)
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
						ID      string `json:"id"`
						Project struct {
							ID    string `json:"id"`
							Title string `json:"title"`
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
		c.Memberships = append(c.Memberships, Membership{ItemID: n.ID, ProjectID: n.Project.ID, Title: oneLine(CleanText(n.Project.Title))})
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

var errNoIDs = errors.New("this Project was read without the ids needed to change it; press R and try again")

func need(ids ...string) error {
	for _, id := range ids {
		if id == "" {
			return errNoIDs
		}
	}
	return nil
}

// RemoveProjectItem takes a card out of the Project. An issue or PR stays where it is; a draft is gone for good.
func RemoveProjectItem(host, projectID, itemID string) error {
	if err := need(projectID, itemID); err != nil {
		return err
	}
	_, err := runWrite(host, deleteItemMutation, map[string]any{"project": projectID, "item": itemID})
	return err
}

// ArchiveProjectItem archives a card; the board does not read archived cards.
func ArchiveProjectItem(host, projectID, itemID string) error {
	if err := need(projectID, itemID); err != nil {
		return err
	}
	_, err := runWrite(host, archiveItemMutation, map[string]any{"project": projectID, "item": itemID})
	return err
}

// AddProjectDraft creates a draft card with a title and returns its id.
func AddProjectDraft(host, projectID, title string) (string, error) {
	if err := need(projectID); err != nil {
		return "", err
	}
	if strings.TrimSpace(title) == "" {
		return "", errors.New("a draft needs a title")
	}
	data, err := runWrite(host, addDraftMutation, map[string]any{"project": projectID, "title": title})
	if err != nil {
		return "", err
	}
	var d struct {
		Add struct {
			Item struct {
				ID string `json:"id"`
			} `json:"projectItem"`
		} `json:"addProjectV2DraftIssue"`
	}
	if err := json.Unmarshal(data, &d); err != nil || d.Add.Item.ID == "" {
		return "", errors.New("GitHub did not return the new card")
	}
	return d.Add.Item.ID, nil
}

// EditProjectDraft changes a draft card's title and/or text; a nil part is left as it is on GitHub.
func EditProjectDraft(host, draftID string, title, body *string) error {
	if err := need(draftID); err != nil {
		return err
	}
	vars := map[string]any{"draft": draftID}
	if title != nil {
		if strings.TrimSpace(*title) == "" {
			return errors.New("a draft needs a title")
		}
		vars["title"] = *title
	}
	if body != nil {
		vars["body"] = *body
	}
	if title == nil && body == nil {
		return errors.New("nothing to change")
	}
	_, err := runWrite(host, editDraftMutation, vars)
	return err
}

// FieldValue is the new value of a field: exactly one of its parts is meaningful for the field's type.
type FieldValue struct {
	Text, Date, Option string
	Number             *float64
}

func (v FieldValue) input(typ string) (map[string]any, error) {
	switch typ {
	case "TEXT":
		return map[string]any{"text": v.Text}, nil
	case "NUMBER":
		if v.Number == nil {
			return nil, errors.New("a number is needed")
		}
		return map[string]any{"number": *v.Number}, nil
	case "DATE":
		if _, err := time.Parse("2006-01-02", v.Date); err != nil {
			return nil, errors.New("a date is written YYYY-MM-DD")
		}
		return map[string]any{"date": v.Date}, nil
	case "SINGLE_SELECT":
		if v.Option == "" {
			return nil, errors.New("an option is needed")
		}
		return map[string]any{"singleSelectOptionId": v.Option}, nil
	case "ITERATION":
		if v.Option == "" {
			return nil, errors.New("an iteration is needed")
		}
		return map[string]any{"iterationId": v.Option}, nil
	}
	return nil, errors.New("this kind of field cannot be changed here")
}

// SetProjectField sets a field of a card; a nil value clears it.
func SetProjectField(host, projectID, itemID, fieldID, typ string, v *FieldValue) error {
	if err := need(projectID, itemID, fieldID); err != nil {
		return err
	}
	if v == nil {
		_, err := runWrite(host, clearFieldMutation, map[string]any{"project": projectID, "item": itemID, "field": fieldID})
		return err
	}
	in, err := v.input(typ)
	if err != nil {
		return err
	}
	_, err = runWrite(host, setFieldMutation, map[string]any{"project": projectID, "item": itemID, "field": fieldID, "value": in})
	return err
}

// MoveProjectItem puts a card after another one (afterID empty: at the top).
func MoveProjectItem(host, projectID, itemID, afterID string) error {
	if err := need(projectID, itemID); err != nil {
		return err
	}
	vars := map[string]any{"project": projectID, "item": itemID}
	if afterID != "" {
		vars["after"] = afterID
	}
	_, err := runWrite(host, moveItemMutation, vars)
	return err
}
