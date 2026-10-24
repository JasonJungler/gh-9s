package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func keys(m tea.Model, ks ...string) model {
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		case "ctrl+d":
			msg = tea.KeyMsg{Type: tea.KeyCtrlD}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m, _ = m.Update(msg)
	}
	return m.(model)
}

func favModel(t *testing.T) model {
	t.Helper()
	fs, _ := favorites.Load(filepath.Join(t.TempDir(), "favorites.json"))
	return model{
		view: viewWorkflows, width: 120, height: 30, favs: fs,
		curRepo: ghcli.RepoRef{Host: "github.com", Owner: "acme", Name: "api"},
		tbl: table{cols: []string{"ID", "NAME", "STATE", "PATH"}, rows: []row{
			{id: "1", extra: "CI", cells: []string{"1", "CI", "active", ".github/workflows/ci.yml"}},
			{id: "2", extra: "Deploy", cells: []string{"2", "Deploy", "active", ".github/workflows/deploy.yml"}},
		}},
	}
}

func TestFavoriteToggleAliasAndOpen(t *testing.T) {
	m := favModel(t)
	m = keys(m, "j", "f") // favorites.Favorite Deploy
	if len(m.favs.Items) != 1 || m.favs.Items[0] != (favorites.Favorite{Host: "github.com", Repo: "acme/api", Workflow: "deploy.yml"}) {
		t.Fatalf("items = %+v", m.favs.Items)
	}
	m = keys(m, "a")
	if m.mode != modeAlias {
		t.Fatalf("mode = %v; want modeAlias", m.mode)
	}
	m = keys(m, "p", "r", "o", "d", "enter")
	if m.favs.Items[0].Alias != "prod" {
		t.Fatalf("alias = %q", m.favs.Items[0].Alias)
	}
	// the alias is persisted
	if again, err := favorites.Load(m.favs.Path()); err != nil || again.Items[0].Alias != "prod" {
		t.Fatalf("not persisted: %+v %v", again, err)
	}
	// typing the alias in the command bar opens its runs
	m.favs.Items[0].Host = "ghe.corp.com" // a favorites.Favorite in another repo/host
	m = keys(m, ":", "p", "r", "o", "d", "enter")
	if m.view != viewRuns || m.wfID != "deploy.yml" || m.wfName != "prod" {
		t.Errorf("view=%v wf=%q name=%q", m.view, m.wfID, m.wfName)
	}
	if want := (ghcli.RepoRef{Host: "ghe.corp.com", Owner: "acme", Name: "api"}); m.target != want {
		t.Errorf("target = %+v; want %+v", m.target, want)
	}
	if !strings.Contains(m.View(), "ghe.corp.com/acme/api") {
		t.Error("header should name the favorite's repo")
	}
	// going to a top-level view returns to the current directory's repo
	m = keys(m, "1")
	if m.target != (ghcli.RepoRef{}) {
		t.Errorf("target not reset: %+v", m.target)
	}
}

func TestFavoriteAliasRejectedKeepsMode(t *testing.T) {
	m := favModel(t)
	m = keys(m, "f", "a", "r", "u", "n", "s", "enter")
	if m.favs.Items[0].Alias != "" {
		t.Errorf("reserved alias accepted: %q", m.favs.Items[0].Alias)
	}
	if !strings.Contains(m.status, "built-in") {
		t.Errorf("status = %q", m.status)
	}
}

func TestFavoritesDisabledWhenNil(t *testing.T) {
	m := favModel(t)
	m.favs = nil
	m = keys(m, "f")
	if !strings.Contains(m.status, "disabled") {
		t.Errorf("status = %q", m.status)
	}
}

func TestEscFromFavoriteRunsReturnsToFavs(t *testing.T) {
	m := favModel(t)
	m.view = viewFavs
	m.favs.Items = []favorites.Favorite{{Host: "ghe.corp.com", Repo: "acme/api", Workflow: "deploy.yml"}}
	m.tbl = table{rows: []row{{id: "k", extra: m.favs.Items[0].Key(), cells: []string{"-", "acme/api", "deploy.yml", "x", "1m"}}}}
	m = keys(m, "enter")
	if m.view != viewRuns || m.target.Host != "ghe.corp.com" {
		t.Fatalf("view=%v target=%+v", m.view, m.target)
	}
	m = keys(m, "esc")
	if m.view != viewFavs || m.target != (ghcli.RepoRef{}) || m.wfID != "" {
		t.Errorf("after esc: view=%v target=%+v wf=%q", m.view, m.target, m.wfID)
	}
}

func TestTriggerFormLifecycle(t *testing.T) {
	m := favModel(t)
	form := newTriggerForm(ghcli.RepoRef{}, "deploy.yml", "main", []string{"main", "dev"},
		[]workflow.DispatchInput{{Name: "tag", Type: "string", Required: true}})
	next, _ := m.Update(formReadyMsg{form: form, name: "Deploy", dispatchable: true})
	m = next.(model)
	if m.form == nil || !strings.Contains(m.View(), "Trigger deploy.yml") {
		t.Fatal("form should be open and rendered")
	}
	// enter with the required input empty keeps the form open
	m = keys(m, "enter")
	if m.form == nil || m.form.err == "" {
		t.Fatalf("expected blocked submit, form=%+v", m.form)
	}
	// changing the ref asks for the inputs to be re-read
	next, cmd := m.Update(k("right"))
	m = next.(model)
	if m.form.ref != "dev" {
		t.Errorf("ref = %q; want dev", m.form.ref)
	}
	if cmd == nil {
		t.Error("ref change should return a reload command")
	}
	m = keys(m, "esc")
	if m.form != nil {
		t.Error("esc should close the form")
	}
}

func TestNonDispatchableShowsMessage(t *testing.T) {
	m := favModel(t)
	next, _ := m.Update(formReadyMsg{name: "CI", dispatchable: false})
	m = next.(model)
	if m.form != nil || !strings.Contains(m.status, "no workflow_dispatch") {
		t.Errorf("form=%v status=%q", m.form, m.status)
	}
}
