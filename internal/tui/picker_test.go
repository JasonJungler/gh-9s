package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/recent"
)

type pickerFakes struct {
	skipped      []string // organisations the fake could not list
	listHosts    []string
	searchHosts  []string
	searchCalls  []string
	searchResult map[string][]ghcli.RepoInfo
	searchErr    error
}

func info(name string) ghcli.RepoInfo {
	r, _ := ghcli.ParseRepoRef(name)
	return ghcli.RepoInfo{Ref: r, Language: "Go", Stars: 3, UpdatedAt: "2026-10-20T10:00:00Z"}
}

// installPickerFakes replaces every outside call the picker makes. The debounce fires at once.
func installPickerFakes(t *testing.T, access []ghcli.RepoInfo) *pickerFakes {
	t.Helper()
	f := &pickerFakes{searchResult: map[string][]ghcli.RepoInfo{}}
	oldList, oldSearch, oldDebounce := listRepos, searchRepos, debounce
	listRepos = func(host string) ([]ghcli.RepoInfo, []string, error) {
		f.listHosts = append(f.listHosts, host)
		return access, f.skipped, nil
	}
	searchRepos = func(host, q string, limit int) ([]ghcli.RepoInfo, error) {
		f.searchHosts = append(f.searchHosts, host)
		f.searchCalls = append(f.searchCalls, q)
		return f.searchResult[q], f.searchErr
	}
	debounce = func(seq int) tea.Cmd { return func() tea.Msg { return pickerDebounceMsg{seq} } }
	t.Cleanup(func() { listRepos, searchRepos, debounce = oldList, oldSearch, oldDebounce })
	return f
}

func pickerModel(t *testing.T) model {
	m := favModel(t) // current directory is github.com/acme/api
	rs, _ := recent.Load(t.TempDir() + "/recent.json")
	m.recents = rs
	return m
}

// openPicker runs ":repo" (no argument) and delivers the access list the picker asked for.
func openPicker(t *testing.T, m model) model {
	t.Helper()
	return typeCommand(t, m, "repo")
}

func pickKeys(t *testing.T, m model, keysToPress ...string) model {
	t.Helper()
	for _, key := range keysToPress {
		next, c := m.Update(k(key))
		m = next.(model)
		for c != nil { // run only picker messages (debounce, search results); ignore fetches
			msg := c()
			if msg == nil {
				break
			}
			switch msg.(type) {
			case pickerDebounceMsg, pickerResultMsg, pickerLoadedMsg, repoSwitchedMsg:
				next, c = m.Update(msg)
				m = next.(model)
			default:
				c = nil
			}
		}
	}
	return m
}

func typeText(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		m = pickKeys(t, m, string(r))
	}
	return m
}

func TestRowsStartWithTheCurrentDirectory(t *testing.T) {
	p := picker{}
	rows := p.rows(ghcli.RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, nil)
	if len(rows) == 0 || !rows[0].cwd || !strings.Contains(rows[0].label, "current directory") || !strings.Contains(rows[0].label, "acme/api") {
		t.Errorf("rows = %+v; want the current directory first, naming its repo", rows)
	}
}

func TestEmptyQueryShowsRecents(t *testing.T) {
	p := picker{cache: []ghcli.RepoInfo{info("a/one")}}
	rows := p.rows(ghcli.RepoRef{}, []ghcli.RepoRef{{Owner: "r", Name: "x"}, {Owner: "r", Name: "y"}})
	if len(rows) != 3 || rows[1].ref.Name != "x" || rows[2].ref.Name != "y" {
		t.Errorf("rows = %+v; want cwd, then the recents in order (not the whole access list)", rows)
	}
}

func TestTypingFiltersTheAccessListWithoutAnyRequest(t *testing.T) {
	f := installPickerFakes(t, []ghcli.RepoInfo{info("acme/api"), info("acme/web"), info("me/apple-notes")})
	m := openPicker(t, pickerModel(t))
	if m.picker == nil || len(f.listHosts) != 1 {
		t.Fatalf("picker=%v listCalls=%d; want it open after one list call", m.picker != nil, len(f.listHosts))
	}
	m = typeText(t, m, "web")
	rows := m.picker.rows(m.curTarget(), m.recents.List())
	if len(rows) != 2 || rows[1].ref.Name != "web" {
		t.Errorf("rows = %+v; want cwd + acme/web", rows)
	}
	if len(f.searchCalls) != 0 || len(f.listHosts) != 1 {
		t.Errorf("filtering must be local: searches=%v lists=%d", f.searchCalls, len(f.listHosts))
	}
}

