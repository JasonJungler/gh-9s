package tui

import (
	"os/exec"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.form != nil {
		return m.formKey(k)
	}
	if m.mode == modeConfirm {
		mode := m.mode
		m.mode = modeNormal
		if mode == modeConfirm && (k.String() == "y" || k.String() == "Y") {
			return m, m.confirm()
		}
		m.status = styDim.Render("cancelled")
		return m, nil
	}
	if m.mode != modeNormal {
		return m.handleInput(k)
	}

	m.status = ""
	if m.view == viewLogs {
		if next, cmd, ok := m.logKey(k); ok {
			return next, cmd
		}
	}
	if m.view == viewGraph {
		if next, cmd, ok := m.graphKey(k); ok {
			return next, cmd
		}
	}
	n := len(m.visible())
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "backspace":
		if m.filter != "" {
			m.filter, m.cursor, m.top = "", 0, 0
		} else if len(m.back) > 0 {
			prev := m.back[len(m.back)-1]
			m.back = m.back[:len(m.back)-1]
			if prev != viewRuns {
				m.wfID, m.wfName = "", ""
			}
			if prev == viewFavs {
				m.target = ghcli.RepoRef{}
			}
			return m, m.switchTo(prev)
		}
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, n-1))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, n-1)
	case "ctrl+d", "pgdown":
		m.cursor = min(m.cursor+10, max(0, n-1))
	case "ctrl+u", "pgup":
		m.cursor = max(m.cursor-10, 0)
	case "/":
		m.mode, m.input = modeFilter, m.filter
	case ":":
		m.mode, m.input = modeCommand, ""
	case "1":
		return m.goTop(viewRuns)
	case "2":
		return m.goTop(viewWorkflows)
	case "3":
		return m.goTop(viewPRs)
	case "P":
		return m, func() tea.Msg {
			pr, err := currentPR()
			return gotoMsg{viewChecks, pr, err}
		}
	case "R", "ctrl+r":
		m.loading = true
		return m, m.fetch()
	case "o":
		if r, ok := m.selected(); ok && r.url != "" {
			_ = ghcli.OpenURL(r.url)
			m.status = styDim.Render("opened " + r.url)
		}
	case "enter":
		return m.enter()
	case "t":
		if r, ok := m.selected(); ok && (m.view == viewWorkflows || m.view == viewFavs) {
			return m.startTrigger(r)
		}
		m.status = styDim.Render("trigger works from the workflows (2) or favorites (:fav) view")
	case "w":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			c := exec.Command("gh", ghcli.Scoped(m.target, "run", "watch", r.id, "--exit-status")...)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return actionMsg{msg: "watch finished: run " + r.id}
			})
		}
	case "l":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, logsCmd(m.target, r.id, false)
		}
	case "L":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, logsCmd(m.target, r.id, true)
		}
	case "r":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, runAction("rerun run "+r.id, ghcli.Scoped(m.target, "run", "rerun", r.id)...)
		}
	case "F":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, runAction("rerun failed jobs of "+r.id, ghcli.Scoped(m.target, "run", "rerun", r.id, "--failed")...)
		}
	case "c":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			m.mode, m.confirmMsg = modeConfirm, "cancel run "+r.id+"? (y/N)"
			m.confirm = func() tea.Cmd {
				return runAction("cancelled run "+r.id, ghcli.Scoped(m.target, "run", "cancel", r.id)...)
			}
		}
	case "d":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			m.mode, m.confirmMsg = modeConfirm, "delete run "+r.id+"? (y/N)"
			m.confirm = func() tea.Cmd {
				return runAction("deleted run "+r.id, ghcli.Scoped(m.target, "run", "delete", r.id)...)
			}
		}
	case "f":
		if r, ok := m.selected(); ok && (m.view == viewWorkflows || m.view == viewFavs) {
			return m.toggleFavorite(r)
		}
	case "a":
		if r, ok := m.selected(); ok && (m.view == viewWorkflows || m.view == viewFavs) {
			return m.startAlias(r)
		}
	case "e":
		if r, ok := m.selected(); ok && m.view == viewWorkflows {
			sub := "disable"
			if r.cells[2] != "active" {
				sub = "enable"
			}
			return m, runAction(sub+"d workflow "+r.extra, ghcli.Scoped(m.target, "workflow", sub, r.id)...)
		}
	}
	m.scroll()
	return m, nil
}

