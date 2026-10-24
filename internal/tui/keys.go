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
	if m.askIcons {
		return m.iconAskKey(k)
	}
	if m.helpOpen {
		return m.helpKeyPress(k)
	}
	if m.dlg != nil {
		return m.dialogKey(k)
	}
	if m.picker != nil {
		return m.pickerKey(k)
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
	if k.String() == "?" {
		m.helpOpen, m.helpTop = true, 0
		return m, nil
	}
	if k.String() == "p" && (m.view == viewPRs || m.view == viewIssues || m.view == viewInbox) {
		m.previewOff = !m.previewOff // for this session; the setting `preview` is the lasting choice
		if m.previewOff {
			m.status = styDim.Render("preview pane off (p brings it back)")
		}
		return m, nil
	}
	if (k.String() == "J" || k.String() == "K") && m.previewShown() {
		step := previewScrollStep
		if k.String() == "K" {
			step = -step
		}
		return m.previewScroll(step), nil
	}
	if m.view == viewFavs {
		if next, cmd, ok := m.favTabKey(k); ok {
			return next, cmd
		}
	}
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
	if m.view == viewPRs {
		if next, cmd, handled := m.prListKey(k); handled {
			return next, cmd
		}
	}
	if s, ok := screens[m.view]; ok && s.key != nil {
		if next, cmd, handled := s.key(m, k); handled {
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
		} else if next, cmd, ok := m.popBack(); ok {
			return next, cmd
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
		m.mode, m.input, m.histPos = modeCommand, "", -1
	case "0":
		return m.goTop(viewInbox)
	case "1":
		return m.goTop(viewRuns)
	case "2":
		return m.goTop(viewWorkflows)
	case "3":
		return m.goTop(viewPRs)
	case "4":
		return m.goTop(viewIssues)
	case "P":
		return m.currentPR()
	case "R", "ctrl+r":
		m.loading = true
		return m, m.fetch()
	case "o":
		if r, ok := m.selected(); ok && r.url != "" {
			m = m.open(r.url)
			if m.status == "" {
				m.status = styDim.Render("opened " + r.url)
			}
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
	case "r", "F", "c", "d":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m.startRunAct(runActionOf(k.String()), r.id, r.status, r.conclusion)
		}
		if m.view == viewJobs {
			return m.runActOnShown(runActionOf(k.String()))
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
	m.back, m.wfID, m.wfName, m.pr, m.target = nil, "", "", "", m.base
	m.prTarget = m.base
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
		m.graph, m.gsel, m.top, m.expanded, m.runStatus, m.runMeta = graph.Graph{}, 0, 0, nil, "", ghcli.RunMeta{}
		return m, m.push(viewGraph)
	case viewJobs:
		m.jobID, m.jobName, m.jobURL = r.id, r.extra, r.url
		return m, m.push(viewLogs)
	case viewFavs:
		if f, ok := m.favForRow(r); ok {
			return m, m.openFavorite(f, true)
		}
	case viewPRs:
		m.pr, m.prTarget = r.id, m.target
		m = m.withScreenData(viewPRDetail, prDetailState{}) // a different PR starts at the top
		return m, m.push(viewPRDetail)
	case viewChecks:
		return m.openCheck(r)
	}
	return m, nil
}

func (m model) handleInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Type != tea.KeyTab {
		m.compBase = "" // any other key ends a Tab cycle
	}
	if m.mode == modeCommand {
		switch k.Type {
		case tea.KeyTab:
			return m.tabComplete(), nil
		case tea.KeyRight:
			if c := m.completions(m.input); len(c) > 0 {
				m.input = c[0]
			}
			return m, nil
		case tea.KeyUp, tea.KeyDown:
			return m.walkHistory(k.Type == tea.KeyUp), nil
		}
	}
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
		if mode == modeCommand && in != "" && (len(m.history) == 0 || m.history[len(m.history)-1] != in) {
			m.history = append(append([]string(nil), m.history...), in)
		}
		m.histPos = -1
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
				if in == "" {
					m.status = styOK.Render("✓ alias cleared")
				} else {
					m.status = styOK.Render("✓ alias set: " + in)
				}
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
	c = m.resolveCommand(c)
	if f := strings.Fields(c); len(f) > 0 && strings.ToLower(f[0]) == "repo" {
		return m.repoCommand(f[1:])
	} else if len(f) > 0 && (strings.ToLower(f[0]) == "fav" || strings.ToLower(f[0]) == "favs" || strings.ToLower(f[0]) == "favorites") && len(f) > 1 {
		return m.favCommand(f[1:])
	} else if len(f) > 0 && strings.ToLower(f[0]) == "set" {
		return m.setCommand(f[1:])
	}
	switch strings.ToLower(c) {
	case "runs", "run", "r":
		return m.goTop(viewRuns)
	case "workflows", "workflow", "wf", "w":
		return m.goTop(viewWorkflows)
	case "pulls", "pull", "prs", "pr":
		return m.goTop(viewPRs)
	case "issues", "issue", "is":
		return m.goTop(viewIssues)
	case "inbox", "in":
		return m.goTop(viewInbox)
	case "icons":
		return m.iconsCommand()
	case "settings":
		if m.view == viewSettings {
			return m, nil
		}
		cmd := m.push(viewSettings)
		return m, cmd
	case "fav", "favs", "favorites":
		return m.favCommand(nil)
	case "cpr", "checks":
		return m.currentPR()
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
	case "r", "F", "c", "d":
		next, cmd := m.runActOnShown(runActionOf(s))
		return next, cmd, true
	case "f":
		next, cmd := m.jumpToFailure()
		return next, cmd, true
	case "o":
		if m.gsel < len(g.Nodes) && g.Nodes[m.gsel].Job.URL != "" {
			m = m.open(g.Nodes[m.gsel].Job.URL)
		}
	case "ctrl+d", "pgdown", "ctrl+u", "pgup":
		lines, _ := m.graphLines()
		step := max(1, m.logHeight()/2)
		if s == "ctrl+u" || s == "pgup" {
			step = -step
		}
		m.top = min(max(m.top+step, 0), max(0, len(lines)-m.logHeight()))
	case "esc", "backspace", "q", "ctrl+c", "R", "ctrl+r", "1", "2", "3", "4", ":", "P":
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
			m = m.open(m.jobURL)
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
	if m.view == viewDiff { // the cursor goes where the match is, so a comment lands on that line
		if st, ok := m.screenData(viewDiff).(diffState); ok && len(st.d.Files) > 0 {
			*m = m.withScreenData(viewDiff, setCursor(st, m.matches[i]))
		}
	}
}

// selectedNodeID is the ID of the selected graph node ("" if none).
func (m model) selectedNodeID() string {
	if n := m.shown().Nodes; m.gsel >= 0 && m.gsel < len(n) {
		return n[m.gsel].ID
	}
	return ""
}

// reselect points the selection at the node with the given ID after the graph
// changed (jobs appear and finish while watching), or clamps it if that node is gone.
func (m *model) reselect(id string) {
	nodes := m.shown().Nodes
	for i, n := range nodes {
		if id != "" && n.ID == id {
			m.gsel = i
			return
		}
	}
	m.gsel = min(m.gsel, max(0, len(nodes)-1))
}

// prListKey sets the PR list filter: u = opened by me, e = review requested from me, a = all.
func (m model) prListKey(k tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch k.String() {
	case "u":
		m.prFilter = ghcli.PRFilter{Mine: true}
	case "e":
		m.prFilter = ghcli.PRFilter{ReviewRequested: true}
	case "a":
		m.prFilter = ghcli.PRFilter{}
	default:
		return m, nil, false
	}
	m.cursor, m.top, m.loading = 0, 0, true
	return m, m.fetch(), true
}

// prFilterName words the active PR filter for the title ("" when there is none).
func prFilterName(f ghcli.PRFilter) string {
	switch {
	case f.Mine && f.ReviewRequested:
		return "opened by me + review requested"
	case f.Mine:
		return "opened by me"
	case f.ReviewRequested:
		return "review requested"
	}
	return ""
}

// tabComplete inserts the best suggestion; further Tabs cycle through the others.
func (m model) tabComplete() model {
	if m.compBase == "" {
		m.compBase, m.compIdx = m.input, -1
	}
	c := m.completions(m.compBase)
	if len(c) == 0 {
		return m
	}
	m.compIdx = (m.compIdx + 1) % len(c)
	m.input = c[m.compIdx]
	return m
}

// walkHistory moves through earlier commands; coming back past the newest restores what was typed.
func (m model) walkHistory(older bool) model {
	if m.histPos == 0 && m.histDraft == "" && len(m.history) == 0 {
		return m
	}
	if m.histPos == -1 {
		if !older {
			return m
		}
		m.histDraft, m.histPos = m.input, len(m.history)
	}
	if older {
		m.histPos = max(m.histPos-1, 0)
	} else {
		m.histPos++
	}
	if m.histPos >= len(m.history) {
		m.input, m.histPos = m.histDraft, -1
		return m
	}
	if len(m.history) > 0 {
		m.input = m.history[m.histPos]
	}
	return m
}

// popBack goes back one screen, with the bookkeeping going back needs; ok is false at the first screen.
func (m model) popBack() (model, tea.Cmd, bool) {
	if len(m.back) == 0 {
		return m, nil, false
	}
	prev := m.back[len(m.back)-1]
	m.back = m.back[:len(m.back)-1]
	if prev != viewRuns {
		m.wfID, m.wfName = "", ""
	}
	if prev == viewFavs {
		m.target = m.base
	}
	if prev == viewInbox {
		m.target, m.prTarget = m.base, m.base // the inbox is not about one repo
	}
	if prev == viewChecks || prev == viewPRDetail {
		m.target = m.prTarget // a check may have taken us to a run in another repo
	}
	cmd := m.switchTo(prev)
	return m, cmd, true
}
