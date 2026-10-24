package tui

import (
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles one message: data arriving, ticks, and key presses.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case infoMsg:
		m.repo, m.branch, m.curRepo = msg.repo, msg.branch, msg.cur
	case triggeredMsg:
		switch {
		case msg.runID != "":
			m.runID, m.target = msg.runID, msg.target
			m.graph, m.gsel, m.top, m.expanded, m.runStatus = graph.Graph{}, 0, 0, nil, ""
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
	case screenMsg:
		if msg.view != m.view {
			return m, nil // a late result for a view we have left
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
	case runMetaMsg:
		if msg.err == nil && msg.runID == m.runID && msg.meta.Status != "" {
			return m.observeRun(msg.meta)
		}
		return m, nil
	case dataMsg:
		if msg.view != m.view {
			return m, nil
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
		} else if msg.err == nil {
			m.tbl = msg.tbl
			if n := len(m.visible()); m.cursor >= n {
				m.cursor = max(0, n-1)
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
		return m, m.switchRepo(msg.ref)
	case gotoMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
			return m, nil
		}
		m.pr = msg.pr
		return m, m.push(msg.view)
	case tea.KeyMsg:
		return m.handleKey(msg)
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
	m.runStatus = meta.Status
	return m, cmd
}
