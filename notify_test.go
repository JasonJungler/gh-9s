package main

import (
	"strings"
	"testing"
)

func TestRunFinished(t *testing.T) {
	cases := []struct {
		prev, cur string
		want      bool
	}{
		{"in_progress", "completed", true},
		{"queued", "completed", true},
		{"completed", "completed", false},
		{"", "completed", false},
		{"in_progress", "in_progress", false},
		{"in_progress", "", false},
	}
	for _, c := range cases {
		if got := runFinished(c.prev, c.cur); got != c.want {
			t.Errorf("runFinished(%q,%q) = %v; want %v", c.prev, c.cur, got, c.want)
		}
	}
}

func TestRunOutcome(t *testing.T) {
	cases := map[string]string{
		"success": "succeeded", "failure": "failed", "cancelled": "was cancelled",
		"timed_out": "timed out", "": "finished", "skipped": "skipped",
	}
	for in, want := range cases {
		if got := runOutcome(in); got != want {
			t.Errorf("runOutcome(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestOsascriptEscapesQuotes(t *testing.T) {
	got := strings.Join(osascriptArgs(`say "hi"`, `a\b`), " ")
	if !strings.Contains(got, `\"hi\"`) || !strings.Contains(got, `a\\b`) {
		t.Errorf("args not escaped: %s", got)
	}
}

func recordNotifications(t *testing.T) *[]string {
	t.Helper()
	var got []string
	old := notifier
	notifier = func(title, body string) { got = append(got, title+"|"+body) }
	t.Cleanup(func() { notifier = old })
	return &got
}

var twoJobSpecs = []jobSpec{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}}

func feedGraph(m model, jobs []runJob, status, conclusion string) model {
	g := buildGraph(jobs, twoJobSpecs)
	next, cmd := m.Update(dataMsg{view: viewGraph, graph: &g, jobs: jobs, run: runMeta{status, conclusion}})
	if cmd != nil {
		cmd()
	}
	return next.(model)
}

func TestGraphNotifiesOnceWhenRunFinishes(t *testing.T) {
	got := recordNotifications(t)
	m := graphModel()
	m.runStatus = ""
	m = feedGraph(m, []runJob{ok("a"), job("b", "in_progress", "")}, "in_progress", "")
	if len(*got) != 0 {
		t.Fatalf("notified while still running: %v", *got)
	}
	m = feedGraph(m, []runJob{ok("a"), ok("b")}, "completed", "success")
	if len(*got) != 1 || !strings.Contains((*got)[0], "succeeded") || !strings.Contains((*got)[0], "42") {
		t.Fatalf("notifications = %v; want one mentioning run 42 and 'succeeded'", *got)
	}
	feedGraph(m, []runJob{ok("a"), ok("b")}, "completed", "success")
	if len(*got) != 1 {
		t.Errorf("repeat refresh of a finished run notified again: %v", *got)
	}
}

// Between two stages every *listed* job can be completed while the run is still going.
func TestNoNotificationWhileRunStillInProgress(t *testing.T) {
	got := recordNotifications(t)
	m := feedGraph(graphModel(), []runJob{ok("a"), job("b", "in_progress", "")}, "in_progress", "")
	m = feedGraph(m, []runJob{ok("a"), ok("b")}, "in_progress", "") // stage gap: all listed jobs done
	if len(*got) != 0 {
		t.Errorf("notified in the gap between stages: %v", *got)
	}
	if !m.shouldPoll() {
		t.Error("must keep polling while the run is in progress")
	}
}

func TestCancelledRunIsReportedAsCancelled(t *testing.T) {
	got := recordNotifications(t)
	m := feedGraph(graphModel(), []runJob{job("a", "in_progress", ""), ok("b")}, "in_progress", "")
	feedGraph(m, []runJob{job("a", "completed", "cancelled"), ok("b")}, "completed", "cancelled")
	if len(*got) != 1 || !strings.Contains((*got)[0], "cancelled") || strings.Contains((*got)[0], "succeeded") {
		t.Errorf("notifications = %v", *got)
	}
}

func TestFlatFallbackStillNotifies(t *testing.T) {
	got := recordNotifications(t)
	m := graphModel()
	flat := flatGraph([]runJob{ok("x")}, "why")
	next, _ := m.Update(dataMsg{view: viewGraph, graph: &flat, jobs: []runJob{job("x", "in_progress", "")}, run: runMeta{"in_progress", ""}})
	m = next.(model)
	if m.view != viewJobs {
		t.Fatalf("view = %v", m.view)
	}
	next, cmd := m.Update(dataMsg{view: viewJobs, tbl: jobsTable([]runJob{ok("x")}), run: runMeta{"completed", "failure"}})
	if cmd != nil {
		cmd()
	}
	if len(*got) != 1 || !strings.Contains((*got)[0], "failed") {
		t.Errorf("notifications = %v; want one 'failed' from the jobs-list fallback", *got)
	}
}

func TestLogViewKeepsWatchingRun(t *testing.T) {
	got := recordNotifications(t)
	m := logModel()
	m.runStatus = "in_progress"
	next, cmd := m.Update(runMetaMsg{runID: "1", meta: runMeta{"completed", "success"}})
	if cmd != nil {
		cmd()
	}
	if len(*got) != 1 {
		t.Errorf("notifications = %v; want one while reading a job's log", *got)
	}
	_ = next
}
