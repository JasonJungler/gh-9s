package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/pins"
)

type boardFakes struct {
	project ghcli.Project
	err     error
	calls   []string
	now     time.Time
	listed  map[string][]ghcli.ProjectRef
	listErr error
}

func projItem(typ, repo string, n int, title, status, state string) ghcli.ProjectItem {
	return ghcli.ProjectItem{Status: status, Type: typ, Number: n, Title: title, Repo: repo, State: state,
		URL: fmt.Sprintf("https://github.com/%s/%d", repo, n), Updated: "2026-10-24T10:00:00Z", Assignees: []string{"ann"}, Labels: []string{"bug"}}
}

func sampleProject() ghcli.Project {
	return ghcli.Project{Title: "Roadmap", URL: "https://github.com/orgs/acme/projects/3",
		StatusOrder: []string{"Todo", "In Progress", "Done"}, Total: 5, Items: []ghcli.ProjectItem{
			projItem("Issue", "acme/app", 1, "Fix login", "Todo", "OPEN"),
			projItem("PullRequest", "acme/app", 2, "Add retry", "In Progress", "OPEN"),
			projItem("PullRequest", "acme/app", 3, "Old work", "Done", "MERGED"),
			{Type: "DraftIssue", Title: "Idea", Draft: true},
			{Title: "(no access)"},
		}}
}

func installBoard(t *testing.T) *boardFakes {
	t.Helper()
	f := &boardFakes{project: sampleProject(), now: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	boardLogins = map[string]string{}
	t.Cleanup(func() { boardLogins = map[string]string{} })
	oldFetch, oldNow, oldOpen, oldList, oldHosts := fetchProject, boardNow, openURL, listProjects, inboxHosts
	listProjects = func(host string) ([]ghcli.ProjectRef, error) { return f.listed[host], f.listErr }
	inboxHosts = func() ([]string, error) { return []string{"github.com"}, nil }
	fetchProject = func(host, owner string, n int) (ghcli.Project, error) {
		f.calls = append(f.calls, fmt.Sprintf("%s|%s|%d", host, owner, n))
		return f.project, f.err
	}
	boardNow = func() time.Time { return f.now }
	openURL = func(string) error { return nil }
	t.Cleanup(func() {
		fetchProject, boardNow, openURL, listProjects, inboxHosts = oldFetch, oldNow, oldOpen, oldList, oldHosts
	})
	return f
}

// boardOn opens the board of a Project pinned as acme/3 through the Projects tab.
func boardOn(t *testing.T) (model, *boardFakes) {
	t.Helper()
	f := installBoard(t)
	m := pinsModel(t)
	m.previewOff = true
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Roadmap", URL: "https://github.com/orgs/acme/projects/3"})
	m = pressTab(pressTab(openFavs(t, m), "tab"), "tab")
	m = keyAll(m, "enter")
	return m, f
}

func TestProjectsTabListsPins(t *testing.T) {
	installBoard(t)
	m := pinsModel(t)
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Roadmap"})
	m.pins.ToggleProject(pins.Project{Host: "ghe.corp.com", Owner: "corp", Number: 9, Title: "Ops"})
	m = pressTab(pressTab(openFavs(t, m), "tab"), "tab")
	out := m.View()
	for _, want := range []string{"Projects (2)", "Roadmap", "acme/#3", "Ops", "ghe.corp.com/corp/#9"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
}

func TestPinAProjectReadsItOnceAndStoresTitle(t *testing.T) {
	f := installBoard(t)
	m := pressTab(pressTab(openFavs(t, pinsModel(t)), "tab"), "tab")
	m = keyAll(typeAll(keyAll(m, "a", "enter"), "acme/3"), "enter")
	if len(f.calls) != 1 || f.calls[0] != "|acme|3" {
		t.Fatalf("calls %v", f.calls)
	}
	if len(m.pins.Projects) != 1 || m.pins.Projects[0].Title != "Roadmap" || m.pins.Projects[0].URL == "" {
		t.Fatalf("pins %+v", m.pins.Projects)
	}
	if again, _ := pins.Load(m.pins.Path()); len(again.Projects) != 1 {
		t.Error("not saved")
	}
	// a bad name never reaches GitHub
	m = keyAll(typeAll(keyAll(m, "a", "enter"), "acme/zero"), "enter")
	if len(f.calls) != 1 || !strings.Contains(m.status, "invalid") {
		t.Errorf("calls %v status %q", f.calls, m.status)
	}
}

func TestPinUnknownProjectAndMissingScope(t *testing.T) {
	f := installBoard(t)
	m := pressTab(pressTab(openFavs(t, pinsModel(t)), "tab"), "tab")
	f.err = errors.New("project not found or no access")
	m = keyAll(typeAll(keyAll(m, "a", "enter"), "acme/8"), "enter")
	if len(m.pins.Projects) != 0 || !strings.Contains(m.status, "not found") {
		t.Errorf("unknown: %+v %q", m.pins.Projects, m.status)
	}
	f.err = fmt.Errorf("%w: x", ghcli.ErrMissingScope)
	m = keyAll(typeAll(keyAll(m, "a", "enter"), "ghe.corp.com/corp/2"), "enter")
	if len(m.pins.Projects) != 0 || !strings.Contains(m.status, "gh auth refresh -s read:project --hostname ghe.corp.com") {
		t.Errorf("scope: %q", m.status)
	}
	m = pressTab(m, "tab") // the tabs still work
	if m.favTab != favTabWorkflows {
		t.Errorf("tab %d", m.favTab)
	}
}

func TestBoardTabsFollowTheStatusOrder(t *testing.T) {
	m, _ := boardOn(t)
	out := ansi.Strip(m.View())
	i := func(s string) int { return strings.Index(out, s) }
	if !(i("All (5)") >= 0 && i("All (5)") < i("Todo (1)") && i("Todo (1)") < i("In Progress (1)") && i("In Progress (1)") < i("Done (1)") && i("Done (1)") < i("No status (2)")) {
		t.Errorf("tabs:\n%s", out)
	}
	for _, want := range []string{"Fix login", "Add retry", "acme/app #1", "Idea", "(no access)"} {
		if !strings.Contains(out, want) {
			t.Errorf("All lacks %q", want)
		}
	}
	m = keyAll(m, "tab") // Todo
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "Fix login") || strings.Contains(out, "Add retry") {
		t.Errorf("Todo column:\n%s", out)
	}
}

func TestBoardNoStatusTabOnlyWhenNeededAndEmptyOptionShowsZero(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", StatusOrder: []string{"Todo", "Done"}, Total: 1,
		Items: []ghcli.ProjectItem{projItem("Issue", "acme/app", 1, "One", "Todo", "OPEN")}}
	m := pinsModel(t)
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	out := ansi.Strip(m.View())
	if strings.Contains(out, "No status") || !strings.Contains(out, "Done (0)") {
		t.Errorf("tabs:\n%s", out)
	}
	m = keyAll(m, "tab", "tab")
	if !strings.Contains(ansi.Strip(m.View()), "nothing in this column") {
		t.Errorf("empty column:\n%s", m.View())
	}
}

