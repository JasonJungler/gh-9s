package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type submitCall struct {
	repo     ghcli.RepoRef
	number   string
	head     string
	kind     ghcli.ReviewKind
	body     string
	comments []ghcli.InlineComment
}

type submitFakes struct {
	calls []submitCall
	err   error
	head  string // what the PR's head is "now"
}

func installSubmit(t *testing.T) *submitFakes {
	t.Helper()
	f := &submitFakes{head: "aaaaaaa1111111"}
	oldS, oldH := submitReviewInline, prHead
	submitReviewInline = func(r ghcli.RepoRef, number, head string, kind ghcli.ReviewKind, body string, cs []ghcli.InlineComment) error {
		f.calls = append(f.calls, submitCall{r, number, head, kind, body, cs})
		return f.err
	}
	prHead = func(r ghcli.RepoRef, number string) (string, error) { return f.head, nil }
	t.Cleanup(func() { submitReviewInline, prHead = oldS, oldH })
	return f
}

// reviewWithDrafts is a review screen at width 100 with two drafts, fetched at head aaaaaaa1111111.
func reviewWithDrafts(t *testing.T) (model, *submitFakes, string) {
	t.Helper()
	f := installSubmit(t)
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "why log?")
	m = goToRow(t, m, "+ var x = 2")
	m = comment(m, "c", "magic number")
	if len(diffSt(m).drafts) != 2 {
		t.Fatalf("drafts = %+v", diffSt(m).drafts)
	}
	return m, f, path
}

func TestApproveSendsTheDraftsAsOneReviewAfterAConfirmation(t *testing.T) {
	m, f, path := reviewWithDrafts(t)
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"} // must not be used
	m = keyAll(m, "r", "enter")                           // Approve is the first choice
	if m.dlg == nil || m.dlg.kind != dialogConfirm || !strings.Contains(m.dlg.title, "acme/api#482") || !strings.Contains(m.dlg.title+m.dlg.detail, "2 inline comments") {
		t.Fatalf("dialog = %+v; want a confirmation naming the PR and the 2 comments", m.dlg)
	}
	if len(f.calls) != 0 {
		t.Fatal("nothing may be sent before the confirmation")
	}
	m = keyAll(m, "y")
	if len(f.calls) != 1 {
		t.Fatalf("%d reviews sent", len(f.calls))
	}
	c := f.calls[0]
	if c.repo != acme || c.number != "482" || c.head != f.head || c.kind != ghcli.ReviewApprove || c.body != "" {
		t.Errorf("call = %+v", c)
	}
	want := []ghcli.InlineComment{
		{Path: "main.go", Line: 2, Side: "RIGHT", Body: "why log?"},
		{Path: "main.go", Line: 21, Side: "RIGHT", Body: "magic number"},
	}
	if len(c.comments) != 2 || c.comments[0] != want[0] || c.comments[1] != want[1] {
		t.Errorf("comments = %+v; want %+v", c.comments, want)
	}
	if !strings.Contains(m.status, "✓") {
		t.Errorf("status = %q", m.status)
	}
	if len(diffSt(m).drafts) != 0 || len(stored(t, path)) != 0 {
		t.Error("a sent review uses the drafts up: gone from memory and disk")
	}
}

