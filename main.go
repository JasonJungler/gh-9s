package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewKind int

const (
	viewRuns viewKind = iota
	viewWorkflows
	viewPRs
	viewChecks
	viewJobs
	viewLogs
	viewFavs
	viewGraph
)

var viewNames = map[viewKind]string{
	viewRuns: "runs", viewWorkflows: "workflows", viewPRs: "pulls", viewChecks: "checks", viewJobs: "jobs", viewLogs: "logs", viewFavs: "favorites", viewGraph: "graph",
}

type inputMode int

const (
	modeNormal inputMode = iota
	modeFilter
	modeCommand
	modeConfirm
	modeAlias
)

type model struct {
	repo, branch  string
	view          viewKind
	back          []viewKind
	tbl           table
	cursor, top   int
	width, height int
	loading       bool
	err           error
	status        string

	// scope for the current view
	target       repoRef // zero value: repo of the current directory
	wfID, wfName string  // runs filtered by workflow
	pr           string  // checks for a PR ("" = current branch)

	mode                          inputMode
	input                         string
	filter                        string
	runID, jobID, jobName, jobURL string // jobs / logs scope
	logs                          []string
	follow                        bool

	graph    graph
	gstyle   graphStyle
	gsel     int
	expanded map[string]bool
	specs    map[string][]jobSpec // workflow job specs, by run ID
	form     *triggerForm         // non-nil while the trigger form is open
	favs     *favStore            // nil when the favorites file could not be loaded
	curRepo  repoRef              // repo of the current directory, host included
	latest   map[string]latestEntry
	aliasFor favorite

	confirm    func() tea.Cmd
	confirmMsg string
}

type dataMsg struct {
	view   viewKind
	tbl    table
	logs   []string
	latest map[string]latestEntry
	graph  *graph
	jobs   []runJob
	specs  map[string][]jobSpec
	err    error
}
type actionMsg struct {
	msg string
	err error
}
type tickMsg struct{}

// formReadyMsg carries a freshly loaded trigger form (or why there is none).
type formReadyMsg struct {
	form         triggerForm
	name         string
	dispatchable bool
	err          error
}

// inputsMsg carries the inputs re-read for a newly chosen ref.
type inputsMsg struct {
	ref          string
	inputs       []dispatchInput
	dispatchable bool
	err          error
}
type infoMsg struct {
	repo, branch string
	cur          repoRef
}
type gotoMsg struct {
	view viewKind
	pr   string
	err  error
}

var (
	styHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
	styCols   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	stySel    = lipgloss.NewStyle().Reverse(true).Bold(true)
	styOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styFail   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styRun    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

func main() {
	m := model{loading: true}
	if fs, err := loadFavs(favsPath()); err != nil {
		m.status = styErr.Render("favorites disabled: " + err.Error())
	} else {
		m.favs = fs
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh9s:", err)
		os.Exit(1)
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetch(), tick(), func() tea.Msg {
		r, b := repoInfo()
		cur, _ := currentRepo()
		return infoMsg{r, b, cur}
	})
}

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) fetch() tea.Cmd {
	v, wf, pr, br, run, job, tg := m.view, m.wfID, m.pr, m.branch, m.runID, m.jobID, m.target
	var pinned []favorite
	var snap *favStore
	if m.favs != nil {
		pinned = m.favs.forRepo(m.curTarget())
		snap = &favStore{items: append([]favorite(nil), m.favs.items...)}
	}
	cache := m.latest
	specCache := m.specs
	return func() tea.Msg {
		var t table
		var latest map[string]latestEntry
		var gr *graph
		var gjobs []runJob
		var newSpecs map[string][]jobSpec
		var logs []string
		var err error
		switch v {
		case viewRuns:
			t, err = fetchRuns(tg, wf)
		case viewWorkflows:
			t, err = fetchWorkflows(tg)
			t.rows = pinFavorites(t.rows, pinned)
		case viewPRs:
			t, err = fetchPRs(tg, br)
		case viewChecks:
			t, err = fetchChecks(tg, pr)
		case viewJobs:
			t, err = fetchJobs(tg, run)
		case viewLogs:
			logs, err = fetchJobLog(tg, run, job)
		case viewGraph:
			var sha, path string
			gjobs, sha, path, err = fetchRunJobs(tg, run)
			if err != nil {
				break
			}
			sp, cached := specCache[run]
			if !cached && path != "" {
				if src, e := fetchWorkflowFile(tg, path, sha); e == nil {
					if parsed, e := parseJobSpecs(src); e == nil {
						sp, newSpecs = parsed, map[string][]jobSpec{run: parsed}
					}
				}
			}
			g := buildGraph(gjobs, sp)
			gr = &g
		case viewFavs:
			if snap != nil {
				t, latest = fetchFavorites(snap, cache, time.Now())
			}
		}
		return dataMsg{v, t, logs, latest, gr, gjobs, newSpecs, err}
	}
}