func TestBoardWithMoreThanTwoHundredItemsSaysSo(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "Big", StatusOrder: []string{"Todo"}, Total: 250}
	for i := 1; i <= 200; i++ {
		f.project.Items = append(f.project.Items, projItem("Issue", "acme/app", i, "t", "Todo", "OPEN"))
	}
	m := pinsModel(t)
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Big"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "All (200+)") || !strings.Contains(out, "Todo (200+)") || !strings.Contains(out, "50 more in the Project") {
		t.Errorf("view:\n%s", out)
	}
}

func TestBoardStateWordsAndNerdIcons(t *testing.T) {
	m, _ := boardOn(t)
	out := ansi.Strip(m.View())
	for _, want := range []string{"open", "merged", "draft"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain set lacks %q:\n%s", want, out)
		}
	}
	useNerd(t)
	out = ansi.Strip(m.rebuildBoard(m.boardSt(), "").View())
	if !strings.Contains(out, g(iconsPROpen())) || !strings.Contains(out, g(iconsPRMerged())) {
		t.Errorf("Nerd set lacks the icons:\n%s", out)
	}
}

func TestBoardEnterOpensTheItemInItsOwnRepoAndHost(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", StatusOrder: []string{"Todo"}, Total: 3, Items: []ghcli.ProjectItem{
		projItem("Issue", "corp/tool", 7, "Enterprise issue", "Todo", "OPEN"),
		projItem("PullRequest", "corp/tool", 8, "Enterprise PR", "Todo", "OPEN"),
		{Type: "DraftIssue", Title: "Idea", Status: "Todo", Draft: true},
	}}
	fk := installIssues(t, fixtureIssue())
	pk := installPRFetch(t, fixturePR())
	m := pinsModel(t)
	m.previewOff = true
	m.pins.ToggleProject(pins.Project{Host: "ghe.corp.com", Owner: "corp", Number: 2, Title: "P", URL: "https://ghe.corp.com/orgs/corp/projects/2"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	want := ghcli.RepoRef{Host: "ghe.corp.com", Owner: "corp", Name: "tool"}
	m = keyAll(m, "enter") // the issue
	if m.view != viewIssue || m.issue != "7" || m.target != want || len(fk.viewTargets) != 1 || fk.viewTargets[0] != want {
		t.Fatalf("view %v issue %q target %+v fetched %v", m.view, m.issue, m.target, fk.viewTargets)
	}
	m = keyAll(m, "esc")
	if m.view != viewBoard || m.target != m.base {
		t.Fatalf("back: view %v target %+v", m.view, m.target)
	}
	m = keyAll(m, "j", "enter") // the PR
	if m.view != viewPRDetail || m.pr != "8" || len(pk.targets) != 1 || pk.targets[0] != want {
		t.Fatalf("view %v pr %q fetched %v", m.view, m.pr, pk.targets)
	}
	m = keyAll(m, "esc")
	m = keyAll(m, "j", "j", "enter") // the draft
	if m.view != viewBoard || !strings.Contains(m.status, "only in the Project") {
		t.Errorf("draft: view %v status %q", m.view, m.status)
	}
}

func TestBoardOpenKeyUsesTheItemOrTheProject(t *testing.T) {
	m, _ := boardOn(t)
	var opened []string
	openURL = func(u string) error { opened = append(opened, u); return nil }
	m = keyAll(m, "o")
	m = keyAll(m, "j", "j", "j", "o") // the draft
	if len(opened) != 2 || opened[0] != "https://github.com/acme/app/1" || opened[1] != "https://github.com/orgs/acme/projects/3" {
		t.Errorf("opened %v", opened)
	}
}

func TestBoardRefreshThrottleAndReentry(t *testing.T) {
	m, f := boardOn(t)
	if len(f.calls) != 1 {
		t.Fatalf("calls %v", f.calls)
	}
	m = keyAll(m, "R")
	if len(f.calls) != 1 || !strings.Contains(m.status, "just refreshed") {
		t.Errorf("R within 5 s: calls %v status %q", f.calls, m.status)
	}
	f.now = f.now.Add(6 * time.Second)
	m = keyAll(m, "R")
	m = keyAll(m, "R")
	if len(f.calls) != 2 {
		t.Errorf("R after 5 s twice: calls %v", f.calls)
	}
	m = keyAll(m, "esc") // back to the Projects tab
	if m.view != viewFavs || m.favTab != favTabProjects {
		t.Fatalf("esc: view %v tab %d", m.view, m.favTab)
	}
	f.now = f.now.Add(30 * time.Second)
	m = keyAll(m, "enter")
	if len(f.calls) != 2 {
		t.Errorf("re-entry within 60 s refetched: %v", f.calls)
	}
	m = keyAll(m, "esc")
	f.now = f.now.Add(2 * time.Minute)
	m = keyAll(m, "enter")
	if len(f.calls) != 3 {
		t.Errorf("re-entry after 60 s: %v", f.calls)
	}
}

func TestBoardAnswerForAnotherProjectIsDropped(t *testing.T) {
	m, f := boardOn(t)
	late := m.fetch()
	m.board = pins.Project{Owner: "other", Number: 1}
	f.project = ghcli.Project{Title: "Other"}
	next, _ := m.Update(late())
	if got := next.(model).boardSt(); got.loaded {
		t.Errorf("a late answer for acme/#3 was applied to other/#1: %+v", got)
	}
}

func TestBoardErrorShowsTheHintAndKeepsNavigation(t *testing.T) {
	f := installBoard(t)
	f.err = fmt.Errorf("%w: x", ghcli.ErrMissingScope)
	m := pinsModel(t)
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Roadmap"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	if out := m.View(); !strings.Contains(out, "gh auth refresh -s read:project") {
		t.Errorf("no hint:\n%s", out)
	}
	m = keyAll(m, "esc")
	if m.view != viewFavs {
		t.Errorf("esc: %v", m.view)
	}
}

