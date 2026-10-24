package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

type row struct {
	id    string
	cells []string
	state state
	url   string
	extra string // view specific: workflow id, PR number, ...
}

type table struct {
	cols []string
	rows []row
}

func fetchRuns(r ghcli.RepoRef, workflowID string) (table, error) {
	var runs []struct {
		ID         int64  `json:"databaseId"`
		Title      string `json:"displayTitle"`
		Workflow   string `json:"workflowName"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		Branch     string `json:"headBranch"`
		Event      string `json:"event"`
		CreatedAt  string `json:"createdAt"`
		URL        string `json:"url"`
	}
	args := []string{"run", "list", "-L", "50", "--json",
		"databaseId,displayTitle,workflowName,status,conclusion,headBranch,event,createdAt,url"}
	if workflowID != "" {
		args = append(args, "-w", workflowID)
	}
	if err := ghcli.JSON(&runs, ghcli.Scoped(r, args...)...); err != nil {
		return table{}, err
	}
	t := table{cols: []string{"ID", "STATUS", "WORKFLOW", "TITLE", "BRANCH", "EVENT", "AGE"}}
	for _, r := range runs {
		label, st := runState(r.Status, r.Conclusion)
		id := fmt.Sprint(r.ID)
		t.rows = append(t.rows, row{id: id, state: st, url: r.URL,
			cells: []string{id, label, r.Workflow, r.Title, r.Branch, r.Event, age(r.CreatedAt)}})
	}
	return t, nil
}

func fetchWorkflows(r ghcli.RepoRef) (table, error) {
	var wfs []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
		Path  string `json:"path"`
	}
	if err := ghcli.JSON(&wfs, ghcli.Scoped(r, "workflow", "list", "--all", "--json", "id,name,state,path")...); err != nil {
		return table{}, err
	}
	t := table{cols: []string{"ID", "NAME", "STATE", "PATH"}}
	for _, w := range wfs {
		st := stOK
		if w.State != "active" {
			st = stOther
		}
		id := fmt.Sprint(w.ID)
		t.rows = append(t.rows, row{id: id, state: st, extra: w.Name,
			cells: []string{id, w.Name, w.State, w.Path}})
	}
	return t, nil
}

func rollupState(checks []struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}) (string, state) {
	res, st := "-", stOther
	if len(checks) == 0 {
		return res, st
	}
	ok := 0
	for _, c := range checks {
		s, c2 := strings.ToUpper(c.Status), strings.ToUpper(c.Conclusion)
		if c.State != "" { // StatusContext
			s, c2 = "COMPLETED", strings.ToUpper(c.State)
		}
		switch {
		case s != "COMPLETED" || c2 == "PENDING" || c2 == "EXPECTED":
			return fmt.Sprintf("● pending %d/%d", ok, len(checks)), stRun
		case c2 == "FAILURE" || c2 == "ERROR" || c2 == "TIMED_OUT" || c2 == "CANCELLED":
			return "✗ failing", stFail
		default:
			ok++
		}
	}
	return fmt.Sprintf("✓ %d/%d", ok, len(checks)), stOK
}

// prListArgs is the `gh pr list` command for r, narrowed by f.
func prListArgs(r ghcli.RepoRef, f ghcli.PRFilter) []string {
	args := append([]string{"pr", "list", "-L", "50", "--json",
		"number,title,author,headRefName,isDraft,reviewDecision,updatedAt,url,statusCheckRollup"}, ghcli.PRFilterArgs(f)...)
	return ghcli.Scoped(r, args...)
}

// issueListArgs is the `gh issue list` command for r, narrowed by f.
func issueListArgs(r ghcli.RepoRef, f ghcli.IssueFilter) []string {
	args := append([]string{"issue", "list", "-L", "50", "--json",
		"number,title,author,state,labels,assignees,updatedAt,url"}, ghcli.IssueFilterArgs(f)...)
	return ghcli.Scoped(r, args...)
}

func fetchIssues(r ghcli.RepoRef, f ghcli.IssueFilter) (table, error) {
	var issues []struct {
		Number    int                     `json:"number"`
		Title     string                  `json:"title"`
		Author    *struct{ Login string } `json:"author"`
		State     string                  `json:"state"`
		UpdatedAt string                  `json:"updatedAt"`
		URL       string                  `json:"url"`
		Labels    []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
	}
	if err := ghcli.JSON(&issues, issueListArgs(r, f)...); err != nil {
		return table{}, err
	}
	t := table{cols: []string{"#", "STATE", "TITLE", "AUTHOR", "LABELS", "ASSIGNEES", "UPDATED"}}
	for _, i := range issues {
		author := "ghost"
		if i.Author != nil && i.Author.Login != "" {
			author = i.Author.Login
		}
		var labels, assignees []string
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		for _, a := range i.Assignees {
			assignees = append(assignees, a.Login)
		}
		st := stOK
		if i.State != "OPEN" {
			st = stOther
		}
		t.rows = append(t.rows, row{id: fmt.Sprint(i.Number), state: st, url: i.URL,
			cells: []string{fmt.Sprint(i.Number), i.State, i.Title, author,
				orDash(strings.Join(labels, ",")), orDash(strings.Join(assignees, ",")), age(i.UpdatedAt)}})
	}
	return t, nil
}

func fetchPRs(r ghcli.RepoRef, currentBranch string, f ghcli.PRFilter) (table, error) {
	var prs []struct {
		Number       int                    `json:"number"`
		Title        string                 `json:"title"`
		Author       struct{ Login string } `json:"author"`
		Head         string                 `json:"headRefName"`
		Draft        bool                   `json:"isDraft"`
		Review       string                 `json:"reviewDecision"`
		UpdatedAt    string                 `json:"updatedAt"`
		URL          string                 `json:"url"`
		StatusRollup []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := ghcli.JSON(&prs, prListArgs(r, f)...); err != nil {
		return table{}, err
	}
	t := table{cols: []string{"#", "CHECKS", "TITLE", "AUTHOR", "BRANCH", "REVIEW", "UPDATED"}}
	for _, p := range prs {
		label, st := rollupState(p.StatusRollup)
		num := fmt.Sprint(p.Number)
		title := p.Title
		if p.Draft {
			title = "[draft] " + title
		}
		if p.Head == currentBranch {
			num = "*" + num
		}
		review := p.Review
		if review == "" {
			review = "-"
		}
		t.rows = append(t.rows, row{id: fmt.Sprint(p.Number), state: st, url: p.URL,
			cells: []string{num, label, title, p.Author.Login, p.Head, review, age(p.UpdatedAt)}})
	}
	return t, nil
}

func fetchChecks(r ghcli.RepoRef, pr string) (table, error) {
	var checks []struct {
		Name     string `json:"name"`
		State    string `json:"state"`
		Bucket   string `json:"bucket"`
		Workflow string `json:"workflow"`
		Link     string `json:"link"`
		Started  string `json:"startedAt"`
	}
	args := []string{"pr", "checks", "--json", "name,state,bucket,workflow,link,startedAt"}
	if pr != "" {
		args = append(args[:2], append([]string{pr}, args[2:]...)...)
	}
	if err := ghcli.JSON(&checks, ghcli.Scoped(r, args...)...); err != nil {
		return table{}, err
	}
	t := table{cols: []string{"STATUS", "NAME", "WORKFLOW", "STATE", "AGE"}}
	for _, c := range checks {
		st, label := stOther, "○ "+c.Bucket
		switch c.Bucket {
		case "pass":
			st, label = stOK, "✓ pass"
		case "fail":
			st, label = stFail, "✗ fail"
		case "pending":
			st, label = stRun, "● pending"
		}
		t.rows = append(t.rows, row{id: c.Name, state: st, url: c.Link,
			cells: []string{label, c.Name, c.Workflow, c.State, age(c.Started)}})
	}
	return t, nil
}

func currentPR() (string, error) {
	var p struct {
		Number int `json:"number"`
	}
	if err := ghcli.JSON(&p, "pr", "view", "--json", "number"); err != nil {
		return "", err
	}
	return fmt.Sprint(p.Number), nil
}

func fetchJobs(r ghcli.RepoRef, runID string) (table, error) {
	jobs, _, _, _, err := ghcli.FetchRunJobs(r, runID)
	if err != nil {
		return table{}, err
	}
	return jobsTable(jobs), nil
}

// jobsTable lists a run's jobs; it is the fallback when no graph can be drawn.
func jobsTable(jobs []graph.Job) table {
	t := table{cols: []string{"ID", "STATUS", "JOB", "STEP", "DURATION"}}
	for _, j := range jobs {
		label, st := j.State()
		step := j.Step
		if step == "" {
			step = "-"
		}
		end := ""
		if !j.End.IsZero() {
			end = j.End.Format(time.RFC3339)
		}
		start := ""
		if !j.Start.IsZero() {
			start = j.Start.Format(time.RFC3339)
		}
		t.rows = append(t.rows, row{id: j.ID, state: st, url: j.URL, extra: j.Name,
			cells: []string{j.ID, label, j.Name, step, duration(start, end)}})
	}
	return t
}