func (m *model) push(v viewKind) tea.Cmd {
	m.back = append(m.back, m.view)
	return m.switchTo(v)
}

func (m *model) switchTo(v viewKind) tea.Cmd {
	m.view, m.tbl, m.cursor, m.top, m.filter, m.err = v, table{}, 0, 0, "", nil
	m.logs, m.follow = nil, false
	m.loading = true
	return m.fetch()
}

func (m model) visible() []row {
	if m.filter == "" {
		return m.tbl.rows
	}
	f := strings.ToLower(m.filter)
	var out []row
	for _, r := range m.tbl.rows {
		if strings.Contains(strings.ToLower(strings.Join(r.cells, " ")), f) {
			out = append(out, r)
		}
	}
	return out
}

func (m model) selected() (row, bool) {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return row{}, false
	}
	return v[m.cursor], true
}

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
			m.graph, m.gsel, m.top, m.expanded = graph{}, 0, 0, nil
			m.status = styOK.Render("✓ triggered " + msg.workflow + " → run " + msg.runID)
			return m, m.push(viewGraph)
		case msg.dispatched:
			m.status = styErr.Render("triggered " + msg.workflow + ", but its run was not found yet: " + msg.err.Error())
			return m, m.fetch()
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
			}
			m.form = &f
		}
		return m, nil
	case tickMsg:
		if m.view == viewLogs && !m.follow {
			return m, tick() // logs are static unless following
		}
		if m.view == viewGraph && !m.graph.active() {
			return m, tick() // a finished run does not change
		}
		return m, tea.Batch(m.fetch(), tick())
	case dataMsg:
		if msg.view != m.view {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.latest != nil {
			m.latest = msg.latest
		}
		for k, v := range msg.specs {
			if m.specs == nil {
				m.specs = map[string][]jobSpec{}
			}
			m.specs[k] = v
		}
		if msg.err == nil && msg.graph != nil {
			if msg.graph.flat {
				// dependencies unknown: show the plain jobs list instead of guessing a graph
				m.view, m.tbl, m.cursor, m.top = viewJobs, jobsTable(msg.jobs), 0, 0
				m.status = styDim.Render("graph unavailable: " + msg.graph.reason)
				return m, nil
			}
			m.graph = *msg.graph
			m.gsel = min(m.gsel, max(0, len(m.shown().nodes)-1))
			return m, nil
		}
		if msg.err == nil && m.view == viewLogs {
			m.logs = msg.logs
			if m.follow {
				m.top = m.maxTop()
			}
		} else if msg.err == nil {
			m.tbl = msg.tbl
			if n := len(m.visible()); m.cursor >= n {
				m.cursor = max(0, n-1)
			}
		}
	case actionMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
		} else {
			m.status = styOK.Render("✓ " + msg.msg)
		}
		return m, m.fetch()
	case gotoMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
			return m, nil
		}
		m.pr, m.target = msg.pr, repoRef{}
		return m, m.push(msg.view)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

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
				m.target = repoRef{}
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
			_ = openURL(r.url)
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
			c := exec.Command("gh", scoped(m.target, "run", "watch", r.id, "--exit-status")...)
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
			return m, runAction("rerun run "+r.id, scoped(m.target, "run", "rerun", r.id)...)
		}
	case "F":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, runAction("rerun failed jobs of "+r.id, scoped(m.target, "run", "rerun", r.id, "--failed")...)
		}
	case "c":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			m.mode, m.confirmMsg = modeConfirm, "cancel run "+r.id+"? (y/N)"
			m.confirm = func() tea.Cmd { return runAction("cancelled run "+r.id, scoped(m.target, "run", "cancel", r.id)...) }
		}
	case "d":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			m.mode, m.confirmMsg = modeConfirm, "delete run "+r.id+"? (y/N)"
			m.confirm = func() tea.Cmd { return runAction("deleted run "+r.id, scoped(m.target, "run", "delete", r.id)...) }
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
			return m, runAction(sub+"d workflow "+r.extra, scoped(m.target, "workflow", sub, r.id)...)
		}
	}
	m.scroll()
	return m, nil
}