func TestPreviewPaneFollowsBoardRows(t *testing.T) {
	m, _ := boardOn(t)
	m.previewOff, m.width = false, 160
	m = pump(m, m.fetch())
	if _, _, _, ok := m.previewTarget(); !ok {
		t.Fatal("the pane should follow an issue row")
	}
	m = keyAll(m, "j", "j", "j") // the draft
	if _, _, _, ok := m.previewTarget(); ok {
		t.Error("a draft card has nothing to preview")
	}
}

func iconsPROpen() icons.Name   { return icons.PROpen }
func iconsPRMerged() icons.Name { return icons.PRMerged }

func TestLateOrDuplicatePinAnswerDoesNotUnpin(t *testing.T) {
	installBoard(t)
	m := pinsModel(t)
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Roadmap"})
	next, _ := m.Update(projectPinMsg{owner: "acme", number: 3, p: sampleProject()})
	if !next.(model).pins.HasProject("", "acme", 3) {
		t.Error("a second answer unpinned the Project")
	}
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
	next, _ = m.Update(repoPinMsg{repo: pins.Repo{Repo: "acme/one"}})
	if !next.(model).pins.HasRepo("", "acme/one") {
		t.Error("a late check unpinned the repo")
	}
}

func TestHTTPOnlyServerStillPins(t *testing.T) {
	m := pinsModel(t)
	p := sampleProject()
	p.URL = "http://ghe.local/orgs/acme/projects/3"
	next, _ := m.Update(projectPinMsg{host: "ghe.local", owner: "acme", number: 3, p: p})
	got := next.(model).pins
	if !got.HasProject("ghe.local", "acme", 3) || got.Projects[0].URL != "" {
		t.Errorf("%+v", got.Projects)
	}
}

func TestPickerUnpinRefreshesTheReposTab(t *testing.T) {
	installPickerFakes(t, []ghcli.RepoInfo{info("acme/one")})
	m := pickerModel(t)
	ps, _ := pins.Load(filepath.Join(t.TempDir(), "pins.json"))
	m.pins = ps
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
	m.view, m.favTab = viewFavs, favTabRepos
	m = typeText(t, openPicker(t, m), "acme")
	m = pickKeys(t, m, "down")
	_, c := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if c == nil {
		t.Error("no refresh after unpinning from the picker")
	}
}

func TestBoardOpensOnlyPlainHTTPSAndValidRepos(t *testing.T) {
	f := installBoard(t)
	f.project.URL = "javascript:alert(1)"
	f.project.Items = []ghcli.ProjectItem{{Type: "Issue", Number: 1, Title: "x", Repo: "-evil/--repo", URL: "file:///etc/passwd", State: "OPEN"}}
	m := pinsModel(t)
	m.previewOff = true
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	var opened []string
	openURL = func(u string) error { opened = append(opened, u); return nil }
	m = keyAll(m, "o")
	m = keyAll(m, "enter")
	if len(opened) != 0 || m.view != viewBoard || m.target != m.base {
		t.Errorf("opened %v view %v target %+v", opened, m.view, m.target)
	}
}