func TestTabTogglesScopeAndTheTitleSaysWhich(t *testing.T) {
	installPickerFakes(t, nil)
	m := openPicker(t, pickerModel(t))
	if v := m.View(); !strings.Contains(v, "my access") {
		t.Errorf("default scope should be named:\n%s", v)
	}
	m = pickKeys(t, m, "tab")
	if !m.picker.global || !strings.Contains(m.View(), "global") {
		t.Errorf("tab should switch to global and say so:\n%s", m.View())
	}
	m = pickKeys(t, m, "tab")
	if m.picker.global {
		t.Error("a second tab should switch back")
	}
}

func TestGlobalQueryUnderTwoCharsDoesNotCallSearch(t *testing.T) {
	f := installPickerFakes(t, nil)
	m := openPicker(t, pickerModel(t))
	m = pickKeys(t, m, "tab")
	m = typeText(t, m, "a")
	if len(f.searchCalls) != 0 {
		t.Errorf("one character must not reach the search API: %v", f.searchCalls)
	}
}

func TestGlobalSearchIsDebouncedAndOnlyTheNewestResultShows(t *testing.T) {
	f := installPickerFakes(t, nil)
	f.searchResult["api"] = []ghcli.RepoInfo{info("cli/api-new")}
	m := openPicker(t, pickerModel(t))
	m = pickKeys(t, m, "tab")
	// type three characters before any debounce timer fires, then let the timers fire in order
	var timers []tea.Cmd
	for _, r := range "api" {
		next, c := m.Update(k(string(r)))
		m = next.(model)
		if c != nil {
			timers = append(timers, c)
		}
	}
	if len(timers) != 2 {
		t.Fatalf("timers = %d; 'a' is under the minimum, 'p' and 'i' each schedule one", len(timers))
	}
	var search tea.Cmd
	for _, c := range timers {
		next, sc := m.Update(c())
		m = next.(model)
		if sc != nil {
			search = sc
		}
	}
	if search == nil {
		t.Fatal("the newest timer should start a search")
	}
	next, _ := m.Update(search())
	m = next.(model)
	if len(f.searchCalls) != 1 || f.searchCalls[0] != "api" {
		t.Fatalf("searches = %v; want exactly one, for the final query", f.searchCalls)
	}
	// a slower, older response arrives after the newest one
	next, _ = m.Update(pickerResultMsg{seq: m.picker.seq - 1, items: []ghcli.RepoInfo{info("old/stale")}})
	m = next.(model)
	rows := m.picker.rows(m.curTarget(), nil)
	if len(rows) != 2 || rows[1].ref.Name != "api-new" {
		t.Errorf("rows = %+v; the stale response must not replace the newest", rows)
	}
}

func TestSearchErrorIsShownInThePicker(t *testing.T) {
	f := installPickerFakes(t, nil)
	f.searchErr = errors.New("API rate limit exceeded")
	m := openPicker(t, pickerModel(t))
	m = pickKeys(t, m, "tab")
	m = typeText(t, m, "api")
	if v := m.View(); !strings.Contains(v, "rate limit") {
		t.Errorf("the error should be visible in the picker:\n%s", v)
	}
	if m.picker == nil {
		t.Error("an error must not close the picker")
	}
}

func TestEnterOnARowSwitchesViaTheVerifiedPathAndRecordsIt(t *testing.T) {
	installPickerFakes(t, []ghcli.RepoInfo{info("acme/api"), info("acme/web")})
	checked := installRepoChecker(t, nil)
	m := openPicker(t, pickerModel(t))
	m = typeText(t, m, "web")
	m = pickKeys(t, m, "down", "enter")
	want := ghcli.RepoRef{Owner: "acme", Name: "web"}
	if m.picker != nil || len(*checked) != 1 || (*checked)[0] != want || m.base != want {
		t.Errorf("picker open=%v checked=%v base=%+v; want a verified switch to acme/web", m.picker != nil, *checked, m.base)
	}
	if l := m.recents.List(); len(l) != 1 || l[0] != want {
		t.Errorf("recents = %v; want the chosen repo recorded", l)
	}
}

