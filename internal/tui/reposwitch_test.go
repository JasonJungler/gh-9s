package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

var acme = ghcli.RepoRef{Owner: "acme", Name: "api"}

func installRepoChecker(t *testing.T, err error) *[]ghcli.RepoRef {
	t.Helper()
	var checked []ghcli.RepoRef
	old := checkRepo
	checkRepo = func(r ghcli.RepoRef) error { checked = append(checked, r); return err }
	t.Cleanup(func() { checkRepo = old })
	return &checked
}

// typeCommand runs ":<cmd>" and then executes the command it returned, feeding the result back.
func typeCommand(t *testing.T, m model, cmd string) model {
	t.Helper()
	next, c := m.Update(k(":"))
	m = next.(model)
	for _, r := range cmd {
		if r == ' ' {
			next, _ = m.Update(k("space"))
		} else {
			next, _ = m.Update(k(string(r)))
		}
		m = next.(model)
	}
	next, c = m.Update(k("enter"))
	m = next.(model)
	if c != nil {
		// run only the command :repo returned (verification); the data fetches that follow
		// a switch would call the real gh, which unit tests must not do
		next, _ = m.Update(c())
		m = next.(model)
	}
	return m
}

func workflowsModel(t *testing.T) model {
	m := favModel(t) // on the workflows view of acme/api's cwd
	m.branch = "feature-x"
	return m
}

func TestRepoCommandSwitchesEveryView(t *testing.T) {
	checked := installRepoChecker(t, nil)
	m := typeCommand(t, workflowsModel(t), "repo acme/other")
	want := ghcli.RepoRef{Owner: "acme", Name: "other"}
	if m.base != want || m.target != want {
		t.Fatalf("base=%+v target=%+v; want both %+v", m.base, m.target, want)
	}
	if len(*checked) != 1 || (*checked)[0] != want {
		t.Errorf("repo should be verified before switching, checked=%v", *checked)
	}
	if m.view != viewWorkflows {
		t.Errorf("view = %v; want to stay on the workflows view", m.view)
	}
	if !strings.Contains(m.View(), "acme/other") {
		t.Errorf("title should name the repo:\n%s", m.View())
	}
	if got := m.curTarget(); got != want {
		t.Errorf("curTarget = %+v; favorites and triggers should follow the switch", got)
	}
}

func TestRepoSwitchIsStickyAcrossTopLevelViews(t *testing.T) {
	installRepoChecker(t, nil)
	m := typeCommand(t, workflowsModel(t), "repo acme/other")
	for _, key := range []string{"1", "2", "3"} {
		m = keys(m, key)
		if m.target != m.base || m.base.Name != "other" {
			t.Errorf("after %q: target=%+v base=%+v; the chosen repo must stay", key, m.target, m.base)
		}
	}
}

func TestRepoCommandWithoutArgumentReturnsToTheCurrentDirectory(t *testing.T) {
	installRepoChecker(t, nil)
	m := typeCommand(t, workflowsModel(t), "repo acme/other")
	m = typeCommand(t, m, "repo")
	if m.base != (ghcli.RepoRef{}) || m.target != (ghcli.RepoRef{}) {
		t.Errorf("base=%+v target=%+v; want the current directory's repo", m.base, m.target)
	}
}

func TestRepoCommandRejectsBadInputWithoutAsking(t *testing.T) {
	checked := installRepoChecker(t, nil)
	for _, bad := range []string{"repo nonsense", "repo a/b/c/d", "repo -x/y", "repo acme/a b"} {
		m := typeCommand(t, workflowsModel(t), bad)
		if m.base != (ghcli.RepoRef{}) || !strings.Contains(m.status, "repo") {
			t.Errorf("%q: base=%+v status=%q; want a refusal", bad, m.base, m.status)
		}
	}
	if len(*checked) != 0 {
		t.Errorf("malformed input must not reach gh: %v", *checked)
	}
}

func TestRepoCommandStaysPutWhenTheRepoIsNotFound(t *testing.T) {
	installRepoChecker(t, errors.New("Could not resolve to a Repository"))
	m := typeCommand(t, workflowsModel(t), "repo acme/missing")
	if m.base != (ghcli.RepoRef{}) {
		t.Errorf("base = %+v; a missing repo must not be switched to", m.base)
	}
	if !strings.Contains(m.status, "acme/missing") || !strings.Contains(m.status, "Could not resolve") {
		t.Errorf("status = %q; want the repo and the reason", m.status)
	}
}

func TestRepoSwitchFromADeepViewStartsAtTheRunsList(t *testing.T) {
	installRepoChecker(t, nil)
	m := typeCommand(t, graphModel(), "repo acme/other")
	if m.view != viewRuns || len(m.back) != 0 || m.runID != "" {
		t.Errorf("view=%v back=%v runID=%q; a deep view cannot belong to the new repo", m.view, m.back, m.runID)
	}
}

func TestCurrentBranchPRIsOnlyForTheCurrentDirectory(t *testing.T) {
	installRepoChecker(t, nil)
	m := typeCommand(t, workflowsModel(t), "repo acme/other")
	next, cmd := m.Update(k("P"))
	m = next.(model)
	if cmd != nil || !strings.Contains(m.status, "current directory") {
		t.Errorf("cmd=%v status=%q; P should explain instead of asking gh about the wrong repo", cmd != nil, m.status)
	}
	m = typeCommand(t, m, "cpr")
	if !strings.Contains(m.status, "current directory") {
		t.Errorf(":cpr status = %q", m.status)
	}
}

func TestFavoriteInAnotherRepoReturnsToTheChosenRepo(t *testing.T) {
	installRepoChecker(t, nil)
	m := typeCommand(t, workflowsModel(t), "repo acme/other")
	m.view = viewFavs
	m.favs.Items = []favorites.Favorite{{Host: "github.com", Repo: "acme/third", Workflow: "deploy.yml"}}
	m.tbl = table{rows: []row{{id: "k", extra: m.favs.Items[0].Key(), cells: []string{"-", "acme/third", "deploy.yml", "x", "1m"}}}}
	m = keys(m, "enter")
	if m.target.Name != "third" {
		t.Fatalf("target = %+v; want the favorite's repo", m.target)
	}
	m = keys(m, "esc")
	if m.target != m.base || m.base.Name != "other" {
		t.Errorf("after esc: target=%+v base=%+v; want back on the chosen repo", m.target, m.base)
	}
}

func TestPRListDoesNotMarkTheLocalBranchInAnotherRepo(t *testing.T) {
	m := workflowsModel(t)
	m.view = viewPRs
	if got := m.prBranch(); got != "feature-x" {
		t.Errorf("prBranch = %q; want the local branch in the current directory's repo", got)
	}
	m.target = acme
	if got := m.prBranch(); got != "" {
		t.Errorf("prBranch = %q; another repo has no meaning for the local branch", got)
	}
}