func TestTabBarsFitNarrowTerminals(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", Total: 1, Items: []ghcli.ProjectItem{projItem("Issue", "a/b", 1, "t", "状态四十", "OPEN")}}
	for i := 0; i < 40; i++ {
		f.project.StatusOrder = append(f.project.StatusOrder, fmt.Sprintf("状态%d", i))
	}
	f.project.StatusOrder = append(f.project.StatusOrder, "状态四十")
	for _, w := range []int{60, 80, 200} {
		m := pinsModel(t)
		m.width, m.previewOff = w, true
		m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
		m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
		m = keyAll(m, "G") // no-op on tabs; then walk to the last column
		for i := 0; i < 41; i++ {
			m = keyAll(m, "tab")
		}
		lines := strings.Split(m.View(), "\n")
		if got := ansi.StringWidth(lines[2]); got > w {
			t.Errorf("width %d: tab bar is %d wide", w, got)
		}
		if w <= 80 && !strings.Contains(ansi.Strip(lines[2]), "状态四十") {
			t.Errorf("width %d: selected column scrolled out of the bar: %q", w, ansi.Strip(lines[2]))
		}
	}
}

func TestFPinsAndUnpinsWhereTheThingIsShown(t *testing.T) {
	for _, v := range []viewKind{viewRuns, viewPRs, viewIssues} {
		m := pinsModel(t)
		m.view, m.previewOff = v, true
		m.tbl = table{cols: []string{"A"}, rows: []row{{id: "1", cells: []string{"x"}}}}
		m = keyAll(m, "f")
		if !m.pins.HasRepo("", "acme/api") {
			t.Fatalf("view %v: f did not pin the repo (%q)", v, m.status)
		}
		m = keyAll(m, "f")
		if m.pins.HasRepo("", "acme/api") {
			t.Errorf("view %v: second f did not unpin", v)
		}
	}
	m, _ := boardOn(t)
	m = keyAll(m, "f")
	if len(m.pins.Projects) != 0 {
		t.Fatalf("f on a pinned Project's board should unpin it: %+v", m.pins.Projects)
	}
	m = keyAll(m, "f")
	if len(m.pins.Projects) != 1 || m.pins.Projects[0].Title != "Roadmap" {
		t.Errorf("f did not pin again: %+v", m.pins.Projects)
	}
}

func TestPinAProjectFromTheList(t *testing.T) {
	f := installBoard(t)
	f.listed = map[string][]ghcli.ProjectRef{"github.com": {
		{Owner: "me", Number: 1, Title: "Personal", URL: "https://github.com/users/me/projects/1"},
		{Owner: "ai-ocp", Number: 1, Title: "one-click-podcast", URL: "https://github.com/orgs/ai-ocp/projects/1"},
	}}
	m := pressTab(pressTab(openFavs(t, pinsModel(t)), "tab"), "tab")
	m = keyAll(m, "a")
	if m.dlg == nil || !strings.Contains(m.View(), "one-click-podcast") || !strings.Contains(m.View(), "Type OWNER/NUMBER") {
		t.Fatalf("no list:\n%s", m.View())
	}
	m = keyAll(typeAll(m, "podcast"), "enter")
	if len(m.pins.Projects) != 1 || m.pins.Projects[0].Owner != "ai-ocp" || m.pins.Projects[0].Title != "one-click-podcast" || len(f.calls) != 0 {
		t.Fatalf("pins %+v reads %v", m.pins.Projects, f.calls)
	}
	m = keyAll(m, "a")
	if !strings.Contains(m.View(), "already pinned") {
		t.Errorf("a pinned Project should be marked:\n%s", m.View())
	}
}

func TestProjectListMissingScopeAndLateAnswer(t *testing.T) {
	f := installBoard(t)
	f.listErr = fmt.Errorf("%w: x", ghcli.ErrMissingScope)
	m := pressTab(pressTab(openFavs(t, pinsModel(t)), "tab"), "tab")
	m = keyAll(m, "a")
	if m.dlg != nil || !strings.Contains(m.status, "gh auth refresh -s read:project") {
		t.Errorf("dlg %v status %q", m.dlg, m.status)
	}
	// the list arrives after the user left the Projects tab
	m = pressTab(m, "tab")
	next, _ := m.Update(projectListMsg{refs: []ghcli.ProjectRef{{Owner: "a", Number: 1, Title: "t"}}})
	if next.(model).dlg != nil {
		t.Error("a late list opened a dialog on another screen")
	}
}

func viewsProject() ghcli.Project {
	p := sampleProject()
	p.Views = []ghcli.ProjectView{
		{Name: "Backlog", Layout: "BOARD_LAYOUT"},
		{Name: "My items", Layout: "TABLE_LAYOUT", Filter: "assignee:@me"},
		{Name: "Bugs by title", Layout: "TABLE_LAYOUT", Filter: "is:pr", SortField: "Title"},
		{Name: "Sprint", Layout: "TABLE_LAYOUT", Filter: "iteration:@current is:issue", SortField: "Priority"},
		{Name: "Plan", Layout: "ROADMAP_LAYOUT"},
	}
	return p
}