func TestEnterOnTheCurrentDirectoryRowReturnsHome(t *testing.T) {
	installPickerFakes(t, nil)
	installRepoChecker(t, nil)
	m := pickerModel(t)
	m.base, m.target = acme, acme
	m = openPicker(t, m)
	m = pickKeys(t, m, "enter") // row 0 is the current directory
	if m.base != (ghcli.RepoRef{}) || m.picker != nil {
		t.Errorf("base=%+v picker open=%v; want home", m.base, m.picker != nil)
	}
	if len(m.recents.List()) != 0 {
		t.Errorf("going home is not recorded as a recent repo: %v", m.recents.List())
	}
}

func TestEscClosesWithoutSwitching(t *testing.T) {
	installPickerFakes(t, []ghcli.RepoInfo{info("acme/web")})
	checked := installRepoChecker(t, nil)
	m := openPicker(t, pickerModel(t))
	m = pickKeys(t, m, "esc")
	if m.picker != nil || m.base != (ghcli.RepoRef{}) || len(*checked) != 0 {
		t.Errorf("open=%v base=%+v checked=%v", m.picker != nil, m.base, *checked)
	}
}

func TestHostComesFromTheShownRepo(t *testing.T) {
	f := installPickerFakes(t, nil)
	m := pickerModel(t)
	m.curRepo = ghcli.RepoRef{Host: "ghe.corp.com", Owner: "acme", Name: "api"}
	m = openPicker(t, m)
	m = pickKeys(t, m, "tab")
	m = typeText(t, m, "api")
	allEnterprise := len(f.searchHosts) > 0
	for _, h := range f.searchHosts {
		allEnterprise = allEnterprise && h == "ghe.corp.com"
	}
	if len(f.listHosts) != 1 || f.listHosts[0] != "ghe.corp.com" || !allEnterprise {
		t.Errorf("list hosts %v, search hosts %v; want the shown repo's host for both", f.listHosts, f.searchHosts)
	}
}

func TestPickerShowsHostOnlyForNonDefaultHosts(t *testing.T) {
	if got := displayRef(ghcli.RepoRef{Host: "github.com", Owner: "a", Name: "b"}); got != "a/b" {
		t.Errorf("default host: %q", got)
	}
	if got := displayRef(ghcli.RepoRef{Owner: "a", Name: "b"}); got != "a/b" {
		t.Errorf("no host: %q", got)
	}
	if got := displayRef(ghcli.RepoRef{Host: "ghe.corp.com", Owner: "a", Name: "b"}); got != "ghe.corp.com/a/b" {
		t.Errorf("enterprise host: %q", got)
	}
}

func TestPickerTellsWhenSomeOrganisationsCouldNotBeListed(t *testing.T) {
	f := installPickerFakes(t, []ghcli.RepoInfo{info("acme/web")})
	f.skipped = []string{"big-org", "locked-org"}
	m := openPicker(t, pickerModel(t))
	v := m.View()
	if !strings.Contains(v, "big-org") || !strings.Contains(v, "locked-org") || !strings.Contains(v, "could not be listed") {
		t.Errorf("the picker should say the list may be partial:\n%s", v)
	}
	// and still says so when the list comes from the cache
	m = pickKeys(t, m, "esc")
	m = openPicker(t, m)
	if v := m.View(); !strings.Contains(v, "big-org") {
		t.Errorf("a cached list must keep its warning:\n%s", v)
	}
}

func TestGlobalSearchClearsOldResultsWhenTheQueryChanges(t *testing.T) {
	f := installPickerFakes(t, nil)
	f.searchResult["api"] = []ghcli.RepoInfo{info("cli/api-new")}
	m := openPicker(t, pickerModel(t))
	m = pickKeys(t, m, "tab")
	m = typeText(t, m, "api")
	if rows := m.picker.rows(m.curRepo, nil); len(rows) != 2 {
		t.Fatalf("setup: rows = %+v", rows)
	}
	m = pickKeys(t, m, "x") // the query is now "apix": the old results are not answers to it
	for _, r := range m.picker.rows(m.curRepo, nil)[1:] {
		t.Errorf("stale row %q is still shown for the new query", r.label)
	}
}
