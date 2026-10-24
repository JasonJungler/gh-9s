package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

var errNoPermission = errors.New("HTTP 403: you do not have permission to comment")

type issueFakes struct {
	listTargets []ghcli.RepoRef
	listFilters []ghcli.IssueFilter
	viewTargets []ghcli.RepoRef
	comments    []prActionCall
	commentErr  error
}

func issueRow(n, title string) row {
	return row{id: n, state: stOK, url: "https://github.com/acme/api/issues/" + n,
		cells: []string{n, "OPEN", title, "dana", "bug", "erin", "2d"}}
}

func fixtureIssue() ghcli.IssueDetail {
	return ghcli.IssueDetail{
		Number: 14812, Title: "auth login hangs", Author: "dana", State: "OPEN",
		URL:  "https://github.com/acme/api/issues/14812",
		Body: "### Describe the bug\n\nIt hangs.", Labels: []string{"bug", "needs triage"}, Assignees: []string{"erin"},
		Comments: []ghcli.Comment{{Author: "bot", Body: "Thanks for the report", CreatedAt: "2026-10-23T10:00:00Z"}},
	}
}

func installIssues(t *testing.T, detail ghcli.IssueDetail) *issueFakes {
	t.Helper()
	f := &issueFakes{}
	oldList, oldView, oldComment := fetchIssueList, fetchIssue, commentIssue
	fetchIssueList = func(r ghcli.RepoRef, flt ghcli.IssueFilter) (table, error) {
		f.listTargets, f.listFilters = append(f.listTargets, r), append(f.listFilters, flt)
		return table{cols: []string{"#", "STATE", "TITLE", "AUTHOR", "LABELS", "ASSIGNEES", "UPDATED"},
			rows: []row{issueRow("14812", "auth login hangs")}}, nil
	}
	fetchIssue = func(r ghcli.RepoRef, number string) (ghcli.IssueDetail, error) {
		f.viewTargets = append(f.viewTargets, r)
		return detail, nil
	}
	commentIssue = func(r ghcli.RepoRef, number, body string) error {
		f.comments = append(f.comments, prActionCall{repo: r, number: number, body: body})
		return f.commentErr
	}
	t.Cleanup(func() { fetchIssueList, fetchIssue, commentIssue = oldList, oldView, oldComment })
	return f
}

// issuesLoaded is a model on the issue list with the list already fetched.
func issuesLoaded(t *testing.T) model {
	t.Helper()
	m := model{view: viewIssues, width: 130, height: 30, target: acme, prTarget: acme, base: ghcli.RepoRef{}}
	return pump(m, m.fetch())
}

func TestFourKeyAndTheIssuesCommandOpenTheList(t *testing.T) {
	installIssues(t, fixtureIssue())
	m := model{view: viewRuns, width: 130, height: 30}
	if m = keyAll(m, "4"); m.view != viewIssues {
		t.Errorf("4 -> view %v; want the issue list", m.view)
	}
	m = model{view: viewRuns, width: 130, height: 30}
	installRepoChecker(t, nil)
	if m = typeCommand(t, m, "issues"); m.view != viewIssues {
		t.Errorf(":issues -> view %v; want the issue list", m.view)
	}
}

func TestIssuesListShowsRowsAndTheFilterInTheTitle(t *testing.T) {
	installIssues(t, fixtureIssue())
	m := issuesLoaded(t)
	v := m.View()
	for _, want := range []string{"<issues>", "auth login hangs", "dana", "bug", "erin", "open"} {
		if !strings.Contains(v, want) {
			t.Errorf("list lacks %q:\n%s", want, v)
		}
	}
	m = keyAll(m, "C")
	if v := m.View(); !strings.Contains(v, "closed") {
		t.Errorf("title should name the closed filter:\n%s", v)
	}
}

func TestIssuesListFilterKeysRefetchWithTheFilter(t *testing.T) {
	f := installIssues(t, fixtureIssue())
	m := issuesLoaded(t)
	steps := []struct {
		key  string
		want ghcli.IssueFilter
	}{
		{"C", ghcli.IssueFilter{State: "closed"}},
		{"A", ghcli.IssueFilter{State: "all"}},
		{"u", ghcli.IssueFilter{State: "all", AssignedToMe: true}},
		{"m", ghcli.IssueFilter{State: "all", AssignedToMe: true, OpenedByMe: true}},
		{"u", ghcli.IssueFilter{State: "all", OpenedByMe: true}},
		{"O", ghcli.IssueFilter{State: "open", OpenedByMe: true}},
	}
	for _, s := range steps {
		before := len(f.listFilters)
		m = keyAll(m, s.key)
		if m.issueFilter != s.want || len(f.listFilters) != before+1 || f.listFilters[before] != s.want {
			t.Errorf("key %q: filter=%+v fetched=%d times; want %+v fetched once", s.key, m.issueFilter, len(f.listFilters)-before, s.want)
		}
	}
}