func boardWithViews(t *testing.T, me string) (model, *boardFakes) {
	t.Helper()
	f := installBoard(t)
	f.project = viewsProject()
	old := inboxLogin
	inboxLogin = func(string) (string, error) {
		if me == "" {
			return "", errors.New("no login")
		}
		return me, nil
	}
	t.Cleanup(func() { inboxLogin = old })
	m := pinsModel(t)
	m.previewOff = true
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "Roadmap"})
	return keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter"), f
}

func TestBoardListsViewsAndSwitchesWithBrackets(t *testing.T) {
	m, _ := boardWithViews(t, "ann")
	out := ansi.Strip(m.View())
	for _, want := range []string{"All items", "Backlog", "My items", "Plan (roadmap)", "All (5)"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	m = keyAll(m, "]", "]") // My items: assignee:@me
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "All (3)") || strings.Contains(out, "Idea") {
		t.Errorf("My items should keep the three issue/PR cards assigned to ann:\n%s", out)
	}
	m = keyAll(m, "[", "[")
	if !strings.Contains(ansi.Strip(m.View()), "All (5)") {
		t.Error("back on All items")
	}
}

func TestBoardViewFilterSortAndHonestNotes(t *testing.T) {
	m, _ := boardWithViews(t, "ann")
	m = keyAll(m, "]", "]", "]") // Bugs by title: is:pr sorted by title
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "All (2)") || strings.Index(out, "Add retry") > strings.Index(out, "Old work") {
		t.Errorf("pr view:\n%s", out)
	}
	m = keyAll(m, "]") // Sprint: unsupported iteration term and sort field
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "filter not fully applied: iteration:@current") || !strings.Contains(out, "sort by Priority not applied") {
		t.Errorf("notes:\n%s", out)
	}
	if !strings.Contains(out, "All (1)") { // is:issue still applied
		t.Errorf("supported term dropped:\n%s", out)
	}
	m = keyAll(m, "]") // roadmap
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "roadmap shows no date field") || !strings.Contains(out, "All (5)") {
		t.Errorf("roadmap:\n%s", out)
	}
}

func TestBoardViewNeedingMeWithoutLoginShowsAllWithANote(t *testing.T) {
	m, _ := boardWithViews(t, "")
	m = keyAll(m, "]", "]")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "needs your login") || !strings.Contains(out, "All (5)") {
		t.Errorf("%s", out)
	}
}

func TestBoardWithoutViewsHasNoViewRowAndDeletedViewResets(t *testing.T) {
	m, f := boardOn(t)
	if strings.Contains(ansi.Strip(m.View()), "All items") {
		t.Error("a Project with no views shows no view row")
	}
	m, f = boardWithViews(t, "ann")
	m = keyAll(m, "]", "]", "]", "]", "]") // Plan, the last one
	f.project = sampleProject()            // the views were deleted on GitHub
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	if st := m.boardSt(); st.view != 0 {
		t.Errorf("view %d after the views vanished", st.view)
	}
	m = keyAll(m, "]") // no-op without views
	if !strings.Contains(ansi.Strip(m.View()), "All (5)") {
		t.Error("view")
	}
}

func TestBoardViewRowFitsNarrowTerminals(t *testing.T) {
	for _, w := range []int{50, 80, 200} {
		m, _ := boardWithViews(t, "ann")
		m.width = w
		m = keyAll(m, "[") // last view: Plan
		lines := strings.Split(m.View(), "\n")
		for i := 2; i <= 4; i++ {
			if got := ansi.StringWidth(lines[i]); got > w {
				t.Errorf("width %d line %d is %d wide: %q", w, i, got, ansi.Strip(lines[i]))
			}
		}
		if w == 50 && !strings.Contains(ansi.Strip(lines[2]), "Plan") {
			t.Errorf("selected view not visible: %q", ansi.Strip(lines[2]))
		}
	}
}

func TestBoardKeepsTheViewByNameAcrossRefreshAndProjects(t *testing.T) {
	m, f := boardWithViews(t, "ann")
	m = keyAll(m, "]", "]", "]") // Bugs by title
	// a view before it is deleted: the same view must stay selected
	f.project.Views = append([]ghcli.ProjectView{}, f.project.Views[1:]...)
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	if st := m.boardSt(); st.viewName != "Bugs by title" || st.view != 2 {
		t.Errorf("view %d %q after a view before it was deleted", st.view, st.viewName)
	}
	// leave for another Project and come back
	m = keyAll(m, "esc")
	other := pins.Project{Owner: "other", Number: 1, Title: "Other"}
	m.pins.ToggleProject(other)
	m = pump(m, m.fetch())
	m, _ = m.openBoard(other)
	m = pump(m, m.fetch())
	m = keyAll(m, "esc")
	m = pump(m, m.fetch())
	m = keyAll(m, "k", "enter")
	if st := m.boardSt(); st.viewName != "Bugs by title" {
		t.Errorf("the view was not kept for the Project: %d %q", st.view, st.viewName)
	}
}

