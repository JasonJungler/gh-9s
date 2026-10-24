package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// Outside calls the PR actions make; tests replace them.
var (
	commentPR = ghcli.CommentPR
	reviewPR  = ghcli.ReviewPR
)

// prAction is a PR action being put together through dialogs. It remembers the repo and
// number it was started for, so the answer always goes to the PR the user was looking at.
type prAction struct {
	target  ghcli.RepoRef
	number  string
	label   string // "acme/api#482", shown in every question
	kind    ghcli.ReviewKind
	body    string
	want    string   // for a list being fetched: "labels" or "assignees"
	current []string // labels or assignees the issue has now, for diffing a multi-choice answer
}

// prActionDoneMsg reports how an action ended.
type prActionDoneMsg struct {
	text string // what was done, for the status line
	err  error
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
	return func() tea.Msg {
		err := reviewPR(p.target, p.number, p.kind, p.body)
		return prActionDoneMsg{text: reviewVerb(p.kind) + " " + p.label, err: err}
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
			return m.withDialog(newConfirmDialog("pr.review.confirm", "Approve "+p.label+"?", "Submit an approving review.")), nil
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
			return m.withDialog(newConfirmDialog("pr.review.confirm", "Request changes on "+p.label+"?", p.body)), nil
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
		return m, submitReview(*p)
	}
}
