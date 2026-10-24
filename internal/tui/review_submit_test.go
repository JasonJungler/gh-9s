package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

// ---- findings of the review of release 3d ----

func TestDraftsNeverFollowYouToAnotherPR(t *testing.T) {
	installSubmit(t)
	installDiff(t, reviewDiff, nil)
	installThreads(t, nil, nil)
	m := model{view: viewDiff, pr: "482", target: acme, width: 100, height: 30} // no drafts file
	next, _ := m.Update(m.fetch()())
	m = goToRow(t, next.(model), `+ import "log"`)
	m = comment(m, "c", "for 482 only")
	m.pr = "483" // back, then the diff of the next PR, which touches the same line
	next, _ = m.Update(m.fetch()())
	if n := len(diffSt(next.(model)).drafts); n != 0 {
		t.Errorf("%d draft(s) of PR 482 are on PR 483", n)
	}
	zero := model{view: viewDiff, pr: "482", width: 100, height: 30} // repo not resolved yet: no key at all
	next, _ = zero.Update(zero.fetch()())
	z := goToRow(t, next.(model), `+ import "log"`)
	z = comment(z, "c", "x")
	z.pr = "483"
	next, _ = z.Update(z.fetch()())
	if n := len(diffSt(next.(model)).drafts); n != 0 {
		t.Errorf("without a repo, %d draft(s) still crossed to another PR", n)
	}
}

func TestDraftsWrittenForAnOlderCommitAreNotSentUntilYouSayYes(t *testing.T) {
	f := installSubmit(t)
	f.head = "aaaaaaa1111111"
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "about the log import")
	if got := stored(t, path); len(got) != 1 || got[0].Head != "aaaaaaa1111111" {
		t.Fatalf("a draft remembers its commit: %+v", got)
	}
	f.head = "bbbbbbb2222222" // the author pushed
	next, _ := m.Update(m.fetch()())
	m = next.(model)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "older commit") {
		t.Errorf("a draft from an older commit is marked:\n%s", v)
	}
	m = keyAll(m, "r")
	if m.dlg != nil || !strings.Contains(m.status, "older commit") || len(f.calls) != 0 {
		t.Fatalf("dialog=%v status=%q; stale drafts block the review until adopted", m.dlg != nil, m.status)
	}
	m = keys(m, "A")
	m = keyAll(m, "r", "enter", "y")
	if len(f.calls) != 1 || f.calls[0].head != "bbbbbbb2222222" {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestADiffAndHeadReadAcrossAPushAreNotTrusted(t *testing.T) {
	f := installSubmit(t)
	heads := []string{"aaaaaaa1111111", "bbbbbbb2222222"}
	n := 0
	prHead = func(ghcli.RepoRef, string) (string, error) { n++; return heads[min(n-1, 1)], nil }
	m, _ := draftModel(t, 100, reviewDiff)
	if h := diffSt(m).head; h != "" {
		t.Errorf("head = %q; the head changed while the diff was being fetched, so it is unknown", h)
	}
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "x")
	m = keyAll(m, "r")
	if m.dlg != nil || !strings.Contains(m.status, "reload") || len(f.calls) != 0 {
		t.Errorf("dialog=%v status=%q", m.dlg != nil, m.status)
	}
}

func TestOnlyOneReviewIsInFlightAndOnlyTheSentDraftsAreCleared(t *testing.T) {
	m, f, path := reviewWithDrafts(t)
	key, _ := m.draftsKey()
	m.sendingKey = key
	m = keyAll(m, "r")
	if m.dlg != nil || !strings.Contains(m.status, "being sent") || len(f.calls) != 0 {
		t.Errorf("dialog=%v status=%q; a second review while one is in flight is refused", m.dlg != nil, m.status)
	}
	m.sendingKey = ""
	st := diffSt(m)
	sent, later := st.drafts[:1], st.drafts[1:]
	m = m.clearSentDrafts(key, sent)
	if got := diffSt(m).drafts; len(got) != 1 || got[0] != later[0] {
		t.Errorf("drafts left = %+v; want only the one that was not sent", got)
	}
	if got := stored(t, path); len(got) != 1 || got[0] != later[0] {
		t.Errorf("stored = %+v", got)
	}
}

