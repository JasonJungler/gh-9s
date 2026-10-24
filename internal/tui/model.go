// Package tui is the interactive terminal UI (Bubble Tea): model, keys, views, and the
// commands that fetch data through package ghcli.
package tui

import (
	"strings"
	"time"

	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/recent"
	"github.com/MurabitoB/gh-9s/internal/settings"
	"github.com/MurabitoB/gh-9s/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
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
	viewPRDetail
	viewDiff
	viewIssues
	viewIssue
	viewInbox
	viewSettings
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
	modeSearch
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
	base         ghcli.RepoRef     // the repo chosen with :repo; zero value: the current directory's repo
	prTarget     ghcli.RepoRef     // the repo of the PR being looked at; restored when coming back from a check's run
	target       ghcli.RepoRef     // what the current view shows: base, or a favorite's repo while visiting one
	wfID, wfName string            // runs filtered by workflow
	pr           string            // the PR shown (checks, detail); "" = the current branch's PR
	issueFilter  ghcli.IssueFilter // issue list filter (zero value = open issues)
	issue        string            // the issue shown
	prFilter     ghcli.PRFilter    // PR list filter

	mode                          inputMode
	input                         string
	compBase                      string   // what was typed when Tab started cycling suggestions ("" = not cycling)
	compIdx                       int      // which suggestion Tab last inserted
	history                       []string // commands run before, oldest first
	histPos                       int      // position while walking history with up/down (-1 = not walking)
	histDraft                     string   // what was being typed when the history walk started
	filter                        string
	runID, jobID, jobName, jobURL string // jobs / logs scope
	logs                          []string
	query                         string // log search
	matches                       []int
	mi                            int
	follow                        bool

	note         string                    // explanation tied to the current view (cleared when the view changes)
	banner       string                    // sticky startup problem, always shown
	runMeta      ghcli.RunMeta             // the watched run's status and conclusion as last seen
	runStatus    string                    // last seen status of the watched run, for the finished notification
	pendingMerge *mergeAction              // the merge being collected through dialogs
	settings     *settings.Store           // the configuration; nil means in-memory defaults (see cfg)
	drafts       *drafts.Store             // inline review comments not sent yet, kept on disk
	rerunWait    int                       // polls left to wait for a re-run's new attempt to show up
	logJump      bool                      // the next log to load opens at its first failure
	pendingRun   *runAct                   // a run action waiting for its confirmation
	askIcons     bool                      // the first-run question about Nerd Font icons is on screen
	iconHelp     bool                      // ... with its help text
	sendingKey   string                    // the PR whose review is being sent right now
	pendingDraft *pendingDraft             // a comment being written in a dialog
	pending      *prAction                 // the PR action being collected through dialogs
	dlg          *dialog                   // a question being asked (text, choice or yes/no)
	repoCache    map[string]repoCacheEntry // the repos I can access, per host, reused for a while
	picker       *picker                   // the repo picker overlay, when open
	recents      *recent.Store             // repos chosen before; nil if the file could not be read
	sdata        map[viewKind]any          // per-screen data, see screen.go
	graph        graph.Graph
	gstyle       graph.Style
	gsel         int
	expanded     map[string]bool
	specs        map[string][]workflow.JobSpec // workflow job specs, by run ID
	form         *triggerForm                  // non-nil while the trigger form is open
	favs         *favorites.Store              // nil when the favorites file could not be loaded
	curRepo      ghcli.RepoRef                 // repo of the current directory, host included
	latest       map[string]latestEntry
	aliasFor     favorites.Favorite

	prChecks     map[string]prCheck // the checks column of the PR list, by PR number, for prChecksKey
	prChecksKey  string             // which repo and filter prChecks is of
	prChecksAt   time.Time          // when they were last asked for
	prChecksBusy bool

	preview    *previewState // what the pane shows and has loaded; nil until the first list is drawn
	helpOpen   bool          // the ? overlay is showing
	helpTop    int
	previewOff bool // the pane was switched off for this session
	paneList   bool // this model is the list half of a split screen: its selected row fills the width

	confirm    func() tea.Cmd
	confirmMsg string
}

type dataMsg struct {
	view   viewKind
	tbl    table
	logs   []string
	latest map[string]latestEntry
	graph  *graph.Graph
	jobs   []graph.Job
	run    ghcli.RunMeta // the run's own status/conclusion (graph and jobs views)
	runID  string        // which run this data is of: a late answer for another run is dropped
	specs  map[string][]workflow.JobSpec
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
	inputs       []workflow.DispatchInput
	dispatchable bool
	err          error
}
type infoMsg struct {
	repo, branch string
	cur          ghcli.RepoRef
}
type gotoMsg struct {
	view viewKind
	pr   string
	err  error
}

// Run starts the TUI and blocks until the user quits.
func Run() error {
	m := model{loading: true}
	if fs, err := favorites.Load(favorites.DefaultPath()); err != nil {
		m.banner = "favorites disabled: " + err.Error()
	} else {
		m.favs = fs
	}
	var banner string
	m.settings, banner = loadSettings()
	m.banner = joinBanner(m.banner, banner)
	if ds, err := drafts.Load(drafts.DefaultPath()); err != nil {
		m.banner = joinBanner(m.banner, "drafts are not saved: "+err.Error())
	} else {
		m.drafts = ds
	}
	if rs, err := recent.Load(recent.DefaultPath()); err == nil {
		m.recents = rs
	}
	m = m.startIcons()
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if m.cfg().Bool("mouse") {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	_, err := tea.NewProgram(m, opts...).Run()
	return err
}

// Init starts the first fetch, the refresh tick, and the repo lookup.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetch(), tick(), func() tea.Msg {
		r, b := ghcli.LocalRepo()
		cur, _ := ghcli.CurrentRepo()
		return infoMsg{r, b, cur}
	})
}

func (m *model) push(v viewKind) tea.Cmd {
	m.back = append(m.back, m.view)
	return m.switchTo(v)
}

func (m *model) switchTo(v viewKind) tea.Cmd {
	m.view, m.tbl, m.cursor, m.top, m.filter, m.err = v, table{}, 0, 0, "", nil
	m.logs, m.follow = nil, false
	if v != viewLogs {
		m.logJump = false
	}
	m.query, m.matches, m.mi = "", nil, 0
	m.note = ""
	m.pending, m.pendingMerge = nil, nil // half-finished actions belong to the screen we are leaving
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

// curTarget is the repo views are showing: the explicit target, else the
// repo of the current directory.
func (m model) curTarget() ghcli.RepoRef {
	if m.target != (ghcli.RepoRef{}) {
		return m.target
	}
	return m.curRepo
}

func (m model) shown() graph.Graph { return graph.Expand(m.graph, m.expanded) }

// shouldPoll says whether the 5s tick should refetch the current view.
func (m model) shouldPoll() bool {
	if s, ok := screens[m.view]; ok {
		return s.poll
	}
	switch m.view {
	case viewLogs:
		return m.follow // logs are static unless following
	case viewGraph, viewJobs:
		return m.runStatus != "completed" // a finished run does not change
	case viewFavs:
		return false // cached ~30s; R refreshes. No continuous polling.
	}
	return true
}
