package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type discCall struct {
	repo                      ghcli.RepoRef
	discussionID, body, reply string
}

type discFakes struct {
	listCats []string // the category of every list read
	opened   []string // the number of every discussion read
	comments []discCall
	err      error
	list     ghcli.DiscussionList
	detail   ghcli.DiscussionDetail
}

func fixtureDiscList() ghcli.DiscussionList {
	return ghcli.DiscussionList{Enabled: true, Total: 2,
		Categories: []ghcli.DiscCategory{{ID: "C1", Name: "Q&A", Answerable: true}, {ID: "C2", Name: "Ideas"}},
		Rows: []ghcli.DiscussionRow{
			{ID: "D1", Number: 42, Title: "How do I retry?", Body: "Please **help** with retries.", Author: "ann", Category: "Q&A", UpdatedAt: "2026-10-23T10:00:00Z", Answered: true, Comments: 3, URL: "https://github.com/acme/api/discussions/42"},
			{ID: "D2", Number: 41, Title: "Dark theme", Author: "bob", Category: "Ideas", UpdatedAt: "2026-10-22T10:00:00Z", Closed: true, Locked: true, URL: "https://github.com/acme/api/discussions/41"},
		}}
}

func fixtureDisc() ghcli.DiscussionDetail {
	return ghcli.DiscussionDetail{ID: "D1", Number: 42, Title: "How do I retry?", Body: "Please help.", Author: "ann", Category: "Q&A", Enabled: true,
		URL: "https://github.com/acme/api/discussions/42", Answered: true, CommentTotal: 2,
		Comments: []ghcli.DiscComment{
			{ID: "K1", Author: "bob", Body: "Use a loop.", CreatedAt: "2026-10-23T11:00:00Z", IsAnswer: true, ReplyTotal: 2,
				Replies: []ghcli.DiscReply{{ID: "R1", Author: "cy", Body: "Thanks!", CreatedAt: "2026-10-23T12:00:00Z"}, {ID: "R2", Author: "ann", Body: "Works.", CreatedAt: "2026-10-23T13:00:00Z"}}},
			{ID: "K2", Author: "dee", Body: "Or a library.", CreatedAt: "2026-10-23T14:00:00Z"},
		}}
}

func installDisc(t *testing.T) *discFakes {
	t.Helper()
	f := &discFakes{list: fixtureDiscList(), detail: fixtureDisc()}
	oL, oD, oC := fetchDiscList, fetchDisc, commentDisc
	fetchDiscList = func(tg ghcli.RepoRef, cat string) (ghcli.DiscussionList, error) {
		f.listCats = append(f.listCats, cat)
		return f.list, nil
	}
	fetchDisc = func(tg ghcli.RepoRef, number string) (ghcli.DiscussionDetail, error) {
		f.opened = append(f.opened, number)
		return f.detail, nil
	}
	commentDisc = func(tg ghcli.RepoRef, id, body, reply string) error {
		f.comments = append(f.comments, discCall{tg, id, body, reply})
		return f.err
	}
	t.Cleanup(func() { fetchDiscList, fetchDisc, commentDisc = oL, oD, oC })
	return f
}

func discListModel(t *testing.T) model {
	t.Helper()
	m := model{view: viewDiscussions, width: 130, height: 30, target: acme, prTarget: acme, previewOff: true}
	return pump(m, m.fetch())
}

func discOpen(t *testing.T) model {
	t.Helper()
	m := discListModel(t)
	return keyAll(m, "enter")
}

func screenText(m model) string { return ansi.Strip(m.View()) }

func TestFiveKeyAndTheDiscussionsCommandOpenTheList(t *testing.T) {
	f := installDisc(t)
	m := model{view: viewRuns, width: 130, height: 30, target: acme}
	if m = keyAll(m, "5"); m.view != viewDiscussions || len(f.listCats) != 1 {
		t.Errorf("5 -> view %v after %d reads", m.view, len(f.listCats))
	}
	for _, c := range []string{"discussions", "disc"} {
		m = model{view: viewRuns, width: 130, height: 30, target: acme}
		next, cmd := m.command(c)
		if got := pump(next.(model), cmd); got.view != viewDiscussions {
			t.Errorf(":%s -> view %v", c, got.view)
		}
	}
}

