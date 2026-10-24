package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls the issue screens make; tests replace them.
var (
	fetchIssueList = fetchIssues
	fetchIssue     = ghcli.FetchIssue
	commentIssue   = ghcli.CommentIssue
)

// issueState is what the issue screen keeps in the model.
type issueState struct {
	d      ghcli.IssueDetail
	scroll int
}

func init() {
	register(viewIssues, screen{
		name: "issues",
		fetch: func(m model) tea.Cmd {
			tg, f := m.target, m.issueFilter
			return func() tea.Msg {
				t, err := fetchIssueList(tg, f)
				return screenMsg{view: viewIssues, data: t, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			m.tbl = msg.data.(table)
			if n := len(m.visible()); m.cursor >= n {
				m.cursor = max(0, n-1)
			}
			return m
		},
		key:    issuesKey,
		body:   func(m model) string { return m.tableBody() },
		footer: "enter open  O/C/A open/closed/all  u assigned to me  m opened by me  o browser",
		poll:   false,
	})
	register(viewIssue, screen{
		name: "issue",
		fetch: func(m model) tea.Cmd {
			tg, number := m.target, m.issue
			return func() tea.Msg {
				d, err := fetchIssue(tg, number)
				return screenMsg{view: viewIssue, data: d, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			old, _ := m.screenData(viewIssue).(issueState)
			return m.withScreenData(viewIssue, issueState{d: msg.data.(ghcli.IssueDetail), scroll: old.scroll})
		},
		key:    issueKey,
		body:   issueBody,
		footer: "C comment  o open  j/k scroll  g/G top/bottom",
	})
	dialogHandlers["issue.comment"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		m.pending = nil
		if !r.ok || p == nil {
			return m, nil
		}
		return m, func() tea.Msg {
			return prActionDoneMsg{text: "commented on " + p.label, err: commentIssue(p.target, p.number, r.text)}
		}
	}
}

// issueFilterName words the active issue filter for the title.
func issueFilterName(f ghcli.IssueFilter) string {
	name := "open"
	switch f.State {
	case "closed", "all":
		name = f.State
	}
	if f.AssignedToMe {
		name += " + assigned to me"
	}
	if f.OpenedByMe {
		name += " + opened by me"
	}
	return name
}

// issuesKey sets the list filter (O/C/A = open/closed/all; u, m toggle assigned to me / opened by me)
// and opens an issue. Everything else (movement, /, o, esc) is the shared table behaviour.
func issuesKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	switch k.String() {
	case "O":
		m.issueFilter.State = "open"
	case "C":
		m.issueFilter.State = "closed"
	case "A":
		m.issueFilter.State = "all"
	case "u":
		m.issueFilter.AssignedToMe = !m.issueFilter.AssignedToMe
	case "m":
		m.issueFilter.OpenedByMe = !m.issueFilter.OpenedByMe
	case "enter":
		r, ok := m.selected()
		if !ok {
			return m, nil, true
		}
		m.issue = r.id
		m = m.withScreenData(viewIssue, issueState{}) // a different issue starts at the top
		cmd := m.push(viewIssue)
		return m, cmd, true
	default:
		return m, nil, false
	}
	m.cursor, m.top, m.loading = 0, 0, true
	return m, m.fetch(), true
}

func issueKey(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewIssue).(issueState)
	h := m.logHeight()
	maxScroll := max(0, len(issueLines(st.d, m.contentWidth()))-h)
	switch k.String() {
	case "j", "down":
		st.scroll = min(st.scroll+1, maxScroll)
	case "k", "up":
		st.scroll = max(st.scroll-1, 0)
	case "ctrl+d", "pgdown", " ":
		st.scroll = min(st.scroll+h/2, maxScroll)
	case "ctrl+u", "pgup":
		st.scroll = max(st.scroll-h/2, 0)
	case "g", "home":
		st.scroll = 0
	case "G", "end":
		st.scroll = maxScroll
	case "C":
		label := displayRef(m.curTarget()) + "#" + m.issue
		m.pending = &prAction{target: m.target, number: m.issue, label: label}
		return m.withDialog(newTextDialog("issue.comment", "Comment on "+label, "Your comment")), nil, true
	case "o":
		if st.d.URL != "" {
			_ = openURL(st.d.URL)
		}
		return m, nil, true
	default:
		return m, nil, false
	}
	return m.withScreenData(viewIssue, st), nil, true
}

func issueBody(m model) string {
	st, _ := m.screenData(viewIssue).(issueState)
	h := m.logHeight()
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.Err.Render(" "+m.err.Error()) + "\n")
		h--
	}
	lines := issueLines(st.d, m.contentWidth())
	n := 0
	for i := min(st.scroll, max(0, len(lines)-1)); i < len(lines) && n < h; i++ {
		b.WriteString(" " + lines[i] + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

// issueLines lays out the issue screen as plain lines at the given width.
func issueLines(d ghcli.IssueDetail, width int) []string {
	if d.Number == 0 && d.Title == "" {
		return []string{"loading…"}
	}
	out := []string{fmt.Sprintf("#%d  %s", d.Number, d.Title), fmt.Sprintf("%s   %s", d.Author, d.State)}
	labels, assignees := "no labels", "unassigned"
	if len(d.Labels) > 0 {
		labels = "Labels: " + strings.Join(d.Labels, ", ")
	}
	if len(d.Assignees) > 0 {
		assignees = "Assignees: " + strings.Join(d.Assignees, ", ")
	}
	out = append(out, labels+"   "+assignees, "", "── Description ──")
	if strings.TrimSpace(d.Body) == "" {
		out = append(out, "(no description)")
	} else {
		out = append(out, wrap(d.Body, width)...)
	}
	out = append(out, "", "── Discussion ──")
	return append(out, discussionLines(d.Comments, nil, width)...)
}
