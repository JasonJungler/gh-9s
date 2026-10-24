package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/pins"
)

// pinsModel is the favorites screen with a temp pins file and no network.
func pinsModel(t *testing.T) model {
	t.Helper()
	oldCheck, oldLatest := checkRepo, latestFetcher
	checkRepo = func(ghcli.RepoRef) error { return nil }
	latestFetcher = func(ghcli.RepoRef, string) (string, state, string, error) { return "ok", stOther, "1m", nil }
	t.Cleanup(func() { checkRepo, latestFetcher = oldCheck, oldLatest })
	m := favModel(t)
	ps, err := pins.Load(filepath.Join(t.TempDir(), "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.pins = ps
	return m
}

func openFavs(t *testing.T, m model) model {
	t.Helper()
	m = typeCommand(t, m, "fav")
	return pump(m, m.fetch())
}

func pressTab(m model, name string) model {
	var msg tea.KeyMsg
	if name == "shift+tab" {
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	} else {
		msg = k(name)
	}
	next, c := m.Update(msg)
	return pump(next.(model), c)
}

func TestFavTabBarCountsAndStartsOnWorkflows(t *testing.T) {
	m := pinsModel(t)
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
	m = openFavs(t, m)
	if m.view != viewFavs || m.favTab != favTabWorkflows {
		t.Fatalf("view %v tab %d", m.view, m.favTab)
	}
	out := m.View()
	for _, want := range []string{"Workflows (0)", "Repos (1)", "Projects (0)"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab bar lacks %q:\n%s", want, out)
		}
	}
}

func TestFavTabsCycleAndAreRemembered(t *testing.T) {
	m := openFavs(t, pinsModel(t))
	m = pressTab(m, "tab")
	if m.favTab != favTabRepos {
		t.Fatalf("tab %d", m.favTab)
	}
	m = pressTab(m, "l")
	m = pressTab(m, "l")
	if m.favTab != favTabWorkflows {
		t.Fatalf("wrap: %d", m.favTab)
	}
	m = pressTab(m, "shift+tab")
	if m.favTab != favTabProjects {
		t.Fatalf("back: %d", m.favTab)
	}
	m = pressTab(m, "h")
	if m.favTab != favTabRepos {
		t.Fatalf("h: %d", m.favTab)
	}
	m = typeCommand(t, m, "runs")
	m = openFavs(t, m)
	if m.favTab != favTabRepos {
		t.Errorf("tab not remembered: %d", m.favTab)
	}
}

func TestReposTabListsPinsAndShowsHostOnlyWhenNotGitHub(t *testing.T) {
	m := pinsModel(t)
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
	m.pins.ToggleRepo(pins.Repo{Host: "ghe.corp.com", Repo: "acme/two"})
	m = pressTab(openFavs(t, m), "tab")
	out := m.View()
	if !strings.Contains(out, "acme/one") || !strings.Contains(out, "ghe.corp.com/acme/two") || strings.Contains(out, "github.com/acme/one") {
		t.Errorf("rows:\n%s", out)
	}
}

func TestPinRepoFromTheReposTab(t *testing.T) {
	m := pressTab(openFavs(t, pinsModel(t)), "tab")
	m = keyAll(m, "a")
	if m.dlg == nil {
		t.Fatal("no dialog")
	}
	m = keyAll(typeAll(m, "acme/new"), "enter")
	if !m.pins.HasRepo("", "acme/new") {
		t.Fatalf("not pinned: %+v %q", m.pins.Repos, m.status)
	}
	again, err := pins.Load(m.pins.Path())
	if err != nil || !again.HasRepo("", "acme/new") {
		t.Fatalf("not saved: %v", err)
	}
	if !strings.Contains(m.View(), "acme/new") {
		t.Error("row missing")
	}
}

func TestPinRepoRejectsBadNamesAndMissingRepos(t *testing.T) {
	m := pressTab(openFavs(t, pinsModel(t)), "tab")
	m = keyAll(typeAll(keyAll(m, "a"), "not a repo"), "enter")
	if len(m.pins.Repos) != 0 || !strings.Contains(m.status, "invalid") {
		t.Errorf("bad name: %+v %q", m.pins.Repos, m.status)
	}
	checkRepo = func(ghcli.RepoRef) error { return os.ErrNotExist }
	m = keyAll(typeAll(keyAll(m, "a"), "acme/ghost"), "enter")
	if len(m.pins.Repos) != 0 || !strings.Contains(m.status, "acme/ghost") {
		t.Errorf("missing repo: %+v %q", m.pins.Repos, m.status)
	}
}

func TestUnpinRepoAsksFirst(t *testing.T) {
	m := pinsModel(t)
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
	m.pins.Save()
	m = pressTab(openFavs(t, m), "tab")
	m = keyAll(m, "f")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "acme/one") {
		t.Fatalf("no confirm: %+v", m.dlg)
	}
	m = keyAll(m, "esc")
	if !m.pins.HasRepo("", "acme/one") {
		t.Fatal("esc removed the pin")
	}
	m = keyAll(keyAll(m, "d"), "y")
	if m.pins.HasRepo("", "acme/one") {
		t.Fatal("pin kept")
	}
	if again, _ := pins.Load(m.pins.Path()); again.HasRepo("", "acme/one") {
		t.Error("not saved")
	}
}

