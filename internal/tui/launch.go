package tui

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/launch"
)

// Sending an issue to a coding agent: a on the issue list or the issue screen opens the launch
// dialog (launchdialog.go) and enter runs the launch in the background. Nothing is created before
// that enter.

// Outside calls the launch makes; tests replace them.
var (
	launcher = launch.Launcher{Run: launch.ExecRun, Getenv: os.Getenv, Fetch: ghcli.FetchBase}
	// launchDo runs a launch; it is launcher.Do unless a test fakes it.
	launchDo            = func(r launch.Request) (launch.Result, error) { return launcher.Do(r) }
	launchModels        = launch.ListModels
	launchDir           = os.Getwd
	launchDefaultBranch = ghcli.DefaultBranch
	launchPrefsPath     = func() string {
		if d := configDir(); d != "" {
			return filepath.Join(d, "launch.json")
		}
		return ""
	}
)

// launchIssueMsg carries the issue fetched for a launch from the list.
type launchIssueMsg struct {
	id  string // m.itemID of the row a was pressed on; an answer for another row is dropped
	d   ghcli.IssueDetail
	err error
}

// launchModelsMsg carries the models of an agent for the open dialog.
type launchModelsMsg struct {
	seq    int // the dialog it was asked for (model.launchSeq)
	agent  string
	models []launch.Model
}

// launchBaseMsg carries the default Orca base for the open dialog.
type launchBaseMsg struct {
	seq  int
	base string
}

// launchDoneMsg says how a launch ended.
type launchDoneMsg struct {
	seq          int
	agent, model string // what was launched, remembered on success
	res          launch.Result
	err          error
}

// loadLaunchPrefs reads the remembered models and words any problem for the banner. The prefs are
// never nil.
func loadLaunchPrefs() (*launch.Prefs, string) {
	p, err := launch.LoadPrefs(launchPrefsPath())
	if err != nil {
		return p, err.Error()
	}
	return p, ""
}

// launchRefusal is the target to launch in, or the status line that says why there is none: the
// screen shows another repo than the one gh-9s runs in, or no supported environment was found.
func (m model) launchRefusal(override string) (launch.Target, string) {
	if !sameRepo(m.curTarget(), m.curRepo) {
		here := displayRef(m.curRepo)
		if here == "" {
			here = "unknown"
		}
		return "", styDim.Render("sending to an agent only works for the current directory's repo (" + here + "), not " + displayRef(m.curTarget()))
	}
	t, err := launch.Detect(envGet, override)
	if err != nil {
		return "", styErr.Render("✗ " + err.Error())
	}
	return t, ""
}

// launchFromList is a on a list row: the row has only the number, so the whole issue is fetched first.
func (m model) launchFromList(number string) (model, tea.Cmd) {
	if _, why := m.launchRefusal(m.cfg().String("launch_target")); why != "" {
		m.status = why
		return m, nil
	}
	m.status = styDim.Render("loading the issue…")
	tg, id := m.target, m.itemID(number)
	return m, func() tea.Msg {
		d, err := fetchIssue(tg, number)
		return launchIssueMsg{id: id, d: d, err: err}
	}
}

// launchFromIssue is a on the issue screen, which has the issue already.
func (m model) launchFromIssue(d ghcli.IssueDetail) (model, tea.Cmd) {
	t, why := m.launchRefusal(m.cfg().String("launch_target"))
	switch {
	case why != "":
		m.status = why
		return m, nil
	case d.State == "":
		m.status = styDim.Render("still loading the issue")
		return m, nil
	}
	return m.openLaunch(d, t)
}

// launchIssueLoaded opens the dialog for the fetched issue, if the list still stands on its row and
// nothing else is open.
func (m model) launchIssueLoaded(msg launchIssueMsg) (model, tea.Cmd) {
	r, ok := m.selected()
	if m.view != viewIssues || !ok || m.itemID(r.id) != msg.id || m.mode != modeNormal ||
		m.dlg != nil || m.picker != nil || m.form != nil || m.launchDlg != nil || m.helpOpen {
		return m, nil
	}
	if msg.err != nil {
		m.status = styErr.Render("✗ " + msg.err.Error())
		return m, nil
	}
	t, why := m.launchRefusal(m.cfg().String("launch_target"))
	if why != "" {
		m.status = why
		return m, nil
	}
	return m.openLaunch(msg.d, t)
}

