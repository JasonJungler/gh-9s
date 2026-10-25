package tui

import (
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles one message: data arriving, ticks, and key presses. Afterwards the preview pane
// follows the selection.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if nm, ok := next.(model); ok {
		var pc tea.Cmd
		nm, pc = nm.previewSync()
		next, cmd = nm, tea.Batch(cmd, pc)
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.wheel(msg)
	case previewTickMsg:
		return m.previewTick(msg)
	case previewLoadedMsg:
		return m.previewLoaded(msg), nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.view == viewDiff {
			m = m.syncDiffLayout().keepCursorVisible()
		}
	case imageDoneMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ image viewer: " + msg.err.Error())
		}
		return m, nil
	case infoMsg:
		m.repo, m.branch, m.curRepo = msg.repo, msg.branch, msg.cur
		if m.startPending && msg.cur == (ghcli.RepoRef{}) {
			cmd := m.switchTo(viewInbox)
			m.note = "not in a repo with a GitHub remote: showing the inbox. :repo OWNER/REPO picks a repo"
			return m, cmd
		}
		m.startPending = false
	case triggeredMsg:
		switch {
		case msg.runID != "":
			m.runID, m.target = msg.runID, msg.target
			m.graph, m.gsel, m.top, m.expanded, m.runStatus, m.runMeta = graph.Graph{}, 0, 0, nil, "", ghcli.RunMeta{}
			m.status = styOK.Render("✓ triggered " + msg.workflow + " → run " + msg.runID)
			return m, m.push(viewGraph)
		case msg.dispatched:
			// show that workflow's runs so the new one appears as soon as GitHub lists it
			m.status = styErr.Render("triggered " + msg.workflow + ", but its run was not found yet: " + msg.err.Error())
			m.target, m.wfID, m.wfName = msg.target, msg.workflow, msg.workflow
			return m, m.push(viewRuns)
		default:
			m.status = styErr.Render("✗ " + msg.err.Error())
		}
		return m, nil
	case formReadyMsg:
		switch {
		case msg.err != nil:
			m.status = styErr.Render("✗ " + msg.err.Error())
		case !msg.dispatchable:
			m.status = styErr.Render(msg.name + " has no workflow_dispatch trigger")
		default:
			f := msg.form
			m.form = &f
		}
		return m, nil
	case inputsMsg:
		if m.form != nil && m.form.ref == msg.ref {
			f := *m.form
			switch {
			case msg.err != nil:
				f.err = "could not read inputs at " + msg.ref + ": " + msg.err.Error()
			case !msg.dispatchable:
				f.err = "no workflow_dispatch trigger at " + msg.ref
			default:
				f = f.withInputs(msg.inputs)
				f.err = ""
			}
			m.form = &f
		}
		return m, nil
	case choicesMsg:
		return m.choicesLoaded(msg), nil
	case launchIssueMsg:
		return m.launchIssueLoaded(msg)
	case launchModelsMsg:
		return m.launchModelsLoaded(msg), nil
	case launchBaseMsg:
		return m.launchBaseLoaded(msg), nil
	case launchPromptMsg:
		return m.launchPromptEdited(msg), nil
	case launchDoneMsg:
		return m.launchDone(msg), nil
	case mergeInfoMsg:
		return m.mergeInfo(msg), nil
	case runActDoneMsg:
		return m.runActDone(msg)
	case prActionDoneMsg:
		m.sendingKey, m.discBusy = "", false
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
			return m, nil
		}
		m.status = styOK.Render("✓ " + msg.text)
		if msg.sentDrafts != "" {
			m = m.clearSentDrafts(msg.sentDrafts, msg.sent)
		}
		return m, m.fetch() // the PR changed: reload what is on screen
	case dialogResultMsg:
		if h, ok := dialogHandlers[msg.id]; ok {
			return h(m, msg)
		}
		return m, nil
	case wikiOpenedMsg:
		return m.wikiOpened(msg)
	case wikiDoneMsg:
		return m.wikiDone(msg)
	case editedMsg:
		if m.wikiEdit != nil && m.dlg == nil { // the editor a wiki page was opened in
			return m.wikiEdited(msg)
		}
		if m.dlg != nil && m.dlg.kind == dialogText {
			d := *m.dlg
			if msg.err != nil {
				d.err = msg.err.Error()
			} else {
				d.text, d.err = strings.TrimRight(msg.text, "\r\n"), ""
			}
			m.dlg = &d
		}
		return m, nil
	case pickerLoadedMsg, pickerDebounceMsg, pickerResultMsg, pickerHostsMsg:
		return m.pickerMsg(msg)
	case screenMsg:
		if msg.view != m.view {
			return m, nil // a late result for a view we have left
		}
		if s := screens[msg.view]; s.identity != nil && msg.id != s.identity(m) {
			return m, nil // a late result for a PR, issue, repo or filter we have left
		}
		m.loading, m.err = false, msg.err
		if s := screens[msg.view]; msg.err == nil && s.apply != nil {
			m = s.apply(m, msg)
		}
		return m, nil
	case tickMsg:
		cmds := []tea.Cmd{tick()}
		if m.shouldPoll() {
			cmds = append(cmds, m.fetch())
		}
		if m.view == viewLogs && m.runStatus != "" && m.runStatus != "completed" {
			cmds = append(cmds, runMetaCmd(m.target, m.runID)) // keep watching the run while reading a log
		}
		return m, tea.Batch(cmds...)
	case prChecksMsg:
		return m.prChecksArrived(msg), nil
	case runMetaMsg:
		if msg.err == nil && msg.runID == m.runID && msg.meta.Status != "" {
			return m.observeRun(msg.meta)
		}
		return m, nil
	case dataMsg:
		if msg.view != m.view {
			return m, nil
		}
		if msg.view == viewFavs && msg.tab != m.favTab {
			return m, nil // the tab was changed while this was loading
		}
		if (msg.view == viewGraph || msg.view == viewJobs || msg.view == viewLogs) && msg.runID != "" && msg.runID != m.runID {
			return m, nil // a late answer about a run we have left
		}
		m.loading = false
		m.err = msg.err
		if msg.latest != nil {
			m.latest = msg.latest
		}
		if len(msg.specs) > 0 {
			// copy-on-write: in-flight fetch goroutines hold the old map
			merged := make(map[string][]workflow.JobSpec, len(m.specs)+len(msg.specs))
			for k, v := range m.specs {
				merged[k] = v
			}
			for k, v := range msg.specs {
				merged[k] = v
			}
			m.specs = merged
		}
		var notifyCmd tea.Cmd
		if msg.err == nil && msg.run.Status != "" && (msg.view == viewGraph || msg.view == viewJobs) {
			m, notifyCmd = m.observeRun(msg.run)
		}
		if msg.err == nil && msg.graph != nil {
			if msg.graph.Flat {
				// dependencies unknown: show the plain jobs list instead of guessing a graph
				m.view, m.tbl, m.cursor, m.top = viewJobs, jobsTable(msg.jobs), 0, 0
				m.note = "graph unavailable: " + msg.graph.Reason
				return m, notifyCmd
			}
			keep := m.selectedNodeID()
			m.graph = *msg.graph
			m.reselect(keep)
			return m, notifyCmd
		}
		if msg.err == nil && m.view == viewLogs {
			m.logs = msg.logs
			if m.query != "" {
				m.matches = searchMatches(m.logs, m.query)
				m.mi = min(m.mi, max(0, len(m.matches)-1))
			}
			if m.follow {
				m.top = m.maxTop()
			}
			if m.logJump { // opened from "first failure": start there, once
				m.logJump = false
				m.top = min(max(firstFailureLine(m.logs)-3, 0), m.maxTop())
			}
		} else if msg.err == nil {
			m.tbl = msg.tbl
			if n := len(m.visible()); m.cursor >= n {
				m.cursor = max(0, n-1)
			}
			if m.view == viewPRs {
				var checksCmd tea.Cmd
				m, checksCmd = m.prChecksAfterList()
				return m, tea.Batch(notifyCmd, checksCmd)
			}
		}
		return m, notifyCmd
	case actionMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
		} else {
			m.status = styOK.Render("✓ " + msg.msg)
		}
		return m, m.fetch()
	case repoSwitchedMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.ref.String() + ": " + msg.err.Error())
			return m, nil
		}
		if msg.ref != (ghcli.RepoRef{}) && m.recents != nil {
			m.recents.Add(msg.ref)
			_ = m.recents.Save() // best effort: a failed save only loses the shortcut
		}
		return m, m.switchRepo(msg.ref)
	case repoPinMsg:
		return m.onRepoPin(msg)
	case projectListMsg:
		return m.onProjectList(msg)
	case projLoadedMsg:
		return m.onProjLoaded(msg)
	case projDoneMsg:
		return m.onProjDone(msg)
	case boardWriteDoneMsg:
		return m.onBoardWriteDone(msg)
	case boardAssigneesMsg:
		return m.onBoardAssignees(msg)
	case boardResolvedMsg:
		return m.onBoardResolved(msg)
	case projectPinMsg:
		return m.onProjectPin(msg)
	case gotoMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
			return m, nil
		}
		m.pr, m.prTarget = msg.pr, m.target
		return m, m.push(msg.view)
	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		if nm, ok := next.(model); ok && nm.view == viewDiff {
			next = nm.keepCursorVisible()
		}
		if nm, ok := next.(model); ok && nm.view == viewInbox {
			next = nm.rememberInboxCursor()
		}
		return next, cmd
	}
	return m, nil
}

// observeRun records the watched run's status and notifies once when it finishes.
func (m model) observeRun(meta ghcli.RunMeta) (model, tea.Cmd) {
	var cmd tea.Cmd
	if runFinished(m.runStatus, meta.Status) {
		text := "gh-9s: run " + m.runID + " " + runOutcome(meta.Conclusion)
		cmd = func() tea.Msg { notifier(text, text); return nil }
	}
	if m.rerunWait > 0 { // just re-run: GitHub may still report the old attempt as completed
		if meta.Status == "completed" {
			m.rerunWait--
			if m.rerunWait > 0 {
				return m, nil
			}
		} else {
			m.rerunWait = 0
		}
	}
	m.runStatus, m.runMeta = meta.Status, meta
	return m, cmd
}
