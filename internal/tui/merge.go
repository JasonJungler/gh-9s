package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// Outside calls the merge flow makes; tests replace them.
var (
	allowedMethods = ghcli.AllowedMergeMethods
	mergePR        = ghcli.MergePR
)

// mergeAction is a merge being put together through dialogs. It remembers the repo and PR it
// was started for.
type mergeAction struct {
	target  ghcli.RepoRef // always the explicit repo (never empty when it is known), so gh cannot touch a local checkout
	number  string
	label   string // "acme/api#482"
	branch  string
	base    string // the branch it merges into
	head    string // the commit that was read; the merge is refused if the PR has moved on
	method  ghcli.MergeMethod
	verdict ghcli.MergeVerdict
}

// mergeInfoMsg is what m collects before asking anything: a fresh read of the PR and the
// merge methods the repo allows.
type mergeInfoMsg struct {
	id      string // the PR the options were fetched for
	d       ghcli.PRDetail
	methods []ghcli.MergeMethod
	err     error
}

var methodNames = map[ghcli.MergeMethod]string{
	ghcli.MergeCommit: "Create a merge commit",
	ghcli.MergeSquash: "Squash and merge",
	ghcli.MergeRebase: "Rebase and merge",
}

// startMerge reads the PR again (what is on screen may be old) and asks the repo which merge
// methods it allows; mergeInfoMsg then opens the first dialog.
func (m model) startMerge() (model, tea.Cmd) {
	tg, number, id := m.target, m.pr, m.itemID(m.pr)
	m.pendingMerge = &mergeAction{target: m.curTarget(), number: number, label: m.prLabel()}
	m.status = styDim.Render("checking merge options for " + m.pendingMerge.label + "…")
	return m, func() tea.Msg {
		d, err := fetchPR(tg, number)
		if err != nil {
			return mergeInfoMsg{id: id, err: err}
		}
		methods, err := allowedMethods(tg)
		return mergeInfoMsg{id: id, d: d, methods: methods, err: err}
	}
}

// mergeInfo opens the method dialog for a PR whose state has just been read.
func (m model) mergeInfo(msg mergeInfoMsg) model {
	p := m.pendingMerge
	if p == nil || m.view != viewPRDetail || msg.id != m.itemID(m.pr) || m.dlg != nil {
		return m // nothing is waiting for this, or the user has moved on or is busy with a dialog
	}
	if msg.err != nil {
		m.pendingMerge = nil
		m.status = styErr.Render("✗ " + msg.err.Error())
		return m
	}
	old, _ := m.screenData(viewPRDetail).(prDetailState)
	m = m.withScreenData(viewPRDetail, prDetailState{d: msg.d, scroll: old.scroll}) // show what we decided on
	if len(msg.methods) == 0 {
		m.pendingMerge = nil
		m.status = styErr.Render("✗ no merge method is allowed for " + displayRef(m.curTarget()) + " (or its settings are not visible to you)")
		return m
	}
	p.verdict, p.branch, p.base, p.head = ghcli.EvaluateMerge(msg.d), msg.d.HeadRef, msg.d.BaseRef, msg.d.HeadSHA
	reasons := strings.Join(p.verdict.Reasons, "; ")
	var prompt string
	switch p.verdict.Level {
	case ghcli.MergeOK:
		prompt = "GitHub reports no problems."
	case ghcli.MergeWarn:
		prompt = "⚠ " + reasons + "."
	default:
		prompt = "✗ Cannot merge: " + reasons + "."
	}
	prompt += fmt.Sprintf("   Checks ✓%d ✗%d ●%d   Review %s", msg.d.ChecksPassed, msg.d.ChecksFailed, msg.d.ChecksPending, orDash(msg.d.ReviewDecision))
	var choices []choice
	for _, method := range msg.methods {
		c := choice{label: methodNames[method], value: string(method)}
		if p.verdict.Level == ghcli.MergeBlocked {
			c.disabled, c.note = true, reasons
		}
		choices = append(choices, c)
	}
	d := newChoiceDialog("pr.merge.method", "Merge "+p.label, choices, false, nil)
	d.prompt = prompt
	if p.verdict.Level == ghcli.MergeWarn {
		d.needBang, d.bangHint = true, "GitHub would accept this, but: "+reasons+". Press ! to merge anyway."
	}
	return m.withDialog(d)
}

func init() {
	dialogHandlers["pr.merge.method"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pendingMerge
		if !r.ok || p == nil || len(r.values) != 1 {
			m.pendingMerge = nil
			return m, nil
		}
		p.method = ghcli.MergeMethod(r.values[0])
		d := newConfirmDialog("pr.merge.confirm", "Merge "+p.label+"?",
			fmt.Sprintf("%s into %s. Head branch: %s.", methodNames[p.method], orDash(p.base), p.branch))
		d.toggleLabel, d.toggleKey = "Delete branch "+p.branch+" after merge", "d"
		d.toggleOn = loadPrefs(m.prefsFile).DeleteBranchAfterMerge
		return m.withDialog(d), nil
	}
	dialogHandlers["pr.merge.confirm"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pendingMerge
		m.pendingMerge = nil
		if !r.ok || p == nil {
			return m, nil
		}
		if m.prefsFile != "" {
			_ = prefs{DeleteBranchAfterMerge: r.toggled}.save(m.prefsFile) // best effort
		}
		return m, func() tea.Msg {
			err := mergePR(p.target, p.number, p.method, r.toggled, p.head)
			return prActionDoneMsg{text: fmt.Sprintf("merged %s (%s)", p.label, p.method), err: err}
		}
	}
}