func TestDiscussionListShowsMarksCountsAndTheCategory(t *testing.T) {
	installDisc(t)
	m := discListModel(t)
	s := screenText(m)
	for _, want := range []string{"CATEGORY", "✓ How do I retry?", "[closed] [locked] Dark theme", "Q&A", "Ideas", "ann", "bob"} {
		if !strings.Contains(s, want) {
			t.Errorf("list lacks %q:\n%s", want, s)
		}
	}
}

func TestDiscussionListSaysWhenThereAreMoreThanItShows(t *testing.T) {
	f := installDisc(t)
	f.list.Total = 73
	m := discListModel(t)
	if !strings.Contains(ansi.Strip(m.status), "first 2 of 73") {
		t.Errorf("status %q", m.status)
	}
}

func TestDiscussionsDisabledIsSaidNotAnError(t *testing.T) {
	f := installDisc(t)
	f.list = ghcli.DiscussionList{Enabled: false}
	m := discListModel(t)
	if m.err != nil || !strings.Contains(screenText(m), "Discussions are not enabled in acme/api") {
		t.Errorf("err %v\n%s", m.err, screenText(m))
	}
}

func TestCategoryPickNarrowsTheListOnTheServer(t *testing.T) {
	f := installDisc(t)
	m := discListModel(t)
	m = keyAll(m, "c")
	if m.dlg == nil || m.dlg.id != "disc.category" || len(m.dlg.choices) != 3 || !m.dlg.choices[0].disabled {
		t.Fatalf("dialog %+v", m.dlg)
	}
	m, cmd := answer(m, "disc.category", true, "", "C2")
	m = pump(m, cmd)
	if got := f.listCats[len(f.listCats)-1]; got != "C2" || m.discCat != "C2" || m.discCatName != "Ideas" {
		t.Errorf("read category %q, model %q/%q", got, m.discCat, m.discCatName)
	}
	if !strings.Contains(screenText(m), "category=Ideas") || !strings.Contains(screenText(m), "How do I retry?") {
		t.Errorf("title lacks the category, or the answer for it was dropped:\n%s", screenText(m))
	}
	// back to all
	m = keyAll(m, "c")
	if ch := m.dlg.choices; ch[0].disabled || ch[1].disabled || !ch[2].disabled {
		t.Errorf("only the current category (Ideas) should be disabled: %+v", ch)
	}
	m, cmd = answer(m, "disc.category", true, "", discAllCategories)
	m = pump(m, cmd)
	if got := f.listCats[len(f.listCats)-1]; got != "" || m.discCat != "" {
		t.Errorf("All still reads category %q", got)
	}
}

func TestCategoryAnswerForAnUnknownCategoryOrAnotherScreenChangesNothing(t *testing.T) {
	f := installDisc(t)
	m := discListModel(t)
	n := len(f.listCats)
	m, cmd := answer(m, "disc.category", true, "", "GONE")
	if cmd != nil || m.discCat != "" || len(f.listCats) != n {
		t.Error("an unknown category was used")
	}
	m.view = viewIssues
	m, cmd = answer(m, "disc.category", true, "", "C1")
	if cmd != nil || m.discCat != "" {
		t.Error("an answer that came after leaving the list was used")
	}
}

func TestLateListOfAnotherCategoryIsDropped(t *testing.T) {
	installDisc(t)
	m := discListModel(t)
	m.discCat, m.discCatName = "C2", "Ideas"
	late := screenMsg{view: viewDiscussions, id: m.curTarget().String() + "|C1", data: ghcli.DiscussionList{Enabled: true, Rows: []ghcli.DiscussionRow{{Number: 7, Title: "stale"}}}}
	next, _ := m.Update(late)
	if strings.Contains(screenText(next.(model)), "stale") {
		t.Error("a list of the category we left was shown")
	}
}