func TestRequestChangesNeedsABodyAndTheDraftsGoWithIt(t *testing.T) {
	m, f, _ := reviewWithDrafts(t)
	m = keyAll(m, "r", "down", "enter") // Request changes
	m = keyAll(m, "enter")              // empty body: refused
	if m.dlg == nil || m.dlg.kind != dialogText {
		t.Fatalf("dialog = %+v; an empty body must keep the text dialog open", m.dlg)
	}
	m = typeAll(m, "Please address these")
	m = keyAll(m, "enter", "y")
	if len(f.calls) != 1 || f.calls[0].kind != ghcli.ReviewRequestChanges || f.calls[0].body != "Please address these" || len(f.calls[0].comments) != 2 {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestACommentReviewWithDraftsIsConfirmedToo(t *testing.T) {
	m, f, _ := reviewWithDrafts(t)
	m = keyAll(m, "r", "down", "down", "enter") // Comment
	m = typeAll(m, "A few thoughts")
	m = keyAll(m, "enter")
	if m.dlg == nil || m.dlg.kind != dialogConfirm || len(f.calls) != 0 {
		t.Fatalf("dialog = %+v calls=%d; inline comments always get a confirmation", m.dlg, len(f.calls))
	}
	m = keyAll(m, "y")
	if len(f.calls) != 1 || f.calls[0].kind != ghcli.ReviewComment {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestDecliningSendsNothingAndKeepsTheDrafts(t *testing.T) {
	m, f, path := reviewWithDrafts(t)
	m = keyAll(m, "r", "enter", "n")
	if len(f.calls) != 0 || len(diffSt(m).drafts) != 2 || len(stored(t, path)) != 2 {
		t.Errorf("calls=%d drafts=%d", len(f.calls), len(diffSt(m).drafts))
	}
}

func TestAMovedHeadRefusesAndKeepsTheDrafts(t *testing.T) {
	m, f, path := reviewWithDrafts(t)
	f.head = "bbbbbbb2222222" // someone pushed
	m = keyAll(m, "r", "enter", "y")
	if len(f.calls) != 0 {
		t.Fatal("comments must never be posted against a commit the reviewer did not see")
	}
	if !strings.Contains(m.status, "new commits") || len(diffSt(m).drafts) != 2 || len(stored(t, path)) != 2 {
		t.Errorf("status=%q drafts=%d", m.status, len(diffSt(m).drafts))
	}
}

func TestAGitHubErrorIsShownAsIsAndKeepsTheDrafts(t *testing.T) {
	m, f, path := reviewWithDrafts(t)
	f.err = errors.New("gh: Unprocessable Entity (HTTP 422): User can only have one pending review per pull request")
	m = keyAll(m, "r", "enter", "y")
	if !strings.Contains(m.status, "one pending review") || strings.Contains(m.status, "✓") {
		t.Errorf("status = %q", m.status)
	}
	if len(diffSt(m).drafts) != 2 || len(stored(t, path)) != 2 {
		t.Error("a failed send keeps the drafts")
	}
}

func TestPressingYTwiceSendsOneReview(t *testing.T) {
	m, f, _ := reviewWithDrafts(t)
	m = keyAll(m, "r", "enter", "y", "y")
	if len(f.calls) != 1 {
		t.Errorf("%d reviews sent", len(f.calls))
	}
}

func TestWithoutDraftsTheOldReviewPathIsUsed(t *testing.T) {
	f := installSubmit(t)
	r := installPRActions(t)
	m, _ := draftModel(t, 100, reviewDiff)
	m = keyAll(m, "r", "enter", "y")
	if len(f.calls) != 0 || len(r.reviews) != 1 || r.reviews[0].kind != ghcli.ReviewApprove {
		t.Errorf("inline calls=%d plain reviews=%+v", len(f.calls), r.reviews)
	}
}

func TestAnUnknownHeadRefusesToSend(t *testing.T) {
	f := installSubmit(t)
	m, _ := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "x")
	st := diffSt(m)
	st.head = ""
	m = m.withScreenData(viewDiff, st)
	m = keyAll(m, "r")
	if m.dlg != nil || !strings.Contains(m.status, "reload") || len(f.calls) != 0 {
		t.Errorf("dialog=%v status=%q", m.dlg != nil, m.status)
	}
}

func TestTheDraftsSentAreTheOnesOfThisPR(t *testing.T) {
	f := installSubmit(t)
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "mine")
	s, _ := drafts.Load(path)
	_ = s.Put("github.com/acme/api#999", "abc1234", []drafts.Draft{{Path: "x.go", Line: 1, Side: "RIGHT", Body: "other PR"}})
	m = keyAll(m, "r", "enter", "y")
	if len(f.calls) != 1 || len(f.calls[0].comments) != 1 || f.calls[0].comments[0].Body != "mine" {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestARangeDraftGoesOutAsARange(t *testing.T) {
	f := installSubmit(t)
	var b strings.Builder
	b.WriteString("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,0 +10,3 @@\n+a\n+b\n+c\n")
	m, _ := draftModel(t, 100, b.String())
	m = goToRow(t, m, "+ a")
	m = keys(m, "V", "j", "j")
	m = comment(m, "c", "all three")
	m = keyAll(m, "r", "enter", "y")
	if len(f.calls) != 1 || len(f.calls[0].comments) != 1 {
		t.Fatalf("calls = %+v", f.calls)
	}
	if c := f.calls[0].comments[0]; c.StartLine != 10 || c.Line != 12 || c.Side != "RIGHT" {
		t.Errorf("comment = %+v; want lines 10–12", c)
	}
}
