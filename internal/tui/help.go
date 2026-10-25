package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
)

// ? opens a list of every key of the current screen. The footer line has room for only the most used
// ones; this is where all of them are written down.

type helpKey struct{ keys, what string }

type helpGroup struct {
	title string
	rows  []helpKey
}

var helpScroll = []helpKey{
	{"j k ↓ ↑", "one line"}, {"ctrl+d ctrl+u", "half a page"}, {"space pgdn pgup", "a page"}, {"g G home end", "top / bottom"},
}

// helpGroups are the key groups of the screen m is on, the screen's own first and the ones every screen
// has last.
func (m model) helpGroups() []helpGroup {
	var gs []helpGroup
	switch m.view {
	case viewRuns:
		gs = append(gs, helpGroup{"Runs", []helpKey{{"j k", "move"}, {"enter", "job graph of the run"}, {"l", "full log in the pager"}, {"L", "log of the failed jobs"},
			{"f", "pin / unpin this repo"}, {"w", "watch the run (gh run watch)"}, {"r", "re-run (asks first)"}, {"F", "re-run the failed jobs"}, {"c", "cancel"}, {"d", "delete"}, {"o", "open in the browser"}}})
	case viewFavs:
		gs = append(gs, helpGroup{"Favorites tabs", []helpKey{{"tab l", "next tab (Workflows, Repos, Projects)"}, {"shift+tab h", "previous tab"}}})
		switch m.favTab {
		case favTabRepos:
			gs = append(gs, helpGroup{"Repos", []helpKey{{"j k", "move"}, {"enter", "switch to the repo (like :repo)"}, {"a", "pin a repo"}, {"f d", "unpin (asks first)"}, {"o", "open in the browser"}}})
		case favTabProjects:
			gs = append(gs, helpGroup{"Projects", []helpKey{{"j k", "move"}, {"enter", "open the board"}, {"a", "pin a Project (pick from the list, or type OWNER/NUMBER)"}, {"f d", "unpin (asks first)"}, {"o", "open in the browser"}}})
		default:
			gs = append(gs, helpGroup{"Workflows", []helpKey{{"j k", "move"}, {"enter", "its runs"}, {"t", "trigger (form)"}, {"f", "unfavorite"}, {"a", "set an alias"}, {"o", "open in the browser"}}})
		}
	case viewWorkflows:
		gs = append(gs, helpGroup{"Workflows / favorites", []helpKey{{"j k", "move"}, {"enter", "its runs"}, {"t", "trigger (form)"}, {"f", "favorite / unfavorite"},
			{"a", "set an alias"}, {"e", "enable / disable (workflows)"}, {"o", "open in the browser"}}})
	case viewPRs:
		gs = append(gs, helpGroup{"Pull requests", []helpKey{{"j k", "move"}, {"enter", "open the PR"}, {"u", "only mine"}, {"e", "review requested from me"}, {"a", "all"}, {"f", "pin / unpin this repo"}, {"o", "open in the browser"}}})
	case viewIssues:
		gs = append(gs, helpGroup{"Issues", []helpKey{{"j k", "move"}, {"enter", "open the issue"}, {"O C A", "open / closed / all"}, {"u", "assigned to me"}, {"m", "opened by me"}, {"f", "pin / unpin this repo"}, {"o", "open in the browser"}}})
	case viewBoard:
		gs = append(gs, helpGroup{"Project board (read-only)", []helpKey{{"tab l →", "next column (Status)"}, {"shift+tab h ←", "previous column"}, {"j k", "move"}, {"enter", "open the issue or PR (in its own repo)"}, {"s", "change the card's Status (asks first)"}, {"H L", "in columns: move the card to the previous / next column"}, {"a", "change assignees of an issue or PR card"}, {"n", "add an issue or PR to the Project"}, {"x X", "remove the card from the Project / archive it"}, {"space", "mark the card; s x X a and H L then act on all marked cards (esc clears the marks)"}, {"ctrl+up ctrl+down", "move the card one place up / down (not in a sorted view)"}, {"u", "undo the last Status or field change, added card or new draft"}, {"N", "new draft card (gets the Status of the column you are in)"}, {"e", "edit a draft card's title and text"}, {"v", "change another field of the card (single select, iteration, text, number, date)"}, {"b", "columns (all Status columns side by side) / list"}, {"h j k l", "in columns: change column / card"}, {"] [", "next / previous saved view of the Project"}, {"f", "pin / unpin this Project"}, {"o", "open in the browser (a draft opens the Project)"}, {"R", "refresh (at most once in a while)"}}})
	case viewInbox:
		gs = append(gs, helpGroup{"Inbox", []helpKey{{"tab l →", "next tab"}, {"shift+tab h ←", "previous tab"}, {"j k", "move"}, {"enter", "open the item (in its own repo)"}, {"o", "open in the browser"}, {"R", "refresh (at most once in a while)"}}})
	case viewPRDetail:
		gs = append(gs, helpGroup{"Pull request", []helpKey{{"c", "checks"}, {"d", "diff / code review"}, {"C", "comment"}, {"r", "review: approve / request changes / comment"}, {"m", "merge"}, {"tab", "select an image"}, {"enter", "show the selected image"}, {"o", "open (the image, else the PR)"}}})
		gs = append(gs, helpGroup{"Scroll", helpScroll})
	case viewIssue:
		gs = append(gs, helpGroup{"Issue", []helpKey{{"C", "comment"}, {"x", "close"}, {"X", "reopen"}, {"l", "labels"}, {"s", "assignees"}, {"tab", "select an image"}, {"enter", "show the selected image"}, {"o", "open"}}})
		gs = append(gs, helpGroup{"Scroll", helpScroll})
	case viewDiff:
		gs = append(gs,
			helpGroup{"Moving", append(append([]helpKey{}, helpScroll...), helpKey{"] [", "next / previous file"}, helpKey{"} {", "next / previous hunk"}, helpKey{"/ then n N", "search, next / previous match"})},
			helpGroup{"Comments", []helpKey{{"c", "comment on the line (new side)"}, {"C", "comment on the old side"}, {"V", "start a range, move, then c"}, {"x", "remove the draft on this line"}, {"X", "discard all drafts"},
				{"A", "adopt the drafts for the new head"}, {"r", "send the review (asks first)"}, {"esc", "cancel a range"}},
			},
			helpGroup{"Files", []helpKey{{"tab f", "focus the file tree (side by side) / the file list (single column)"}, {"j k g G", "move in the tree"}, {"enter space", "fold / unfold a directory; on a file jump to it"},
				{"h l", "fold / unfold (h on a file selects its directory)"}, {"v", "mark viewed (a file, or everything under a directory)"}, {"esc", "back to the diff"}, {"s", "side by side / single column"}, {"o", "open in the browser"}}},
		)
	case viewChecks:
		gs = append(gs, helpGroup{"Checks", []helpKey{{"j k", "move"}, {"enter o", "open the check"}}})
	case viewJobs:
		gs = append(gs, helpGroup{"Jobs", []helpKey{{"j k", "move"}, {"enter", "log of the job"}, {"r F c d", "re-run / re-run failed / cancel / delete the run"}, {"o", "open"}}})
	case viewGraph:
		gs = append(gs, helpGroup{"Job graph", []helpKey{{"h j k l ←↑↓→", "move"}, {"enter", "log of the job, or expand a matrix"}, {"x", "collapse"}, {"v", "graph style"}, {"f", "log of the first failed job at its first error"},
			{"r F c d", "re-run / re-run failed / cancel / delete"}, {"o", "open"}, {"ctrl+d ctrl+u", "scroll"}}})
	case viewLogs:
		gs = append(gs, helpGroup{"Log", append(append([]helpKey{}, helpScroll...), helpKey{"/ then n N", "search, next / previous match"}, helpKey{"f", "follow a running job"}, helpKey{"o", "open"})})
	case viewSettings:
		gs = append(gs, helpGroup{"Settings", []helpKey{{"j k", "move"}, {"enter →", "next value"}, {"←", "previous value"}, {":set key value", "set one by name"}}})
	}
	if m.previewShown() {
		gs = append(gs, helpGroup{"Preview pane", []helpKey{{"J K", "scroll the pane"}, {"p", "hide / show it for this run"}}})
	}
	gs = append(gs, helpGroup{"Everywhere", []helpKey{{"1 2 3 4 0", "runs / workflows / pulls / issues / inbox"}, {":", "command bar (runs, pulls, issues, inbox, repo, set, settings, fav, cpr, icons, quit)"},
		{"/", "filter the list"}, {"P", "the PR of the current branch"}, {"R", "refresh"}, {"esc", "back"}, {"?", "this help"}, {"q ctrl+c", "quit"}}})
	return gs
}