func (m model) goTop(v viewKind) (tea.Model, tea.Cmd) {
	m.back, m.wfID, m.wfName, m.pr, m.target = nil, "", "", "", ghcli.RepoRef{}
	return m, m.switchTo(v)
}

func (m model) enter() (tea.Model, tea.Cmd) {
	r, ok := m.selected()
	if !ok {
		return m, nil
	}
	switch m.view {
	case viewWorkflows:
		m.wfID, m.wfName = r.id, r.extra
		return m, m.push(viewRuns)
	case viewRuns:
		m.runID = r.id
		m.graph, m.gsel, m.top, m.expanded, m.runStatus = graph.Graph{}, 0, 0, nil, ""
		return m, m.push(viewGraph)
	case viewJobs:
		m.jobID, m.jobName, m.jobURL = r.id, r.extra, r.url
		return m, m.push(viewLogs)
	case viewFavs:
		if f, ok := m.favForRow(r); ok {
			return m, m.openFavorite(f, true)
		}
	case viewPRs:
		m.pr = r.id
		return m, m.push(viewChecks)
	case viewChecks:
		if r.url != "" {
			_ = ghcli.OpenURL(r.url)
		}
	}
	return m, nil
}

func (m model) handleInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		if m.mode == modeFilter {
			m.filter = ""
		}
		if m.mode == modeSearch {
			m.query, m.matches = "", nil
		}
		m.mode, m.input = modeNormal, ""
		return m, nil
	case tea.KeyEnter:
		mode, in := m.mode, strings.TrimSpace(m.input)
		m.mode, m.input = modeNormal, ""
		switch mode {
		case modeFilter:
			m.filter, m.cursor, m.top = in, 0, 0
		case modeCommand:
			return m.command(in)
		case modeSearch:
			m.query, m.matches, m.mi = in, searchMatches(m.logs, in), 0
			if len(m.matches) == 0 {
				m.status = styDim.Render("no match: " + in)
				return m, nil
			}
			for i, l := range m.matches {
				if l >= m.top {
					m.mi = i
					break
				}
			}
			m.gotoMatch(m.mi)
		case modeAlias:
			if err := m.favs.SetAlias(m.aliasFor, in); err != nil {
				m.status = styErr.Render("✗ " + err.Error())
			} else if err := m.favs.Save(); err != nil {
				m.status = styErr.Render("✗ " + err.Error())
			} else {
				m.status = styOK.Render("✓ alias set: " + in)
			}
			return m, m.fetch()
		}
		return m, nil
	case tea.KeyBackspace:
		if _, sz := utf8.DecodeLastRuneInString(m.input); sz > 0 {
			m.input = m.input[:len(m.input)-sz]
		}
	case tea.KeySpace:
		m.input += " "
	case tea.KeyRunes:
		m.input += string(k.Runes)
	}
	if m.mode == modeFilter {
		m.filter, m.cursor, m.top = m.input, 0, 0
	}
	if m.mode == modeSearch {
		m.query, m.matches, m.mi = m.input, searchMatches(m.logs, m.input), 0
	}
	return m, nil
}

func (m model) command(c string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(c) {
	case "runs", "run", "r":
		return m.goTop(viewRuns)
	case "workflows", "workflow", "wf", "w":
		return m.goTop(viewWorkflows)
	case "pulls", "pull", "prs", "pr":
		return m.goTop(viewPRs)
	case "fav", "favs", "favorites":
		return m.goTop(viewFavs)
	case "cpr", "checks":
		return m, func() tea.Msg {
			pr, err := currentPR()
			return gotoMsg{viewChecks, pr, err}
		}
	case "q", "quit", "exit":
		return m, tea.Quit
	case "":
	default:
		if m.favs != nil {
			if f, ok := m.favs.ByAlias(c); ok {
				return m, m.openFavorite(f, false)
			}
		}
		m.status = styErr.Render("unknown command: " + c)
	}
	return m, nil
}

