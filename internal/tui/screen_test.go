package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const viewTest viewKind = 900

// installFakeScreen registers a throwaway screen for viewTest and removes it afterwards.
func installFakeScreen(t *testing.T) {
	t.Helper()
	register(viewTest, screen{
		name: "fake",
		fetch: func(m model) tea.Cmd {
			return func() tea.Msg { return screenMsg{view: viewTest, data: "hello"} }
		},
		apply: func(m model, msg screenMsg) model { return m.withScreenData(viewTest, msg.data) },
		key: func(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
			if k.String() == "x" {
				return m.withScreenData(viewTest, "x-pressed"), nil, true
			}
			return m, nil, false
		},
		body: func(m model) string {
			s, _ := m.screenData(viewTest).(string)
			return "FAKE BODY: " + s + "\n"
		},
		footer: "x press-me",
		poll:   true,
	})
	t.Cleanup(func() { delete(screens, viewTest); delete(viewNames, viewTest) })
}

func TestRegisteredScreenIsFetchedAndApplied(t *testing.T) {
	installFakeScreen(t)
	m := model{view: viewTest, width: 100, height: 20}
	msg := m.fetch()()
	next, _ := m.Update(msg)
	m = next.(model)
	v := m.View()
	for _, want := range []string{"FAKE BODY: hello", "x: press-me", "<fake>"} {
		if !strings.Contains(v, want) {
			t.Errorf("View() lacks %q:\n%s", want, v)
		}
	}
}

func TestScreenKeyRunsBeforeSharedKeysAndFallsThrough(t *testing.T) {
	installFakeScreen(t)
	m := model{view: viewTest, width: 100, height: 20, back: []viewKind{viewRuns}}
	m = keys(m, "x")
	if got, _ := m.screenData(viewTest).(string); got != "x-pressed" {
		t.Errorf("screen key not handled: data = %q", got)
	}
	m = keys(m, "esc")
	if m.view != viewRuns {
		t.Errorf("an unhandled key must fall through to the shared handler; view = %v", m.view)
	}
}

func TestScreenPollFlagDrivesShouldPoll(t *testing.T) {
	installFakeScreen(t)
	m := model{view: viewTest}
	if !m.shouldPoll() {
		t.Error("poll:true screen should poll")
	}
	s := screens[viewTest]
	s.poll = false
	screens[viewTest] = s
	if m.shouldPoll() {
		t.Error("poll:false screen must not poll")
	}
}

func TestScreenDataIsCopyOnWrite(t *testing.T) {
	m := model{}
	m1 := m.withScreenData(viewTest, "a")
	m2 := m1.withScreenData(viewTest, "b")
	if m.screenData(viewTest) != nil || m1.screenData(viewTest) != "a" || m2.screenData(viewTest) != "b" {
		t.Errorf("data leaked between model copies: %v %v %v", m.screenData(viewTest), m1.screenData(viewTest), m2.screenData(viewTest))
	}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	installFakeScreen(t)
	defer func() {
		if recover() == nil {
			t.Error("registering the same view twice should panic")
		}
	}()
	register(viewTest, screen{name: "again"})
}

func TestScreenMessageForAnotherViewIsIgnored(t *testing.T) {
	installFakeScreen(t)
	m := model{view: viewRuns, width: 100, height: 20}
	next, _ := m.Update(screenMsg{view: viewTest, data: "late"})
	if got := next.(model).screenData(viewTest); got != nil {
		t.Errorf("a late result for a view we left was applied: %v", got)
	}
}

// Hints are shown to people: they must read as text, not as leftovers of a rename.
func TestFooterHintsAreHumanText(t *testing.T) {
	for _, v := range []viewKind{viewRuns, viewWorkflows, viewPRs, viewChecks, viewJobs, viewLogs, viewFavs, viewGraph} {
		f := (model{view: v}).footer()
		for _, bad := range []string{"favorites.", "ghcli.", "graph.", "workflow.", "ui."} {
			if strings.Contains(f, bad) {
				t.Errorf("view %d footer contains %q: %q", v, bad, f)
			}
		}
	}
	if f := (model{view: viewWorkflows}).footer(); !strings.Contains(f, "f: favorite ") {
		t.Errorf("workflows footer should say 'f: favorite': %q", f)
	}
}