// helpLines lays the groups out for a screen of the given width.
func (m model) helpLines(width int) []string {
	var out []string
	for i, g := range m.helpGroups() {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, styCols.Render(g.title))
		for _, r := range g.rows {
			const keyW = 16
			k := ansi.Truncate(r.keys, keyW, "…")
			pad := strings.Repeat(" ", max(keyW-ansi.StringWidth(k), 0))
			lines := wrap(r.what, max(width-keyW-3, 10))
			for j, l := range lines {
				if j == 0 {
					out = append(out, "  "+styKey.Render(k)+pad+" "+l)
				} else {
					out = append(out, strings.Repeat(" ", keyW+3)+l)
				}
			}
		}
	}
	return out
}

func (m model) helpHeight() int { return max(m.height-6, 3) }

// helpView is the overlay: the help lines in a box, scrolled.
func (m model) helpView() string {
	lines := m.helpLines(m.boxWidth() - 4)
	h := m.helpHeight()
	top := min(m.helpTop, max(len(lines)-h, 0))
	var b strings.Builder
	for i := top; i < len(lines) && i < top+h; i++ {
		b.WriteString(lines[i] + "\n")
	}
	for n := len(lines) - top; n < h; n++ {
		b.WriteString("\n")
	}
	b.WriteString(styDim.Render("j/k scroll  esc or ? close"))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Padding(0, 1).Width(m.boxWidth()).Render(b.String()) + "\n"
}

// helpKeyPress handles keys while the help is open.
func (m model) helpKeyPress(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxTop := max(len(m.helpLines(m.boxWidth()-4))-m.helpHeight(), 0)
	switch k.String() {
	case "esc", "?", "q":
		m.helpOpen, m.helpTop = false, 0
	case "j", "down":
		m.helpTop = min(m.helpTop+1, maxTop)
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	case "ctrl+d", "pgdown", " ":
		m.helpTop = min(m.helpTop+m.helpHeight()/2, maxTop)
	case "ctrl+u", "pgup":
		m.helpTop = max(m.helpTop-m.helpHeight()/2, 0)
	case "g", "home":
		m.helpTop = 0
	case "G", "end":
		m.helpTop = maxTop
	}
	return m, nil
}
