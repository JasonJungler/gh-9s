package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
	"github.com/charmbracelet/lipgloss"
)

func (m model) graphLines() ([]string, graph.Style) {
	return graph.Render(m.shown(), m.gstyle, m.gsel, m.width, time.Now())
}

// logHeight is the log window size: the table body plus the unused column header row.
func (m model) logHeight() int { return m.bodyHeight() + 1 }

func (m model) maxTop() int { return max(0, len(m.logs)-m.logHeight()) }

// bodyHeight is how many table rows fit; the prompt box, when open, takes three lines of the screen.
func (m model) bodyHeight() int {
	h := m.height - 5 - m.promptRows() - m.inboxExtraRows() - m.favExtraRows() - m.boardExtraRows()
	if !m.paneList && m.previewShown() {
		h -= 2 // the list sits in a box
	}
	return max(1, h)
}

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s + strings.Repeat(" ", w-utf8.RuneCountInString(s))
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

func (m model) widths() []int {
	w := make([]int, len(m.tbl.cols))
	for i, c := range m.tbl.cols {
		w[i] = utf8.RuneCountInString(c)
	}
	for _, r := range m.tbl.rows {
		for i, c := range r.cells {
			w[i] = max(w[i], min(utf8.RuneCountInString(c), 50))
		}
	}
	// shrink the widest column if the terminal is too narrow
	for {
		total := 0
		for _, x := range w {
			total += x + 2
		}
		if total <= m.width || m.width == 0 {
			return w
		}
		wi := 0
		for i := range w {
			if w[i] > w[wi] {
				wi = i
			}
		}
		if w[wi] <= 6 {
			return w
		}
		w[wi]--
	}
}

// View renders the header, the current view, and the footer.
func (m model) View() string {
	if m.askIcons {
		return m.iconAskView()
	}
	var b strings.Builder
	title := fmt.Sprintf(" gh9s  %s@%s  <%s>", m.repo, m.branch, viewNames[m.view])
	if m.target != (ghcli.RepoRef{}) {
		title = fmt.Sprintf(" gh9s  %s  <%s>", m.target, viewNames[m.view])
	}
	if m.wfName != "" && m.view == viewRuns {
		title += " workflow=" + m.wfName
	}
	if m.view == viewChecks || m.view == viewPRDetail || m.view == viewDiff {
		if m.pr != "" {
			title += " pr=#" + m.pr
		}
	}
	if m.view == viewPRs {
		if name := prFilterName(m.prFilter); name != "" {
			title += " filter=" + name
		}
	}
	if m.view == viewIssues {
		title += " filter=" + issueFilterName(m.issueFilter)
	}
	if m.view == viewIssue {
		title += " issue=#" + m.issue
	}
	if m.view == viewBoard {
		title = " gh9s  <board>  " + m.boardTitle()
	}
	if m.view == viewJobs || m.view == viewLogs || m.view == viewGraph {
		title += " run=" + m.runID
	}
	if m.view == viewGraph {
		_, used := m.graphLines()
		title += " style=" + used.String()
		if used != m.gstyle {
			title += " (" + m.gstyle.String() + " too wide)"
		}
	}
	if m.view == viewDiff && len(m.matches) > 0 {
		title += fmt.Sprintf(" match %d/%d", m.mi+1, len(m.matches))
	}
	if m.view == viewLogs {
		title += " job=" + m.jobName
		if m.follow {
			title += " (following)"
		}
		if len(m.matches) > 0 {
			title += fmt.Sprintf(" match %d/%d", m.mi+1, len(m.matches))
		}
	}
	vis := m.visible()
	count := len(vis)
	if m.view == viewLogs {
		count = len(m.logs)
	}
	if m.view == viewGraph {
		count = len(m.shown().Nodes)
	}
	if _, isScreen := screens[m.view]; !isScreen {
		title += fmt.Sprintf("  [%d]", count)
	}
	if m.filter != "" {
		title += "  /" + m.filter
	}
	if m.loading {
		title += "  loading…"
	}
	b.WriteString(styHeader.Render(strings.TrimSpace(title)) + "\n")
	b.WriteString(styDim.Render("<1>runs <2>workflows <3>pulls <4>issues <0>inbox <:fav>favorites <:repo> switch repo <P>current-PR <:>cmd </>filter") + "\n")
	if m.promptActive() {
		b.WriteString(m.promptBox() + "\n")
	}

	if m.helpOpen {
		return b.String() + m.helpView()
	}
	if m.dlg != nil {
		return b.String() + m.boxed(m.dialogView())
	}
	if m.picker != nil {
		return b.String() + m.boxed(m.pickerView())
	}
	if m.form != nil {
		return b.String() + m.boxed(m.form.view(m.width))
	}
	if s, ok := screens[m.view]; ok && s.body != nil {
		if m.previewShown() {
			return b.String() + m.withPane(s.body) + m.footer()
		}
		return b.String() + s.body(m) + m.footer()
	}
	if m.view == viewGraph {
		return b.String() + m.graphBody() + m.footer()
	}
	if m.view == viewLogs {
		return b.String() + m.logBody() + m.footer()
	}

	if m.view == viewFavs {
		b.WriteString(m.favTabBar() + "\n")
	}
	if m.previewShown() {
		b.WriteString(m.withPane(model.tableBody))
	} else {
		b.WriteString(m.tableBody())
	}
	b.WriteString(m.footer())
	return b.String()
}