func TestLoginIsAskedOnceAndAFailureKeepsTheOldOne(t *testing.T) {
	m, f := boardWithViews(t, "ann")
	asks := 0
	inboxLogin = func(string) (string, error) { asks++; return "ann", nil }
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	if asks != 0 {
		t.Errorf("asked %d times although the login was known", asks)
	}
	inboxLogin = func(string) (string, error) { return "", errors.New("offline") }
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	if m.boardSt().me != "ann" {
		t.Errorf("a failed lookup wiped the login: %q", m.boardSt().me)
	}
}

func TestPreviewPaneFillsTheScreenWhateverTheItemCount(t *testing.T) {
	m, _ := boardOn(t)
	m.previewOff, m.width, m.height = false, 160, 40
	m = pump(m, m.fetch())
	if got := strings.Count(m.View(), "\n"); got != 39 {
		t.Errorf("board with a pane is %d lines tall on a 40-line terminal; the pane must not shrink to the item count", got)
	}
	m.tbl.rows = m.tbl.rows[:1] // one item only
	if got := strings.Count(m.View(), "\n"); got != 39 {
		t.Errorf("with one item the view is %d lines", got)
	}
}

func TestPreviewPaneShowsTheTextOfADraftCard(t *testing.T) {
	m, f := boardOn(t)
	f.project.Items[3].Body = "First line\n\nSecond paragraph about the idea"
	f.now = f.now.Add(10 * time.Second)
	m.previewOff, m.width, m.height = false, 160, 40
	m = keyAll(m, "R")
	m = keyAll(m, "j", "j", "j") // the draft
	out := ansi.Strip(m.View())
	for _, want := range []string{"Idea", "draft - only in the Project", "First line", "Second paragraph about the idea"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane lacks %q:\n%s", want, out)
		}
	}
	m = keyAll(m, "k") // a real issue row is still previewed through the fetch, not as a draft
	if _, ok := m.draftLines(40); ok {
		t.Error("a non-draft row was treated as a draft")
	}
}

func columnsBoard(t *testing.T, width, height int) model {
	t.Helper()
	m, _ := boardOn(t)
	m.width, m.height = width, height
	return keyAll(m, "b")
}

func TestBoardColumnsShowEveryStatusSideBySide(t *testing.T) {
	m := columnsBoard(t, 120, 30)
	out := ansi.Strip(m.View())
	for _, want := range []string{"Todo (1)", "In Progress (1)", "Done (1)", "No status (2)", "acme/app #1", "Fix login", "ann · bug", "(draft)", "(no access)"} {
		if !strings.Contains(out, want) {
			t.Errorf("columns lack %q:\n%s", want, out)
		}
	}
	// the headers sit on one line: that is what side by side means
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Todo (1)") && !(strings.Contains(l, "In Progress (1)") && strings.Contains(l, "Done (1)")) {
			t.Errorf("headers are not on one line: %q", l)
		}
	}
	all := strings.Split(m.View(), "\n")
	for i, l := range all[:len(all)-1] { // the last line is the key footer
		if ansi.StringWidth(l) > 120 && i != 1 {
			t.Errorf("line %d is %d wide", i, ansi.StringWidth(l))
		}
	}
}

func TestBoardColumnsMovement(t *testing.T) {
	m := columnsBoard(t, 120, 30)
	m = keyAll(m, "l") // In Progress
	if r, ok := m.boardSelectedRow(); !ok || r.id != "acme/app#2" {
		t.Fatalf("after l: %+v %v", r, ok)
	}
	m = keyAll(m, "l", "l") // Done, then No status
	if r, _ := m.boardSelectedRow(); r.id != "i3" {
		t.Errorf("no-status column starts on its first card: %q", r.id)
	}
	m = keyAll(m, "j")
	if r, _ := m.boardSelectedRow(); r.id != "i4" {
		t.Errorf("j: %q", r.id)
	}
	m = keyAll(m, "j", "k", "k", "k")
	if r, _ := m.boardSelectedRow(); r.id != "i3" {
		t.Errorf("k stops at the first card: %q", r.id)
	}
	m = keyAll(m, "l") // wraps to Todo
	if r, _ := m.boardSelectedRow(); r.id != "acme/app#1" {
		t.Errorf("wrap: %q", r.id)
	}
	m = keyAll(m, "enter")
	if m.view != viewIssue || m.issue != "1" {
		t.Errorf("enter: view %v issue %q", m.view, m.issue)
	}
}

func TestBoardColumnsToggleKeepsTheCard(t *testing.T) {
	m, _ := boardOn(t)
	m.width, m.height = 120, 30
	m = keyAll(m, "tab", "tab") // list: In Progress tab
	m = keyAll(m, "b")
	if r, _ := m.boardSelectedRow(); r.id != "acme/app#2" {
		t.Fatalf("columns select %q", r.id)
	}
	m = keyAll(m, "l", "b") // Done, back to the list
	if r, ok := m.selected(); !ok || r.id != "acme/app#3" || m.boardSt().tab != 3 {
		t.Errorf("list selects %+v tab %d", r, m.boardSt().tab)
	}
}

