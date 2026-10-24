package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/graph"
)

func (m model) graphLines() ([]string, graph.Style) {
	return graph.Render(m.shown(), m.gstyle, m.gsel, m.width, time.Now())
}

// logHeight is the log window size: the table body plus the unused column header row.
func (m model) logHeight() int { return m.bodyHeight() + 1 }

func (m model) maxTop() int { return max(0, len(m.logs)-m.logHeight()) }

func (m model) bodyHeight() int { return max(1, m.height-5) }

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
	var b strings.Builder
	title := fmt.Sprintf(" gh9s  %s@%s  <%s>", m.repo, m.branch, viewNames[m.view])
	if m.target != (ghcli.RepoRef{}) {
		title = fmt.Sprintf(" gh9s  %s  <%s>", m.target, viewNames[m.view])
	}
	if m.wfName != "" && m.view == viewRuns {
		title += " workflow=" + m.wfName
	}
	if m.view == viewChecks || m.view == viewPRDetail {
		if m.pr != "" {
			title += " pr=#" + m.pr
		}
	}
	if m.view == viewPRs {
		if name := prFilterName(m.prFilter); name != "" {
			title += " filter=" + name
		}
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
	b.WriteString(styDim.Render("<1>runs <2>workflows <3>pulls <:fav>favorites <:repo> switch repo <P>current-PR <:>cmd </>filter") + "\n")

	if m.picker != nil {
		return b.String() + "\n" + m.pickerView()
	}
	if m.form != nil {
		return b.String() + "\n" + m.form.view(m.width)
	}
	if s, ok := screens[m.view]; ok && s.body != nil {
		return b.String() + s.body(m) + m.footer()
	}
	if m.view == viewGraph {
		return b.String() + m.graphBody() + m.footer()
	}
	if m.view == viewLogs {
		return b.String() + m.logBody() + m.footer()
	}

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
			s := trunc(c, w[j]) + "  "
			if j == statusCol(m.view) && i != m.cursor {
				s = stateStyle(r.state).Render(s)
			}
			line.WriteString(s)
		}
		if i == m.cursor {
			b.WriteString(stySel.Render(line.String()) + "\n")
		} else {
			b.WriteString(line.String() + "\n")
		}
		lines++
	}
	for ; lines < h-boolToInt(m.err != nil); lines++ {
		b.WriteString("\n")
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

func statusCol(v viewKind) int {
	switch v {
	case viewRuns, viewPRs, viewJobs:
		return 1
	case viewFavs:
		return 3
	case viewChecks:
		return 0
	}
	return 2
}

func (m model) footer() string {
	switch m.mode {
	case modeFilter:
		return styKey.Render("/") + m.input + "█"
	case modeCommand:
		return styKey.Render(":") + m.input + "█"
	case modeSearch:
		return styKey.Render("/") + m.input + "█"
	case modeAlias:
		return styKey.Render("alias for "+m.aliasFor.Workflow+" > ") + m.input + "█"
	case modeConfirm:
		return styErr.Render(m.confirmMsg)
	}
	var keys string
	switch m.view {
	case viewRuns:
		keys = "enter jobs  l pager-logs  L failed-logs  w watch  r rerun  F rerun-failed  c cancel  d delete  o open"
	case viewWorkflows:
		keys = "enter runs  t trigger  f favorite  a alias  e enable/disable  o open"
	case viewPRs:
		keys = "enter detail  u mine  e review-requested  a all  o open"
	case viewFavs:
		keys = "enter runs  f unfavorite  a alias"
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
	keys += "  R refresh  q quit"
	line := m.status
	if line == "" && m.note != "" {
		line = styDim.Render(m.note)
	}
	if line == "" && m.banner != "" {
		line = styErr.Render(m.banner)
	}
	return line + "\n" + styDim.Render(keys)
}
