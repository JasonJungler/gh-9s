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

func TestOnlyTheKnownMutationsExist(t *testing.T) {
	want := map[string]string{
		setStatusMutation: "updateProjectV2ItemFieldValue", addItemMutation: "addProjectV2ItemById", deleteItemMutation: "deleteProjectV2Item",
		archiveItemMutation: "archiveProjectV2Item", addDraftMutation: "addProjectV2DraftIssue", editDraftMutation: "updateProjectV2DraftIssue",
		setFieldMutation: "updateProjectV2ItemFieldValue", clearFieldMutation: "clearProjectV2ItemFieldValue", moveItemMutation: "updateProjectV2ItemPosition",
	}
	for q, name := range want {
		if strings.Count(q, "mutation") != 1 || strings.Count(q, "{"+name+"(") != 1 && !strings.Contains(q, name+"(") {
			t.Errorf("%s is not just %s", q, name)
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

// captured runs a write through the stdin seam and returns the request body it sent.
func captured(t *testing.T, resp string) *[]byte {
	t.Helper()
	var body []byte
	old := projectInput
	projectInput = func(stdin []byte, args ...string) ([]byte, error) { body = stdin; return []byte(resp), nil }
	t.Cleanup(func() { projectInput = old })
	return &body
}

func TestRemoveAndArchiveSendOnlyIdsAsVariables(t *testing.T) {
	body := captured(t, `{"data":{"x":{}}}`)
	if err := RemoveProjectItem("", "P1", "I1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(*body), "deleteProjectV2Item") || !strings.Contains(string(*body), `"item":"I1"`) || strings.Contains(strings.SplitN(string(*body), `"variables"`, 2)[0], "I1") {
		t.Errorf("%s", *body)
	}
	if err := ArchiveProjectItem("", "P1", "I1"); err != nil || !strings.Contains(string(*body), "archiveProjectV2Item") {
		t.Errorf("%v %s", err, *body)
	}
	if RemoveProjectItem("", "", "I1") == nil || ArchiveProjectItem("", "P1", "") == nil {
		t.Error("missing ids accepted")
	}
}

func TestDraftCreateAndEdit(t *testing.T) {
	body := captured(t, `{"data":{"addProjectV2DraftIssue":{"projectItem":{"id":"D1"}}}}`)
	id, err := AddProjectDraft("", "P1", "Idea -- `x`")
	if err != nil || id != "D1" || !strings.Contains(string(*body), `"title":"Idea -- `+"`x`"+`"`) {
		t.Fatalf("%q %v %s", id, err, *body)
	}
	if _, err := AddProjectDraft("", "P1", "  "); err == nil {
		t.Error("empty title accepted")
	}
	captured(t, `{"data":{"updateProjectV2DraftIssue":{"draftIssue":{"id":"D"}}}}`)
	if err := EditProjectDraft("", "D", "t", "b"); err != nil {
		t.Error(err)
	}
	if EditProjectDraft("", "", "t", "b") == nil || EditProjectDraft("", "D", "", "b") == nil {
		t.Error("bad edit accepted")
	}
}

func TestSetFieldValueByType(t *testing.T) {
	body := captured(t, `{"data":{"x":{}}}`)
	n := 2.5
	cases := []struct {
		typ  string
		v    *FieldValue
		want string
		ok   bool
	}{
		{"TEXT", &FieldValue{Text: "hi"}, `"text":"hi"`, true},
		{"NUMBER", &FieldValue{Number: &n}, `"number":2.5`, true},
		{"NUMBER", &FieldValue{}, "", false},
		{"DATE", &FieldValue{Date: "2026-11-05"}, `"date":"2026-11-05"`, true},
		{"DATE", &FieldValue{Date: "5 Nov"}, "", false},
		{"SINGLE_SELECT", &FieldValue{Option: "o1"}, `"singleSelectOptionId":"o1"`, true},
		{"ITERATION", &FieldValue{Option: "it1"}, `"iterationId":"it1"`, true},
		{"ITERATION", &FieldValue{}, "", false},
		{"MILESTONE", &FieldValue{Text: "x"}, "", false},
	}
	for _, c := range cases {
		*body = nil
		err := SetProjectField("", "P", "I", "F", c.typ, c.v)
		if (err == nil) != c.ok || (c.ok && !strings.Contains(string(*body), c.want)) {
			t.Errorf("%s %+v: err=%v body=%s", c.typ, c.v, err, *body)
		}
	}
	*body = nil
	if err := SetProjectField("", "P", "I", "F", "TEXT", nil); err != nil || !strings.Contains(string(*body), "clearProjectV2ItemFieldValue") {
		t.Errorf("clear: %v %s", err, *body)
	}
}

func TestMoveItemAfterAndToTop(t *testing.T) {
	body := captured(t, `{"data":{"x":{}}}`)
	if err := MoveProjectItem("", "P", "I", "A"); err != nil || !strings.Contains(string(*body), `"after":"A"`) {
		t.Errorf("%v %s", err, *body)
	}
	if err := MoveProjectItem("", "P", "I", ""); err != nil || strings.Contains(string(*body), `"after"`) {
		t.Errorf("top: %v %s", err, *body)
	}
}

func TestFieldsAndDraftIdsAreRead(t *testing.T) {
	b := `{"data":{"repositoryOwner":{"projectV2":{"id":"P","title":"T","field":{"id":"S","options":[{"id":"a","name":"Todo"}]},
"fields":{"nodes":[{"id":"S","name":"Status","dataType":"SINGLE_SELECT","options":[{"id":"a","name":"Todo"}]},{"id":"F1","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"p1","name":"High"}]},{"id":"F2","name":"Sprint","dataType":"ITERATION","configuration":{"iterations":[{"id":"i1","title":"Sprint 1"}]}},{"id":"F3","name":"Notes","dataType":"TEXT"},{"id":"F4","name":"Size","dataType":"NUMBER"},{"id":"F5","name":"Due","dataType":"DATE"},{"id":"F6","name":"Labels","dataType":"LABELS"},{}]},
"items":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"I","fieldValues":{"nodes":[
{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"High","field":{"id":"F1","name":"Priority"}},
{"__typename":"ProjectV2ItemFieldNumberValue","number":3,"field":{"id":"F4","name":"Size"}},
{"__typename":"ProjectV2ItemFieldTextValue","text":"a\u001b[31mb","field":{"id":"F3","name":"Notes"}},
{"__typename":"ProjectV2ItemFieldDateValue","date":"2026-11-05","field":{"id":"F5","name":"Due"}}]},
"content":{"__typename":"DraftIssue","id":"DI","title":"Idea","body":"x"}}]}}}}}`
	p, _, _, err := ParseProjectPage([]byte(b))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range p.Fields {
		names = append(names, f.Name+":"+f.Type)
	}
	if strings.Join(names, ",") != "Priority:SINGLE_SELECT,Sprint:ITERATION,Notes:TEXT,Size:NUMBER,Due:DATE" {
		t.Errorf("fields %v", names)
	}
	if len(p.Fields[1].Options) != 1 || p.Fields[1].Options[0].ID != "i1" || p.Fields[0].Options[0].Name != "High" {
		t.Errorf("options %+v", p.Fields)
	}
	it := p.Items[0]
	if it.DraftID != "DI" || it.Values["F1"] != "High" || it.Values["F4"] != "3" || it.Values["F5"] != "2026-11-05" || strings.ContainsRune(it.Values["F3"], 0x1b) {
		t.Errorf("item %+v", it)
	}
}
