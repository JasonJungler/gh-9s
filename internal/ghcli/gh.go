// Package ghcli is the only package that talks to GitHub, by running the gh CLI.
// Authentication, hosts and Enterprise support all come from gh itself.
package ghcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/MurabitoB/gh-9s/internal/graph"
)

// Run runs the gh CLI in the current directory and returns stdout.
func Run(args ...string) ([]byte, error) { return RunHost("", args...) }

// RunHost is Run against another host: GH_HOST is set for commands that have no --hostname flag.
// An empty host or github.com leaves gh's own choice alone.
func RunHost(host string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if host != "" && host != "github.com" {
		cmd.Env = append(os.Environ(), "GH_HOST="+host)
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return out, err
	}
	return out, nil
}

// RunInput is Run with stdin: used for requests whose body is JSON, so no text travels in an argument.
func RunInput(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stdin = bytes.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return out, err
	}
	return out, nil
}

// RunWithTimeout is Run that gives up (and kills gh) after d: for calls to hosts that may not answer.
func RunWithTimeout(d time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("gh did not answer within %s", d)
		}
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return out, err
	}
	return out, nil
}

// JSON runs gh and decodes its stdout. A non-zero exit that still printed JSON (as `gh pr checks` does) is not an error.
func JSON(v any, args ...string) error {
	out, err := Run(args...)
	if err != nil {
		// `gh pr checks` exits non-zero when checks fail but still prints JSON.
		if len(out) == 0 {
			return err
		}
	}
	return json.Unmarshal(out, v)
}

// LocalRepo returns the current directory's OWNER/REPO and checked-out git branch.
func LocalRepo() (repo, branch string) {
	var r struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := JSON(&r, "repo", "view", "--json", "nameWithOwner"); err == nil {
		repo = r.NameWithOwner
	}
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	return
}

// FetchJobLog returns the log of one job as display lines, dropping the
// job-name column and timestamps that `gh run view --log` prefixes.
func FetchJobLog(r RepoRef, runID, jobID string) ([]string, error) {
	out, err := Run(Scoped(r, "run", "view", runID, "--job", jobID, "--log")...)
	if err != nil {
		return nil, err
	}
	return CleanLog(string(out)), nil
}

// CleanLog drops the job-name column and timestamps that gh prefixes to each log line.
func CleanLog(s string) []string {
	s = CleanText(s) // job output can hold colour codes and worse; show the words only
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

// wfPaths remembers workflow file paths by repo and workflow ID, so refreshing a
// live graph does not list workflows every few seconds.
var wfPaths sync.Map

// isActiveStep marks the step worth showing next to a job: the one running or the one that failed.
func isActiveStep(status, conclusion string) bool {
	return status == "in_progress" || conclusion == "failure"
}

// FetchRunJobs returns a run's jobs together with the commit and workflow file
// path they ran from (needed to read the job dependencies).
func FetchRunJobs(r RepoRef, runID string) (jobs []graph.Job, meta RunMeta, headSHA, workflowPath string, err error) {
	var v struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		HeadSHA    string `json:"headSha"`
		WorkflowID int64  `json:"workflowDatabaseId"`
		Jobs       []struct {
			ID          int64     `json:"databaseId"`
			Name        string    `json:"name"`
			Status      string    `json:"status"`
			Conclusion  string    `json:"conclusion"`
			StartedAt   time.Time `json:"startedAt"`
			CompletedAt time.Time `json:"completedAt"`
			URL         string    `json:"url"`
			Steps       []struct {
				Name       string `json:"name"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"steps"`
		} `json:"jobs"`
	}
	if err = JSON(&v, Scoped(r, "run", "view", runID, "--json", "jobs,headSha,workflowDatabaseId,status,conclusion")...); err != nil {
		return nil, RunMeta{}, "", "", err
	}
	meta = RunMeta{v.Status, v.Conclusion}
	for _, j := range v.Jobs {
		rj := graph.Job{ID: fmt.Sprint(j.ID), Name: j.Name, Status: j.Status, Conclusion: j.Conclusion,
			URL: j.URL, Start: j.StartedAt, End: j.CompletedAt}
		for _, s := range j.Steps {
			if isActiveStep(s.Status, s.Conclusion) {
				rj.Step = s.Name
				break
			}
		}
		jobs = append(jobs, rj)
	}
	key := fmt.Sprintf("%s#%d", r, v.WorkflowID)
	if p, ok := wfPaths.Load(key); ok {
		return jobs, meta, v.HeadSHA, p.(string), nil
	}
	var wfs []struct {
		ID   int64  `json:"id"`
		Path string `json:"path"`
	}
	if err = JSON(&wfs, Scoped(r, "workflow", "list", "--all", "--json", "id,path")...); err != nil {
		return jobs, meta, v.HeadSHA, "", err
	}
	for _, w := range wfs {
		if w.ID == v.WorkflowID {
			workflowPath = w.Path
			wfPaths.Store(key, w.Path)
		}
	}
	return jobs, meta, v.HeadSHA, workflowPath, nil
}

// RunMeta is the run's own status and conclusion (not derived from its jobs: between
// two stages every listed job can be completed while the run is still going).
type RunMeta struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// FetchRunMeta reads a run's own status and conclusion.
func FetchRunMeta(r RepoRef, runID string) (RunMeta, error) {
	var v RunMeta
	err := JSON(&v, Scoped(r, "run", "view", runID, "--json", "status,conclusion")...)
	return v, err
}
