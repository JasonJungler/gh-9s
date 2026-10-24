package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type state int

const (
	stOther state = iota
	stOK
	stFail
	stRun
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

// gh runs the gh CLI in the current directory and returns stdout.
func gh(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return out, err
	}
	return out, nil
}

func ghJSON(v any, args ...string) error {
	out, err := gh(args...)
	if err != nil {
		// `gh pr checks` exits non-zero when checks fail but still prints JSON.
		if len(out) == 0 {
			return err
		}
	}
	return json.Unmarshal(out, v)
}

func repoInfo() (repo, branch string) {
	var r struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := ghJSON(&r, "repo", "view", "--json", "nameWithOwner"); err == nil {
		repo = r.NameWithOwner
	}
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	return
}

func age(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil || t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func runState(status, conclusion string) (string, state) {
	if status != "completed" {
		return "● " + status, stRun
	}
	switch conclusion {
	case "success":
		return "✓ success", stOK
	case "failure", "timed_out", "startup_failure":
		return "✗ " + conclusion, stFail
	default:
		return "○ " + conclusion, stOther
	}
}

func fetchRuns(r repoRef, workflowID string) (table, error) {
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
	if err := ghJSON(&runs, scoped(r, args...)...); err != nil {
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

func fetchWorkflows(r repoRef) (table, error) {
	var wfs []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
		Path  string `json:"path"`
	}
	if err := ghJSON(&wfs, scoped(r, "workflow", "list", "--all", "--json", "id,name,state,path")...); err != nil {
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

func fetchPRs(r repoRef, currentBranch string) (table, error) {
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
	if err := ghJSON(&prs, scoped(r, "pr", "list", "-L", "50", "--json",
		"number,title,author,headRefName,isDraft,reviewDecision,updatedAt,url,statusCheckRollup")...); err != nil {
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

func fetchChecks(r repoRef, pr string) (table, error) {
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
	if err := ghJSON(&checks, scoped(r, args...)...); err != nil {
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
	if err := ghJSON(&p, "pr", "view", "--json", "number"); err != nil {
		return "", err
	}
	return fmt.Sprint(p.Number), nil
}

func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func fetchJobs(r repoRef, runID string) (table, error) {
	var v struct {
		Jobs []struct {
			ID          int64  `json:"databaseId"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			Conclusion  string `json:"conclusion"`
			StartedAt   string `json:"startedAt"`
			CompletedAt string `json:"completedAt"`
			URL         string `json:"url"`
			Steps       []struct {
				Name       string `json:"name"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"steps"`
		} `json:"jobs"`
	}
	if err := ghJSON(&v, scoped(r, "run", "view", runID, "--json", "jobs")...); err != nil {
		return table{}, err
	}
	t := table{cols: []string{"ID", "STATUS", "JOB", "STEP", "DURATION"}}
	for _, j := range v.Jobs {
		label, st := runState(j.Status, j.Conclusion)
		// Show the step that is running, or the one that failed.
		step := "-"
		for _, s := range j.Steps {
			if s.Status == "in_progress" || s.Conclusion == "failure" {
				step = s.Name
				break
			}
		}
		id := fmt.Sprint(j.ID)
		t.rows = append(t.rows, row{id: id, state: st, url: j.URL, extra: j.Name,
			cells: []string{id, label, j.Name, step, duration(j.StartedAt, j.CompletedAt)}})
	}
	return t, nil
}

func duration(start, end string) string {
	s, err := time.Parse(time.RFC3339, start)
	if err != nil || s.IsZero() {
		return "-"
	}
	e, err := time.Parse(time.RFC3339, end)
	if err != nil || e.IsZero() || e.Before(s) {
		e = time.Now()
	}
	d := e.Sub(s).Round(time.Second)
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// fetchJobLog returns the log of one job as display lines, dropping the
// job-name column and timestamps that `gh run view --log` prefixes.
func fetchJobLog(r repoRef, runID, jobID string) ([]string, error) {
	out, err := gh(scoped(r, "run", "view", runID, "--job", jobID, "--log")...)
	if err != nil {
		return nil, err
	}
	return cleanLog(string(out)), nil
}

func cleanLog(s string) []string {
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		parts := strings.SplitN(l, "\t", 3)
		if len(parts) == 3 {
			msg := parts[2]
			if i := strings.IndexByte(msg, ' '); i > 0 && strings.HasSuffix(msg[:i], "Z") {
				msg = msg[i+1:]
			}
			l = parts[1] + "  " + msg
		}
		lines = append(lines, strings.ReplaceAll(l, "\t", "    "))
	}
	return lines
}
