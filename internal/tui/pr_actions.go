package tui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// Outside calls the PR actions make; tests replace them.
var (
	commentPR = ghcli.CommentPR
	reviewPR  = ghcli.ReviewPR
)

// prAction is a PR action being put together through dialogs. It remembers the repo and
// number it was started for, so the answer always goes to the PR the user was looking at.
// Outside calls of the inline review; tests replace them.
var (
	submitReviewInline = ghcli.SubmitReview
)

type prAction struct {
	drafts    []drafts.Draft // inline comments that go with the review (the diff screen)
	head      string         // the commit the diff was read at; the review is pinned to it
	draftsKey string         // which PR's drafts these are, to clear them once sent
	target    ghcli.RepoRef
	number    string
	label     string // "acme/api#482", shown in every question
	kind      ghcli.ReviewKind
	body      string
	want      string   // for a list being fetched: "labels" or "assignees"
	current   []string // labels or assignees the issue has now, for diffing a multi-choice answer
}

// prActionDoneMsg reports how an action ended.
type prActionDoneMsg struct {
	text       string // what was done, for the status line
	err        error
	sentDrafts string         // the drafts-file key of a PR whose drafts went out with the review
	sent       []drafts.Draft // exactly the drafts that went out
}

func (m model) prLabel() string { return displayRef(m.curTarget()) + "#" + m.pr }

func (m model) startPRAction() (model, *prAction) {
	p := &prAction{target: m.target, number: m.pr, label: m.prLabel()}
	m.pending = p
	return m, p
}

func (m model) startPRComment() model {
	m, p := m.startPRAction()
	return m.withDialog(newTextDialog("pr.comment", "Comment on "+p.label, "Your comment"))
}

func (m model) startPRReview() model {
	m, p := m.startPRAction()
	return m.withDialog(newChoiceDialog("pr.review.kind", "Review "+p.label, []choice{
		{label: "Approve", value: string(ghcli.ReviewApprove)},
		{label: "Request changes", value: string(ghcli.ReviewRequestChanges)},
		{label: "Comment", value: string(ghcli.ReviewComment)},
	}, false, nil))
}

func submitReview(p prAction) tea.Cmd {
	if len(p.drafts) > 0 {
		return submitInline(p)
	}
	return func() tea.Msg {
		err := reviewPR(p.target, p.number, p.kind, p.body)
		return prActionDoneMsg{text: reviewVerb(p.kind) + " " + p.label, err: err}
	}
}

// submitInline sends the review with its inline comments, but only if the PR is still at the commit
// the reviewer was looking at: comments must never land on code they did not see.
func submitInline(p prAction) tea.Cmd {
	return func() tea.Msg {
		now, err := prHead(p.target, p.number)
		switch {
		case err != nil:
			return prActionDoneMsg{err: fmt.Errorf("couldn't check the PR's latest commit: %w", err)}
		case now != p.head:
			return prActionDoneMsg{err: errors.New("the PR has new commits since this diff was loaded: press R to reload (your drafts are kept)")}
		}
		cs := make([]ghcli.InlineComment, len(p.drafts))
		for i, d := range p.drafts {
			cs[i] = ghcli.InlineComment{Path: d.Path, Line: d.Line, Side: d.Side, StartLine: d.StartLine, Body: d.Body}
		}
		err = submitReviewInline(p.target, p.number, p.head, p.kind, p.body, cs)
		return prActionDoneMsg{text: reviewVerb(p.kind) + " " + p.label + fmt.Sprintf(" with %d inline comments", len(cs)), err: err, sentDrafts: p.draftsKey, sent: p.drafts}
	}
}

func reviewVerb(k ghcli.ReviewKind) string {
	switch k {
	case ghcli.ReviewApprove:
		return "approved"
	case ghcli.ReviewRequestChanges:
		return "requested changes on"
	}
	return "reviewed"
}

func init() {
	dialogHandlers["pr.comment"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		m.pending = nil
		if !r.ok || p == nil {
			return m, nil
		}
		return m, func() tea.Msg {
			return prActionDoneMsg{text: "commented on " + p.label, err: commentPR(p.target, p.number, r.text)}
		}
	}
	dialogHandlers["pr.review.kind"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		if !r.ok || p == nil || len(r.values) != 1 {
			m.pending = nil
			return m, nil
		}
		p.kind = ghcli.ReviewKind(r.values[0])
		if p.kind == ghcli.ReviewApprove {
			return m.withDialog(newConfirmDialog("pr.review.confirm", "Approve "+p.label+"?", reviewDetail(p, "Submit an approving review."))), nil
		}
		title := "Review comment on " + p.label
		if p.kind == ghcli.ReviewRequestChanges {
			title = "Request changes on " + p.label
		}
		return m.withDialog(newTextDialog("pr.review.body", title, "What should change?")), nil
	}
	dialogHandlers["pr.review.body"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		if !r.ok || p == nil {
			m.pending = nil
			return m, nil
		}
		p.body = r.text
		if p.kind == ghcli.ReviewRequestChanges { // blocks the PR: confirm first
			return m.withDialog(newConfirmDialog("pr.review.confirm", "Request changes on "+p.label+"?", reviewDetail(p, p.body))), nil
		}
		if len(p.drafts) > 0 { // inline comments always get a confirmation
			return m.withDialog(newConfirmDialog("pr.review.confirm", "Send your review of "+p.label+"?", reviewDetail(p, p.body))), nil
		}
		m.pending = nil
		return m, submitReview(*p)
	}
	dialogHandlers["pr.review.confirm"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		m.pending = nil
		if !r.ok || p == nil {
			return m, nil
		}
		if len(p.drafts) > 0 {
			m.sendingKey = p.draftsKey // no second review while this one is on its way
		}
		return m, submitReview(*p)
	}
}

// reviewDetail is the text under a review confirmation: what is said, and how many inline comments go along.
func reviewDetail(p *prAction, text string) string {
	if len(p.drafts) == 0 {
		return text
	}
	return fmt.Sprintf("%s\n\nSent together with %d inline comments.", text, len(p.drafts))
}