func TestEnterOnPinnedRepoSwitchesLikeRepoCommand(t *testing.T) {
	m := pinsModel(t)
	m.pins.ToggleRepo(pins.Repo{Host: "ghe.corp.com", Repo: "acme/two"})
	m = pressTab(openFavs(t, m), "tab")
	next, c := m.Update(k("enter"))
	m = next.(model)
	if c == nil {
		t.Fatal("no check")
	}
	next, _ = m.Update(c())
	m = next.(model)
	want := ghcli.RepoRef{Host: "ghe.corp.com", Owner: "acme", Name: "two"}
	if m.base != want || m.target != want {
		t.Errorf("base %v target %v", m.base, m.target)
	}
	if m.view != viewRuns {
		t.Errorf("view %v", m.view)
	}
}

func TestFavRepoCommand(t *testing.T) {
	m := pinsModel(t)
	m = pump(typeCommand(t, m, "fav repo"), nil)
	// the check runs through the command returned; typeCommand delivered it
	if !m.pins.HasRepo("", "acme/api") {
		t.Fatalf("current repo not pinned: %+v %q", m.pins.Repos, m.status)
	}
	m.curRepo, m.target = ghcli.RepoRef{}, ghcli.RepoRef{}
	m = typeCommand(t, m, "fav repo")
	if !strings.Contains(m.status, "cannot tell which repo") {
		t.Errorf("status %q", m.status)
	}
	m = typeCommand(t, m, "fav repo other/thing")
	if !m.pins.HasRepo("", "other/thing") {
		t.Errorf("explicit repo not pinned: %+v", m.pins.Repos)
	}
}

func TestPickerMarksPinnedAndCtrlFToggles(t *testing.T) {
	installPickerFakes(t, []ghcli.RepoInfo{info("acme/one"), info("acme/two")})
	m := pickerModel(t)
	ps, _ := pins.Load(filepath.Join(t.TempDir(), "pins.json"))
	m.pins = ps
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/two"})
	m = typeText(t, openPicker(t, m), "acme")
	if !strings.Contains(m.View(), "★ acme/two") || strings.Contains(m.View(), "★ acme/one") {
		t.Fatalf("marks:\n%s", m.View())
	}
	// move to acme/one and pin it
	for i := 0; i < 10 && !strings.Contains(m.picker.rows(m.curRepo, m.recentList())[m.picker.sel].label, "acme/one"); i++ {
		m = pickKeys(t, m, "down")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = next.(model)
	if !m.pins.HasRepo("", "acme/one") || !strings.Contains(m.View(), "★ acme/one") {
		t.Errorf("not pinned: %+v", m.pins.Repos)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if next.(model).pins.HasRepo("", "acme/one") {
		t.Error("second ctrl+f should unpin")
	}
}

func TestBrokenPinsFileWorksForTheSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	os.WriteFile(path, []byte("{broken"), 0o600)
	m := pinsModel(t)
	m.pins = pins.Memory()
	m = pressTab(openFavs(t, m), "tab")
	m = keyAll(typeAll(keyAll(m, "a"), "acme/new"), "enter")
	if !m.pins.HasRepo("", "acme/new") || !strings.Contains(m.status, "not saved") {
		t.Errorf("%+v %q", m.pins.Repos, m.status)
	}
	if b, _ := os.ReadFile(path); string(b) != "{broken" {
		t.Error("broken file touched")
	}
}

func TestFavTabsKeepWidthsWithWideNames(t *testing.T) {
	for _, w := range []int{60, 100, 200} {
		m := pinsModel(t)
		m.width = w
		m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
		m = pressTab(openFavs(t, m), "tab")
		lines := strings.Split(m.View(), "\n")
		for i, line := range lines[:len(lines)-1] { // the last line is the key footer, as wide as it is on every screen
			if got := ansi.StringWidth(line); got > w && i != 1 { // line 1 is the fixed key hint, wider than 60 columns before this release
				t.Errorf("width %d line %d is %d wide: %q", w, i, got, line)
			}
		}
	}
}

func TestLateAnswerForAnotherTabIsDropped(t *testing.T) {
	m := pinsModel(t)
	m.pins.ToggleRepo(pins.Repo{Repo: "acme/one"})
	m = openFavs(t, m)
	late := m.fetch() // asked on the Workflows tab
	m = pressTab(m, "tab")
	next, _ := m.Update(late())
	m = next.(model)
	if m.favTab != favTabRepos || len(m.tbl.rows) != 1 || m.tbl.rows[0].cells[0] != "acme/one" {
		t.Errorf("late workflows answer replaced the repos table: %+v", m.tbl)
	}
}
