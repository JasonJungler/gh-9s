package ghcli

import (
	"fmt"
	"strings"
	"testing"
)

const repoListJSON = `[
 {"nameWithOwner":"acme/api","description":"The API","primaryLanguage":{"name":"Go"},"stargazerCount":12,"updatedAt":"2026-10-20T10:00:00Z"},
 {"nameWithOwner":"acme/web","description":null,"primaryLanguage":null,"stargazerCount":0,"updatedAt":"2026-10-21T10:00:00Z"}
]`

const searchJSON = `[
 {"fullName":"cli/cli","description":"GitHub's official command line tool","language":"Go","stargazersCount":30000,"updatedAt":"2026-10-24T10:00:00Z"},
 {"fullName":"x/y","description":"","language":"","stargazersCount":1,"updatedAt":""}
]`

func TestParseRepoInfosFromRepoList(t *testing.T) {
	got, err := ParseRepoInfos([]byte(repoListJSON), "")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err %v", got, err)
	}
	if got[0].Ref != (RepoRef{Owner: "acme", Name: "api"}) || got[0].Language != "Go" || got[0].Stars != 12 || got[0].Description != "The API" {
		t.Errorf("first = %+v", got[0])
	}
}

func TestParseRepoInfosFromSearch(t *testing.T) {
	got, err := ParseRepoInfos([]byte(searchJSON), "ghe.corp.com")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err %v", got, err)
	}
	if got[0].Ref != (RepoRef{Host: "ghe.corp.com", Owner: "cli", Name: "cli"}) || got[0].Stars != 30000 || got[0].Language != "Go" {
		t.Errorf("first = %+v", got[0])
	}
}

func TestParseRepoInfosEmptyAndNullFields(t *testing.T) {
	got, err := ParseRepoInfos([]byte(repoListJSON), "")
	if err != nil || got[1].Language != "" || got[1].Description != "" {
		t.Errorf("null fields: %+v err %v", got[1], err)
	}
	if got, err := ParseRepoInfos([]byte(`[]`), ""); err != nil || len(got) != 0 {
		t.Errorf("empty list: %v %v", got, err)
	}
	if _, err := ParseRepoInfos([]byte(`not json`), ""); err == nil {
		t.Error("garbage should be an error")
	}
	// an entry with a malformed name is skipped, not fatal
	got, err = ParseRepoInfos([]byte(`[{"fullName":"nonsense"},{"fullName":"a/b"}]`), "")
	if err != nil || len(got) != 1 || got[0].Ref.Name != "b" {
		t.Errorf("malformed name: %v %v", got, err)
	}
}

func infos(names ...string) []RepoInfo {
	var out []RepoInfo
	for _, n := range names {
		desc := ""
		if i := strings.Index(n, "|"); i >= 0 {
			n, desc = n[:i], n[i+1:]
		}
		r, _ := ParseRepoRef(n)
		out = append(out, RepoInfo{Ref: r, Description: desc})
	}
	return out
}

func refNames(l []RepoInfo) string {
	var s []string
	for _, i := range l {
		s = append(s, i.Ref.String())
	}
	return fmt.Sprint(s)
}

func TestFilterReposRanksPrefixBeforeSubstringBeforeDescription(t *testing.T) {
	list := infos("me/apple-notes", "d/docs|api reference", "x/rapid", "acme/api-gateway", "acme/api", "q/other")
	got := refNames(FilterRepos(list, "api"))
	// name prefix (input order), then name substring, then description-only; apple-notes has no "api"
	want := "[acme/api-gateway acme/api x/rapid d/docs]"
	if got != want {
		t.Errorf("FilterRepos = %s; want %s", got, want)
	}
}

func TestFilterReposIsCaseInsensitive(t *testing.T) {
	if got := refNames(FilterRepos(infos("Acme/API"), "api")); got != "[Acme/API]" {
		t.Errorf("got %s", got)
	}
}

func TestFilterReposEmptyQueryReturnsAllUnchanged(t *testing.T) {
	list := infos("a/b", "c/d")
	if got := refNames(FilterRepos(list, "  ")); got != refNames(list) {
		t.Errorf("got %s", got)
	}
}