func (m model) goTop(v viewKind) (tea.Model, tea.Cmd) {
	m.back, m.wfID, m.wfName, m.pr, m.target = nil, "", "", "", repoRef{}
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
		m.graph, m.gsel, m.top, m.expanded = graph{}, 0, 0, nil
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
			_ = openURL(r.url)
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
		case modeAlias:
			if err := m.favs.setAlias(m.aliasFor, in); err != nil {
				m.status = styErr.Render("✗ " + err.Error())
			} else if err := m.favs.save(); err != nil {
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
			if f, ok := m.favs.byAlias(c); ok {
				return m, m.openFavorite(f, false)
			}
		}
		m.status = styErr.Render("unknown command: " + c)
	}
	return m, nil
}

// curTarget is the repo views are showing: the explicit target, else the
// repo of the current directory.
func (m model) curTarget() repoRef {
	if m.target != (repoRef{}) {
		return m.target
	}
	return m.curRepo
}

func (m model) favForRow(r row) (favorite, bool) {
	switch m.view {
	case viewFavs:
		if m.favs != nil {
			for _, f := range m.favs.items {
				if f.key() == r.extra {
					return f, true
				}
			}
		}
	case viewWorkflows:
		if t := m.curTarget(); t != (repoRef{}) && len(r.cells) > 3 {
			return favorite{Host: hostOrDefault(t.Host), Repo: t.Owner + "/" + t.Name,
				Workflow: workflowFile(r.cells[3])}, true
		}
	}
	return favorite{}, false
}

func (m model) toggleFavorite(r row) (tea.Model, tea.Cmd) {
	f, ok := m.favForRow(r)
	switch {
	case m.favs == nil:
		m.status = styErr.Render("favorites are disabled (see the startup message)")
	case !ok:
		m.status = styErr.Render("cannot tell which repo this is; run gh-9s inside a repo")
	default:
		added := m.favs.toggle(f)
		if err := m.favs.save(); err != nil {
			m.status = styErr.Render("✗ " + err.Error())
		} else if added {
			m.status = styOK.Render("★ added " + f.Workflow)
		} else {
			m.status = styDim.Render("removed " + f.Workflow)
		}
		return m, m.fetch()
	}
	return m, nil
}

func (m model) startAlias(r row) (tea.Model, tea.Cmd) {
	f, ok := m.favForRow(r)
	if m.favs == nil || !ok {
		m.status = styErr.Render("favorites are not available here")
		return m, nil
	}
	i := m.favs.index(f)
	if i < 0 {
		m.status = styDim.Render("press f to make it a favorite first")
		return m, nil
	}
	m.aliasFor, m.mode, m.input = f, modeAlias, m.favs.items[i].Alias
	return m, nil
}

// openFavorite shows a favorite's runs, in its own repo.
func (m *model) openFavorite(f favorite, push bool) tea.Cmd {
	m.target = f.target()
	if m.target == m.curRepo {
		m.target = repoRef{}
	}
	m.wfID, m.wfName = f.Workflow, f.Alias
	if m.wfName == "" {
		m.wfName = f.Workflow
	}
	if push {
		return m.push(viewRuns)
	}
	m.back = nil
	return m.switchTo(viewRuns)
}

const workflowDir = ".github/workflows/"

