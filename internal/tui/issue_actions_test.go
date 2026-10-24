package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type editCall struct {
	repo                                                   ghcli.RepoRef
	number                                                 string
	addLabels, removeLabels, addAssignees, removeAssignees []string
}

type issueActionFakes struct {
	closes, reopens []string
	edits           []editCall
	labels          []string
	assignees       []string
	labelsErr       error
	err             error
	fetches         int
	detail          ghcli.IssueDetail
}

func installIssueActions(t *testing.T, detail ghcli.IssueDetail) *issueActionFakes {
	t.Helper()
	f := &issueActionFakes{detail: detail,
		labels:    []string{"bug", "docs", "needs triage 🚧", "wontfix"},
		assignees: []string{"erin", "frank", "gina"}}
	oldView, oldList, oldComment := fetchIssue, fetchIssueList, commentIssue
	oldLabels, oldAssignees := listLabels, listAssignees
	oldEdit, oldClose, oldReopen := editIssue, closeIssue, reopenIssue
	fetchIssue = func(r ghcli.RepoRef, number string) (ghcli.IssueDetail, error) { f.fetches++; return f.detail, nil }
	fetchIssueList = func(r ghcli.RepoRef, flt ghcli.IssueFilter) (table, error) {
		return table{cols: []string{"#", "STATE", "TITLE"}, rows: []row{issueRow("14812", "auth login hangs")}}, nil
	}
	listLabels = func(r ghcli.RepoRef) ([]string, error) { return f.labels, f.labelsErr }
	listAssignees = func(r ghcli.RepoRef) ([]string, error) { return f.assignees, nil }
	editIssue = func(r ghcli.RepoRef, n string, al, rl, aa, ra []string) error {
		f.edits = append(f.edits, editCall{r, n, al, rl, aa, ra})
		return f.err
	}
	closeIssue = func(r ghcli.RepoRef, n string) error { f.closes = append(f.closes, n); return f.err }
	reopenIssue = func(r ghcli.RepoRef, n string) error { f.reopens = append(f.reopens, n); return f.err }
	t.Cleanup(func() {
		fetchIssue, fetchIssueList, commentIssue = oldView, oldList, oldComment
		listLabels, listAssignees = oldLabels, oldAssignees
		editIssue, closeIssue, reopenIssue = oldEdit, oldClose, oldReopen
	})
	return f
}

func issueScreen(t *testing.T, f *issueActionFakes) model {
	t.Helper()
	m := model{view: viewIssues, width: 130, height: 30, target: acme, prTarget: acme}
	m = pump(m, m.fetch())
	m = keyAll(m, "enter")
	if m.view != viewIssue {
		t.Fatalf("view = %v; want the issue", m.view)
	}
	return m
}

// openIssueDetail is an OPEN issue labelled [bug, needs triage 🚧] and assigned to erin.
func openIssueDetail() ghcli.IssueDetail {
	d := fixtureIssue()
	d.Labels = []string{"bug", "needs triage 🚧"}
	return d
}

func closedIssueDetail() ghcli.IssueDetail {
	d := fixtureIssue()
	d.State = "CLOSED"
	return d
}

func TestXAsksForConfirmationNamingTheIssueBeforeClosing(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := keyAll(issueScreen(t, f), "x")
	if m.dlg == nil || m.dlg.kind != dialogConfirm || !strings.Contains(m.View(), "acme/api#14812") || !strings.Contains(m.View(), "auth login hangs") {
		t.Fatalf("dialog = %+v; want a confirmation naming acme/api#14812 and its title", m.dlg)
	}
	if len(f.closes) != 0 {
		t.Fatal("nothing may be closed before the confirmation")
	}
	declined := keyAll(m, "n")
	if len(f.closes) != 0 || declined.dlg != nil {
		t.Errorf("declining must close nothing: %v", f.closes)
	}
	m = keyAll(m, "y")
	if !reflect.DeepEqual(f.closes, []string{"14812"}) || !strings.Contains(m.status, "✓") || !strings.Contains(m.status, "closed") {
		t.Errorf("closes=%v status=%q", f.closes, m.status)
	}
}

func TestXOnAnIssueThatIsAlreadyClosedSaysSo(t *testing.T) {
	f := installIssueActions(t, closedIssueDetail())
	m := keyAll(issueScreen(t, f), "x")
	if m.dlg != nil || len(f.closes) != 0 || !strings.Contains(m.status, "already closed") {
		t.Errorf("dialog=%v closes=%v status=%q", m.dlg != nil, f.closes, m.status)
	}
}

func TestReopenNeedsNoConfirmation(t *testing.T) {
	f := installIssueActions(t, closedIssueDetail())
	m := keyAll(issueScreen(t, f), "X")
	if m.dlg != nil || !reflect.DeepEqual(f.reopens, []string{"14812"}) || !strings.Contains(m.status, "reopened") {
		t.Errorf("dialog=%v reopens=%v status=%q", m.dlg != nil, f.reopens, m.status)
	}
}

func TestReopenOnAnOpenIssueSaysSo(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := keyAll(issueScreen(t, f), "X")
	if len(f.reopens) != 0 || !strings.Contains(m.status, "already open") {
		t.Errorf("reopens=%v status=%q", f.reopens, m.status)
	}
}

