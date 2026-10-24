package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type prActionCall struct {
	repo   ghcli.RepoRef
	number string
	kind   ghcli.ReviewKind
	body   string
}

type prActionFakes struct {
	comments []prActionCall
	reviews  []prActionCall
	err      error
}

func installPRActions(t *testing.T) *prActionFakes {
	t.Helper()
	f := &prActionFakes{}
	oldC, oldR := commentPR, reviewPR
	commentPR = func(r ghcli.RepoRef, number, body string) error {
		f.comments = append(f.comments, prActionCall{repo: r, number: number, body: body})
		return f.err
	}
	reviewPR = func(r ghcli.RepoRef, number string, kind ghcli.ReviewKind, body string) error {
		f.reviews = append(f.reviews, prActionCall{r, number, kind, body})
		return f.err
	}
	t.Cleanup(func() { commentPR, reviewPR = oldC, oldR })
	return f
}

// pump runs the commands a key produced and feeds their messages back, so a chain such as
// dialog answer -> action -> refresh plays out. Only the seams make calls, so this never reaches gh.
func pump(m model, c tea.Cmd) model {
	for i := 0; i < 8 && c != nil; i++ {
		msg := c()
		if msg == nil {
			break
		}
		var next tea.Model
		next, c = m.Update(msg)
		m = next.(model)
	}
	return m
}

func keyAll(m model, keysToPress ...string) model {
	for _, key := range keysToPress {
		next, c := m.Update(k(key))
		m = pump(next.(model), c)
	}
	return m
}

func typeAll(m model, s string) model {
	for _, r := range s {
		if r == ' ' {
			m = keyAll(m, "space")
		} else {
			m = keyAll(m, string(r))
		}
	}
	return m
}

func prDetailOpen(t *testing.T) (model, *prActionFakes) {
	t.Helper()
	f := installPRActions(t)
	installPRFetch(t, fixturePR())
	m := prListModel()
	m.target, m.prTarget = acme, acme
	return openDetail(t, m), f
}

func TestCKeyOpensATextDialogNamingTheRepoAndPR(t *testing.T) {
	m, _ := prDetailOpen(t)
	m = keyAll(m, "C")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "acme/api#482") {
		t.Fatalf("dialog = %+v; want a comment dialog naming acme/api#482", m.dlg)
	}
	if v := m.View(); !strings.Contains(v, "acme/api#482") {
		t.Errorf("the repo and PR must be visible while typing:\n%s", v)
	}
}

func TestSubmittingTheCommentCallsTheSeamAndRefreshesTheDetail(t *testing.T) {
	m, f := prDetailOpen(t)
	m = keyAll(m, "C")
	m = typeAll(m, "Looks good to me")
	m = keyAll(m, "enter")
	if len(f.comments) != 1 || f.comments[0].number != "482" || f.comments[0].body != "Looks good to me" || f.comments[0].repo != acme {
		t.Fatalf("comments = %+v; want one for acme/api #482", f.comments)
	}
	if !strings.Contains(m.status, "✓") || !strings.Contains(m.status, "acme/api#482") {
		t.Errorf("status = %q; want a confirmation naming the PR", m.status)
	}
	if m.view != viewPRDetail || m.dlg != nil {
		t.Errorf("view=%v dialog open=%v; want back on the detail", m.view, m.dlg != nil)
	}
}

func TestReviewKeyOffersThreeChoices(t *testing.T) {
	m, _ := prDetailOpen(t)
	m = keyAll(m, "r")
	v := m.View()
	for _, want := range []string{"Approve", "Request changes", "Comment"} {
		if !strings.Contains(v, want) {
			t.Errorf("review dialog lacks %q:\n%s", want, v)
		}
	}
}

func TestRequestChangesNeedsABodyAndAConfirmation(t *testing.T) {
	m, f := prDetailOpen(t)
	m = keyAll(m, "r", "down", "enter") // Request changes
	if m.dlg == nil || m.dlg.kind != dialogText {
		t.Fatalf("dialog = %+v; want a text dialog for the reason", m.dlg)
	}
	m = keyAll(m, "enter") // empty
	if m.dlg == nil || m.dlg.kind != dialogText || len(f.reviews) != 0 {
		t.Fatalf("an empty reason must not go on; reviews=%v", f.reviews)
	}
	m = typeAll(m, "Please add a test")
	m = keyAll(m, "enter")
	if m.dlg == nil || m.dlg.kind != dialogConfirm || !strings.Contains(m.View(), "acme/api#482") {
		t.Fatalf("dialog = %+v; want a confirmation naming the PR", m.dlg)
	}
	if len(f.reviews) != 0 {
		t.Fatal("nothing may be sent before the confirmation")
	}
	cancelled := keyAll(m, "n")
	if len(f.reviews) != 0 || cancelled.dlg != nil {
		t.Errorf("declining must send nothing: %v", f.reviews)
	}
	m = keyAll(m, "y")
	if len(f.reviews) != 1 || f.reviews[0].kind != ghcli.ReviewRequestChanges || f.reviews[0].body != "Please add a test" {
		t.Errorf("reviews = %+v", f.reviews)
	}
}

func TestApproveWithoutABodyIsAllowed(t *testing.T) {
	m, f := prDetailOpen(t)
	m = keyAll(m, "r", "enter") // Approve is the first choice
	if m.dlg == nil || m.dlg.kind != dialogConfirm {
		t.Fatalf("dialog = %+v; want a confirmation", m.dlg)
	}
	m = keyAll(m, "y")
	if len(f.reviews) != 1 || f.reviews[0].kind != ghcli.ReviewApprove || f.reviews[0].body != "" {
		t.Errorf("reviews = %+v; want an approval with no body", f.reviews)
	}
}

func TestCommentReviewSendsItsTextWithoutAnExtraConfirmation(t *testing.T) {
	m, f := prDetailOpen(t)
	m = keyAll(m, "r", "down", "down", "enter") // Comment
	m = typeAll(m, "A thought")
	m = keyAll(m, "enter")
	if len(f.reviews) != 1 || f.reviews[0].kind != ghcli.ReviewComment || f.reviews[0].body != "A thought" {
		t.Errorf("reviews = %+v", f.reviews)
	}
}

func TestActionErrorIsShownAndNothingIsClaimedDone(t *testing.T) {
	m, f := prDetailOpen(t)
	f.err = errors.New("GraphQL: Can not approve your own pull request")
	m = keyAll(m, "r", "enter", "y")
	if !strings.Contains(m.status, "own pull request") || strings.Contains(m.status, "✓") {
		t.Errorf("status = %q; want the error and no tick", m.status)
	}
}

func TestCancellingTheKindChoiceDoesNothing(t *testing.T) {
	m, f := prDetailOpen(t)
	m = keyAll(m, "r", "esc")
	if m.dlg != nil || len(f.reviews) != 0 || len(f.comments) != 0 {
		t.Errorf("dialog open=%v reviews=%v comments=%v", m.dlg != nil, f.reviews, f.comments)
	}
}

func TestActionsActOnTheRepoTheViewShowsNotTheChosenBase(t *testing.T) {
	m, f := prDetailOpen(t)
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"} // a favorite from acme/api is open
	m = keyAll(m, "C")
	m = typeAll(m, "hi")
	m = keyAll(m, "enter")
	if len(f.comments) != 1 || f.comments[0].repo != acme {
		t.Errorf("comments = %+v; want the PR's own repo", f.comments)
	}
}
