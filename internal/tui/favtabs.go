package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/pins"
)

// The favorites screen has three tabs; the tab chosen last is kept for the session.
const (
	favTabWorkflows = iota
	favTabRepos
	favTabProjects
)

var favTabNames = []string{"Workflows", "Repos", "Projects"}

func init() {
	dialogHandlers["pin.repo"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		if !r.ok {
			return m, nil
		}
		repo, err := pins.ParseRepo(strings.TrimSpace(r.text))
		if err != nil {
			m.status = styErr.Render("✗ " + ghcli.CleanText(err.Error()))
			return m, nil
		}
		return m.pinRepoChecked(repo)
	}
	dialogHandlers["pin.unrepo"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		repo := m.pinDel
		m.pinDel = nil
		if !r.ok || repo == nil {
			return m, nil
		}
		return m.togglePinnedRepo(*repo)
	}
}

// pinStore is the pins; a model without one (tests, a broken file) gets a store that lives for the session.
func (m model) pinStore() *pins.Store {
	if m.pins == nil {
		return pins.Memory()
	}
	return m.pins
}

// savePins writes the pins and words the outcome; a store that cannot be saved still works for the session.
func (m model) savePins(done string) model {
	if err := m.pinStore().Save(); err != nil {
		m.status = styErr.Render(done + " for this session, but not saved: " + ghcli.CleanText(err.Error()))
	} else {
		m.status = styOK.Render(done)
	}
	return m
}

func repoLabel(r pins.Repo) string {
	if r.Host != "" {
		return r.Host + "/" + r.Repo
	}
	return r.Repo
}

func repoURL(r pins.Repo) string {
	host := r.Host
	if host == "" {
		host = "github.com"
	}
	return "https://" + host + "/" + r.Repo
}

// favExtraRows is the tab bar above the favorites table.
func (m model) favExtraRows() int {
	if m.view == viewFavs {
		return 1
	}
	return 0
}

func (m model) favTabBar() string {
	counts := []int{0, 0, 0}
	if m.favs != nil {
		counts[favTabWorkflows] = len(m.favs.Items)
	}
	st := m.pinStore()
	counts[favTabRepos], counts[favTabProjects] = len(st.Repos), len(st.Projects)
	var parts []string
	for i, n := range favTabNames {
		label := fmt.Sprintf(" %s (%d) ", n, counts[i])
		if i == m.favTab {
			parts = append(parts, stySel.Render(label))
		} else {
			parts = append(parts, styDim.Render(label))
		}
	}
	return m.tabs("favs", parts, m.favTab, styDim.Render("|"))
}

// fitTabs joins tab labels on one line no wider than width, dropping tabs from the left (shown as ‹)
// until the selected one fits, and cutting what is still too long on the right.
func fitTabs(labels []string, sel, width int, sep ...string) string {
	s := " "
	if len(sep) > 0 {
		s = sep[0]
	}
	if width <= 0 || len(labels) == 0 {
		return strings.Join(labels, s)
	}
	start := tabStart(labels, sel, width, s)
	line := strings.Join(labels[start:], s)
	if start > 0 {
		line = "‹ " + line
	}
	return ansi.Truncate(line, width, "…")
}

func (m model) favFooter() string {
	switch m.favTab {
	case favTabRepos:
		return "enter switch to the repo  a pin  f/d unpin  o open  tab next tab"
	case favTabProjects:
		return "enter open the board  a pin  f/d unpin  o open  tab next tab"
	}
	return "enter runs  f unfavorite  a alias  tab next tab"
}

// reposTable lists the pinned repos, no network needed.
func reposTable(st *pins.Store) table {
	t := table{cols: []string{"REPO"}}
	for _, r := range st.Repos {
		t.rows = append(t.rows, row{id: repoLabel(r), extra: repoLabel(r), url: repoURL(r), cells: []string{repoLabel(r)}})
	}
	return t
}

// favTabKey handles the keys the tabbed favorites screen adds; the Workflows tab keeps its old keys.
func (m model) favTabKey(k tea.KeyMsg) (model, tea.Cmd, bool) {
	switch k.String() {
	case "tab", "l":
		m = m.setFavTab((m.favTab + 1) % len(favTabNames))
		return m, m.fetch(), true
	case "shift+tab", "h":
		m = m.setFavTab((m.favTab + len(favTabNames) - 1) % len(favTabNames))
		return m, m.fetch(), true
	}
	switch m.favTab {
	case favTabRepos:
		return m.repoTabKey(k)
	case favTabProjects:
		return m.projectTabKey(k)
	}
	return m, nil, false
}

func (m model) setFavTab(t int) model {
	m.favTab, m.cursor, m.top, m.filter = t, 0, 0, ""
	m.tbl, m.err, m.loading = table{}, nil, true
	return m
}

// selectedRepo is the pinned repo under the cursor.
func (m model) selectedRepo() (pins.Repo, bool) {
	r, ok := m.selected()
	if !ok {
		return pins.Repo{}, false
	}
	for _, p := range m.pinStore().Repos {
		if repoLabel(p) == r.id {
			return p, true
		}
	}
	return pins.Repo{}, false
}