func TestLabelsKeyListsRepoLabelsWithCurrentOnesChecked(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := keyAll(issueScreen(t, f), "l")
	v := m.View()
	for _, want := range []string{"[x] bug", "[ ] docs", "[x] needs triage 🚧", "[ ] wontfix"} {
		if !strings.Contains(v, want) {
			t.Errorf("label dialog lacks %q:\n%s", want, v)
		}
	}
	// untick bug (first row), tick wontfix (fourth row)
	m = keyAll(m, "space", "down", "down", "down", "space", "enter")
	want := editCall{repo: acme, number: "14812", addLabels: []string{"wontfix"}, removeLabels: []string{"bug"}}
	if len(f.edits) != 1 || !reflect.DeepEqual(f.edits[0], want) {
		t.Errorf("edits = %+v; want %+v", f.edits, want)
	}
}

func TestLabelsWithNoChangeSendsNothing(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := keyAll(issueScreen(t, f), "l", "enter")
	if len(f.edits) != 0 || !strings.Contains(m.status, "no label changes") {
		t.Errorf("edits=%v status=%q", f.edits, m.status)
	}
}

func TestALabelOnTheIssueThatTheRepoNoLongerHasCanStillBeRemoved(t *testing.T) {
	d := openIssueDetail()
	d.Labels = []string{"retired-label"}
	f := installIssueActions(t, d)
	m := keyAll(issueScreen(t, f), "l")
	if !strings.Contains(m.View(), "[x] retired-label") {
		t.Fatalf("a label missing from the repo list must still be shown:\n%s", m.View())
	}
	// sorted: bug, docs, needs triage 🚧, retired-label, wontfix -> the fourth row
	m = keyAll(m, "down", "down", "down", "space", "enter")
	want := editCall{repo: acme, number: "14812", removeLabels: []string{"retired-label"}}
	if len(f.edits) != 1 || !reflect.DeepEqual(f.edits[0], want) {
		t.Errorf("edits = %+v; want %+v", f.edits, want)
	}
}

func TestAssigneesKeyListsAssignableUsers(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := keyAll(issueScreen(t, f), "s")
	v := m.View()
	for _, want := range []string{"[x] erin", "[ ] frank", "[ ] gina"} {
		if !strings.Contains(v, want) {
			t.Errorf("assignee dialog lacks %q:\n%s", want, v)
		}
	}
	m = keyAll(m, "down", "space", "enter") // tick frank
	want := editCall{repo: acme, number: "14812", addAssignees: []string{"frank"}}
	if len(f.edits) != 1 || !reflect.DeepEqual(f.edits[0], want) {
		t.Errorf("edits = %+v; want %+v", f.edits, want)
	}
}

func TestIssueWithNoAssigneesOrLabelsOffersTheFullList(t *testing.T) {
	d := openIssueDetail()
	d.Labels, d.Assignees = nil, nil
	f := installIssueActions(t, d)
	m := keyAll(issueScreen(t, f), "l")
	if v := m.View(); strings.Contains(v, "[x]") || !strings.Contains(v, "[ ] bug") {
		t.Errorf("nothing should be ticked:\n%s", v)
	}
	m = keyAll(m, "esc", "s")
	if v := m.View(); strings.Contains(v, "[x]") || !strings.Contains(v, "[ ] erin") {
		t.Errorf("nothing should be ticked:\n%s", v)
	}
}

func TestEditErrorIsShownAndNothingIsClaimedDone(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	f.err = errors.New("HTTP 403: must have push access")
	m := keyAll(issueScreen(t, f), "s", "down", "space", "enter")
	if !strings.Contains(m.status, "push access") || strings.Contains(m.status, "✓") {
		t.Errorf("status = %q", m.status)
	}
}

func TestLabelListFailureIsShownWithoutADialog(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	f.labelsErr = errors.New("HTTP 404")
	m := keyAll(issueScreen(t, f), "l")
	if m.dlg != nil || !strings.Contains(m.status, "404") {
		t.Errorf("dialog=%v status=%q", m.dlg != nil, m.status)
	}
}

func TestDetailReloadsAfterEachAction(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := issueScreen(t, f)
	before := f.fetches
	m = keyAll(m, "x", "y")
	m = keyAll(m, "s", "down", "space", "enter")
	if f.fetches != before+2 {
		t.Errorf("fetches %d -> %d; want one reload after each of the two actions", before, f.fetches)
	}
}

func TestIssueActionsActOnTheRepoTheViewShows(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := issueScreen(t, f)
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"}
	m = keyAll(m, "s", "down", "space", "enter")
	if len(f.edits) != 1 || f.edits[0].repo != acme {
		t.Errorf("edits = %+v; want the issue's own repo, not base", f.edits)
	}
}

func TestCancellingAnIssueDialogChangesNothing(t *testing.T) {
	f := installIssueActions(t, openIssueDetail())
	m := keyAll(issueScreen(t, f), "l", "esc", "s", "esc", "x", "esc")
	if m.dlg != nil || len(f.edits) != 0 || len(f.closes) != 0 {
		t.Errorf("dialog=%v edits=%v closes=%v", m.dlg != nil, f.edits, f.closes)
	}
}