// graphKey handles the keys of the graph view; ok=false falls through to the
// shared handler (quit, back, refresh, view switching).
func (m model) graphKey(k tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	g := m.shown()
	switch s := k.String(); s {
	case "h", "left", "l", "right", "j", "down", "k", "up":
		dir := map[string]rune{"h": 'h', "left": 'h', "l": 'l', "right": 'l', "j": 'j', "down": 'j', "k": 'k', "up": 'k'}[s]
		m.gsel = graph.MoveSel(g, m.gsel, dir)
	case "v":
		m.gstyle = m.gstyle.Next()
	case "x":
		if m.gsel < len(g.Nodes) && g.Nodes[m.gsel].Group != "" {
			id := g.Nodes[m.gsel].Group
			m.expanded = withKey(m.expanded, id, false)
			for i, n := range m.shown().Nodes {
				if n.ID == id {
					m.gsel = i
				}
			}
		}
	case "enter":
		if m.gsel >= len(g.Nodes) {
			break
		}
		n := g.Nodes[m.gsel]
		if len(n.Members) > 0 {
			m.expanded = withKey(m.expanded, n.ID, true)
			for i, e := range m.shown().Nodes {
				if e.Group == n.ID {
					m.gsel = i
					break
				}
			}
			break
		}
		m.jobID, m.jobName, m.jobURL = n.Job.ID, n.Job.Name, n.Job.URL
		return m, m.push(viewLogs), true
	case "o":
		if m.gsel < len(g.Nodes) && g.Nodes[m.gsel].Job.URL != "" {
			_ = ghcli.OpenURL(g.Nodes[m.gsel].Job.URL)
		}
	case "ctrl+d", "pgdown", "ctrl+u", "pgup":
		lines, _ := m.graphLines()
		step := max(1, m.logHeight()/2)
		if s == "ctrl+u" || s == "pgup" {
			step = -step
		}
		m.top = min(max(m.top+step, 0), max(0, len(lines)-m.logHeight()))
	case "esc", "backspace", "q", "ctrl+c", "R", "ctrl+r", "1", "2", "3", ":", "P":
		return m, nil, false
	}
	return m, nil, true
}

// withKey returns a copy of the set with key set to on.
func withKey(set map[string]bool, key string, on bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range set {
		out[k] = v
	}
	if on {
		out[key] = true
	} else {
		delete(out, key)
	}
	return out
}

func (m *model) scroll() {
	h := m.bodyHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
}

// logKey handles scrolling keys in the log viewer; ok=false falls through
// to the shared handler (quit, esc, filter-less navigation, ...).
func (m model) logKey(k tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	h := m.logHeight()
	switch k.String() {
	case "j", "down":
		m.top = min(m.top+1, m.maxTop())
	case "k", "up":
		m.top = max(m.top-1, 0)
	case "ctrl+d", "pgdown", " ":
		m.top = min(m.top+h/2, m.maxTop())
	case "ctrl+u", "pgup":
		m.top = max(m.top-h/2, 0)
	case "g", "home":
		m.top = 0
	case "G", "end":
		m.top = m.maxTop()
	case "/":
		m.mode, m.input = modeSearch, m.query
		return m, nil, true
	case "n", "N":
		if len(m.matches) > 0 {
			d := 1
			if k.String() == "N" {
				d = len(m.matches) - 1
			}
			m.gotoMatch((m.mi + d) % len(m.matches))
		}
		return m, nil, true
	case "f":
		m.follow = !m.follow
		if m.follow {
			m.top = m.maxTop()
			return m, m.fetch(), true
		}
	case "o":
		if m.jobURL != "" {
			_ = ghcli.OpenURL(m.jobURL)
		}
	case "esc":
		if m.query != "" {
			m.query, m.matches, m.mi = "", nil, 0
			return m, nil, true
		}
		return m, nil, false
	case "backspace", "q", "ctrl+c", "R":
		return m, nil, false
	default:
		return m, nil, true // ignore table-only keys such as r/c/d/t
	}
	if m.top < m.maxTop() && (k.String() == "k" || k.String() == "up" || k.String() == "g") {
		m.follow = false
	}
	return m, nil, true
}

// gotoMatch scrolls the log so that match i is near the top.
func (m *model) gotoMatch(i int) {
	m.mi = i
	m.follow = false
	m.top = min(max(m.matches[i]-2, 0), m.maxTop())
}
