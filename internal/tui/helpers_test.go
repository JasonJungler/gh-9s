package tui

import (
	"time"

	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/workflow"
)

// Shared fixtures for the tui tests.

var t0 = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func at(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

func job(name, status, conclusion string) graph.Job {
	return graph.Job{ID: name, Name: name, Status: status, Conclusion: conclusion}
}

func ok(name string) graph.Job { return job(name, "completed", "success") }

// fixture is the spec's example run: lint,test -> build -> deploy-staging,e2e -> release.
func fixture() graph.Graph {
	mk := func(name, status, concl, step string, start, end int) graph.Job {
		j := graph.Job{ID: name, Name: name, Status: status, Conclusion: concl, Step: step}
		if start >= 0 {
			j.Start = at(start)
		}
		if end >= 0 {
			j.End = at(end)
		}
		return j
	}
	jobs := []graph.Job{
		mk("lint", "completed", "success", "", 0, 12),
		mk("test", "completed", "success", "", 0, 48),
		mk("build", "completed", "success", "", 48, 112),
		mk("deploy-staging", "in_progress", "", "Apply manifests", 112, -1),
		mk("e2e", "in_progress", "", "Run playwright", 112, -1),
		mk("release", "queued", "", "", -1, -1),
	}
	specs := []workflow.JobSpec{
		{ID: "lint", Name: "lint"}, {ID: "test", Name: "test"},
		{ID: "build", Name: "build", Needs: []string{"lint", "test"}},
		{ID: "deploy-staging", Name: "deploy-staging", Needs: []string{"build"}},
		{ID: "e2e", Name: "e2e", Needs: []string{"build"}},
		{ID: "release", Name: "release", Needs: []string{"deploy-staging", "e2e"}},
	}
	return graph.Build(jobs, specs)
}
