package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewKind int

const (
	viewRuns viewKind = iota
	viewWorkflows
	viewPRs
	viewChecks
)

var viewNames = map[viewKind]string{
	viewRuns: "runs", viewWorkflows: "workflows", viewPRs: "pulls", viewChecks: "checks",
}

type inputMode int

const (
	modeNormal inputMode = iota
	modeFilter
	modeCommand
	modeTrigger
	modeConfirm
)

type model struct {
	repo, branch  string
	view          viewKind
	back          []viewKind
	tbl           table
	cursor, top   int
	width, height int
	loading       bool
	err           error
	status        string

	// scope for the current view
	wfID, wfName string // runs filtered by workflow
	pr           string // checks for a PR ("" = current branch)

	mode       inputMode
	input      string
	filter     string
	confirm    func() tea.Cmd
	confirmMsg string
}

type dataMsg struct {
	view viewKind
	tbl  table
	err  error
}
type actionMsg struct {
	msg string
	err error
}
type tickMsg struct{}
type infoMsg struct{ repo, branch string }
type gotoMsg struct {
	view viewKind
	pr   string
	err  error
}

var (
	styHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
	styCols   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	stySel    = lipgloss.NewStyle().Reverse(true).Bold(true)
	styOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styFail   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styRun    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

func main() {
	p := tea.NewProgram(model{loading: true}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh9s:", err)
		os.Exit(1)
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetch(), tick(), func() tea.Msg {
		r, b := repoInfo()
		return infoMsg{r, b}
	})
}

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) fetch() tea.Cmd {
	v, wf, pr, br := m.view, m.wfID, m.pr, m.branch
	return func() tea.Msg {
		var t table
		var err error
		switch v {
		case viewRuns:
			t, err = fetchRuns(wf)
		case viewWorkflows:
			t, err = fetchWorkflows()
		case viewPRs:
			t, err = fetchPRs(br)
		case viewChecks:
			t, err = fetchChecks(pr)
		}
		return dataMsg{v, t, err}
	}
}

func (m *model) push(v viewKind) tea.Cmd {
	m.back = append(m.back, m.view)
	return m.switchTo(v)
}

func (m *model) switchTo(v viewKind) tea.Cmd {
	m.view, m.tbl, m.cursor, m.top, m.filter, m.err = v, table{}, 0, 0, "", nil
	m.loading = true
	return m.fetch()
}

func (m model) visible() []row {
	if m.filter == "" {
		return m.tbl.rows
	}
	f := strings.ToLower(m.filter)
	var out []row
	for _, r := range m.tbl.rows {
		if strings.Contains(strings.ToLower(strings.Join(r.cells, " ")), f) {
			out = append(out, r)
		}
	}
	return out
}

