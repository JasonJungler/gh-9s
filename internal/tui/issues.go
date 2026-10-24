package tui

import (
	"fmt"
	"sort"
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
	listLabels     = ghcli.ListLabels
	listAssignees  = ghcli.ListAssignees
	editIssue      = ghcli.EditIssue
	closeIssue     = ghcli.CloseIssue
	reopenIssue    = ghcli.ReopenIssue
)

// issueState is what the issue screen keeps in the model.
type issueState struct {
	d      ghcli.IssueDetail
	scroll int
}

func init() {
	register(viewIssues, screen{
		name:     "issues",
		identity: func(m model) string { return m.target.String() + "|" + issueFilterName(m.issueFilter) },
		fetch: func(m model) tea.Cmd {
			tg, f, id := m.target, m.issueFilter, m.target.String()+"|"+issueFilterName(m.issueFilter)
			return func() tea.Msg {
				t, err := fetchIssueList(tg, f)
				return screenMsg{view: viewIssues, id: id, data: t, err: err}
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
		name:     "issue",
		identity: func(m model) string { return m.itemID(m.issue) },
		fetch: func(m model) tea.Cmd {
			tg, number, id := m.target, m.issue, m.itemID(m.issue)
			return func() tea.Msg {
				d, err := fetchIssue(tg, number)
				return screenMsg{view: viewIssue, id: id, data: d, err: err}
			}
		},
		apply: func(m model, msg screenMsg) model {
			old, _ := m.screenData(viewIssue).(issueState)
			return m.withScreenData(viewIssue, issueState{d: msg.data.(ghcli.IssueDetail), scroll: old.scroll})
		},
		key:    issueKey,
		body:   issueBody,
		footer: "C comment  x close  X reopen  l labels  s assignees  o open  j/k scroll",
	})
	dialogHandlers["issue.close"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		m.pending = nil
		if !r.ok || p == nil {
			return m, nil
		}
		return m, func() tea.Msg {
			return prActionDoneMsg{text: "closed " + p.label, err: closeIssue(p.target, p.number)}
		}
	}
	dialogHandlers["issue.labels"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		m.pending = nil
		if !r.ok || p == nil {
			return m, nil
		}
		add, remove := diffNames(p.current, r.values)
		if len(add)+len(remove) == 0 {
			m.status = styDim.Render("no label changes")
			return m, nil
		}
		return m, func() tea.Msg {
			return prActionDoneMsg{text: "updated labels on " + p.label, err: editIssue(p.target, p.number, add, remove, nil, nil)}
		}
	}
	dialogHandlers["issue.assignees"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		p := m.pending
		m.pending = nil
		if !r.ok || p == nil {
			return m, nil
		}
		add, remove := diffNames(p.current, r.values)
		if len(add)+len(remove) == 0 {
			m.status = styDim.Render("no assignee changes")
			return m, nil
		}
		return m, func() tea.Msg {
			return prActionDoneMsg{text: "updated assignees on " + p.label, err: editIssue(p.target, p.number, nil, nil, add, remove)}
		}
	}
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
		m, p := m.startIssueAction(st)
		return m.withDialog(newTextDialog("issue.comment", "Comment on "+p.label, "Your comment")), nil, true
	case "x":
		if st.d.State != "OPEN" {
			m.status = styDim.Render(stateWord(st.d, "closed") + " — nothing to close")
			return m, nil, true
		}
		m, p := m.startIssueAction(st)
		return m.withDialog(newConfirmDialog("issue.close", "Close "+p.label+"?", st.d.Title)), nil, true
	case "X":
		if st.d.State == "OPEN" {
			m.status = styDim.Render("already open — nothing to reopen")
			return m, nil, true
		}
		if st.d.State == "" {
			m.status = styDim.Render("still loading the issue")
			return m, nil, true
		}
		m, p := m.startIssueAction(st)
		m.pending = nil
		return m, func() tea.Msg {
			return prActionDoneMsg{text: "reopened " + p.label, err: reopenIssue(p.target, p.number)}
		}, true
	case "l", "s":
		m, p := m.startIssueAction(st)
		kind := "labels"
		p.current = st.d.Labels
		if k.String() == "s" {
			kind, p.current = "assignees", st.d.Assignees
		}
		m.status = styDim.Render("loading " + kind + "…")
		tg, id := p.target, m.itemID(m.issue)
		p.want = kind
		return m, func() tea.Msg {
			var names []string
			var err error
			if kind == "labels" {
				names, err = listLabels(tg)
			} else {
				names, err = listAssignees(tg)
			}
			return choicesMsg{id: id, kind: kind, names: names, err: err}
		}, true
	case "o":
		if st.d.URL != "" {
			m = m.open(st.d.URL)
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

// startIssueAction remembers which issue an action was started for.
func (m model) startIssueAction(st issueState) (model, *prAction) {
	p := &prAction{target: m.target, number: m.issue, label: displayRef(m.curTarget()) + "#" + m.issue}
	m.pending = p
	return m, p
}

func stateWord(d ghcli.IssueDetail, word string) string {
	if d.State == "" {
		return "still loading the issue"
	}
	return "already " + word
}

// choicesMsg carries the repo's labels or assignable users for a multi-choice dialog.
type choicesMsg struct {
	id    string // the issue the lists were fetched for
	kind  string // "labels" or "assignees"
	names []string
	err   error
}

// choicesLoaded opens the multi-choice dialog. The issue's current values are always on the
// list, even if the repo no longer offers them, so they can still be removed.
func (m model) choicesLoaded(msg choicesMsg) model {
	p := m.pending
	if p == nil || m.view != viewIssue || msg.id != m.itemID(m.issue) || p.want != msg.kind || m.dlg != nil {
		return m // nothing is waiting for this, or the user has moved on or is busy with a dialog
	}
	if msg.err != nil {
		m.pending = nil
		m.status = styErr.Render("✗ " + msg.err.Error())
		return m
	}
	seen := map[string]bool{}
	var names []string
	for _, n := range append(append([]string(nil), msg.names...), p.current...) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	choices := make([]choice, len(names))
	for i, n := range names {
		choices[i] = choice{label: n, value: n}
	}
	id, title := "issue.labels", "Labels for "+p.label
	if msg.kind == "assignees" {
		id, title = "issue.assignees", "Assignees for "+p.label
	}
	return m.withDialog(newChoiceDialog(id, title, choices, true, p.current))
}

// diffNames reports what to add and what to remove to turn old into want.
func diffNames(old, want []string) (add, remove []string) {
	had, wanted := map[string]bool{}, map[string]bool{}
	for _, n := range old {
		had[n] = true
	}
	for _, n := range want {
		wanted[n] = true
		if !had[n] {
			add = append(add, n)
		}
	}
	for _, n := range old {
		if !wanted[n] {
			remove = append(remove, n)
		}
	}
	return add, remove
}
