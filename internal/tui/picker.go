package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls the picker makes; tests replace them.
var (
	listRepos   = ghcli.ListAccessibleRepos
	searchRepos = ghcli.SearchRepos
	// debounce waits a moment after the last keystroke before a global search is sent.
	debounce = func(seq int) tea.Cmd {
		return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return pickerDebounceMsg{seq} })
	}
)

// repoCacheTTL is how long the list of repos I can access is reused: it takes one request per
// organisation to build and changes slowly.
const repoCacheTTL = 10 * time.Minute

// clock is time.Now; tests replace it.
var clock = time.Now

type repoCacheEntry struct {
	list []ghcli.RepoInfo
	at   time.Time
}

// withRepoCache returns a copy of the cache with host's entry replaced (the map is shared with
// copies of the model, so it is never written in place).
func withRepoCache(c map[string]repoCacheEntry, host string, list []ghcli.RepoInfo) map[string]repoCacheEntry {
	out := make(map[string]repoCacheEntry, len(c)+1)
	for k, v := range c {
		out[k] = v
	}
	out[host] = repoCacheEntry{list, clock()}
	return out
}

const (
	minGlobalQuery = 2  // shorter global queries are not sent: GitHub's search limits are tight
	searchLimit    = 30 // results asked of a global search
)

type pickerRow struct {
	label string
	ref   ghcli.RepoRef
	cwd   bool // the "current directory" row
}

// picker is the repo picker overlay (opened by ":repo" with no argument).
type picker struct {
	query   string
	global  bool // false: repos I have access to; true: all of GitHub
	sel     int
	seq     int // bumped on every query change; only the newest search's results are shown
	loading bool
	err     string
	cache   []ghcli.RepoInfo // repos I have access to, loaded once when the picker opens
	results []ghcli.RepoInfo // results of the newest global search
}

type pickerLoadedMsg struct {
	host  string
	cache []ghcli.RepoInfo
	err   error
}

type pickerDebounceMsg struct{ seq int }

type pickerResultMsg struct {
	seq   int
	items []ghcli.RepoInfo
	err   error
}

// displayRef shows the host only when it is not github.com.
func displayRef(r ghcli.RepoRef) string {
	if r.Host == "" || r.Host == "github.com" {
		return r.Owner + "/" + r.Name
	}
	return r.String()
}

func infoLabel(i ghcli.RepoInfo) string {
	return fmt.Sprintf("%-42s %-12s ★%-6d %s", displayRef(i.Ref), i.Language, i.Stars, ui.Age(i.UpdatedAt))
}

// rows lists what the picker shows: the current directory first; then, with an empty query,
// the recently used repos; otherwise the repos matching the query in the active scope.
func (p picker) rows(cwd ghcli.RepoRef, recents []ghcli.RepoRef) []pickerRow {
	label := "(current directory)"
	if cwd != (ghcli.RepoRef{}) {
		label += "  " + displayRef(cwd)
	}
	rows := []pickerRow{{label: label, cwd: true}}
	if strings.TrimSpace(p.query) == "" {
		for _, r := range recents {
			rows = append(rows, pickerRow{label: displayRef(r) + "  (recent)", ref: r})
		}
		return rows
	}
	list := p.results
	if !p.global {
		list = ghcli.FilterRepos(p.cache, p.query)
	}
	for _, i := range list {
		rows = append(rows, pickerRow{label: infoLabel(i), ref: i.Ref})
	}
	return rows
}

func (m model) recentList() []ghcli.RepoRef {
	if m.recents == nil {
		return nil
	}
	return m.recents.List()
}

// openPicker shows the picker and starts loading the repos the user can access.
func (m *model) openPicker() tea.Cmd {
	host := m.curTarget().Host
	if e, ok := m.repoCache[host]; ok && clock().Sub(e.at) < repoCacheTTL {
		m.picker = &picker{cache: e.list} // fetched recently: no new round of API calls
		return nil
	}
	m.picker = &picker{loading: true}
	return func() tea.Msg {
		l, err := listRepos(host)
		return pickerLoadedMsg{host, l, err}
	}
}

