package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

var otherRepo = ghcli.RepoRef{Owner: "zzz", Name: "other"}

// ---- a result for something you are no longer looking at must not be shown ----

func TestALateResultForAnotherPRIsDropped(t *testing.T) {
	installPRFetch(t, fixturePR())
	m := openDetail(t, prListModel())
	late := m.fetch()() // the fetch for PR 482, still in flight
	m.pr = "999"        // the user moved on
	m = m.withScreenData(viewPRDetail, prDetailState{d: ghcli.PRDetail{Number: 999, Title: "second"}})
	next, _ := m.Update(late)
	if got := next.(model).screenData(viewPRDetail).(prDetailState).d.Title; got != "second" {
		t.Errorf("title = %q; the late result for 482 replaced the PR now on screen", got)
	}
}

func TestALateResultForAnotherIssueIsDropped(t *testing.T) {
	installIssues(t, fixtureIssue())
	m := issuesLoaded(t)
	m = keyAll(m, "enter")
	late := m.fetch()()
	m.issue = "999"
	m = m.withScreenData(viewIssue, issueState{d: ghcli.IssueDetail{Number: 999, Title: "second"}})
	next, _ := m.Update(late)
	if got := next.(model).screenData(viewIssue).(issueState).d.Title; got != "second" {
		t.Errorf("title = %q; the late result replaced the issue now on screen", got)
	}
}

func TestALateDiffForAnotherPRIsDropped(t *testing.T) {
	installDiff(t, sampleDiff, nil)
	m := model{view: viewDiff, pr: "482", target: acme, width: 100, height: 24}
	late := m.fetch()()
	m.pr = "999"
	next, _ := m.Update(late)
	if n := len(next.(model).logs); n != 0 {
		t.Errorf("%d diff lines from PR 482 were shown on PR 999", n)
	}
}

func TestALateIssueListAfterARepoSwitchIsDropped(t *testing.T) {
	installIssues(t, fixtureIssue())
	m := model{view: viewIssues, width: 130, height: 30, target: acme}
	late := m.fetch()()
	m.target = otherRepo // :repo happened meanwhile
	m.tbl = table{}
	next, _ := m.Update(late)
	if n := len(next.(model).tbl.rows); n != 0 {
		t.Errorf("%d rows of acme/api's issues appeared under zzz/other", n)
	}
}

func TestALateMergeResultForAnotherPRIsIgnored(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := mergeStart(t, f)
	next, c := m.Update(k("m"))
	m = next.(model)
	late := c() // the merge options for PR 482
	m.pr = "999"
	next, _ = m.Update(late)
	if next.(model).dlg != nil {
		t.Error("a merge dialog for PR 482 opened while looking at PR 999")
	}
}

func TestALateLabelListForAnotherIssueIsIgnored(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := issueScreen(t, f)
	next, c := m.Update(k("l"))
	m = next.(model)
	late := c()
	m.issue = "999"
	next, _ = m.Update(late)
	if next.(model).dlg != nil {
		t.Error("a label dialog for issue 14812 opened while looking at issue 999")
	}
}

// ---- an async result must not take over a dialog the user is already using ----

func TestAnArrivingLabelListDoesNotReplaceAnOpenDialog(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := issueScreen(t, f)
	next, c := m.Update(k("l"))
	m = next.(model)
	late := c() // labels still on their way
	m = keyAll(m, "C")
	m = typeAll(m, "half-written comment")
	next, _ = m.Update(late)
	m = next.(model)
	if m.dlg == nil || m.dlg.kind != dialogText || m.dlg.text != "half-written comment" {
		t.Errorf("dialog = %+v; the user's comment must survive", m.dlg)
	}
}

func TestAnArrivingMergeResultDoesNotReplaceAnOpenDialog(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := mergeStart(t, f)
	installPRActions(t)
	next, c := m.Update(k("m"))
	m = next.(model)
	late := c()
	m = keyAll(m, "C")
	m = typeAll(m, "typing")
	next, _ = m.Update(late)
	m = next.(model)
	if m.dlg == nil || m.dlg.kind != dialogText || m.dlg.text != "typing" {
		t.Errorf("dialog = %+v; the comment being typed must survive", m.dlg)
	}
}

func TestPendingActionsAreClearedWhenYouNavigate(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := issueScreen(t, f)
	m = keyAll(m, "C", "esc") // a dialog opened and cancelled
	next, _ := m.Update(k("l"))
	m = next.(model)
	if m.pending == nil {
		t.Fatal("setup: l should leave an action pending while the labels load")
	}
	m = keyAll(m, "esc") // back to the list before the labels arrive
	if m.pending != nil || m.pendingMerge != nil {
		t.Errorf("pending=%v pendingMerge=%v; leaving the screen must forget half-finished actions", m.pending, m.pendingMerge)
	}
}

