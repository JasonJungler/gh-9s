package ghcli

import "testing"

func TestParseActionsURL(t *testing.T) {
	cases := []struct {
		url  string
		ok   bool
		want ActionsLink
	}{
		{"https://github.com/acme/api/actions/runs/123", true,
			ActionsLink{Repo: RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, RunID: "123"}},
		{"https://github.com/acme/api/actions/runs/123/job/456", true,
			ActionsLink{Repo: RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, RunID: "123", JobID: "456"}},
		{"https://ghe.corp.com/acme/api/actions/runs/9/job/8", true,
			ActionsLink{Repo: RepoRef{Host: "ghe.corp.com", Owner: "acme", Name: "api"}, RunID: "9", JobID: "8"}},
		{"https://github.com/acme/api/actions/runs/123/", true,
			ActionsLink{Repo: RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, RunID: "123"}},
		{"https://github.com/acme/api/actions/runs/123/job/456?pr=7#step:3:1", true,
			ActionsLink{Repo: RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, RunID: "123", JobID: "456"}},
		{"https://github.com/acme/api/actions/runs/123/attempts/2", true,
			ActionsLink{Repo: RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, RunID: "123"}},
		// not Actions runs
		{"https://circleci.com/gh/acme/api/55", false, ActionsLink{}},
		{"https://github.com/acme/api/runs/456", false, ActionsLink{}}, // old-style check-run link: a job id, not a run
		{"https://github.com/acme/api/pull/7", false, ActionsLink{}},
		{"https://github.com/acme/api/actions/workflows/ci.yml", false, ActionsLink{}},
		{"https://github.com/acme/api/actions/runs/notanumber", false, ActionsLink{}},
		{"https://github.com/acme/api/actions/runs/", false, ActionsLink{}},
		{"https://github.com/-x/api/actions/runs/1", false, ActionsLink{}},
		{"ftp://github.com/acme/api/actions/runs/1", false, ActionsLink{}},
		{"javascript:alert(1)", false, ActionsLink{}},
		{"", false, ActionsLink{}},
		{"not a url at all", false, ActionsLink{}},
	}
	for _, c := range cases {
		got, ok := ParseActionsURL(c.url)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseActionsURL(%q) = %+v, %v; want %+v, %v", c.url, got, ok, c.want, c.ok)
		}
	}
}