func (m model) selected() (row, bool) {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return row{}, false
	}
	return v[m.cursor], true
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case infoMsg:
		m.repo, m.branch = msg.repo, msg.branch
	case tickMsg:
		return m, tea.Batch(m.fetch(), tick())
	case dataMsg:
		if msg.view != m.view {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.tbl = msg.tbl
			if n := len(m.visible()); m.cursor >= n {
				m.cursor = max(0, n-1)
			}
		}
	case actionMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
		} else {
			m.status = styOK.Render("✓ " + msg.msg)
		}
		return m, m.fetch()
	case gotoMsg:
		if msg.err != nil {
			m.status = styErr.Render("✗ " + msg.err.Error())
			return m, nil
		}
		m.pr = msg.pr
		return m, m.push(msg.view)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.mode == modeConfirm {
		mode := m.mode
		m.mode = modeNormal
		if mode == modeConfirm && (k.String() == "y" || k.String() == "Y") {
			return m, m.confirm()
		}
		m.status = styDim.Render("cancelled")
		return m, nil
	}
	if m.mode != modeNormal {
		return m.handleInput(k)
	}

	m.status = ""
	n := len(m.visible())
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "backspace":
		if m.filter != "" {
			m.filter, m.cursor, m.top = "", 0, 0
		} else if len(m.back) > 0 {
			prev := m.back[len(m.back)-1]
			m.back = m.back[:len(m.back)-1]
			if prev != viewRuns {
				m.wfID, m.wfName = "", ""
			}
			return m, m.switchTo(prev)
		}
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, n-1))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, n-1)
	case "ctrl+d", "pgdown":
		m.cursor = min(m.cursor+10, max(0, n-1))
	case "ctrl+u", "pgup":
		m.cursor = max(m.cursor-10, 0)
	case "/":
		m.mode, m.input = modeFilter, m.filter
	case ":":
		m.mode, m.input = modeCommand, ""
	case "1":
		return m.goTop(viewRuns)
	case "2":
		return m.goTop(viewWorkflows)
	case "3":
		return m.goTop(viewPRs)
	case "P":
		return m, func() tea.Msg {
			pr, err := currentPR()
			return gotoMsg{viewChecks, pr, err}
		}
	case "R", "ctrl+r":
		m.loading = true
		return m, m.fetch()
	case "o":
		if r, ok := m.selected(); ok && r.url != "" {
			_ = openURL(r.url)
			m.status = styDim.Render("opened " + r.url)
		}
	case "enter":
		return m.enter()
	case "t":
		if m.view == viewWorkflows {
			if _, ok := m.selected(); ok {
				m.mode, m.input = modeTrigger, m.branch
			}
		} else {
			m.status = styDim.Render("trigger works from the workflows view (2)")
		}
	case "w":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			c := exec.Command("gh", "run", "watch", r.id, "--exit-status")
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return actionMsg{msg: "watch finished: run " + r.id}
			})
		}
	case "l":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, logsCmd(r.id, false)
		}
	case "L":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, logsCmd(r.id, true)
		}
	case "r":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, runAction("rerun run "+r.id, "run", "rerun", r.id)
		}
	case "F":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			return m, runAction("rerun failed jobs of "+r.id, "run", "rerun", r.id, "--failed")
		}
	case "c":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			m.mode, m.confirmMsg = modeConfirm, "cancel run "+r.id+"? (y/N)"
			m.confirm = func() tea.Cmd { return runAction("cancelled run "+r.id, "run", "cancel", r.id) }
		}
	case "d":
		if r, ok := m.selected(); ok && m.view == viewRuns {
			m.mode, m.confirmMsg = modeConfirm, "delete run "+r.id+"? (y/N)"
			m.confirm = func() tea.Cmd { return runAction("deleted run "+r.id, "run", "delete", r.id) }
		}
	case "e":
		if r, ok := m.selected(); ok && m.view == viewWorkflows {
			sub := "disable"
			if r.cells[2] != "active" {
				sub = "enable"
			}
			return m, runAction(sub+"d workflow "+r.extra, "workflow", sub, r.id)
		}
	}
	m.scroll()
	return m, nil
}

func (m model) goTop(v viewKind) (tea.Model, tea.Cmd) {
	m.back, m.wfID, m.wfName, m.pr = nil, "", "", ""
	return m, m.switchTo(v)
}

func (m model) enter() (tea.Model, tea.Cmd) {
	r, ok := m.selected()
	if !ok {
		return m, nil
	}
	switch m.view {
	case viewWorkflows:
		m.wfID, m.wfName = r.id, r.extra
		return m, m.push(viewRuns)
	case viewRuns:
		return m, logsCmd(r.id, false)
	case viewPRs:
		m.pr = r.id
		return m, m.push(viewChecks)
	case viewChecks:
		if r.url != "" {
			_ = openURL(r.url)
		}
	}
	return m, nil
}

func (m model) handleInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		if m.mode == modeFilter {
			m.filter = ""
		}
		m.mode, m.input = modeNormal, ""
		return m, nil
	case tea.KeyEnter:
		mode, in := m.mode, strings.TrimSpace(m.input)
		m.mode, m.input = modeNormal, ""
		switch mode {
		case modeFilter:
			m.filter, m.cursor, m.top = in, 0, 0
		case modeCommand:
			return m.command(in)
		case modeTrigger:
			if r, ok := m.selected(); ok {
				return m, triggerCmd(r, in)
			}
		}
		return m, nil
	case tea.KeyBackspace:
		if _, sz := utf8.DecodeLastRuneInString(m.input); sz > 0 {
			m.input = m.input[:len(m.input)-sz]
		}
	case tea.KeySpace:
		m.input += " "
	case tea.KeyRunes:
		m.input += string(k.Runes)
	}
	if m.mode == modeFilter {
		m.filter, m.cursor, m.top = m.input, 0, 0
	}
	return m, nil
}