// logBody renders the visible window of the log, padded to the body height.
// isMatch reports whether log line i matches the active search.
func (m model) isMatch(i int) bool {
	j := sort.SearchInts(m.matches, i)
	return j < len(m.matches) && m.matches[j] == i
}

func (m model) logBody() string {
	var b strings.Builder
	h := m.logHeight()
	if m.err != nil {
		b.WriteString(styErr.Render(m.err.Error()) + "\n")
		h--
	} else if m.loading && len(m.logs) == 0 {
		b.WriteString(styDim.Render("loading logs…") + "\n")
		h--
	}
	end := min(len(m.logs), m.top+h)
	n := 0
	for i := m.top; i < end; i++ {
		l := m.logs[i]
		if m.width > 0 {
			l = strings.TrimRight(trunc(l, m.width), " ")
		}
		switch {
		case m.isMatch(i) && m.matches[m.mi] == i:
			l = stySel.Render(l)
		case m.isMatch(i):
			l = styKey.Render(l)
		case strings.Contains(l, "##[error]") || strings.Contains(l, "Error:"):
			l = styFail.Render(l)
		case strings.Contains(l, "##[warning]"):
			l = styRun.Render(l)
		case strings.Contains(l, "##[group]"):
			l = styCols.Render(l)
		}
		b.WriteString(l + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) graphBody() string {
	lines, _ := m.graphLines()
	h := m.logHeight()
	top := min(m.top, max(0, len(lines)-h))
	var b strings.Builder
	if m.err != nil {
		b.WriteString(styErr.Render(m.err.Error()) + "\n")
		h--
	}
	n := 0
	for i := top; i < len(lines) && n < h; i++ {
		b.WriteString(lines[i] + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
	return b.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// tableBody draws the column header and the rows of m.tbl (filtered, scrolled, with the cursor).
func (m model) tableBody() string {
	vis := m.visible()
	var b strings.Builder
	w := m.widths()
	var hdr strings.Builder
	for i, c := range m.tbl.cols {
		hdr.WriteString(trunc(c, w[i]) + "  ")
	}
	b.WriteString(styCols.Render(hdr.String()) + "\n")

	h := m.bodyHeight()
	if m.err != nil {
		b.WriteString(styErr.Render(m.err.Error()) + "\n")
	}
	end := min(len(vis), m.top+h)
	lines := 0
	for i := m.top; i < end; i++ {
		r := vis[i]
		var line strings.Builder
		for j, c := range r.cells {
			s := trunc(ghcli.CleanText(c), w[j]) + "  "
			if j == statusCol(m.view) && i != m.cursor {
				s = stateStyle(r.state).Render(s)
			} else if i != m.cursor && j < len(r.tint) && r.tint[j] != "" {
				s = lipgloss.NewStyle().Foreground(lipgloss.Color(r.tint[j])).Render(s)
			}
			line.WriteString(s)
		}
		if i == m.cursor {
			if m.paneList {
				b.WriteString(stySel.Render(padTo(line.String(), m.width)) + "\n")
				lines++
				continue
			}
			b.WriteString(stySel.Render(line.String()) + "\n")
		} else {
			b.WriteString(line.String() + "\n")
		}
		lines++
	}
	for ; lines < h-boolToInt(m.err != nil); lines++ {
		b.WriteString("\n")
	}
	return b.String()
}

func statusCol(v viewKind) int {
	switch v {
	case viewRuns, viewPRs, viewJobs, viewIssues:
		return 1
	case viewFavs:
		return 3
	case viewChecks:
		return 0
	}
	return 2
}

func (m model) footer() string {
	var keys string
	switch m.view {
	case viewRuns:
		keys = "enter jobs  l pager-logs  L failed-logs  w watch  r rerun  F rerun-failed  c cancel  d delete  f pin-repo  o open"
	case viewWorkflows:
		keys = "enter runs  t trigger  f favorite  a alias  e enable/disable  o open"
	case viewPRs:
		keys = "enter detail  u mine  e review-requested  a all  f pin-repo  o open"
	case viewFavs:
		keys = m.favFooter()
	case viewChecks:
		keys = "enter/o open check  esc back"
	case viewJobs:
		keys = "enter logs  o open  esc back"
	case viewGraph:
		keys = "hjkl/arrows move  enter logs or expand  x collapse  v style  o open  ctrl-d/u scroll  esc back"
	case viewLogs:
		keys = "j/k scroll  space/ctrl-d page  g/G top/bottom  / search  n/N next/prev  f follow  esc back"
	}
	if s, ok := screens[m.view]; ok {
		keys = s.footer
	}
	if m.view == viewBoard && m.boardSt().colsActive(m) {
		keys = "h/l column  j/k card  g/G first/last  enter open  b list  [/] view  f pin/unpin Project  o browser  / filter"
	}
	if m.previewShown() {
		keys += "  J/K scroll pane  p hide pane"
	}
	keys += "  ? help  R refresh  q quit"
	line := m.status
	if line == "" && m.note != "" {
		line = styDim.Render(m.note)
	}
	if line == "" && m.banner != "" {
		line = styErr.Render(m.banner)
	}
	return line + "\n" + styDim.Render(keys)
}

// promptActive reports whether a one-line prompt (command, filter, search, alias, confirm) is open.
func (m model) promptActive() bool {
	switch m.mode {
	case modeCommand, modeFilter, modeSearch, modeAlias, modeConfirm:
		return true
	}
	return false
}

func (m model) promptRows() int {
	if m.promptActive() {
		return 3
	}
	return 0
}

// boxWidth is the inner width of a bordered box that fills the screen.
func (m model) boxWidth() int { return max(m.width-2, 20) }

// promptBox draws the open prompt in a rounded box, as k9s does: the text typed so far, the cursor,
// and (in the command bar) the rest of the best suggestion in grey.
func (m model) promptBox() string {
	var content string
	switch m.mode {
	case modeConfirm:
		content = styErr.Render(m.confirmMsg)
	case modeCommand:
		content = styKey.Render(":") + m.input + "█" + styDim.Render(m.ghost())
	case modeAlias:
		content = styKey.Render("alias for "+m.aliasFor.Workflow+" › ") + m.input + "█"
	default: // filter and search
		content = styKey.Render("/") + m.input + "█"
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).
		Width(m.boxWidth()).Render(content)
}

// boxed puts a dialog, form or picker in the same rounded box, pinned under the header.
func (m model) boxed(content string) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).
		Padding(0, 1).Width(m.boxWidth()).Render(strings.TrimRight(content, "\n")) + "\n"
}
