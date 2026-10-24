package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type fakeTrigger struct {
	dispatched [][]string
	runs       []ghcli.RunInfo
}

func (f *fakeTrigger) install(t *testing.T) {
	t.Helper()
	old := triggerDeps
	t.Cleanup(func() { triggerDeps = old })
	triggerDeps.dispatch = func(a []string) error { f.dispatched = append(f.dispatched, a); return nil }
	triggerDeps.user = func(string) (string, error) { return "me", nil }
	triggerDeps.lister = func(string) ghcli.RunLister {
		return func(ghcli.RepoRef, string, string) ([]ghcli.RunInfo, error) { return f.runs, nil }
	}
	triggerDeps.sleep = func(time.Duration) {}
	triggerDeps.now = func() time.Time { return t0 }
	triggerDeps.timeout, triggerDeps.interval = 4*time.Second, 2*time.Second
}

func formOpenModel(t *testing.T) model {
	m := favModel(t)
	f := newTriggerForm(ghcli.RepoRef{}, "deploy.yml", "main", []string{"main"}, nil)
	m.form = &f
	return m
}

func TestTriggerFlowOpensGraph(t *testing.T) {
	fk := &fakeTrigger{runs: []ghcli.RunInfo{{ID: 123, CreatedAt: t0.Add(time.Second), Actor: "me", Event: "workflow_dispatch"}}}
	fk.install(t)
	m := formOpenModel(t)
	next, cmd := m.Update(k("enter"))
	m = next.(model)
	if m.form != nil || cmd == nil {
		t.Fatalf("submit should close the form and return a command (form=%v cmd=%v)", m.form, cmd != nil)
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	want := [][]string{{"workflow", "run", "deploy.yml", "--ref", "main"}}
	if !reflect.DeepEqual(fk.dispatched, want) {
		t.Errorf("dispatched %v; want %v", fk.dispatched, want)
	}
	if m.view != viewGraph || m.runID != "123" {
		t.Errorf("view=%v runID=%q; want graph for run 123", m.view, m.runID)
	}
	if len(m.back) == 0 || m.back[len(m.back)-1] != viewWorkflows {
		t.Errorf("esc should return to workflows, back=%v", m.back)
	}
}

func TestTriggerFlowRunNotFoundOpensTheRunsList(t *testing.T) {
	fk := &fakeTrigger{runs: []ghcli.RunInfo{{ID: 9, CreatedAt: t0.Add(-time.Hour), Actor: "me", Event: "workflow_dispatch"}}}
	fk.install(t)
	m := formOpenModel(t)
	next, cmd := m.Update(k("enter"))
	next, _ = next.Update(cmd())
	m = next.(model)
	if m.view != viewRuns || m.wfID != "deploy.yml" {
		t.Errorf("view=%v wfID=%q; want the runs of deploy.yml so the new run shows up", m.view, m.wfID)
	}
	if len(m.back) == 0 || m.back[len(m.back)-1] != viewWorkflows {
		t.Errorf("esc should return to workflows, back=%v", m.back)
	}
	if !strings.Contains(m.status, "triggered deploy.yml") || !strings.Contains(m.status, "not found") {
		t.Errorf("status = %q", m.status)
	}
	if len(fk.dispatched) != 1 {
		t.Errorf("dispatch should still have happened once: %v", fk.dispatched)
	}
}

func TestTriggerDispatchFailureSaysSo(t *testing.T) {
	fk := &fakeTrigger{}
	fk.install(t)
	triggerDeps.dispatch = func([]string) error { return errors.New("HTTP 422") }
	m := formOpenModel(t)
	next, cmd := m.Update(k("enter"))
	next, _ = next.Update(cmd())
	m = next.(model)
	if !strings.Contains(m.status, "422") || strings.Contains(m.status, "triggered") {
		t.Errorf("status = %q; want the dispatch error and no 'triggered'", m.status)
	}
}

func TestTriggerUnknownUserStillDispatches(t *testing.T) {
	fk := &fakeTrigger{}
	fk.install(t)
	triggerDeps.user = func(string) (string, error) { return "", errors.New("no auth") }
	m := formOpenModel(t)
	next, cmd := m.Update(k("enter"))
	next, _ = next.Update(cmd())
	m = next.(model)
	if len(fk.dispatched) != 1 || m.view != viewRuns || !strings.Contains(m.status, "no auth") {
		t.Errorf("dispatched=%v view=%v status=%q", fk.dispatched, m.view, m.status)
	}
}