// startTrigger loads refs and the workflow's inputs, then opens the form.
func (m model) startTrigger(r row) (tea.Model, tea.Cmd) {
	tgt, file, name := m.target, "", r.extra
	switch m.view {
	case viewWorkflows:
		file = workflowFile(r.cells[3])
	case viewFavs:
		f, ok := m.favForRow(r)
		if !ok {
			return m, nil
		}
		tgt, file, name = f.target(), f.Workflow, f.Workflow
		if tgt == m.curRepo {
			tgt = repoRef{}
		}
	}
	local := ""
	if tgt == (repoRef{}) {
		local = m.branch
	}
	m.status = styDim.Render("loading " + file + "…")
	return m, loadFormCmd(tgt, file, name, local)
}

func readInputs(tgt repoRef, file, ref string) ([]dispatchInput, bool, error) {
	src, err := fetchWorkflowFile(tgt, workflowDir+file, ref)
	if err != nil {
		return nil, false, err
	}
	return parseDispatchInputs(src)
}

func loadFormCmd(tgt repoRef, file, name, localBranch string) tea.Cmd {
	return func() tea.Msg {
		refs, err := fetchBranches(tgt)
		if err != nil {
			return formReadyMsg{err: err}
		}
		ref := refs[0]
		for _, b := range refs {
			if b == localBranch {
				ref = b
			}
		}
		inputs, ok, err := readInputs(tgt, file, ref)
		if err != nil {
			return formReadyMsg{err: fmt.Errorf("cannot read %s: %w", file, err)}
		}
		return formReadyMsg{form: newTriggerForm(tgt, file, ref, refs, inputs), name: name, dispatchable: ok}
	}
}

func (m model) shown() graph { return expandGraph(m.graph, m.expanded) }

func (g graph) active() bool {
	if len(g.nodes) == 0 {
		return true // nothing loaded yet
	}
	for _, n := range g.nodes {
		if n.State == stRun {
			return true
		}
	}
	return false
}

func (m model) graphLines() ([]string, graphStyle) {
	return renderGraph(m.shown(), m.gstyle, m.gsel, m.width, time.Now())
}