func (m model) command(c string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(c) {
	case "runs", "run", "r":
		return m.goTop(viewRuns)
	case "workflows", "workflow", "wf", "w":
		return m.goTop(viewWorkflows)
	case "pulls", "pull", "prs", "pr":
		return m.goTop(viewPRs)
	case "cpr", "checks":
		return m, func() tea.Msg {
			pr, err := currentPR()
			return gotoMsg{viewChecks, pr, err}
		}
	case "q", "quit", "exit":
		return m, tea.Quit
	case "":
	default:
		m.status = styErr.Render("unknown command: " + c)
	}
	return m, nil
}

func runAction(done string, args ...string) tea.Cmd {
	return func() tea.Msg {
		_, err := gh(args...)
		return actionMsg{msg: done, err: err}
	}
}

// triggerCmd dispatches a workflow. Input is "<ref> [key=value ...]".
func triggerCmd(r row, in string) tea.Cmd {
	f := strings.Fields(in)
	args := []string{"workflow", "run", r.id}
	if len(f) > 0 {
		args = append(args, "--ref", f[0])
		for _, kv := range f[1:] {
			args = append(args, "-f", kv)
		}
	}
	return runAction("triggered "+r.extra, args...)
}

func logsCmd(id string, failedOnly bool) tea.Cmd {
	flag := "--log"
	if failedOnly {
		flag = "--log-failed"
	}
	c := exec.Command("sh", "-c", fmt.Sprintf("gh run view %s %s 2>&1 | ${PAGER:-less -R}", id, flag))
	return tea.ExecProcess(c, func(error) tea.Msg { return actionMsg{msg: "closed logs"} })
}

func (m *model) scroll() {
	h := m.bodyHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
}

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

func (m model) View() string {
	var b strings.Builder
	title := fmt.Sprintf(" gh9s  %s@%s  <%s>", m.repo, m.branch, viewNames[m.view])
	if m.wfName != "" && m.view == viewRuns {
		title += " workflow=" + m.wfName
	}
	if m.view == viewChecks {
		if m.pr != "" {
			title += " pr=#" + m.pr
		}
	}
	vis := m.visible()
	title += fmt.Sprintf("  [%d]", len(vis))
	if m.filter != "" {
		title += "  /" + m.filter
	}
	if m.loading {
		title += "  loading…"
	}
	b.WriteString(styHeader.Render(strings.TrimSpace(title)) + "\n")
	b.WriteString(styDim.Render("<1>runs <2>workflows <3>pulls <P>current-PR <:>cmd </>filter <?>keys below") + "\n")

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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func statusCol(v viewKind) int {
	switch v {
	case viewRuns, viewPRs:
		return 1
	case viewChecks:
		return 0
	}
	return 2
}

func stateStyle(s state) lipgloss.Style {
	switch s {
	case stOK:
		return styOK
	case stFail:
		return styFail
	case stRun:
		return styRun
	}
	return styDim
}

func (m model) footer() string {
	switch m.mode {
	case modeFilter:
		return styKey.Render("/") + m.input + "█"
	case modeCommand:
		return styKey.Render(":") + m.input + "█"
	case modeTrigger:
		return styKey.Render("trigger ref [k=v ...] > ") + m.input + "█"
	case modeConfirm:
		return styErr.Render(m.confirmMsg)
	}
	var keys string
	switch m.view {
	case viewRuns:
		keys = "enter/l logs  L failed-logs  w watch  r rerun  F rerun-failed  c cancel  d delete  o open"
	case viewWorkflows:
		keys = "enter runs  t trigger  e enable/disable  o open"
	case viewPRs:
		keys = "enter checks  o open"
	case viewChecks:
		keys = "enter/o open check  esc back"
	}
	keys += "  R refresh  q quit"
	if m.status != "" {
		return m.status + "\n" + styDim.Render(keys)
	}
	return "\n" + styDim.Render(keys)
}