func TestIssueListArgsIncludeTheFilter(t *testing.T) {
	args := strings.Join(issueListArgs(acme, ghcli.IssueFilter{State: "closed", AssignedToMe: true}), " ")
	for _, want := range []string{"issue list", "--state closed", "--assignee @me", "-R acme/api", "--json"} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %q: %s", want, args)
		}
	}
}

func TestIssuesListUsesTargetNotBase(t *testing.T) {
	f := installIssues(t, fixtureIssue())
	m := model{view: viewIssues, width: 130, height: 30, target: acme, base: ghcli.RepoRef{Owner: "chosen", Name: "repo"}}
	pump(m, m.fetch())
	if len(f.listTargets) != 1 || f.listTargets[0] != acme {
		t.Errorf("listed %v; want the repo the view shows (target)", f.listTargets)
	}
}

func openIssue(t *testing.T) (model, *issueFakes) {
	t.Helper()
	f := installIssues(t, fixtureIssue())
	m := issuesLoaded(t)
	m = keyAll(m, "enter")
	if m.view != viewIssue || m.issue != "14812" {
		t.Fatalf("view=%v issue=%q after enter; want the detail of 14812", m.view, m.issue)
	}
	return m, f
}

func TestEnterOpensTheIssueDetailWithEverythingOnIt(t *testing.T) {
	m, _ := openIssue(t)
	v := m.View()
	for _, want := range []string{"#14812", "auth login hangs", "dana", "OPEN", "bug", "needs triage", "erin",
		"It hangs.", "bot commented", "Thanks for the report", "issue=#14812"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail lacks %q:\n%s", want, v)
		}
	}
}

func TestIssueDetailWithNothingRenders(t *testing.T) {
	installIssues(t, ghcli.IssueDetail{Number: 5, Title: "Bare", Author: "ghost", State: "CLOSED"})
	m := issuesLoaded(t)
	m = keyAll(m, "enter")
	v := m.View()
	for _, want := range []string{"Bare", "CLOSED", "no description", "no comments", "no labels", "unassigned"} {
		if !strings.Contains(v, want) {
			t.Errorf("an empty issue should still render, lacks %q:\n%s", want, v)
		}
	}
}

func TestIssueCommentSubmitsAndRefreshes(t *testing.T) {
	m, f := openIssue(t)
	m = keyAll(m, "C")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "acme/api#14812") {
		t.Fatalf("dialog = %+v; want one naming acme/api#14812", m.dlg)
	}
	m = typeAll(m, "same here")
	before := len(f.viewTargets)
	m = keyAll(m, "enter")
	if len(f.comments) != 1 || f.comments[0].number != "14812" || f.comments[0].body != "same here" || f.comments[0].repo != acme {
		t.Fatalf("comments = %+v", f.comments)
	}
	if !strings.Contains(m.status, "✓") || len(f.viewTargets) != before+1 {
		t.Errorf("status=%q reloads=%d; want a tick and one reload of the issue", m.status, len(f.viewTargets)-before)
	}
}

func TestIssueCommentErrorIsShown(t *testing.T) {
	m, f := openIssue(t)
	f.commentErr = errNoPermission
	m = keyAll(m, "C")
	m = typeAll(m, "x")
	m = keyAll(m, "enter")
	if !strings.Contains(m.status, "permission") || strings.Contains(m.status, "✓") {
		t.Errorf("status = %q", m.status)
	}
}

func TestEscFromTheIssueReturnsToTheList(t *testing.T) {
	m, _ := openIssue(t)
	m = keyAll(m, "esc")
	if m.view != viewIssues {
		t.Errorf("view = %v; want the list", m.view)
	}
}

func TestIssueDetailUsesTargetNotBase(t *testing.T) {
	f := installIssues(t, fixtureIssue())
	m := issuesLoaded(t)
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"}
	m = keyAll(m, "enter")
	if len(f.viewTargets) != 1 || f.viewTargets[0] != acme {
		t.Errorf("viewed %v; want acme/api (target)", f.viewTargets)
	}
}

func TestIssueDetailScrollsAndClamps(t *testing.T) {
	d := fixtureIssue()
	d.Body = strings.Repeat("a line of the report\n", 80)
	installIssues(t, d)
	m := issuesLoaded(t)
	m = keyAll(m, "enter", "G")
	end := m.screenData(viewIssue).(issueState).scroll
	if end == 0 {
		t.Fatal("G should scroll a long issue")
	}
	m = keyAll(m, "j", "j")
	if m.screenData(viewIssue).(issueState).scroll != end {
		t.Error("cannot scroll past the end")
	}
	m = keyAll(m, "g")
	if m.screenData(viewIssue).(issueState).scroll != 0 {
		t.Error("g returns to the top")
	}
}

func TestRepoSwitchFromTheIssueListStaysOnIssues(t *testing.T) {
	installIssues(t, fixtureIssue())
	installRepoChecker(t, nil)
	m := issuesLoaded(t)
	m = typeCommand(t, m, "repo acme/other")
	if m.view != viewIssues {
		t.Errorf("view = %v; switching repo from the issue list should stay on issues", m.view)
	}
}
