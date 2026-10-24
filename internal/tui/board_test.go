package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	oldFetch, oldNow, oldOpen := fetchProject, boardNow, openURL
	fetchProject = func(host, owner string, n int) (ghcli.Project, error) {
		f.calls = append(f.calls, fmt.Sprintf("%s|%s|%d", host, owner, n))
		return f.project, f.err
	}
	boardNow = func() time.Time { return f.now }
	openURL = func(string) error { return nil }
	t.Cleanup(func() { fetchProject, boardNow, openURL = oldFetch, oldNow, oldOpen })
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
	m = keyAll(typeAll(keyAll(m, "a"), "acme/3"), "enter")
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
	m = keyAll(typeAll(keyAll(m, "a"), "acme/zero"), "enter")
	if len(f.calls) != 1 || !strings.Contains(m.status, "invalid") {
		t.Errorf("calls %v status %q", f.calls, m.status)
	}
}

func TestPinUnknownProjectAndMissingScope(t *testing.T) {
	f := installBoard(t)
	m := pressTab(pressTab(openFavs(t, pinsModel(t)), "tab"), "tab")
	f.err = errors.New("project not found or no access")
	m = keyAll(typeAll(keyAll(m, "a"), "acme/8"), "enter")
	if len(m.pins.Projects) != 0 || !strings.Contains(m.status, "not found") {
		t.Errorf("unknown: %+v %q", m.pins.Projects, m.status)
	}
	f.err = fmt.Errorf("%w: x", ghcli.ErrMissingScope)
	m = keyAll(typeAll(keyAll(m, "a"), "ghe.corp.com/corp/2"), "enter")
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

func TestProjectsAreOnlyEverRead(t *testing.T) {
	for _, f := range []string{"board.go", "favprojects.go", "favtabs.go", "../ghcli/project.go"} {
		b, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ToLower(string(b))
		for _, bad := range []string{"mutation", "item-edit", "item-add", "item-delete", "item-create", "\"project\", \"edit\"", "-x\", \"post", "--method"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s contains %q: Projects are read-only in this release", f, bad)
			}
		}
	}
}

func iconsPROpen() icons.Name   { return icons.PROpen }
func iconsPRMerged() icons.Name { return icons.PRMerged }