func TestACommentIsCleanedBeforeItIsKept(t *testing.T) {
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "bad \x1b[31m text\r")
	got := stored(t, path)
	if len(got) != 1 || strings.ContainsAny(got[0].Body, "\x1b\r") {
		t.Errorf("stored %+v; control characters must not stop the draft from being saved", got)
	}
}

func TestDraftsNotYetOnDiskSurviveARefetch(t *testing.T) {
	m, _ := draftModel(t, 100, reviewDiff)
	st := diffSt(m)
	st.drafts = []drafts.Draft{{Path: "main.go", Line: 2, Side: "RIGHT", Body: "unsaved", Head: st.head}} // a save that failed
	m = m.withScreenData(viewDiff, st)
	next, _ := m.Update(m.fetch()())
	if got := diffSt(next.(model)).drafts; len(got) != 1 || got[0].Body != "unsaved" {
		t.Errorf("drafts after R = %+v; memory and file are merged, not replaced", got)
	}
}

func TestTheReviewGoesToTheRepoOfTheCurrentDirectoryWhenThereIsNoTarget(t *testing.T) {
	f := installSubmit(t)
	ghe := ghcli.RepoRef{Host: "ghe.corp", Owner: "acme", Name: "api"}
	installDiff(t, reviewDiff, nil)
	installThreads(t, nil, nil)
	m := model{view: viewDiff, pr: "482", curRepo: ghe, width: 100, height: 30}
	next, _ := m.Update(m.fetch()())
	m = goToRow(t, next.(model), `+ import "log"`)
	m = comment(m, "c", "x")
	m = keyAll(m, "r", "enter", "y")
	if len(f.calls) != 1 || f.calls[0].repo != ghe {
		t.Errorf("calls = %+v; want the Enterprise repo of the current directory", f.calls)
	}
	none := model{view: viewDiff, pr: "482", width: 100, height: 30}
	next, _ = none.Update(none.fetch()())
	none = goToRow(t, next.(model), `+ import "log"`)
	none = comment(none, "c", "x")
	f.calls = nil
	none = keyAll(none, "r")
	if none.dlg != nil || len(f.calls) != 0 || none.status == "" {
		t.Errorf("without a known repo a review can't start: dialog=%v status=%q", none.dlg != nil, none.status)
	}
}

func TestARangeAcrossHunksIsRefused(t *testing.T) {
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`) // first hunk
	m = keys(m, "V")
	m = goToRow2(t, m, "+ var x = 2") // second hunk
	m = keyAll(m, "c")
	if m.dlg != nil || !strings.Contains(m.status, "hunk") || len(stored(t, path)) != 0 {
		t.Errorf("dialog=%v status=%q; GitHub rejects a comment across hunks", m.dlg != nil, m.status)
	}
}

// goToRow2 moves the cursor to a row while a range is open (goToRow starts with g, which would not
// cancel the range, but this keeps the intent clear).
func goToRow2(t *testing.T, m model, subs ...string) model {
	t.Helper()
	row := rowWith(t, m, subs...)
	for diffSt(m).cur < row {
		m = keys(m, "j")
	}
	return m
}

func TestSearchPutsTheCursorOnTheMatch(t *testing.T) {
	m, _ := draftModel(t, 100, reviewDiff)
	m = keys(m, "/", "l", "o", "g")
	next, _ := m.Update(k("enter"))
	m = next.(model)
	st := diffSt(m)
	if len(m.matches) == 0 || st.cur != m.matches[m.mi] {
		t.Errorf("cursor row %d, match row %v: a comment would land on the wrong line", st.cur, m.matches)
	}
}
