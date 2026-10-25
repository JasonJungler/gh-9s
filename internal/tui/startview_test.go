package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/settings"
)

func TestGlobalNumberKeysOpenTheirViews(t *testing.T) {
	for key, want := range map[string]viewKind{
		"1": viewPRs, "2": viewIssues, "3": viewRuns, "4": viewWorkflows, "5": viewDiscussions, "6": viewWiki, "0": viewInbox,
	} {
		next, _ := model{view: viewInbox}.Update(k(key))
		if got := next.(model).view; got != want {
			t.Errorf("key %s opened view %v; want %v", key, got, want)
		}
	}
}

func startModel(t *testing.T, value string) model {
	t.Helper()
	content := ""
	if value != "" {
		content = `{"start_view": "` + value + `"}`
	}
	m, _ := settingsModel(t, content, nil)
	return m.startOn()
}

func TestStartViewDefaultIsPullRequests(t *testing.T) {
	if got := startModel(t, "").view; got != viewPRs {
		t.Errorf("default start view = %v; want pull requests", got)
	}
}

func TestStartViewSettingPicksTheView(t *testing.T) {
	for value, want := range map[string]viewKind{
		"pulls": viewPRs, "issues": viewIssues, "runs": viewRuns, "workflows": viewWorkflows,
		"discussions": viewDiscussions, "wiki": viewWiki, "inbox": viewInbox,
	} {
		if got := startModel(t, value).view; got != want {
			t.Errorf("start_view=%s opened %v; want %v", value, got, want)
		}
	}
}

func TestStartViewRejectsAnUnknownValue(t *testing.T) {
	m, _ := settingsModel(t, `{"start_view": "board"}`, nil)
	if got := m.startOn().view; got != viewPRs {
		t.Errorf("an unknown start_view opened %v; want the default, pull requests", got)
	}
	if notes := strings.Join(m.cfg().Notes(), ";"); !strings.Contains(notes, "start_view") {
		t.Errorf("notes %q should name start_view", notes)
	}
	if _, err := m.cfg().Set("start_view", "board"); err == nil || errors.Is(err, settings.ErrNotSaved) {
		t.Errorf(":set start_view board was accepted: %v", err)
	}
}

func infoFor(cur ghcli.RepoRef) infoMsg {
	repo := ""
	if cur != (ghcli.RepoRef{}) {
		repo = cur.Owner + "/" + cur.Name
	}
	return infoMsg{repo: repo, cur: cur}
}

func TestOutsideARepoTheStartViewFallsBackToTheInbox(t *testing.T) {
	for _, value := range []string{"", "runs", "wiki"} {
		next, cmd := startModel(t, value).Update(infoFor(ghcli.RepoRef{}))
		m := next.(model)
		if m.view != viewInbox || cmd == nil {
			t.Errorf("start_view=%q outside a repo: view=%v (cmd nil: %v); want the inbox fetching", value, m.view, cmd == nil)
		}
		if !strings.Contains(m.note, ":repo") {
			t.Errorf("start_view=%q: note %q should point at :repo", value, m.note)
		}
	}
}

func TestInsideARepoTheStartViewStays(t *testing.T) {
	cur := ghcli.RepoRef{Host: "github.com", Owner: "acme", Name: "api"}
	next, _ := startModel(t, "runs").Update(infoFor(cur))
	if m := next.(model); m.view != viewRuns || m.note != "" {
		t.Errorf("view=%v note=%q; want the runs untouched", m.view, m.note)
	}
}

func TestStartingOnTheInboxNeedsNoRepoAndNoNote(t *testing.T) {
	next, _ := startModel(t, "inbox").Update(infoFor(ghcli.RepoRef{}))
	if m := next.(model); m.view != viewInbox || m.note != "" {
		t.Errorf("view=%v note=%q; want the inbox without a note", m.view, m.note)
	}
}

func TestTheFallbackLeavesAUserWhoAlreadyMovedAlone(t *testing.T) {
	next, _ := startModel(t, "").Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")}) // the user pressed 3: runs
	m := next.(model)
	next, _ = m.Update(infoFor(ghcli.RepoRef{}))
	if got := next.(model).view; got != viewRuns {
		t.Errorf("view=%v; want the runs the user chose, not the inbox", got)
	}
}
