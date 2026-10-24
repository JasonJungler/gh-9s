package tui

import (
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func checksModel(url string) model {
	return model{view: viewChecks, pr: "482", target: acme, prTarget: acme, width: 120, height: 30,
		back: []viewKind{viewPRs, viewPRDetail},
		tbl:  table{cols: []string{"STATUS", "NAME"}, rows: []row{{id: "build", url: url, cells: []string{"✓ pass", "build"}}}}}
}

func recordOpenURL(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	old := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openURL = old })
	return &opened
}

func TestEnterOnAnActionsCheckOpensTheGraphInThatRepo(t *testing.T) {
	opened := recordOpenURL(t)
	m := keys(checksModel("https://github.com/acme/api/actions/runs/123/job/456"), "enter")
	if m.view != viewGraph || m.runID != "123" || len(*opened) != 0 {
		t.Errorf("view=%v runID=%q opened=%v; want the graph of run 123 and no browser", m.view, m.runID, *opened)
	}
	if m.target != acme {
		t.Errorf("target = %+v; the PR's own repo stays", m.target)
	}
	if m.gsel != 0 || len(m.graph.Nodes) != 0 || m.runStatus != "" {
		t.Errorf("graph state should start fresh: gsel=%d nodes=%d status=%q", m.gsel, len(m.graph.Nodes), m.runStatus)
	}
}

func TestEnterOnACheckFromAnotherRepoTargetsThatRepo(t *testing.T) {
	recordOpenURL(t)
	m := keys(checksModel("https://github.com/other/lib/actions/runs/9"), "enter")
	want := ghcli.RepoRef{Host: "github.com", Owner: "other", Name: "lib"}
	if m.view != viewGraph || m.runID != "9" || m.target != want {
		t.Errorf("view=%v runID=%q target=%+v; want run 9 of other/lib", m.view, m.runID, m.target)
	}
}

func TestEnterOnTheSameRepoWithADifferentHostSpellingKeepsTheTarget(t *testing.T) {
	recordOpenURL(t)
	m := checksModel("https://github.com/acme/api/actions/runs/5")
	m.target = ghcli.RepoRef{Owner: "acme", Name: "api"} // host spelled "" instead of github.com
	m.prTarget = m.target
	m = keys(m, "enter")
	if m.target != (ghcli.RepoRef{Owner: "acme", Name: "api"}) {
		t.Errorf("target = %+v; the same repo must not be rewritten", m.target)
	}
}

func TestEnterOnANonActionsCheckOpensTheBrowser(t *testing.T) {
	opened := recordOpenURL(t)
	m := keys(checksModel("https://circleci.com/gh/acme/api/55"), "enter")
	if m.view != viewChecks || len(*opened) != 1 || (*opened)[0] != "https://circleci.com/gh/acme/api/55" {
		t.Errorf("view=%v opened=%v; want the browser and no navigation", m.view, *opened)
	}
}

func TestEnterOnACheckWithNoLinkDoesNothing(t *testing.T) {
	opened := recordOpenURL(t)
	m := keys(checksModel(""), "enter")
	if m.view != viewChecks || len(*opened) != 0 {
		t.Errorf("view=%v opened=%v", m.view, *opened)
	}
}

func TestEscFromTheJumpedGraphReturnsToTheChecksInTheirOwnRepo(t *testing.T) {
	recordOpenURL(t)
	m := keys(checksModel("https://github.com/other/lib/actions/runs/9"), "enter")
	m = keys(m, "esc")
	if m.view != viewChecks || m.target != acme {
		t.Errorf("view=%v target=%+v; want the PR's checks back in acme/api", m.view, m.target)
	}
}
