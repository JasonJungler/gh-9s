package tui

import (
	"errors"
	"strings"
	"testing"
)

func failOpenURL(t *testing.T) *[]string {
	t.Helper()
	var asked []string
	old := openURL
	openURL = func(u string) error { asked = append(asked, u); return errors.New("wslview: not found") }
	t.Cleanup(func() { openURL = old })
	return &asked
}

func wantBrowserError(t *testing.T, what string, m model, asked []string) {
	t.Helper()
	if len(asked) != 1 || !strings.Contains(m.status, "can't open the browser") || !strings.Contains(m.status, "wslview: not found") {
		t.Errorf("%s: asked=%v status=%q; want one attempt and the reason in the status", what, asked, m.status)
	}
}

func TestOpenFailureIsShownOnThePRDetail(t *testing.T) {
	asked := failOpenURL(t)
	m, _ := prDetailOpen(t)
	m = keyAll(m, "o")
	wantBrowserError(t, "PR detail", m, *asked)
}

func TestOpenFailureIsShownOnTheIssue(t *testing.T) {
	asked := failOpenURL(t)
	m, _ := openIssue(t)
	m = keyAll(m, "o")
	wantBrowserError(t, "issue", m, *asked)
}

func TestOpenFailureIsShownOnTheDiff(t *testing.T) {
	asked := failOpenURL(t)
	m := diffModel(t, sampleDiff)
	m = m.withScreenData(viewPRDetail, prDetailState{d: fixturePR()})
	m = keyAll(m, "o")
	wantBrowserError(t, "diff", m, *asked)
}

func TestOpenFailureIsShownOnANonActionsCheck(t *testing.T) {
	asked := failOpenURL(t)
	m := keys(checksModel("https://circleci.com/gh/acme/api/55"), "enter")
	wantBrowserError(t, "check", m, *asked)
}

func TestOpenSuccessLeavesTheStatusAlone(t *testing.T) {
	opened := recordOpenURL(t)
	m := keys(checksModel("https://circleci.com/gh/acme/api/55"), "enter")
	if len(*opened) != 1 || m.status != "" {
		t.Errorf("opened=%v status=%q", *opened, m.status)
	}
}

func TestOpenFailureIsShownInTheListViews(t *testing.T) {
	asked := failOpenURL(t)
	m := model{view: viewRuns, width: 100, height: 20, tbl: table{cols: []string{"ID"},
		rows: []row{{id: "1", url: "https://github.com/acme/api/actions/runs/1", cells: []string{"1"}}}}}
	m = keys(m, "o")
	wantBrowserError(t, "list", m, *asked)
	if strings.Contains(m.status, "opened ") {
		t.Errorf("status %q claims the link opened", m.status)
	}
}

func TestOpenFailureIsShownInTheGraphAndLogViews(t *testing.T) {
	asked := failOpenURL(t)
	g := graphModel()
	for i := range g.graph.Nodes {
		g.graph.Nodes[i].Job.URL = "https://github.com/acme/api/actions/runs/42/job/7"
	}
	wantBrowserError(t, "graph", keys(g, "o"), *asked)

	asked2 := failOpenURL(t)
	logs := model{view: viewLogs, width: 100, height: 20, jobURL: "https://github.com/acme/api/actions/runs/42/job/7", logs: []string{"a"}}
	wantBrowserError(t, "logs", keys(logs, "o"), *asked2)
}
