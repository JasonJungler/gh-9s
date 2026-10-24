package tui

import (
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// sidebarSection is one block of the sidebar: its name and its values (empty means "none").
type sidebarSection struct {
	name   string
	values []string
}

// prSections are the blocks of the right-hand side of a PR page on GitHub: reviewers with where they
// stand, assignees, labels, projects, milestone and the issues the PR closes. All of them are always
// there, saying "none" when empty, as GitHub does.
func prSections(p ghcli.PRDetail) []sidebarSection {
	var reviewers, labels, ms []string
	for _, r := range p.Reviewers {
		reviewers = append(reviewers, reviewerLine(r))
	}
	if len(p.Labels) > 0 {
		labels = []string{withIcon(icons.Label, strings.Join(p.Labels, ", "))}
	}
	if p.Milestone != "" {
		ms = []string{p.Milestone}
	}
	return []sidebarSection{{"Reviewers", reviewers}, {"Assignees", p.Assignees}, {"Labels", labels},
		{"Projects", p.Projects}, {"Milestone", ms}, {"Closes", p.LinkedIssues}}
}

// prSidebar lays the sections out one per line group, the name on the left and the values beside it.
func prSidebar(p ghcli.PRDetail, width int) []string {
	const lead = 11 // the section name column
	var out []string
	for _, sec := range prSections(p) {
		values := sec.values
		if len(values) == 0 {
			values = []string{ui.Dim.Render("none")}
		}
		for i, v := range values {
			head := strings.Repeat(" ", lead)
			if i == 0 {
				head = ui.Dim.Render(sec.name + strings.Repeat(" ", lead-len(sec.name)))
			}
			for j, l := range wrap(v, max(width-lead, 10)) {
				if j > 0 {
					head = strings.Repeat(" ", lead)
				}
				out = append(out, head+l)
			}
		}
	}
	return out
}

// prSidebarColumn lays the sections out for a narrow column on the right: each name on its own line,
// its values below, indented.
func prSidebarColumn(p ghcli.PRDetail, width int) []string {
	return sidebarColumn(prSections(p), width)
}

func sidebarColumn(sections []sidebarSection, width int) []string {
	var out []string
	for i, sec := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, ui.Key.Render(sec.name))
		if len(sec.values) == 0 {
			out = append(out, " "+ui.Dim.Render("none"))
		}
		for _, v := range sec.values {
			for _, l := range wrap(v, max(width-1, 5)) {
				out = append(out, " "+l)
			}
		}
	}
	return out
}

// reviewerLine is "<icon> name  status".
func reviewerLine(r ghcli.Reviewer) string {
	icon, word := icons.ReviewRequired, "requested"
	switch r.State {
	case "APPROVED":
		icon, word = icons.ReviewApproved, "approved"
	case "CHANGES_REQUESTED":
		icon, word = icons.ReviewChanges, "changes requested"
	case "COMMENTED":
		icon, word = icons.Comment, "commented"
	case "DISMISSED":
		word = "dismissed"
	}
	return withIcon(icon, r.Name+" "+word)
}
