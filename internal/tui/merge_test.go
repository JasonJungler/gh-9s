package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type mergeCall struct {
	target ghcli.RepoRef
	number string
	method ghcli.MergeMethod
	del    bool
	head   string
}

type mergeFakes struct {
	calls      []mergeCall
	mergeErr   error
	methods    []ghcli.MergeMethod
	methodsErr error
	fetches    int
	prs        []ghcli.PRDetail // what fetchPR returns on successive calls (the last one repeats)
}

func installMerge(t *testing.T, methods []ghcli.MergeMethod, prs ...ghcli.PRDetail) *mergeFakes {
	t.Helper()
	f := &mergeFakes{methods: methods, prs: prs}
	oldPR, oldAllowed, oldMerge, oldComment, oldReview := fetchPR, allowedMethods, mergePR, commentPR, reviewPR
	fetchPR = func(r ghcli.RepoRef, number string) (ghcli.PRDetail, error) {
		i := min(f.fetches, len(f.prs)-1)
		f.fetches++
		return f.prs[i], nil
	}
	allowedMethods = func(r ghcli.RepoRef) ([]ghcli.MergeMethod, error) { return f.methods, f.methodsErr }
	mergePR = func(r ghcli.RepoRef, number string, m ghcli.MergeMethod, del bool, head string) error {
		f.calls = append(f.calls, mergeCall{r, number, m, del, head})
		return f.mergeErr
	}
	t.Cleanup(func() {
		fetchPR, allowedMethods, mergePR, commentPR, reviewPR = oldPR, oldAllowed, oldMerge, oldComment, oldReview
	})
	return f
}

func prState(mergeable, mergeState, review string) ghcli.PRDetail {
	d := fixturePR()
	d.State, d.Mergeable, d.MergeState, d.ReviewDecision = "OPEN", mergeable, mergeState, review
	d.HeadSHA = "deadbeef"
	return d
}

var allMethods = []ghcli.MergeMethod{ghcli.MergeCommit, ghcli.MergeSquash, ghcli.MergeRebase}

func mergeStart(t *testing.T, f *mergeFakes) model {
	t.Helper()
	m := openDetail(t, func() model { m := prListModel(); m.target, m.prTarget = acme, acme; return m }())
	m.prefsFile = filepath.Join(t.TempDir(), "prefs.json")
	_ = f
	return m
}

func TestOnlyAllowedMethodsAreListed(t *testing.T) {
	f := installMerge(t, []ghcli.MergeMethod{ghcli.MergeSquash}, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m")
	v := m.View()
	if !strings.Contains(v, "Squash and merge") || strings.Contains(v, "Rebase and merge") || strings.Contains(v, "merge commit") {
		t.Errorf("only squash is allowed here:\n%s", v)
	}
	f = installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m = keyAll(mergeStart(t, f), "m")
	v = m.View()
	for _, want := range []string{"merge commit", "Squash and merge", "Rebase and merge"} {
		if !strings.Contains(v, want) {
			t.Errorf("three allowed methods should all be listed, lacks %q:\n%s", want, v)
		}
	}
}

func TestNoAllowedMethodExplainsAndOpensNothing(t *testing.T) {
	f := installMerge(t, nil, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m")
	if m.dlg != nil || !strings.Contains(m.status, "no merge method") || !strings.Contains(m.status, "acme/api") {
		t.Errorf("dialog=%v status=%q; want a message and no dialog", m.dlg != nil, m.status)
	}
}

func TestBlockedPRCannotBeMerged(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "BLOCKED", "REVIEW_REQUIRED"))
	m := keyAll(mergeStart(t, f), "m")
	if m.dlg == nil || !strings.Contains(m.View(), "a required review is missing") {
		t.Fatalf("a blocked PR should show why:\n%s", m.View())
	}
	m = keyAll(m, "enter", "!") // neither enter nor ! gets past a block
	if len(f.calls) != 0 || m.dlg == nil || m.dlg.kind != dialogChoice {
		t.Errorf("calls=%v dialog=%+v; nothing may proceed", f.calls, m.dlg)
	}
}

func TestUnknownMergeStateSaysRefreshAndRefuses(t *testing.T) {
	f := installMerge(t, allMethods, prState("UNKNOWN", "UNKNOWN", ""))
	m := keyAll(mergeStart(t, f), "m")
	if v := m.View(); !strings.Contains(v, "refresh") {
		t.Errorf("an unknown state should tell the user to refresh:\n%s", v)
	}
	m = keyAll(m, "enter")
	if len(f.calls) != 0 {
		t.Errorf("must not merge on an unknown state: %v", f.calls)
	}
}