func TestSwitchingRepoForgetsTheCategory(t *testing.T) {
	installDisc(t)
	m := discListModel(t)
	m.discCat, m.discCatName = "C2", "Ideas"
	m.switchRepo(ghcli.RepoRef{Owner: "other", Name: "repo"})
	if m.discCat != "" || m.discCatName != "" {
		t.Errorf("category %q kept for another repo", m.discCat)
	}
	if m.view != viewDiscussions {
		t.Errorf("view %v: the list should stay on the list of the new repo", m.view)
	}
}

func TestDiscussionListPaneShowsTheBodyOfTheSelectedRow(t *testing.T) {
	installDisc(t)
	m := discListModel(t)
	lines, ok := m.discPreviewLines(60)
	txt := plainText(lines)
	if !ok || !strings.Contains(txt, "#42 How do I retry?") || !strings.Contains(txt, "Please help with retries.") || !strings.Contains(txt, "✓ answered") {
		t.Fatalf("pane:\n%s", txt)
	}
	m = keyAll(m, "j")
	lines, _ = m.discPreviewLines(60)
	if txt = plainText(lines); !strings.Contains(txt, "#41 Dark theme") || !strings.Contains(txt, "closed") || !strings.Contains(txt, "locked") {
		t.Errorf("pane after j:\n%s", txt)
	}
}

func TestEnterOpensTheDiscussionAndShowsEverything(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	if m.view != viewDiscussion || len(f.opened) != 1 || f.opened[0] != "42" {
		t.Fatalf("view %v opened %v", m.view, f.opened)
	}
	s := screenText(m)
	for _, want := range []string{"#42  How do I retry?", "ann · Q&A · ✓ answered", "Please help.", "Comments (2)", "bob commented · ", "Use a loop.", "cy replied", "Thanks!", "ann replied", "dee commented", "Or a library."} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "✓ answer") < 2 { // once in the header line, once on the comment
		t.Errorf("the answer is not marked on its comment:\n%s", s)
	}
	if strings.Index(s, "cy replied") < strings.Index(s, "bob commented") || strings.Index(s, "cy replied") > strings.Index(s, "dee commented") {
		t.Errorf("a reply is not under its comment:\n%s", s)
	}
	if !strings.Contains(s, "discussion=#42") {
		t.Errorf("title:\n%s", s)
	}
}

func TestMoreThanWasReadIsSaid(t *testing.T) {
	f := installDisc(t)
	f.detail.CommentTotal = 130
	f.detail.Comments[0].ReplyTotal = 70
	s := plainText(discLines(f.detail, 80).lines)
	if !strings.Contains(s, "… 68 more replies not shown") || !strings.Contains(s, "… 128 more comments not shown") {
		t.Errorf("%s", s)
	}
}

func TestNoCommentsIsSaid(t *testing.T) {
	f := installDisc(t)
	f.detail.Comments, f.detail.CommentTotal = nil, 0
	if s := plainText(discLines(f.detail, 80).lines); !strings.Contains(s, "No comments yet") {
		t.Errorf("%s", s)
	}
}

func TestNAndNSelectCommentsAndRepliesInOrder(t *testing.T) {
	installDisc(t)
	m := discOpen(t)
	var seen []string
	for i := 0; i < 5; i++ {
		m = keyAll(m, "n")
		st, _ := m.screenData(viewDiscussion).(discState)
		seen = append(seen, st.sel)
	}
	if fmt.Sprint(seen) != "[K1 R1 R2 K2 K2]" {
		t.Errorf("n selected %v", seen)
	}
	var back []string
	for i := 0; i < 5; i++ {
		m = keyAll(m, "N")
		st, _ := m.screenData(viewDiscussion).(discState)
		back = append(back, st.sel)
	}
	if fmt.Sprint(back) != "[R2 R1 K1 K1 K1]" {
		t.Errorf("N selected %v", back)
	}
	if !strings.Contains(screenText(m), "▌") {
		t.Error("the selected comment has no mark in the margin")
	}
}

