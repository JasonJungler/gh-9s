package ghcli

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const projectFixture = `{"data":{"repositoryOwner":{"projectV2":{"title":"Road\u001b[31mmap","closed":false,
"field":{"options":[{"name":"Todo"},{"name":"In Progress"},{"name":"Done"}]},
"items":{"totalCount":7,"pageInfo":{"hasNextPage":false,"endCursor":"c1"},"nodes":[
{"fieldValueByName":{"name":"Todo"},"content":{"__typename":"Issue","number":1,"title":"Fix\nit","state":"OPEN","url":"https://github.com/acme/app/issues/1","updatedAt":"2026-10-01T00:00:00Z","repository":{"nameWithOwner":"acme/app"},"assignees":{"nodes":[{"login":"ann"}]},"labels":{"nodes":[{"name":"bug\u001b[1m"}]}}},
{"fieldValueByName":{"name":"In Progress"},"content":{"__typename":"PullRequest","number":2,"title":"Open PR","state":"OPEN","isDraft":false,"merged":false,"url":"u2","updatedAt":"t","repository":{"nameWithOwner":"acme/app"},"assignees":{"nodes":[]},"labels":{"nodes":[]}}},
{"fieldValueByName":{"name":"Done"},"content":{"__typename":"PullRequest","number":3,"title":"Merged","state":"MERGED","isDraft":false,"merged":true,"url":"u3","updatedAt":"t","repository":{"nameWithOwner":"acme/app"}}},
{"fieldValueByName":{"name":"Todo"},"content":{"__typename":"PullRequest","number":4,"title":"Draft","state":"OPEN","isDraft":true,"merged":false,"url":"u4","updatedAt":"t","repository":{"nameWithOwner":"acme/app"}}},
{"fieldValueByName":{"name":"Done"},"content":{"__typename":"PullRequest","number":5,"title":"Closed","state":"CLOSED","merged":false,"url":"u5","updatedAt":"t","repository":{"nameWithOwner":"acme/app"}}},
{"fieldValueByName":null,"content":{"__typename":"DraftIssue","title":"Idea"}},
{"fieldValueByName":null,"content":null}
]}}}}}`

func TestParseProjectPage(t *testing.T) {
	p, more, _, err := ParseProjectPage([]byte(projectFixture))
	if err != nil || more {
		t.Fatal(err, more)
	}
	if p.Title != "Road[31mmap" && p.Title != "Roadmap" && !strings.HasPrefix(p.Title, "Road") {
		t.Errorf("title %q", p.Title)
	}
	if strings.ContainsRune(p.Title, 0x1b) {
		t.Errorf("escape kept in %q", p.Title)
	}
	if fmt.Sprint(p.StatusOrder) != "[Todo In Progress Done]" || p.Total != 7 || len(p.Items) != 7 {
		t.Fatalf("%+v", p)
	}
	it := p.Items
	if it[0].Title != "Fix it" || it[0].Status != "Todo" || it[0].Repo != "acme/app" || it[0].Assignees[0] != "ann" || strings.ContainsRune(it[0].Labels[0], 0x1b) {
		t.Errorf("issue %+v", it[0])
	}
	if it[2].State != "MERGED" || !it[3].Draft || it[4].State != "CLOSED" {
		t.Errorf("states %+v %+v %+v", it[2], it[3], it[4])
	}
	if it[5].Type != "DraftIssue" || !it[5].Draft || it[5].Status != "" || it[5].Number != 0 {
		t.Errorf("draft issue %+v", it[5])
	}
	if it[6].Type != "" || it[6].Title != "(no access)" {
		t.Errorf("no access %+v", it[6])
	}
}

func TestParseProjectPageErrors(t *testing.T) {
	_, _, _, err := ParseProjectPage([]byte(`{"data":{"repositoryOwner":{"projectV2":null}}}`))
	if err == nil || !strings.Contains(err.Error(), "not found or no access") {
		t.Errorf("null project: %v", err)
	}
	_, _, _, err = ParseProjectPage([]byte(`{"data":{"repositoryOwner":null},"errors":[{"message":"Could not resolve to a User"}]}`))
	if err == nil || errors.Is(err, ErrMissingScope) {
		t.Errorf("%v", err)
	}
	_, _, _, err = ParseProjectPage([]byte(`{"data":null,"errors":[{"message":"Your token has not been granted the required scopes to execute this query"}]}`))
	if !errors.Is(err, ErrMissingScope) {
		t.Errorf("scope: %v", err)
	}
	if _, _, _, err = ParseProjectPage([]byte(`nope`)); err == nil {
		t.Error("garbage accepted")
	}
}

func pageJSON(n int, next bool, cursor string, total int) string {
	var nodes []string
	for i := 0; i < n; i++ {
		nodes = append(nodes, `{"fieldValueByName":null,"content":{"__typename":"Issue","number":1,"title":"t","state":"OPEN","url":"u","updatedAt":"t","repository":{"nameWithOwner":"a/b"}}}`)
	}
	return fmt.Sprintf(`{"data":{"repositoryOwner":{"projectV2":{"title":"P","closed":false,"items":{"totalCount":%d,"pageInfo":{"hasNextPage":%v,"endCursor":%q},"nodes":[%s]}}}}}`,
		total, next, cursor, strings.Join(nodes, ","))
}

func stubRun(t *testing.T, f func(args []string) ([]byte, error)) *[][]string {
	t.Helper()
	var calls [][]string
	old := projectRun
	projectRun = func(_ time.Duration, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return f(args)
	}
	t.Cleanup(func() { projectRun = old })
	return &calls
}