func TestWarnNeedsTheDeliberateKey(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "UNSTABLE", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m", "down", "enter") // Squash
	if m.dlg == nil || m.dlg.kind != dialogChoice || !strings.Contains(m.dlg.err, "!") || len(f.calls) != 0 {
		t.Fatalf("enter must be refused with a hint: dialog=%+v calls=%v", m.dlg, f.calls)
	}
	if v := m.View(); !strings.Contains(v, "non-required checks") || !strings.Contains(v, "! accept anyway") {
		t.Errorf("the warning's reason and the way forward should be visible:\n%s", v)
	}
	m = keyAll(m, "!")
	if m.dlg == nil || m.dlg.kind != dialogConfirm {
		t.Fatalf("! should move on to the confirmation, dialog=%+v", m.dlg)
	}
	m = keyAll(m, "y")
	if len(f.calls) != 1 || f.calls[0].method != ghcli.MergeSquash {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestCleanPRMergesAfterAConfirmationNamingEverything(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m", "down", "enter") // Squash
	if m.dlg == nil || m.dlg.kind != dialogConfirm {
		t.Fatalf("dialog = %+v; want a confirmation", m.dlg)
	}
	v := m.View()
	for _, want := range []string{"acme/api#482", "Squash and merge", "fix/retry"} {
		if !strings.Contains(v, want) {
			t.Errorf("confirmation lacks %q:\n%s", want, v)
		}
	}
	if len(f.calls) != 0 {
		t.Fatal("nothing may be merged before the confirmation")
	}
	m = keyAll(m, "y")
	want := mergeCall{acme, "482", ghcli.MergeSquash, false, "deadbeef"}
	if len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("calls = %+v; want %+v", f.calls, want)
	}
	if !strings.Contains(m.status, "✓") || !strings.Contains(m.status, "acme/api#482") {
		t.Errorf("status = %q", m.status)
	}
}

func TestDeleteBranchChoiceIsRemembered(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := mergeStart(t, f)
	m = keyAll(m, "m", "down", "enter")
	if !strings.Contains(m.View(), "[ ] Delete branch") {
		t.Fatalf("the first time the option is off:\n%s", m.View())
	}
	m = keyAll(m, "d", "y")
	if len(f.calls) != 1 || !f.calls[0].del {
		t.Fatalf("calls = %+v; want the branch deleted", f.calls)
	}
	if b, _ := os.ReadFile(m.prefsFile); !strings.Contains(string(b), "true") {
		t.Errorf("prefs file = %q; the choice should be saved", b)
	}
	m = keyAll(m, "m", "down", "enter")
	if !strings.Contains(m.View(), "[x] Delete branch") {
		t.Errorf("the saved choice should be the default next time:\n%s", m.View())
	}
}

func TestMergeErrorIsShown(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	f.mergeErr = errors.New("Pull request is not mergeable: the base branch policy prohibits the merge")
	m := keyAll(mergeStart(t, f), "m", "down", "enter", "y")
	if !strings.Contains(m.status, "policy prohibits") || strings.Contains(m.status, "✓") {
		t.Errorf("status = %q; want the error and no tick", m.status)
	}
}

func TestMergeRefreshesTheDetailAfterSuccess(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := mergeStart(t, f) // opening the detail fetched once
	before := f.fetches
	m = keyAll(m, "m", "down", "enter", "y")
	// one fresh read when m was pressed, one reload after the merge
	if f.fetches != before+2 {
		t.Errorf("fetches went %d -> %d; want a fresh read before and a reload after", before, f.fetches)
	}
	_ = m
}

func TestMergeDecidesOnAFreshReadNotTheScreen(t *testing.T) {
	// the screen was loaded while blocked; by the time m is pressed GitHub says clean
	f := installMerge(t, allMethods, prState("MERGEABLE", "BLOCKED", "REVIEW_REQUIRED"), prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m", "down", "enter")
	if m.dlg == nil || m.dlg.kind != dialogConfirm {
		t.Errorf("dialog = %+v; the fresh clean state should allow the merge", m.dlg)
	}
	// and the opposite: loaded clean, now blocked
	f = installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"), prState("MERGEABLE", "BLOCKED", "CHANGES_REQUESTED"))
	m = keyAll(mergeStart(t, f), "m", "down", "enter")
	if m.dlg == nil || m.dlg.kind != dialogChoice || len(f.calls) != 0 {
		t.Errorf("dialog = %+v; a PR that became blocked must not be offered", m.dlg)
	}
}

func TestCancelAtAnyStepMergesNothing(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := keyAll(mergeStart(t, f), "m", "esc")
	m = keyAll(m, "m", "down", "enter", "n")
	if len(f.calls) != 0 || m.dlg != nil {
		t.Errorf("calls=%v dialog open=%v", f.calls, m.dlg != nil)
	}
}

func TestMergeActsOnTheRepoTheViewShows(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	m := mergeStart(t, f)
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"}
	m = keyAll(m, "m", "down", "enter", "y")
	if len(f.calls) != 1 || f.calls[0].target != acme {
		t.Errorf("calls = %+v; want the PR's own repo, not base", f.calls)
	}
}

func TestMethodLookupFailureIsShown(t *testing.T) {
	f := installMerge(t, allMethods, prState("MERGEABLE", "CLEAN", "APPROVED"))
	f.methodsErr = errors.New("HTTP 404")
	m := keyAll(mergeStart(t, f), "m")
	if m.dlg != nil || !strings.Contains(m.status, "404") {
		t.Errorf("dialog=%v status=%q", m.dlg != nil, m.status)
	}
}