// graphKey handles the keys of the graph view; ok=false falls through to the
// shared handler (quit, back, refresh, view switching).
func (m model) graphKey(k tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	g := m.shown()
	switch s := k.String(); s {
	case "h", "left", "l", "right", "j", "down", "k", "up":
		dir := map[string]rune{"h": 'h', "left": 'h', "l": 'l', "right": 'l', "j": 'j', "down": 'j', "k": 'k', "up": 'k'}[s]
		m.gsel = moveSel(g, m.gsel, dir)
	case "v":
		m.gstyle = m.gstyle.next()
	case "x":
		if m.gsel < len(g.nodes) && g.nodes[m.gsel].Group != "" {
			id := g.nodes[m.gsel].Group
			m.expanded = withKey(m.expanded, id, false)
			for i, n := range m.shown().nodes {
				if n.ID == id {
					m.gsel = i
				}
			}
		}
	case "enter":
		if m.gsel >= len(g.nodes) {
			break
		}
		n := g.nodes[m.gsel]
		if len(n.Members) > 0 {
			m.expanded = withKey(m.expanded, n.ID, true)
			for i, e := range m.shown().nodes {
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
		if m.gsel < len(g.nodes) && g.nodes[m.gsel].Job.URL != "" {
			_ = openURL(g.nodes[m.gsel].Job.URL)
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

func (m model) formKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	next, submit, cancel := m.form.update(k)
	switch {
	case cancel:
		m.form = nil
		return m, nil
	case submit:
		m.form = nil
		m.status = styDim.Render("triggering " + next.workflow + "…")
		run := triggerAndFind(next)
		return m, func() tea.Msg { return run() }
	}
	var cmd tea.Cmd
	if next.pendingReload {
		next.pendingReload, next.loadedRef = false, next.ref
		tgt, file, ref := next.target, next.workflow, next.ref
		cmd = func() tea.Msg {
			in, ok, err := readInputs(tgt, file, ref)
			return inputsMsg{ref, in, ok, err}
		}
	}
	m.form = &next
	return m, cmd
}

func runAction(done string, args ...string) tea.Cmd {
	return func() tea.Msg {
		_, err := gh(args...)
		return actionMsg{msg: done, err: err}
	}
}

func logsCmd(tg repoRef, id string, failedOnly bool) tea.Cmd {
	flag := "--log"
	if failedOnly {
		flag = "--log-failed"
	}
	scope := ""
	if a := tg.args(); a != nil {
		scope = " -R " + a[1]
	}
	c := exec.Command("sh", "-c", fmt.Sprintf("gh run view %s %s%s 2>&1 | ${PAGER:-less -R}", id, flag, scope))
	return tea.ExecProcess(c, func(error) tea.Msg { return actionMsg{msg: "closed logs"} })
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
	case "f":
		m.follow = !m.follow
		if m.follow {
			m.top = m.maxTop()
			return m, m.fetch(), true
		}
	case "o":
		if m.jobURL != "" {
			_ = openURL(m.jobURL)
		}
	case "esc", "backspace", "q", "ctrl+c", "R":
		return m, nil, false
	default:
		return m, nil, true // ignore table-only keys such as r/c/d/t
	}
	if m.top < m.maxTop() && (k.String() == "k" || k.String() == "up" || k.String() == "g") {
		m.follow = false
	}
	return m, nil, true
}

// logHeight is the log window size: the table body plus the unused column header row.
func (m model) logHeight() int { return m.bodyHeight() + 1 }

func (m model) maxTop() int { return max(0, len(m.logs)-m.logHeight()) }

func (m model) bodyHeight() int { return max(1, m.height-5) }

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s + strings.Repeat(" ", w-utf8.RuneCountInString(s))
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

func (m model) widths() []int {
	w := make([]int, len(m.tbl.cols))
	for i, c := range m.tbl.cols {
		w[i] = utf8.RuneCountInString(c)
	}
	for _, r := range m.tbl.rows {
		for i, c := range r.cells {
			w[i] = max(w[i], min(utf8.RuneCountInString(c), 50))
		}
	}
	// shrink the widest column if the terminal is too narrow
	for {
		total := 0
		for _, x := range w {
			total += x + 2
		}
		if total <= m.width || m.width == 0 {
			return w
		}
		wi := 0
		for i := range w {
			if w[i] > w[wi] {
				wi = i
			}
		}
		if w[wi] <= 6 {
			return w
		}
		w[wi]--
	}
}

func (m model) View() string {
	var b strings.Builder
	title := fmt.Sprintf(" gh9s  %s@%s  <%s>", m.repo, m.branch, viewNames[m.view])
	if m.target != (repoRef{}) {
		title = fmt.Sprintf(" gh9s  %s  <%s>", m.target, viewNames[m.view])
	}
	if m.wfName != "" && m.view == viewRuns {
		title += " workflow=" + m.wfName
	}
	if m.view == viewChecks {
		if m.pr != "" {
			title += " pr=#" + m.pr
		}
	}
	if m.view == viewJobs || m.view == viewLogs || m.view == viewGraph {
		title += " run=" + m.runID
	}
	if m.view == viewGraph {
		_, used := m.graphLines()
		title += " style=" + used.String()
		if used != m.gstyle {
			title += " (" + m.gstyle.String() + " too wide)"
		}
	}
	if m.view == viewLogs {
		title += " job=" + m.jobName
		if m.follow {
			title += " (following)"
		}
	}
	vis := m.visible()
	count := len(vis)
	if m.view == viewLogs {
		count = len(m.logs)
	}
	if m.view == viewGraph {
		count = len(m.shown().nodes)
	}
	title += fmt.Sprintf("  [%d]", count)
	if m.filter != "" {
		title += "  /" + m.filter
	}
	if m.loading {
		title += "  loading…"
	}
	b.WriteString(styHeader.Render(strings.TrimSpace(title)) + "\n")
	b.WriteString(styDim.Render("<1>runs <2>workflows <3>pulls <P>current-PR <:>cmd </>filter") + "\n")

	if m.form != nil {
		return b.String() + "\n" + m.form.view(m.width)
	}
	if m.view == viewGraph {
		return b.String() + m.graphBody() + m.footer()
	}
	if m.view == viewLogs {
		return b.String() + m.logBody() + m.footer()
	}

	w := m.widths()
	var hdr strings.Builder
	for i, c := range m.tbl.cols {
		hdr.WriteString(trunc(c, w[i]) + "  ")
	}
	b.WriteString(styCols.Render(hdr.String()) + "\n")

	h := m.bodyHeight()
	if m.err != nil {
		b.WriteString(styErr.Render(m.err.Error()) + "\n")
	}
	end := min(len(vis), m.top+h)
	lines := 0
	for i := m.top; i < end; i++ {
		r := vis[i]
		var line strings.Builder
		for j, c := range r.cells {
			s := trunc(c, w[j]) + "  "
			if j == statusCol(m.view) && i != m.cursor {
				s = stateStyle(r.state).Render(s)
			}
			line.WriteString(s)
		}
		if i == m.cursor {
			b.WriteString(stySel.Render(line.String()) + "\n")
		} else {
			b.WriteString(line.String() + "\n")
		}
		lines++
	}
	for ; lines < h-boolToInt(m.err != nil); lines++ {
		b.WriteString("\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

// logBody renders the visible window of the log, padded to the body height.
func (m model) logBody() string {
	var b strings.Builder
	h := m.logHeight()
	if m.err != nil {
		b.WriteString(styErr.Render(m.err.Error()) + "\n")
		h--
	} else if m.loading && len(m.logs) == 0 {
		b.WriteString(styDim.Render("loading logs…") + "\n")
		h--
	}
	end := min(len(m.logs), m.top+h)
	n := 0
	for i := m.top; i < end; i++ {
		l := m.logs[i]
		if m.width > 0 {
			l = strings.TrimRight(trunc(l, m.width), " ")
		}
		switch {
		case strings.Contains(l, "##[error]") || strings.Contains(l, "Error:"):
			l = styFail.Render(l)
		case strings.Contains(l, "##[warning]"):
			l = styRun.Render(l)
		case strings.Contains(l, "##[group]"):
			l = styCols.Render(l)
		}
		b.WriteString(l + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) graphBody() string {
	lines, _ := m.graphLines()
	h := m.logHeight()
	top := min(m.top, max(0, len(lines)-h))
	var b strings.Builder
	if m.err != nil {
		b.WriteString(styErr.Render(m.err.Error()) + "\n")
		h--
	}
	n := 0
	for i := top; i < len(lines) && n < h; i++ {
		b.WriteString(lines[i] + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func statusCol(v viewKind) int {
	switch v {
	case viewRuns, viewPRs, viewJobs:
		return 1
	case viewFavs:
		return 3
	case viewChecks:
		return 0
	}
	return 2
}

func stateStyle(s state) lipgloss.Style {
	switch s {
	case stOK:
		return styOK
	case stFail:
		return styFail
	case stRun:
		return styRun
	}
	return styDim
}

func (m model) footer() string {
	switch m.mode {
	case modeFilter:
		return styKey.Render("/") + m.input + "█"
	case modeCommand:
		return styKey.Render(":") + m.input + "█"
	case modeAlias:
		return styKey.Render("alias for "+m.aliasFor.Workflow+" > ") + m.input + "█"
	case modeConfirm:
		return styErr.Render(m.confirmMsg)
	}
	var keys string
	switch m.view {
	case viewRuns:
		keys = "enter jobs  l pager-logs  L failed-logs  w watch  r rerun  F rerun-failed  c cancel  d delete  o open"
	case viewWorkflows:
		keys = "enter runs  t trigger  f favorite  a alias  e enable/disable  o open"
	case viewPRs:
		keys = "enter checks  o open"
	case viewFavs:
		keys = "enter runs  f unfavorite  a alias"
	case viewChecks:
		keys = "enter/o open check  esc back"
	case viewJobs:
		keys = "enter logs  o open  esc back"
	case viewGraph:
		keys = "hjkl/arrows move  enter logs or expand  x collapse  v style  o open  ctrl-d/u scroll  esc back"
	case viewLogs:
		keys = "j/k scroll  space/ctrl-d page  g/G top/bottom  f follow  esc back"
	}
	keys += "  R refresh  q quit"
	if m.status != "" {
		return m.status + "\n" + styDim.Render(keys)
	}
	return "\n" + styDim.Render(keys)
}