func (m model) repoTabKey(k tea.KeyMsg) (model, tea.Cmd, bool) {
	switch k.String() {
	case "enter":
		p, ok := m.selectedRepo()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.repoCommand([]string{p.Target().String()})
		return next.(model), cmd, true
	case "a":
		m.dlg = &dialog{id: "pin.repo", title: "Pin a repo", prompt: "[HOST/]OWNER/NAME", kind: dialogText}
		return m, nil, true
	case "f", "d":
		p, ok := m.selectedRepo()
		if !ok {
			return m, nil, true
		}
		m.pinDel = &p
		d := newConfirmDialog("pin.unrepo", "Unpin "+repoLabel(p)+"?", "It stays on GitHub; only the pin goes.")
		m.dlg = &d
		return m, nil, true
	case "t", "e":
		m.status = styDim.Render("pinned repos: enter switches to the repo")
		return m, nil, true
	}
	return m, nil, false
}

// togglePinnedRepo flips the pin, saves, and refreshes the tab.
func (m model) togglePinnedRepo(r pins.Repo) (model, tea.Cmd) {
	if m.pins == nil {
		m.pins = pins.Memory()
	}
	on, err := m.pins.ToggleRepo(r)
	if err != nil {
		m.status = styErr.Render("✗ " + ghcli.CleanText(err.Error()))
		return m, nil
	}
	done := "removed pin " + repoLabel(r)
	if on {
		done = "★ pinned " + repoLabel(r)
	}
	m = m.savePins(done)
	if m.view == viewFavs {
		return m, m.fetch()
	}
	return m, nil
}

// repoPinMsg is the outcome of checking that a repo to pin exists.
type repoPinMsg struct {
	repo pins.Repo
	err  error
}

// pinRepoChecked pins repo once GitHub confirms it exists, so a typo does not end up in the list.
func (m model) pinRepoChecked(repo pins.Repo) (model, tea.Cmd) {
	if m.pinStore().HasRepo(repo.Host, repo.Repo) {
		m.status = styDim.Render(repoLabel(repo) + " is already pinned")
		return m, nil
	}
	m.status = styDim.Render("checking " + repoLabel(repo) + "…")
	ref := repo.Target()
	return m, func() tea.Msg { return repoPinMsg{repo, checkRepo(ref)} }
}

func (m model) onRepoPin(msg repoPinMsg) (model, tea.Cmd) {
	if msg.err != nil {
		m.status = styErr.Render("✗ " + repoLabel(msg.repo) + ": " + ghcli.CleanText(msg.err.Error()))
		return m, nil
	}
	if m.pinStore().HasRepo(msg.repo.Host, msg.repo.Repo) { // pinned meanwhile: an answer must not unpin it
		m.status = styDim.Render(repoLabel(msg.repo) + " is already pinned")
		return m, nil
	}
	return m.togglePinnedRepo(msg.repo)
}

// favCommand is ":fav" (the screen), ":fav repo" (pin the current repo) and ":fav repo OWNER/NAME".
func (m model) favCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m.goTop(viewFavs)
	}
	if strings.ToLower(args[0]) != "repo" || len(args) > 2 {
		m.status = styErr.Render("usage: :fav   :fav repo [OWNER/NAME]")
		return m, nil
	}
	var repo pins.Repo
	if len(args) == 2 {
		r, err := pins.ParseRepo(args[1])
		if err != nil {
			m.status = styErr.Render("✗ " + ghcli.CleanText(err.Error()))
			return m, nil
		}
		repo = r
	} else {
		t := m.curTarget()
		if t == (ghcli.RepoRef{}) {
			m.status = styErr.Render("cannot tell which repo this is; run gh-9s inside a repo or give OWNER/NAME")
			return m, nil
		}
		repo = pins.Repo{Host: t.Host, Repo: t.Owner + "/" + t.Name}
		if repo.Host == "github.com" {
			repo.Host = ""
		}
	}
	if m.pinStore().HasRepo(repo.Host, repo.Repo) {
		m.status = styDim.Render(repoLabel(repo) + " is already pinned")
		return m, nil
	}
	return m.pinRepoChecked(repo)
}

// toggleCurrentRepoPin is f on the lists of one repo: pin the repo being shown, or unpin it.
func (m model) toggleCurrentRepoPin() (tea.Model, tea.Cmd) {
	t := m.curTarget()
	if t == (ghcli.RepoRef{}) {
		m.status = styErr.Render("cannot tell which repo this is; run gh-9s inside a repo or use :repo")
		return m, nil
	}
	repo := pins.Repo{Host: t.Host, Repo: t.Owner + "/" + t.Name}
	if repo.Host == "github.com" {
		repo.Host = ""
	}
	if m.pinStore().HasRepo(repo.Host, repo.Repo) {
		return m.togglePinnedRepo(repo)
	}
	return m.pinRepoChecked(repo)
}

// tabStart is the first label drawn: tabs drop off the left until the selected one fits.
func tabStart(labels []string, sel, width int, sep string) int {
	start := 0
	for start < sel && ansi.StringWidth(strings.Join(labels[start:sel+1], sep))+2 > width {
		start++
	}
	return start
}