// ---- targets ----

func TestPFromTheCurrentDirectoryDoesNotRestoreAStalePRRepo(t *testing.T) {
	recordOpenURL(t)
	// earlier the user looked at a PR in zzz/other; now they are home and press P
	m := model{view: viewRuns, width: 120, height: 30, prTarget: otherRepo}
	next, _ := m.Update(gotoMsg{view: viewChecks, pr: "5"})
	m = next.(model)
	m.tbl = table{rows: []row{{id: "build", url: "https://github.com/zzz/other/actions/runs/9", cells: []string{"✓", "build"}}}}
	m = keys(m, "enter") // an Actions check in yet another repo
	if m.view != viewGraph || m.target != otherRepoWithHost() {
		t.Fatalf("view=%v target=%+v", m.view, m.target)
	}
	m = keys(m, "esc")
	if m.view != viewChecks || m.target != (ghcli.RepoRef{}) {
		t.Errorf("view=%v target=%+v; coming back must restore the repo the checks belong to (the current directory), not an old PR's", m.view, m.target)
	}
}

func otherRepoWithHost() ghcli.RepoRef {
	return ghcli.RepoRef{Host: "github.com", Owner: "zzz", Name: "other"}
}

func TestPickerCurrentDirectoryRowNamesTheRealCurrentDirectory(t *testing.T) {
	installPickerFakes(t, nil)
	installRepoChecker(t, nil)
	m := pickerModel(t) // the current directory is acme/api
	m.base, m.target = otherRepo, otherRepo
	m = openPicker(t, m)
	rows := m.picker.rows(m.curRepo, nil)
	if !strings.Contains(rows[0].label, "acme/api") || strings.Contains(rows[0].label, "zzz/other") {
		t.Errorf("first row = %q; want the real current directory", rows[0].label)
	}
	if v := m.View(); !strings.Contains(v, "(current directory)  acme/api") {
		t.Errorf("the picker should show it:\n%s", v)
	}
}

func TestMergeAlwaysNamesTheRepoExplicitly(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := prListModel()
	m.curRepo = acme // the PR is in the current directory's repo: target is empty
	m = openDetail(t, m)
	m = keyAll(m, "m", "down", "enter", "y")
	if len(f.calls) != 1 || f.calls[0].target != acme {
		t.Errorf("calls = %+v; without -R gh would also delete the local branch and switch the checkout", f.calls)
	}
}

func TestMergePinsTheCommitTheUserSaw(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m", "down", "enter", "y")
	if len(f.calls) != 1 || f.calls[0].head != "deadbeef" {
		t.Errorf("calls = %+v; want the merge pinned to the head commit that was read", f.calls)
	}
	_ = m
}

// ---- the repo list is fetched once, not at every open ----

func TestPickerReusesTheAccessListWithinTheTTL(t *testing.T) {
	f := installPickerFakes(t, []ghcli.RepoInfo{info("acme/web")})
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	old := clock
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = old })
	m := pickerModel(t)
	m = openPicker(t, m)
	m = pickKeys(t, m, "esc")
	m = openPicker(t, m)
	if len(f.listHosts) != 1 {
		t.Fatalf("list calls = %d after two opens; want 1 (cached)", len(f.listHosts))
	}
	if rows := m.picker.rows(m.curTarget(), nil); len(rows) == 0 || m.picker.cache == nil {
		t.Error("the cached list should be in the picker straight away")
	}
	m = pickKeys(t, m, "esc")
	now = now.Add(11 * time.Minute)
	m = openPicker(t, m)
	if len(f.listHosts) != 2 {
		t.Errorf("list calls = %d after the TTL; want 2", len(f.listHosts))
	}
}

func TestAFailedRepoListIsNotCached(t *testing.T) {
	f := installPickerFakes(t, nil)
	calls := 0
	listRepos = func(host string) ([]ghcli.RepoInfo, error) {
		calls++
		f.listHosts = append(f.listHosts, host)
		return nil, errNoPermission
	}
	m := pickerModel(t)
	m = openPicker(t, m)
	m = pickKeys(t, m, "esc")
	m = openPicker(t, m)
	if calls != 2 {
		t.Errorf("list calls = %d; a failure must be retried on the next open", calls)
	}
}

// ---- remote text never reaches the terminal as control sequences ----

func TestTablesNeverPrintRemoteControlCharacters(t *testing.T) {
	m := model{view: viewRuns, width: 120, height: 20, tbl: table{
		cols: []string{"ID", "TITLE"},
		rows: []row{{id: "1", cells: []string{"1", "fix \x1b]52;c;ZXZpbA==\x07 \x1b[2Jthe thing"}}},
	}}
	if v := m.View(); strings.ContainsAny(v, "\x1b\x07") {
		t.Errorf("the screen carries a control character from remote text: %q", v)
	}
}

var _ = tea.Quit
