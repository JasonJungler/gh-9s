package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

// triggerDeps are the side effects of "trigger then watch"; tests replace them.
var triggerDeps = struct {
	dispatch          func(args []string) error
	user              func(host string) (string, error)
	lister            func(user string) ghcli.RunLister
	sleep             func(time.Duration)
	now               func() time.Time
	timeout, interval time.Duration
}{
	dispatch: func(a []string) error { _, err := ghcli.Run(a...); return err },
	user:     ghcli.CurrentUser,
	lister:   ghcli.NewRunLister,
	sleep:    time.Sleep,
	now:      time.Now,
	timeout:  30 * time.Second,
	interval: 2 * time.Second,
}

// triggeredMsg reports the outcome of a dispatch and the search for its run.
type triggeredMsg struct {
	workflow   string
	target     ghcli.RepoRef
	runID      string
	dispatched bool
	err        error
}

// triggerAndFind dispatches the form's workflow, then looks for the run it created.
// The dispatch always happens; failing to identify the run is reported, not fatal.
func triggerAndFind(f triggerForm) func() triggeredMsg {
	return func() triggeredMsg {
		d := triggerDeps
		msg := triggeredMsg{workflow: f.workflow, target: f.target}
		since := d.now()
		user, userErr := d.user(f.target.Host)
		if err := d.dispatch(f.args()); err != nil {
			msg.err = err
			return msg
		}
		msg.dispatched = true
		if userErr != nil {
			msg.err = fmt.Errorf("cannot look up your login: %w", userErr)
			return msg
		}
		id, err := ghcli.FindTriggeredRun(d.lister(user), f.target, f.workflow, f.ref, user, since, d.timeout, d.interval, d.sleep)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.runID = fmt.Sprint(id)
		return msg
	}
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
		tgt, file, name = f.Target(), f.Workflow, f.Workflow
		if tgt == m.curRepo {
			tgt = ghcli.RepoRef{}
		}
	}
	local := ""
	if tgt == (ghcli.RepoRef{}) {
		local = m.branch
	}
	m.status = styDim.Render("loading " + file + "…")
	return m, loadFormCmd(tgt, file, name, local)
}

func readInputs(tgt ghcli.RepoRef, file, ref string) ([]workflow.DispatchInput, bool, error) {
	src, err := ghcli.FetchWorkflowFile(tgt, workflowDir+file, ref)
	if err != nil {
		return nil, false, err
	}
	return workflow.ParseDispatchInputs(src)
}

func loadFormCmd(tgt ghcli.RepoRef, file, name, localBranch string) tea.Cmd {
	return func() tea.Msg {
		refs, err := ghcli.FetchBranches(tgt)
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