func TestFetchProjectTwoPages(t *testing.T) {
	calls := stubRun(t, func(args []string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "after=c1") {
			return []byte(pageJSON(100, true, "c2", 250)), nil
		}
		return []byte(pageJSON(100, true, "c1", 250)), nil
	})
	p, err := FetchProject("ghe.corp.com", "acme", 3)
	if err != nil || len(p.Items) != 200 || p.Total != 250 {
		t.Fatalf("%d items total %d err %v", len(p.Items), p.Total, err)
	}
	if len(*calls) != 2 {
		t.Fatalf("calls %d", len(*calls))
	}
	j := strings.Join((*calls)[0], " ")
	if !strings.Contains(j, "--hostname ghe.corp.com") || !strings.Contains(j, "owner=acme") || !strings.Contains(j, "number=3") {
		t.Errorf("args %v", (*calls)[0])
	}
	for _, a := range (*calls)[0] {
		if strings.HasPrefix(a, "query=") && (strings.Contains(a, "acme") || strings.Contains(a, "ghe.corp")) {
			t.Error("owner pasted into the query text")
		}
	}
}

func TestFetchProjectSinglePageAndRetry(t *testing.T) {
	n := 0
	calls := stubRun(t, func(args []string) ([]byte, error) {
		n++
		if n == 1 {
			return nil, errors.New("HTTP 504: Gateway Timeout")
		}
		return []byte(pageJSON(2, false, "", 2)), nil
	})
	p, err := FetchProject("", "acme", 1)
	if err != nil || len(p.Items) != 2 || len(*calls) != 2 {
		t.Fatal(err, len(p.Items), len(*calls))
	}
	if strings.Contains(strings.Join((*calls)[0], " "), "--hostname") {
		t.Errorf("host named for the default host: %v", (*calls)[0])
	}
}

func TestFetchProjectErrors(t *testing.T) {
	stubRun(t, func([]string) ([]byte, error) {
		return nil, errors.New("error: your authentication token is missing required scopes [read:project]")
	})
	if _, err := FetchProject("", "acme", 1); !errors.Is(err, ErrMissingScope) {
		t.Errorf("scope: %v", err)
	}
	stubRun(t, func([]string) ([]byte, error) {
		return nil, errors.New("HTTP 403: Resource protected by organization SAML enforcement")
	})
	if _, err := FetchProject("", "acme", 1); err == nil || errors.Is(err, ErrMissingScope) {
		t.Errorf("saml: %v", err)
	}
	calls := stubRun(t, func([]string) ([]byte, error) { return nil, errors.New("HTTP 404") })
	FetchProject("", "acme", 1)
	if len(*calls) != 1 {
		t.Errorf("404 retried %d times", len(*calls))
	}
}

func TestScopeHintAndReadOnly(t *testing.T) {
	if got := ScopeHint(""); got != "Projects need the read:project permission. Run: gh auth refresh -s read:project" {
		t.Error(got)
	}
	if got := ScopeHint("github.com"); strings.Contains(got, "--hostname") {
		t.Error(got)
	}
	if got := ScopeHint("ghe.corp.com"); !strings.HasSuffix(got, "-s read:project --hostname ghe.corp.com") {
		t.Error(got)
	}
	if strings.Contains(strings.ToLower(projectQuery), "mutation") {
		t.Error("the query writes")
	}
}

func TestParseProjectList(t *testing.T) {
	b := `{"data":{"viewer":{"login":"me","projectsV2":{"nodes":[{"number":1,"title":"Mine\u001b[1m","closed":false,"url":"https://github.com/users/me/projects/1"}]},
"organizations":{"nodes":[{"login":"acme","projectsV2":{"nodes":[{"number":4,"title":"Roadmap","closed":true,"url":"u"},{"number":0,"title":"bad"}]}},{"login":"empty","projectsV2":{"nodes":[]}}]}}}}`
	got, err := ParseProjectList("github.com", []byte(b))
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].Owner != "me" || got[0].Host != "" || strings.ContainsRune(got[0].Title, 0x1b) || got[1].Owner != "acme" || !got[1].Closed {
		t.Errorf("%+v", got)
	}
	_, err = ParseProjectList("", []byte(`{"data":null,"errors":[{"message":"missing required scopes [read:project]"}]}`))
	if !errors.Is(err, ErrMissingScope) {
		t.Errorf("%v", err)
	}
	if strings.Contains(strings.ToLower(listProjectsQuery), "mutation") {
		t.Error("the list query writes")
	}
}

func TestParseProjectViews(t *testing.T) {
	b := `{"data":{"repositoryOwner":{"projectV2":{"title":"P","closed":false,"views":{"nodes":[
{"name":"Backlog","layout":"BOARD_LAYOUT","filter":null,"sortByFields":{"nodes":[{"field":{"name":"Priority"},"direction":"ASC"}]}},
{"name":"My\u001b[1m items","layout":"TABLE_LAYOUT","filter":"assignee:@me","sortByFields":{"nodes":[{"field":{"name":"Updated at"},"direction":"DESC"}]}},
{"name":"Roadmap","layout":"ROADMAP_LAYOUT","filter":null,"sortByFields":{"nodes":[]}}]},
"items":{"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`
	p, _, _, err := ParseProjectPage([]byte(b))
	if err != nil || len(p.Views) != 3 {
		t.Fatalf("%+v %v", p.Views, err)
	}
	v := p.Views
	if v[0].SortField != "Priority" || v[0].SortDesc || v[1].Filter != "assignee:@me" || !v[1].SortDesc || strings.ContainsRune(v[1].Name, 0x1b) || v[2].Layout != "ROADMAP_LAYOUT" || v[2].SortField != "" {
		t.Errorf("%+v", v)
	}
	if !strings.Contains(projectQuery, "views(first:20)") || strings.Contains(strings.ToLower(projectQuery), "mutation") {
		t.Error("query")
	}
}
