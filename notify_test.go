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

func TestRunStatusAndOutcome(t *testing.T) {
	running := []runJob{ok("a"), job("b", "in_progress", "")}
	done := []runJob{ok("a"), ok("b")}
	failed := []runJob{ok("a"), job("b", "completed", "failure")}
	if runStatus(nil) != "" || runStatus(running) != "in_progress" || runStatus(done) != "completed" {
		t.Errorf("runStatus: %q %q %q", runStatus(nil), runStatus(running), runStatus(done))
	}
	if runOutcome(done) != "succeeded" || runOutcome(failed) != "failed" {
		t.Errorf("runOutcome: %q %q", runOutcome(done), runOutcome(failed))
	}
}

func TestOsascriptEscapesQuotes(t *testing.T) {
	got := strings.Join(osascriptArgs(`say "hi"`, `a\b`), " ")
	if !strings.Contains(got, `\"hi\"`) || !strings.Contains(got, `a\\b`) {
		t.Errorf("args not escaped: %s", got)
	}
}

func TestGraphNotifiesOnceWhenRunFinishes(t *testing.T) {
	var got []string
	old := notifier
	notifier = func(title, body string) { got = append(got, title+"|"+body) }
	defer func() { notifier = old }()

	m := graphModel()
	m.runStatus = ""
	feed := func(m model, jobs []runJob) (model, bool) {
		g := buildGraph(jobs, []jobSpec{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}})
		next, cmd := m.Update(dataMsg{view: viewGraph, graph: &g, jobs: jobs})
		if cmd != nil {
			cmd()
		}
		return next.(model), cmd != nil
	}
	m, _ = feed(m, []runJob{ok("a"), job("b", "in_progress", "")})
	if len(got) != 0 {
		t.Fatalf("notified while still running: %v", got)
	}
	m, _ = feed(m, []runJob{ok("a"), ok("b")})
	if len(got) != 1 || !strings.Contains(got[0], "succeeded") || !strings.Contains(got[0], "42") {
		t.Fatalf("notifications = %v; want one mentioning run 42 and 'succeeded'", got)
	}
	feed(m, []runJob{ok("a"), ok("b")})
	if len(got) != 1 {
		t.Errorf("repeat refresh of a finished run notified again: %v", got)
	}
}
