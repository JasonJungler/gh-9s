package tui

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/favorites"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/recent"
)

func promptModel(t *testing.T) model {
	t.Helper()
	fs, _ := favorites.Load(filepath.Join(t.TempDir(), "favorites.json"))
	fs.Items = []favorites.Favorite{{Host: "github.com", Repo: "acme/api", Workflow: "deploy.yml", Alias: "prod"}}
	rs, _ := recent.Load(filepath.Join(t.TempDir(), "recent.json"))
	rs.Add(ghcli.RepoRef{Owner: "acme", Name: "web"})
	return model{
		view: viewRuns, width: 100, height: 20, favs: fs, recents: rs,
		tbl: table{cols: []string{"ID", "STATUS", "WORKFLOW"}, rows: []row{
			{id: "1", cells: []string{"1", "✓ success", "CI"}},
			{id: "2", cells: []string{"2", "✗ failure", "Deploy"}},
		}},
		repoCache: map[string]repoCacheEntry{"": {list: []ghcli.RepoInfo{info("acme/api"), info("zed/other")}}},
	}
}

// rendered is the screen as a list of lines
func rendered(m model) []string { return strings.Split(m.View(), "\n") }

func indexOf(ls []string, sub string) int {
	for i, l := range ls {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

func TestPromptBoxIsAtTheTopWhileTyping(t *testing.T) {
	m := keys(promptModel(t), ":", "w", "o")
	ls := rendered(m)
	box, header := indexOf(ls, ":wo"), indexOf(ls, "WORKFLOW")
	if box < 0 || header < 0 || box > 4 || box >= header {
		t.Fatalf("prompt line %d, table header line %d; the prompt must sit above the table near the top:\n%s", box, header, m.View())
	}
	if strings.Contains(strings.Join(ls[len(ls)-2:], "\n"), ":wo") {
		t.Errorf("the prompt must not also be at the bottom:\n%s", m.View())
	}
	if indexOf(ls, "╭") < 0 || indexOf(ls, "╰") < 0 {
		t.Errorf("the prompt is drawn in a rounded box:\n%s", m.View())
	}
}

func TestPromptBoxKeepsTheScreenTheSameHeight(t *testing.T) {
	m := promptModel(t)
	normal := len(rendered(m))
	if typing := len(rendered(keys(m, ":"))); typing != normal {
		t.Errorf("screen is %d lines while typing and %d otherwise; the table must shrink to make room", typing, normal)
	}
	logs := model{view: viewLogs, width: 100, height: 20, logs: make([]string, 80)}
	if a, b := len(rendered(logs)), len(rendered(keys(logs, "/"))); a != b {
		t.Errorf("log view: %d lines normally, %d while searching", a, b)
	}
}

func TestFilterAndSearchPromptsAreAlsoAtTheTop(t *testing.T) {
	m := keys(promptModel(t), "/", "C", "I")
	if i := indexOf(rendered(m), "/CI"); i < 0 || i > 4 {
		t.Errorf("filter prompt at line %d:\n%s", i, m.View())
	}
	logs := model{view: viewLogs, width: 100, height: 20, logs: []string{"a error", "b"}}
	logs = keys(logs, "/", "e", "r")
	if i := indexOf(rendered(logs), "/er"); i < 0 || i > 4 {
		t.Errorf("search prompt at line %d:\n%s", i, logs.View())
	}
}

func TestAliasPromptIsAtTheTopAndNamesTheWorkflow(t *testing.T) {
	m := promptModel(t)
	m.mode, m.aliasFor, m.input = modeAlias, favorites.Favorite{Workflow: "deploy.yml"}, "pro"
	ls := rendered(m)
	if i := indexOf(ls, "alias for deploy.yml"); i < 0 || i > 4 {
		t.Errorf("alias prompt at line %d:\n%s", i, m.View())
	}
}

func TestCompletions(t *testing.T) {
	m := promptModel(t)
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"wo", []string{"workflows"}},
		{"WO", []string{"workflows"}},
		{"r", []string{"runs", "repo"}},
		{"pr", []string{"prod"}}, // a favorite's alias
		{"i", []string{"issues"}},
		{"zz", nil},
		{"workflows", nil}, // already complete
		{"repo ac", []string{"repo acme/web", "repo acme/api"}}, // recent first, then the cached list
		{"repo zed/", []string{"repo zed/other"}},
		{"repo ", []string{"repo acme/web", "repo acme/api", "repo zed/other"}},
		{"repo nope", nil},
		{"quit now", nil},
	}
	for _, c := range cases {
		if got := m.completions(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("completions(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestGhostTextShowsTheRestOfTheFirstSuggestion(t *testing.T) {
	m := keys(promptModel(t), ":", "w", "o", "r")
	if v := m.View(); !strings.Contains(v, ":wor█kflows") {
		t.Errorf("the box should show the typed text, the cursor, then the rest of the suggestion:\n%s", v)
	}
	m = keys(promptModel(t), ":", "z", "z")
	if v := m.View(); strings.Contains(v, "█ ") && strings.Contains(v, "zz█workflows") {
		t.Errorf("no suggestion, no ghost:\n%s", v)
	}
}

func TestTabAcceptsThenCyclesTheSuggestions(t *testing.T) {
	m := keys(promptModel(t), ":", "r")
	var seen []string
	for i := 0; i < 3; i++ {
		next, _ := m.Update(k("tab"))
		m = next.(model)
		seen = append(seen, m.input)
	}
	if !reflect.DeepEqual(seen, []string{"runs", "repo", "runs"}) {
		t.Errorf("successive tabs gave %q; want runs, repo, then back round", seen)
	}
}

func TestTypingAfterTabStopsTheCycleAndStartsFromWhatIsTyped(t *testing.T) {
	m := keys(promptModel(t), ":", "r")
	next, _ := m.Update(k("tab"))
	m = next.(model) // "runs"
	m = keys(m, "x") // "runsx": no suggestion any more
	next, _ = m.Update(k("tab"))
	if got := next.(model).input; got != "runsx" {
		t.Errorf("input = %q; tab with nothing to complete must leave the text alone", got)
	}
}

func TestRightArrowAcceptsTheSuggestion(t *testing.T) {
	m := keys(promptModel(t), ":", "w", "o")
	next, _ := m.Update(k("right"))
	if got := next.(model).input; got != "workflows" {
		t.Errorf("input = %q after →", got)
	}
}

func TestEnterOnAUniquePrefixRunsTheCommand(t *testing.T) {
	m := keys(promptModel(t), ":", "w", "o")
	next, _ := m.Update(k("enter"))
	if v := next.(model).view; v != viewWorkflows {
		t.Errorf("view = %v after :wo⏎; a unique prefix should run", v)
	}
	m = keys(promptModel(t), ":", "z", "z")
	next, _ = m.Update(k("enter"))
	if got := next.(model); got.view != viewRuns || !strings.Contains(got.status, "unknown command") {
		t.Errorf("view=%v status=%q; garbage must still be refused", got.view, got.status)
	}
}

func TestHistoryWithUpAndDown(t *testing.T) {
	installIssues(t, fixtureIssue())
	m := promptModel(t)
	for _, c := range []string{"issues", "runs"} {
		m = keys(m, ":")
		for _, r := range c {
			m = keys(m, string(r))
		}
		next, cmd := m.Update(k("enter"))
		m = pump(next.(model), cmd)
	}
	m = keys(m, ":", "z")
	step := func(key string) string {
		next, _ := m.Update(k(key))
		m = next.(model)
		return m.input
	}
	if got := []string{step("up"), step("up"), step("down"), step("down")}; !reflect.DeepEqual(got, []string{"runs", "issues", "runs", "z"}) {
		t.Errorf("history walk = %q; want runs, issues, runs, then back to what was being typed", got)
	}
}

func TestDialogsAndThePickerAreBoxedAtTheTop(t *testing.T) {
	m := promptModel(t).withDialog(newConfirmDialog("x", "Merge acme/api#7?", "squash"))
	ls := rendered(m)
	if i := indexOf(ls, "Merge acme/api#7?"); i < 0 || i > 6 || indexOf(ls, "╭") < 0 || indexOf(ls, "╰") < 0 {
		t.Errorf("a dialog is a rounded box near the top (title at line %d):\n%s", i, m.View())
	}
	installPickerFakes(t, nil)
	p := openPicker(t, promptModel(t))
	if v := p.View(); !strings.Contains(v, "╭") || !strings.Contains(v, "Switch repo") {
		t.Errorf("the picker is boxed too:\n%s", v)
	}
}
