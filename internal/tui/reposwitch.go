package tui

import (
	"regexp"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// safeRepoPart is what a host, owner or repo name may look like; it cannot start with "-".
var safeRepoPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// checkRepo verifies a repo before switching to it; tests replace it.
var checkRepo = ghcli.CheckRepo

// repoSwitchedMsg is the outcome of verifying a repo typed after :repo.
type repoSwitchedMsg struct {
	ref ghcli.RepoRef
	err error
}

// repoCommand handles ":repo [HOST/]OWNER/REPO"; ":repo" alone opens the picker.
func (m model) repoCommand(args []string) (tea.Model, tea.Cmd) {
	switch len(args) {
	case 0:
		return m, m.openPicker()
	case 1:
		ref, err := ghcli.ParseRepoRef(args[0])
		if err != nil || !validRepoName(ref) {
			m.status = ui.Err.Render("✗ repo: want OWNER/REPO or HOST/OWNER/REPO, got " + args[0])
			return m, nil
		}
		m.status = ui.Dim.Render("checking " + ref.String() + "…")
		return m, func() tea.Msg { return repoSwitchedMsg{ref, checkRepo(ref)} }
	}
	m.status = ui.Err.Render("✗ repo: want one argument, OWNER/REPO")
	return m, nil
}

// validRepoName keeps option-like or odd names out of the gh command line.
func validRepoName(r ghcli.RepoRef) bool {
	for _, p := range []string{r.Host, r.Owner, r.Name} {
		if p == "" {
			continue
		}
		if !safeRepoPart.MatchString(p) {
			return false
		}
	}
	return r.Owner != "" && r.Name != ""
}

// switchRepo makes ref (zero = the current directory's repo) the repo every view shows.
// Anything deeper than a top-level view belongs to the old repo, so it starts over.
func (m *model) switchRepo(ref ghcli.RepoRef) tea.Cmd {
	m.base, m.target = ref, ref
	m.back, m.wfID, m.wfName, m.pr = nil, "", "", ""
	m.discCat, m.discCatName, m.disc = "", "", "" // a category id belongs to its repo
	m.runID, m.graph, m.gsel, m.expanded, m.runStatus, m.runMeta = "", graph.Graph{}, 0, nil, "", ghcli.RunMeta{}
	v := m.view
	if v == viewFavs && m.favTab != favTabWorkflows {
		v = viewRuns // pinned repos and Projects are not about the repo just chosen
	}
	switch v {
	case viewRuns, viewWorkflows, viewPRs, viewFavs, viewIssues, viewDiscussions:
	default:
		v = viewRuns
	}
	if ref == (ghcli.RepoRef{}) {
		m.status = ui.OK.Render("✓ repo: the current directory")
	} else {
		m.status = ui.OK.Render("✓ repo: " + ref.String())
	}
	return m.switchTo(v)
}

// currentPR opens the checks of the PR for the local branch. That branch only exists in the
// current directory's repo, so it is refused after :repo.
func (m model) currentPR() (tea.Model, tea.Cmd) {
	if m.base != (ghcli.RepoRef{}) {
		m.status = ui.Dim.Render("the current-branch PR belongs to the current directory's repo; use :repo to go back")
		return m, nil
	}
	return m, func() tea.Msg {
		pr, err := currentPR()
		return gotoMsg{viewChecks, pr, err}
	}
}

// prBranch is the branch the PR list should mark with *: the local one, in the local repo only.
func (m model) prBranch() string {
	if m.target != (ghcli.RepoRef{}) {
		return ""
	}
	return m.branch
}