func TestBoardColumnsNeedWidthAndWindowAround(t *testing.T) {
	m := columnsBoard(t, 50, 30)
	if m.boardSt().cols || !strings.Contains(m.status, "at least 60") {
		t.Errorf("narrow terminal: cols %v status %q", m.boardSt().cols, m.status)
	}
	m = columnsBoard(t, 70, 30) // two columns fit: 4 exist
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "›") || strings.Contains(out, "Done (1)") {
		t.Errorf("window:\n%s", out)
	}
	m = keyAll(m, "l", "l", "l")
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "‹") || !strings.Contains(out, "No status (2)") || strings.Contains(out, "Todo (1)") {
		t.Errorf("window after moving right:\n%s", out)
	}
	for i, l := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(l) > 70 && i != 1 && i != len(strings.Split(m.View(), "\n"))-1 {
			t.Errorf("line %d is %d wide", i, ansi.StringWidth(l))
		}
	}
}

func TestBoardColumnsScrollAndFilterAndEmpty(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", StatusOrder: []string{"Todo", "Done"}, Total: 30}
	for i := 1; i <= 30; i++ {
		f.project.Items = append(f.project.Items, projItem("Issue", "acme/app", i, fmt.Sprintf("card number %d", i), "Todo", "OPEN"))
	}
	m := pinsModel(t)
	m.previewOff, m.width, m.height = true, 100, 24
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	m = keyAll(m, "b")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "Done (0)") || !strings.Contains(out, "(empty)") {
		t.Errorf("empty column:\n%s", out)
	}
	for i := 0; i < 25; i++ {
		m = keyAll(m, "j")
	}
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "card number 26") || strings.Contains(out, "card number 1 ") {
		t.Errorf("the column did not scroll to the selected card:\n%s", out)
	}
	if got := strings.Count(m.View(), "\n"); got > 23 {
		t.Errorf("view is %d lines on a 24-line terminal", got+1)
	}
	m = keyAll(m, "l", "j", "j") // the empty column: nothing to move to
	if _, ok := m.boardSelectedRow(); ok {
		t.Error("an empty column has no selected card")
	}
	m = keyAll(m, "enter", "o") // and must not crash or open anything
	m = keyAll(m, "h", "g")
	if r, _ := m.boardSelectedRow(); r.id != "acme/app#1" {
		t.Errorf("g: %q", r.id)
	}
}

func TestBoardColumnsToggleKeepsTheSecondCard(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", StatusOrder: []string{"Todo"}, Total: 3, Items: []ghcli.ProjectItem{
		projItem("Issue", "acme/app", 1, "one", "Todo", "OPEN"),
		projItem("Issue", "acme/app", 2, "two", "Todo", "OPEN"),
		projItem("Issue", "acme/app", 3, "three", "Todo", "OPEN"),
	}}
	m := pinsModel(t)
	m.previewOff, m.width, m.height = true, 100, 30
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	m = keyAll(m, "tab", "j", "j", "b") // list on Todo, third card, to columns
	if r, _ := m.boardSelectedRow(); r.id != "acme/app#3" {
		t.Errorf("columns select %q, want the third card", r.id)
	}
	m = keyAll(m, "k", "b")
	if r, _ := m.selected(); r.id != "acme/app#2" {
		t.Errorf("back in the list the card is %q, want the second", r.id)
	}
}

func TestBoardColumnsFillTheTerminalExactly(t *testing.T) {
	for _, withViews := range []bool{false, true} {
		for _, h := range []int{20, 24, 40} {
			for _, w := range []int{60, 100, 200} {
				var m model
				if withViews {
					m, _ = boardWithViews(t, "ann")
				} else {
					m, _ = boardOn(t)
				}
				m.width, m.height = w, h
				list := strings.Count(m.View(), "\n")
				m = keyAll(m, "b")
				if got := strings.Count(m.View(), "\n"); got != list {
					t.Errorf("views=%v %dx%d: columns are %d lines, the list %d", withViews, w, h, got+1, list+1)
				}
			}
		}
	}
}

func TestBoardColumnsSurviveRefreshViewSwitchFilterAndErrors(t *testing.T) {
	m, f := boardWithViews(t, "ann")
	m.width, m.height = 120, 30
	m = keyAll(m, "b", "l", "l") // Done: acme/app#3
	// a new card arrives first in Done; R must keep the selection on #3 by id
	f.project.Items = append([]ghcli.ProjectItem{projItem("Issue", "acme/app", 9, "new", "Done", "OPEN")}, f.project.Items...)
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	if r, _ := m.boardSelectedRow(); r.id != "acme/app#3" {
		t.Errorf("after R the card is %q", r.id)
	}
	// a view without a "No status" column: enter still works on what is selected
	m = keyAll(m, "l", "]", "]") // No status, then My items (assignee:@me): every item has a Status there
	if _, ok := m.boardSelectedRow(); !ok {
		t.Error("after a view switch nothing is selected although the column has cards")
	}
	m = keyAll(m, "enter")
	if m.view == viewBoard {
		t.Error("enter did nothing after the view switch")
	}
	// a failed refresh is visible
	m = keyAll(m, "esc")
	f.err = errors.New("boom network")
	f.now = f.now.Add(10 * time.Second)
	m = keyAll(m, "R")
	if !strings.Contains(ansi.Strip(m.View()), "boom network") {
		t.Errorf("error not shown in columns:\n%s", m.View())
	}
}