func TestSelectionScrollsIntoViewAndSurvivesARefresh(t *testing.T) {
	f := installDisc(t)
	var cs []ghcli.DiscComment
	for i := 0; i < 30; i++ {
		cs = append(cs, ghcli.DiscComment{ID: fmt.Sprintf("K%d", i), Author: "bob", Body: "line one\n\nline two", CreatedAt: "2026-10-23T11:00:00Z"})
	}
	f.detail.Comments, f.detail.CommentTotal = cs, 30
	m := discOpen(t)
	for i := 0; i < 25; i++ {
		m = keyAll(m, "n")
	}
	st, _ := m.screenData(viewDiscussion).(discState)
	doc := discLines(st.d, m.contentWidth())
	e, _, _ := doc.entry(st.sel)
	if st.sel != "K24" || e.start < st.scroll || e.end > st.scroll+m.logHeight() {
		t.Errorf("K24 is at lines %d-%d but the screen shows %d-%d", e.start, e.end, st.scroll, st.scroll+m.logHeight())
	}
	m = pump(m, m.fetch()) // a refresh
	if st2, _ := m.screenData(viewDiscussion).(discState); st2.sel != "K24" || st2.scroll != st.scroll {
		t.Errorf("after a refresh: sel %q scroll %d (was %d)", st2.sel, st2.scroll, st.scroll)
	}
}

func TestCommentAsksWithTheRepoAndNumberThenSends(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	m = keyAll(m, "C")
	if m.dlg == nil || m.dlg.id != "disc.comment" || !strings.Contains(m.dlg.title, "acme/api#42") {
		t.Fatalf("dialog %+v", m.dlg)
	}
	if len(f.comments) != 0 {
		t.Fatal("sent before the answer")
	}
	m, cmd := answer(m, "disc.comment", true, "Thanks all")
	m = runCmd(m, cmd)
	if len(f.comments) != 1 || f.comments[0].discussionID != "D1" || f.comments[0].body != "Thanks all" || f.comments[0].reply != "" || f.comments[0].repo != acme {
		t.Fatalf("sent %+v", f.comments)
	}
	if !strings.Contains(ansi.Strip(m.status), "commented on acme/api#42") || len(f.opened) < 2 {
		t.Errorf("status %q, reads %v (the discussion should be read again)", m.status, f.opened)
	}
}

func TestCancellingTheCommentSendsNothing(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	m = keyAll(m, "C")
	m, cmd := answer(m, "disc.comment", false, "never")
	runCmd(m, cmd)
	if len(f.comments) != 0 {
		t.Errorf("sent %+v", f.comments)
	}
}

func TestReplyGoesToTheThreadOfTheSelectedComment(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	if m = keyAll(m, "r"); m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "select a comment first") {
		t.Fatalf("a reply without a selection: dlg %v status %q", m.dlg, m.status)
	}
	m = keyAll(m, "n", "r") // K1
	if m.dlg == nil || !strings.Contains(m.dlg.title, "Reply to bob in acme/api#42") {
		t.Fatalf("dialog %+v", m.dlg)
	}
	m, cmd := answer(m, "disc.comment", true, "Good point")
	m = runCmd(m, cmd)
	if len(f.comments) != 1 || f.comments[0].reply != "K1" || f.comments[0].discussionID != "D1" {
		t.Fatalf("sent %+v", f.comments)
	}
	if !strings.Contains(ansi.Strip(m.status), "replied in acme/api#42") {
		t.Errorf("status %q", m.status)
	}
	// a reply to a reply goes to the same top-level comment
	m = keyAll(m, "n", "r") // R1 (K1 stays selected through the refresh, n moves on)
	st, _ := m.screenData(viewDiscussion).(discState)
	if st.sel != "R1" {
		t.Fatalf("selected %q", st.sel)
	}
	m, cmd = answer(m, "disc.comment", true, "And you")
	runCmd(m, cmd)
	if len(f.comments) != 2 || f.comments[1].reply != "K1" {
		t.Errorf("a reply to a reply went to %q, want the thread K1", f.comments[len(f.comments)-1].reply)
	}
}