// openLaunch shows the dialog for d at once, filled from the settings and the remembered model, and
// starts loading the agent's models and (for Orca) the default base.
func (m model) openLaunch(d ghcli.IssueDetail, t launch.Target) (model, tea.Cmd) {
	dir, err := launchDir()
	if err != nil {
		m.status = styErr.Render("✗ cannot tell the working directory: " + err.Error())
		return m, nil
	}
	cfg := m.cfg()
	agent := cfg.String("launch_agent")
	dlg := newLaunchDialog(launchInput{
		Issue:      d,
		Target:     t,
		AutoTarget: cfg.String("launch_target") == "auto",
		Agent:      agent,
		Model:      m.launchPrefs.Model(agent),
		Models:     []launch.Model{{}},
		Name:       launch.BranchName(d.Number, d.Title),
		Place:      launch.Place(cfg.String("launch_tmux_place")),
		Split:      launch.Split(cfg.String("launch_split")),
		ContextLen: utf8.RuneCountInString(launch.BuildContext(d)),
	})
	m.launchSeq++
	m.launchDlg, m.launchWD, m.status = &dlg, dir, ""
	cmds := []tea.Cmd{launchModelsCmd(m.launchSeq, agent)}
	if t == launch.TargetOrca {
		seq, run, branch := m.launchSeq, launcher.Run, launchDefaultBranch
		cmds = append(cmds, func() tea.Msg {
			base, ok := launch.OrcaDefaultBase(run, dir)
			if !ok {
				base = "origin/" + branch(dir)
			}
			return launchBaseMsg{seq: seq, base: base}
		})
	}
	return m, tea.Batch(cmds...)
}

// launchModelsCmd lists the models of an agent in the background (Devin's come from a command).
func launchModelsCmd(seq int, agentID string) tea.Cmd {
	a, ok := launch.LookupAgent(agentID)
	if !ok {
		return nil
	}
	list, run := launchModels, launcher.Run
	return func() tea.Msg { return launchModelsMsg{seq: seq, agent: agentID, models: list(a, run)} }
}

// launchKey gives every key to the open dialog and does what it asks.
func (m model) launchKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	d, act := m.launchDlg.update(k)
	switch act {
	case actCancel:
		m.launchDlg = nil
		m.status = styDim.Render("not sent")
		return m, nil
	case actAgentChanged:
		d.in.Model = m.launchPrefs.Model(d.in.Agent)
		m.launchDlg = &d
		return m, launchModelsCmd(m.launchSeq, d.in.Agent)
	case actSubmit:
		req := d.request()
		req.Context = launch.BuildContext(d.in.Issue)
		req.Dir = m.launchWD
		d = d.start()
		m.launchDlg = &d
		seq, do := m.launchSeq, launchDo
		return m, func() tea.Msg {
			res, err := do(req)
			return launchDoneMsg{seq: seq, agent: req.Agent.ID, model: req.Model, res: res, err: err}
		}
	}
	m.launchDlg = &d
	return m, nil
}

// launchOpen reports whether the dialog that seq was asked for is still the one on screen.
func (m model) launchOpen(seq int) bool { return m.launchDlg != nil && seq == m.launchSeq }

// launchModelsLoaded puts the models in the dialog, unless it was closed or shows another agent now.
func (m model) launchModelsLoaded(msg launchModelsMsg) model {
	if !m.launchOpen(msg.seq) || m.launchDlg.in.Agent != msg.agent {
		return m
	}
	d := m.launchDlg.withModels(msg.models)
	m.launchDlg = &d
	return m
}

// launchBaseLoaded fills in the default base, unless the dialog was closed.
func (m model) launchBaseLoaded(msg launchBaseMsg) model {
	if !m.launchOpen(msg.seq) {
		return m
	}
	d := m.launchDlg.withBase(msg.base)
	m.launchDlg = &d
	return m
}

// launchDone closes the dialog on success and remembers the agent and model; a failure stays in the
// dialog with its step. A result for a dialog that is gone only reports a success.
func (m model) launchDone(msg launchDoneMsg) model {
	current := m.launchOpen(msg.seq)
	if msg.err != nil {
		if current {
			d := m.launchDlg.failed(msg.err)
			m.launchDlg = &d
		}
		return m
	}
	status := styOK.Render("✓ launched: " + strings.TrimSpace(msg.res.Where+" "+msg.res.Branch))
	if msg.res.Note != "" {
		status += styDim.Render(" · " + msg.res.Note)
	}
	if current {
		m.launchDlg = nil
		status += m.rememberLaunch(msg.agent, msg.model)
	}
	m.status = status
	return m
}

// rememberLaunch keeps the agent in the settings and its model in the launch prefs. A failure to
// save is only a note: the launch itself worked.
func (m model) rememberLaunch(agent, model string) string {
	var notes []string
	if _, err := m.cfg().Set("launch_agent", agent); err != nil {
		notes = append(notes, "agent not remembered: "+err.Error())
	}
	m.launchPrefs.SetModel(agent, model)
	if err := m.launchPrefs.Save(); err != nil {
		notes = append(notes, "model not remembered: "+err.Error())
	}
	if len(notes) == 0 {
		return ""
	}
	return styErr.Render(" · " + strings.Join(notes, "; "))
}
