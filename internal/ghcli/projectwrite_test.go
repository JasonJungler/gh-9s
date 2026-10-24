package ghcli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func stubInput(t *testing.T, out string, err error) *[]struct {
	body string
	args []string
} {
	t.Helper()
	var calls []struct {
		body string
		args []string
	}
	old := projectInput
	projectInput = func(stdin []byte, args ...string) ([]byte, error) {
		calls = append(calls, struct {
			body string
			args []string
		}{string(stdin), args})
		return []byte(out), err
	}
	t.Cleanup(func() { projectInput = old })
	return &calls
}

func TestSetProjectStatusSendsIdsAsVariables(t *testing.T) {
	calls := stubInput(t, `{"data":{"updateProjectV2ItemFieldValue":{"projectV2Item":{"id":"i"}}}}`, nil)
	if err := SetProjectStatus("ghe.corp.com", "PVT_1", "PVTI_2", "PVTSSF_3", "opt4"); err != nil {
		t.Fatal(err)
	}
	c := (*calls)[0]
	var body struct {
		Query     string
		Variables map[string]string
	}
	if err := json.Unmarshal([]byte(c.body), &body); err != nil {
		t.Fatal(err)
	}
	if body.Variables["project"] != "PVT_1" || body.Variables["item"] != "PVTI_2" || body.Variables["field"] != "PVTSSF_3" || body.Variables["option"] != "opt4" {
		t.Errorf("%+v", body.Variables)
	}
	for _, id := range []string{"PVT_1", "PVTI_2", "PVTSSF_3", "opt4"} {
		if strings.Contains(body.Query, id) {
			t.Errorf("id %s pasted into the query", id)
		}
	}
	if !strings.Contains(body.Query, "updateProjectV2ItemFieldValue") || strings.Contains(body.Query, "delete") {
		t.Errorf("query %s", body.Query)
	}
	if j := strings.Join(c.args, " "); !strings.Contains(j, "--hostname ghe.corp.com") || !strings.Contains(j, "--input -") {
		t.Errorf("args %v", c.args)
	}
	if err := SetProjectStatus("", "", "i", "f", "o"); err == nil {
		t.Error("missing ids accepted")
	}
}

func TestWriteScopeAndOtherErrors(t *testing.T) {
	stubInput(t, `{"data":null,"errors":[{"type":"INSUFFICIENT_SCOPES","message":"Your token has not been granted the required scopes to execute this query. The 'id' field requires one of the following scopes: ['project']"}]}`, nil)
	err := SetProjectStatus("", "p", "i", "f", "o")
	if !errors.Is(err, ErrNeedsProjectScope) || !strings.Contains(err.Error(), "gh auth refresh -s project") {
		t.Errorf("%v", err)
	}
	stubInput(t, `{"data":null,"errors":[{"message":"Could not resolve to a node with the global id of 'x'"}]}`, nil)
	err = SetProjectStatus("", "p", "i", "f", "o")
	if err == nil || errors.Is(err, ErrNeedsProjectScope) {
		t.Errorf("%v", err)
	}
	stubInput(t, "", errors.New("HTTP 403: Resource protected by organization SAML enforcement"))
	if err = SetProjectStatus("", "p", "i", "f", "o"); err == nil || errors.Is(err, ErrNeedsProjectScope) {
		t.Errorf("%v", err)
	}
}

func TestResolveAndAddItem(t *testing.T) {
	calls := stubInput(t, `{"data":{"repository":{"issueOrPullRequest":{"__typename":"PullRequest","id":"PR_9","title":"Fix\u001b[1m it"}}}}`, nil)
	c, err := ResolveContent("", "acme", "app", 9)
	if err != nil || c.ID != "PR_9" || c.Kind != "PullRequest" || strings.ContainsRune(c.Title, 0x1b) {
		t.Fatalf("%+v %v", c, err)
	}
	if strings.Contains((*calls)[0].body, `"query":"query($owner`) == false || strings.Contains((*calls)[0].body, "acme\"}") == false {
		t.Errorf("body %s", (*calls)[0].body)
	}
	stubInput(t, `{"data":{"repository":{"issueOrPullRequest":null}}}`, nil)
	if _, err := ResolveContent("", "acme", "app", 1); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("%v", err)
	}
	stubInput(t, `{"data":{"addProjectV2ItemById":{"item":{"id":"PVTI_new"}}}}`, nil)
	id, err := AddProjectItem("", "PVT_1", "PR_9")
	if err != nil || id != "PVTI_new" {
		t.Errorf("%q %v", id, err)
	}
	if _, err := AddProjectItem("", "", "x"); err == nil {
		t.Error("missing id accepted")
	}
}

func TestOnlyTheThreeKnownMutationsExist(t *testing.T) {
	for _, q := range []string{setStatusMutation, addItemMutation} {
		if strings.Count(q, "mutation") != 1 || strings.Contains(q, "delete") || strings.Contains(q, "archive") {
			t.Errorf("%s", q)
		}
	}
	for _, q := range []string{projectQuery, listProjectsQuery, contentQuery} {
		if strings.Contains(strings.ToLower(q), "mutation") {
			t.Errorf("a read query writes: %s", q)
		}
	}
}

func TestEditPRAssigneesArgs(t *testing.T) {
	got := strings.Join(EditPRAssigneesArgs(RepoRef{Owner: "a", Name: "b"}, "7", []string{"ann"}, []string{"bob"}), " ")
	if !strings.HasPrefix(got, "pr edit 7 --add-assignee=ann --remove-assignee=bob") || !strings.Contains(got, "a/b") {
		t.Error(got)
	}
}

func TestProjectIdsAreRead(t *testing.T) {
	b := `{"data":{"repositoryOwner":{"projectV2":{"id":"PVT_1","title":"P","closed":false,"field":{"id":"F1","options":[{"id":"o1","name":"Todo"},{"id":"o2","name":"Done"}]},
"items":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"PVTI_1","fieldValueByName":null,"content":{"__typename":"Issue","number":1,"title":"t","state":"OPEN","url":"u","updatedAt":"x","repository":{"nameWithOwner":"a/b"}}}]}}}}}`
	p, _, _, err := ParseProjectPage([]byte(b))
	if err != nil || p.ID != "PVT_1" || p.StatusField != "F1" || p.StatusIDs["Done"] != "o2" || p.Items[0].ID != "PVTI_1" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestSameLookingStatusOptionsKeepTheirOwnIds(t *testing.T) {
	page := `{"data":{"repositoryOwner":{"projectV2":{"id":"P","title":"T","field":{"id":"F","options":[{"id":"a","name":"Todo"},{"id":"b","name":"Todo"}]},"items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}`
	p, _, _, err := ParseProjectPage([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.StatusOrder) != 2 || p.StatusOrder[0] == p.StatusOrder[1] || p.StatusIDs[p.StatusOrder[0]] != "a" || p.StatusIDs[p.StatusOrder[1]] != "b" {
		t.Errorf("order %v ids %v", p.StatusOrder, p.StatusIDs)
	}
}