func TestBoardColumnsFilterClampsEveryColumn(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", StatusOrder: []string{"Todo", "Done"}, Total: 30}
	for i := 1; i <= 30; i++ {
		f.project.Items = append(f.project.Items, projItem("Issue", "acme/app", i, fmt.Sprintf("card number %d", i), "Todo", "OPEN"))
	}
	m := pinsModel(t)
	m.previewOff, m.width, m.height = true, 100, 24
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	m = keyAll(m, "b", "G", "l")
	m = keyAll(typeAll(keyAll(m, "/"), "card number 1"), "enter")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "acme/app #1") {
		t.Errorf("the Todo column is blank after a filter:\n%s", out)
	}
}

func TestBoardColumnsShrinkAndSessionMemoryAndP(t *testing.T) {
	m, _ := boardOn(t)
	m.width, m.height = 120, 30
	m = keyAll(m, "b")
	if m.status != "" && strings.Contains(m.status, "preview") {
		t.Error("status")
	}
	m = keyAll(m, "p")
	if !strings.Contains(m.status, "not shown in columns") {
		t.Errorf("p says %q", m.status)
	}
	m.width = 50 // the terminal shrinks: the list is on screen
	m = keyAll(m, "tab")
	tab := m.boardSt().tab
	m = keyAll(m, "b")
	if m.boardSt().cols || m.boardSt().tab != tab {
		t.Errorf("b on a shrunk terminal changed the list: cols %v tab %d (was %d)", m.boardSt().cols, m.boardSt().tab, tab)
	}
	// the choice follows the session to another Project
	m.width = 120
	m = keyAll(m, "b")
	m = keyAll(m, "esc")
	other := pins.Project{Owner: "other", Number: 1, Title: "Other"}
	m.pins.ToggleProject(other)
	m = pump(m, m.fetch())
	m, _ = m.openBoard(other)
	m = pump(m, m.fetch())
	if !m.boardSt().cols {
		t.Error("columns were not kept for the session")
	}
}

func TestWrapTitleKeepsWordsAndCutsLongOnes(t *testing.T) {
	cases := []struct {
		title string
		w     int
		want  []string
	}{
		{"short", 20, []string{"short", ""}},
		{"fix the login page now", 12, []string{"fix the", "login page …"}},
		{"Supercalifragilisticexpialidocious_long", 10, []string{"Supercali…", ""}},
		{"a Supercalifragilisticexpialidocious b", 10, []string{"a", "Supercali…"}},
		{"日本語のタイトルがとても長い場合", 8, []string{"日本語の", ""}},
	}
	for _, c := range cases {
		got := wrapTitle(c.title, c.w)
		if c.title == "日本語のタイトルがとても長い場合" {
			if ansi.StringWidth(got[0]) > c.w || ansi.StringWidth(got[1]) > c.w {
				t.Errorf("wide chars overflow: %q", got)
			}
			continue
		}
		if got[0] != c.want[0] || got[1] != c.want[1] {
			t.Errorf("wrapTitle(%q,%d) = %q, want %q", c.title, c.w, got, c.want)
		}
	}
}

func TestColumnHeaderKeepsTheCountAndTheFooterListsTheKeys(t *testing.T) {
	f := installBoard(t)
	f.project = ghcli.Project{Title: "P", StatusOrder: []string{"A very long status name that cannot fit", "Another quite long status name", "Third long status name here"}, Total: 1,
		Items: []ghcli.ProjectItem{projItem("Issue", "acme/app", 1, "x", "A very long status name that cannot fit", "OPEN")}}
	m := pinsModel(t)
	m.previewOff, m.width, m.height = true, 60, 24
	m.pins.ToggleProject(pins.Project{Owner: "acme", Number: 3, Title: "P"})
	m = keyAll(pressTab(pressTab(openFavs(t, m), "tab"), "tab"), "enter")
	m = keyAll(m, "b")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "(1)") || !strings.Contains(out, "…") {
		t.Errorf("header:\n%s", out)
	}
	if !strings.Contains(out, "j/k card") {
		t.Errorf("footer:\n%s", out)
	}
}

func TestDraftCardTextScrollsInThePane(t *testing.T) {
	m, f := boardOn(t)
	var long []string
	for i := 1; i <= 60; i++ {
		long = append(long, fmt.Sprintf("line %d of the draft", i))
	}
	f.project.Items[3].Body = strings.Join(long, "\n")
	f.project.Items[4] = ghcli.ProjectItem{Type: "DraftIssue", Title: "Other idea", Draft: true, Body: "second draft text\n" + strings.Join(long, "\n")}
	f.now = f.now.Add(10 * time.Second)
	m.previewOff, m.width, m.height = false, 160, 30
	m = keyAll(m, "R")
	m = keyAll(m, "j", "j", "j") // the draft
	if !strings.Contains(ansi.Strip(m.View()), "line 1 of the draft") {
		t.Fatalf("no draft text:\n%s", m.View())
	}
	m = keyAll(m, "J", "J", "J", "J", "J", "J", "J", "J")
	out := ansi.Strip(m.View())
	if strings.Contains(out, "line 1 of the draft") || !strings.Contains(out, "line 25 of the draft") {
		t.Errorf("J did not scroll the draft text:\n%s", out)
	}
	m = keyAll(m, "j") // the next card is a draft too: its text starts at the top
	if !strings.Contains(ansi.Strip(m.View()), "second draft text") {
		t.Errorf("the scroll was kept for a different draft:\n%s", m.View())
	}
}