func TestReplyKeepsTheThreadTakenAtKeyPress(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	m = keyAll(m, "n", "r") // K1
	// the screen is read again meanwhile and the selection is gone
	m = m.withScreenData(viewDiscussion, discState{d: f.detail})
	m, cmd := answer(m, "disc.comment", true, "late")
	runCmd(m, cmd)
	if len(f.comments) != 1 || f.comments[0].reply != "K1" {
		t.Errorf("sent %+v", f.comments)
	}
}

func TestLockedAndLoadingDiscussionsRefuseToComment(t *testing.T) {
	f := installDisc(t)
	f.detail.Locked = true
	m := discOpen(t)
	for _, key := range []string{"C", "r"} {
		m = keyAll(m, "n", key)
		if m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "locked") {
			t.Errorf("%s on a locked discussion: dlg %v status %q", key, m.dlg, m.status)
		}
	}
	m = model{view: viewDiscussion, width: 130, height: 30, target: acme, disc: "42"}
	if m = keyAll(m, "C"); m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "still loading") {
		t.Errorf("while loading: dlg %v status %q", m.dlg, m.status)
	}
}

func TestFailedCommentIsShownAndNotRetried(t *testing.T) {
	f := installDisc(t)
	f.err = errors.New("HTTP 403: not allowed")
	m := discOpen(t)
	m = keyAll(m, "C")
	m, cmd := answer(m, "disc.comment", true, "hello")
	m = runCmd(m, cmd)
	if len(f.comments) != 1 || !strings.Contains(ansi.Strip(m.status), "not allowed") {
		t.Errorf("sent %d, status %q", len(f.comments), m.status)
	}
}

func TestAnswerWithoutAPendingCommentSendsNothing(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	m, cmd := answer(m, "disc.comment", true, "orphan")
	runCmd(m, cmd)
	if len(f.comments) != 0 {
		t.Errorf("sent %+v", f.comments)
	}
}

func TestOOpensTheDiscussionInTheBrowser(t *testing.T) {
	installDisc(t)
	var opened string
	old := openURL
	openURL = func(u string) error { opened = u; return nil }
	t.Cleanup(func() { openURL = old })
	m := discOpen(t)
	keyAll(m, "o")
	if opened != "https://github.com/acme/api/discussions/42" {
		t.Errorf("opened %q", opened)
	}
}

func TestPHidesAndShowsThePaneOnTheDiscussionList(t *testing.T) {
	installDisc(t)
	m := discListModel(t)
	m.previewOff = false
	if m = keyAll(m, "p"); !m.previewOff {
		t.Error("p did not hide the pane")
	}
	if m = keyAll(m, "p"); m.previewOff {
		t.Error("p did not bring the pane back")
	}
}

func TestOnlyOneCommentIsSentAtATime(t *testing.T) {
	f := installDisc(t)
	m := discOpen(t)
	m = keyAll(m, "n", "C")
	m, cmd := answer(m, "disc.comment", true, "first")
	if !m.discBusy {
		t.Fatal("a comment is on its way but nothing says so")
	}
	for _, key := range []string{"C", "r"} {
		if m = keyAll(m, key); m.dlg != nil || !strings.Contains(ansi.Strip(m.status), "still being sent") {
			t.Errorf("%s while one is on its way: dlg %v status %q", key, m.dlg, m.status)
		}
	}
	m = runCmd(m, cmd)
	if m.discBusy || len(f.comments) != 1 {
		t.Fatalf("busy %v after the answer, %d sent", m.discBusy, len(f.comments))
	}
	if m = keyAll(m, "C"); m.dlg == nil {
		t.Error("C is still refused after the comment went out")
	}
}

func TestFailedCommentFreesTheNextOne(t *testing.T) {
	f := installDisc(t)
	f.err = errors.New("HTTP 500")
	m := discOpen(t)
	m = keyAll(m, "C")
	m, cmd := answer(m, "disc.comment", true, "x")
	m = runCmd(m, cmd)
	if m.discBusy {
		t.Error("still busy after a failure")
	}
}