// queryChanged resets the selection and, for a global search, schedules the debounced request.
func (p *picker) queryChanged() tea.Cmd {
	p.sel = 0
	p.seq++
	if !p.global {
		return nil
	}
	if utf8.RuneCountInString(strings.TrimSpace(p.query)) < minGlobalQuery {
		p.results, p.err, p.loading = nil, "", false
		return nil
	}
	return debounce(p.seq)
}

func (m model) pickerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := *m.picker
	rows := p.rows(m.curRepo, m.recentList())
	var cmd tea.Cmd
	switch k.Type {
	case tea.KeyEsc:
		m.picker = nil
		return m, nil
	case tea.KeyEnter:
		if p.sel < 0 || p.sel >= len(rows) {
			return m, nil
		}
		row := rows[p.sel]
		m.picker = nil
		if row.cwd {
			return m, func() tea.Msg { return repoSwitchedMsg{} }
		}
		ref := row.ref
		m.status = ui.Dim.Render("checking " + displayRef(ref) + "…")
		return m, func() tea.Msg { return repoSwitchedMsg{ref, checkRepo(ref)} }
	case tea.KeyUp, tea.KeyCtrlP:
		p.sel = max(p.sel-1, 0)
	case tea.KeyDown, tea.KeyCtrlN:
		p.sel = min(p.sel+1, max(len(rows)-1, 0))
	case tea.KeyTab:
		p.global = !p.global
		p.err = ""
		cmd = p.queryChanged()
	case tea.KeyBackspace:
		if _, sz := utf8.DecodeLastRuneInString(p.query); sz > 0 {
			p.query = p.query[:len(p.query)-sz]
		}
		cmd = p.queryChanged()
	case tea.KeySpace:
		p.query += " "
		cmd = p.queryChanged()
	case tea.KeyRunes:
		p.query += string(k.Runes)
		cmd = p.queryChanged()
	}
	m.picker = &p
	return m, cmd
}

// pickerMsg handles the picker's asynchronous messages.
func (m model) pickerMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		return m, nil // the picker was closed while a request was in flight
	}
	p := *m.picker
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case pickerLoadedMsg:
		p.loading = false
		if msg.err != nil {
			p.err = msg.err.Error()
		} else {
			p.cache = msg.cache
			m.repoCache = withRepoCache(m.repoCache, msg.host, msg.cache)
		}
	case pickerDebounceMsg:
		if msg.seq != p.seq {
			return m, nil // typed since: a newer search is coming
		}
		p.loading, p.err = true, ""
		host, q, seq := m.curTarget().Host, strings.TrimSpace(p.query), p.seq
		cmd = func() tea.Msg {
			items, err := searchRepos(host, q, searchLimit)
			return pickerResultMsg{seq, items, err}
		}
	case pickerResultMsg:
		if msg.seq != p.seq {
			return m, nil // an older search finishing late must not replace the newest
		}
		p.loading = false
		if msg.err != nil {
			p.err = msg.err.Error()
		} else {
			p.results, p.err = msg.items, ""
		}
	}
	m.picker = &p
	return m, cmd
}

func (m model) pickerView() string {
	p := *m.picker
	scope := "▸my access   ○global"
	if p.global {
		scope = "○my access   ▸global"
	}
	var b strings.Builder
	b.WriteString(ui.Cols.Render(" Switch repo") + "    scope: " + scope + ui.Dim.Render("   (tab)") + "\n")
	b.WriteString(" > " + p.query + "█\n")
	switch {
	case p.err != "":
		b.WriteString(ui.Err.Render(" "+p.err) + "\n")
	case p.loading:
		b.WriteString(ui.Dim.Render(" loading…") + "\n")
	default:
		b.WriteString("\n")
	}
	rows := p.rows(m.curRepo, m.recentList())
	h := max(5, m.height-9)
	start := 0
	if p.sel >= h {
		start = p.sel - h + 1
	}
	for i := start; i < len(rows) && i < start+h; i++ {
		line := strings.TrimRight(trunc(rows[i].label, max(m.width-4, 20)), " ")
		if i == p.sel {
			b.WriteString(ui.Sel.Render(" ▸ "+line) + "\n")
		} else {
			b.WriteString("   " + line + "\n")
		}
	}
	b.WriteString("\n" + ui.Dim.Render(" ↑↓ select   enter switch   tab scope   esc cancel") + "\n")
	return b.String()
}
