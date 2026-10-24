package tui

import (
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) fetch() tea.Cmd {
	v, wf, pr, br, run, job, tg := m.view, m.wfID, m.pr, m.branch, m.runID, m.jobID, m.target
	var pinned []favorites.Favorite
	var snap *favorites.Store
	if m.favs != nil {
		pinned = m.favs.ForRepo(m.curTarget())
		snap = &favorites.Store{Items: append([]favorites.Favorite(nil), m.favs.Items...)}
	}
	cache := m.latest
	specCache := m.specs
	return func() tea.Msg {
		var t table
		var latest map[string]latestEntry
		var gr *graph.Graph
		var gjobs []graph.Job
		var meta ghcli.RunMeta
		var newSpecs map[string][]workflow.JobSpec
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
			var jobs []graph.Job
			if jobs, meta, _, _, err = ghcli.FetchRunJobs(tg, run); err == nil {
				t = jobsTable(jobs)
			}
		case viewLogs:
			logs, err = ghcli.FetchJobLog(tg, run, job)
		case viewGraph:
			var sha, path string
			gjobs, meta, sha, path, err = ghcli.FetchRunJobs(tg, run)
			if err != nil {
				break
			}
			sp, cached := specCache[run]
			if !cached && path != "" {
				if src, e := ghcli.FetchWorkflowFile(tg, path, sha); e == nil {
					if parsed, e := workflow.ParseJobSpecs(src); e == nil {
						sp, newSpecs = parsed, map[string][]workflow.JobSpec{run: parsed}
					}
				}
			}
			g := graph.Build(gjobs, sp)
			gr = &g
		case viewFavs:
			if snap != nil {
				t, latest = fetchFavorites(snap, cache, time.Now())
			}
		}
		return dataMsg{view: v, tbl: t, logs: logs, latest: latest, graph: gr, jobs: gjobs, specs: newSpecs, run: meta, err: err}
	}
}

type runMetaMsg struct {
	runID string
	meta  ghcli.RunMeta
	err   error
}

func runMetaCmd(tg ghcli.RepoRef, runID string) tea.Cmd {
	return func() tea.Msg {
		meta, err := ghcli.FetchRunMeta(tg, runID)
		return runMetaMsg{runID, meta, err}
	}
}

func runAction(done string, args ...string) tea.Cmd {
	return func() tea.Msg {
		_, err := ghcli.Run(args...)
		return actionMsg{msg: done, err: err}
	}
}

// pagerCommand shows a run's log in the pager. The values are passed to the shell
// as positional arguments, never pasted into the script.
func pagerCommand(tg ghcli.RepoRef, id string, failedOnly bool) *exec.Cmd {
	flag := "--log"
	if failedOnly {
		flag = "--log-failed"
	}
	args := ghcli.Scoped(tg, "run", "view", id, flag)
	return exec.Command("sh", append([]string{"-c", `gh "$@" 2>&1 | ${PAGER:-less -R}`, "sh"}, args...)...)
}

func logsCmd(tg ghcli.RepoRef, id string, failedOnly bool) tea.Cmd {
	return tea.ExecProcess(pagerCommand(tg, id, failedOnly), func(error) tea.Msg { return actionMsg{msg: "closed logs"} })
}
