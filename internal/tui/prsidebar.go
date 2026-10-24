package tui

import (
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// prSidebar lays out what the right-hand side of a PR page on GitHub shows: reviewers with where they
// stand, assignees, labels, projects, milestone and the issues the PR closes. Every section is always
// there, saying "none" when it is empty, as GitHub does.
func prSidebar(p ghcli.PRDetail, width int) []string {
	const lead = 11 // the section name column
	var out []string
	section := func(name string, values []string) {
		if len(values) == 0 {
			values = []string{ui.Dim.Render("none")}
		}
		for i, v := range values {
			head := strings.Repeat(" ", lead)
			if i == 0 {
				head = ui.Dim.Render(name + strings.Repeat(" ", lead-len(name)))
			}
			for j, l := range wrap(v, max(width-lead, 10)) {
				if j > 0 {
					head = strings.Repeat(" ", lead)
				}
				out = append(out, head+l)
			}
		}
	}
	var reviewers []string
	for _, r := range p.Reviewers {
		reviewers = append(reviewers, reviewerLine(r))
	}
	section("Reviewers", reviewers)
	section("Assignees", p.Assignees)
	var labels []string
	if len(p.Labels) > 0 {
		labels = []string{withIcon(icons.Label, strings.Join(p.Labels, ", "))}
	}
	section("Labels", labels)
	section("Projects", p.Projects)
	var ms []string
	if p.Milestone != "" {
		ms = []string{p.Milestone}
	}
	section("Milestone", ms)
	section("Closes", p.LinkedIssues)
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
