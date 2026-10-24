package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside calls and timing of the inbox; tests replace them.
var (
	inboxHosts = ghcli.InboxHosts
	inboxLogin = ghcli.FetchLogin
	inboxFetch = ghcli.FetchInbox
	inboxNow   = time.Now
)

const (
	inboxEvery  = time.Minute     // the screen refreshes itself at most this often
	inboxManual = 5 * time.Second // and R at most this often
	inboxID     = "inbox"         // the inbox shows one thing: results for another identity are dropped
)

// Only one round of searches runs at a time: a tick or an R while one is still going (a slow host)
// must not start another. A round that has run longer than inboxStuck is given up on.
var (
	inboxBusy      atomic.Bool
	inboxBusySince atomic.Int64 // unix nanoseconds
	inboxRounds    atomic.Int64
)

const inboxStuck = 2 * time.Minute

func inboxRunning() bool {
	return inboxBusy.Load() && inboxNow().UnixNano()-inboxBusySince.Load() < int64(inboxStuck)
}

var inboxTabNames = [4]string{"Assigned", "Review requested", "My PRs", "Mentions"}

var inboxEmpty = [4]string{
	"nothing assigned to you", "no reviews are waiting for you", "you have no open pull requests", "no mentions",
}

// hostTab is what one host returned for one tab.
type hostTab struct {
	items []ghcli.InboxItem
	total int
}

type hostResult struct {
	tabs [4]hostTab
	err  error
}

// inboxPayload is one round of searches.
type inboxPayload struct {
	hosts     []string
	results   map[string]hostResult
	notes     []string
	round     int64
	unchanged bool // a throttled round: nothing was asked, just show what we have
}

// inboxTabState is a tab as the user sees it.
type inboxTabState struct {
	items       []ghcli.InboxItem // merged across hosts, newest first
	total       int               // what GitHub says there are, across hosts
	cursor, top int
	fresh       map[string]bool // rows that appeared since the last round, until the tab is left
}

type inboxState struct {
	tab      int
	tabs     [4]inboxTabState
	byHost   map[string][4]hostTab
	notes    []string
	loaded   bool
	round    int64 // the newest round applied: an older one arriving later is dropped
	loadedAt time.Time
	force    bool // the next fetch ignores the throttle (R)
}

func inboxKey(it ghcli.InboxItem) string { return fmt.Sprintf("%s/%s#%d", it.Host, it.Repo, it.Number) }

func init() {
	register(viewInbox, screen{
		name:     "inbox",
		identity: func(m model) string { return inboxID },
		fetch:    inboxFetchCmd,
		apply:    inboxApply,
		key:      inboxKeyHandler,
		body:     inboxBody,
		footer:   "tab/h/l switch tab  enter open  o browser  / filter  R refresh",
		poll:     true, // every tick; the throttle in the fetch keeps it to once a minute
	})
}

func unchangedMsg() tea.Msg {
	return screenMsg{view: viewInbox, id: inboxID, data: inboxPayload{unchanged: true}}
}

func inboxFetchCmd(m model) tea.Cmd {
	st, _ := m.screenData(viewInbox).(inboxState)
	if inboxRunning() || (st.loaded && !st.force && inboxNow().Sub(st.loadedAt) < inboxEvery) {
		return func() tea.Msg { return unchangedMsg() }
	}
	round := inboxRounds.Add(1)
	inboxBusy.Store(true)
	inboxBusySince.Store(inboxNow().UnixNano())
	return func() tea.Msg {
		defer func() {
			if inboxRounds.Load() == round { // a newer round took over: leave its flag alone
				inboxBusy.Store(false)
			}
		}()
		hosts, herr := inboxHosts()
		p := inboxPayload{round: round, hosts: hosts, results: map[string]hostResult{}}
		if herr != nil {
			p.notes = append(p.notes, "couldn't list the logged-in hosts: "+herr.Error())
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, h := range hosts {
			wg.Add(1)
			go func(host string) {
				defer wg.Done()
				res := hostResult{}
				login, err := inboxLogin(host)
				if err != nil {
					res.err = err
				} else {
					var tw sync.WaitGroup
					var tmu sync.Mutex
					for tab := 0; tab < 4; tab++ {
						tw.Add(1)
						go func(tab int) {
							defer tw.Done()
							items, total, err := inboxFetch(host, ghcli.InboxTab(tab), login)
							tmu.Lock()
							defer tmu.Unlock()
							if err != nil && res.err == nil {
								res.err = err
							}
							res.tabs[tab] = hostTab{items, total}
						}(tab)
					}
					tw.Wait()
				}
				mu.Lock()
				p.results[host] = res
				mu.Unlock()
			}(h)
		}
		wg.Wait()
		return screenMsg{view: viewInbox, id: inboxID, data: p}
	}
}

func inboxApply(m model, msg screenMsg) model {
	p := msg.data.(inboxPayload)
	st, _ := m.screenData(viewInbox).(inboxState)
	if p.unchanged {
		m.loading = inboxBusy.Load() // a forced round may still be on its way
		if len(m.tbl.cols) == 0 {    // coming back to the screen: show the tab where it was left
			for i := range st.tabs {
				st.tabs[i].fresh = map[string]bool{} // and what was new is old now
			}
			return m.rebuildInbox(st, "")
		}
		return m
	}
	if p.round < st.round {
		return m // an older round that arrived after a newer one
	}
	keepID := ""
	if r, ok := m.selected(); ok && m.view == viewInbox {
		keepID = r.id
	}
	prev := map[string]bool{}
	for _, t := range st.tabs {
		for _, it := range t.items {
			prev[inboxKey(it)] = true
		}
	}
	byHost := map[string][4]hostTab{}
	var notes []string
	notes = append(notes, p.notes...)
	for _, h := range p.hosts {
		res := p.results[h]
		if res.err != nil {
			notes = append(notes, h+": "+ghcli.CleanText(res.err.Error()))
			if old, ok := st.byHost[h]; ok { // keep showing the last good answer of a host that failed now
				byHost[h] = old
			}
			continue
		}
		byHost[h] = res.tabs
	}
	for i := 0; i < 4; i++ {
		var items []ghcli.InboxItem
		total := 0
		for _, h := range p.hosts {
			if ht, ok := byHost[h]; ok {
				items = append(items, ht[i].items...)
				total += ht[i].total
			}
		}
		sort.SliceStable(items, func(a, b int) bool { return items[a].UpdatedAt > items[b].UpdatedAt })
		fresh := map[string]bool{}
		for _, it := range items {
			k := inboxKey(it)
			_, hostKnown := st.byHost[it.Host] // a host with no earlier answer has nothing to be new against
			if st.tabs[i].fresh[k] || (st.loaded && hostKnown && !prev[k]) {
				fresh[k] = true
			}
		}
		st.tabs[i].items, st.tabs[i].total, st.tabs[i].fresh = items, total, fresh
	}
	st.byHost, st.notes = byHost, notes
	st.loaded, st.loadedAt, st.force, st.round = true, inboxNow(), false, p.round
	return m.rebuildInbox(st, keepID)
}

// rebuildInbox shows the active tab: its table, with the cursor on keepID if it is still there,
// else where the tab's cursor was.
func (m model) rebuildInbox(st inboxState, keepID string) model {
	m = m.withScreenData(viewInbox, st)
	tab := st.tabs[st.tab]
	cols, rows := inboxRows(st.tab, tab)
	m.tbl = table{cols: cols, rows: rows}
	m.cursor, m.top = tab.cursor, tab.top
	if keepID != "" {
		for i, r := range m.visible() {
			if r.id == keepID {
				m.cursor = i
			}
		}
	}
	if n := len(m.visible()); m.cursor >= n {
		m.cursor = max(0, n-1)
	}
	m.scroll()
	return m
}

func checksCell(c string) (string, state) {
	switch c {
	case "SUCCESS":
		return "✓ pass", stOK
	case "FAILURE", "ERROR":
		return "✗ fail", stFail
	case "PENDING", "EXPECTED":
		return "● pending", stRun
	}
	return "-", stOther
}

func reviewCell(it ghcli.InboxItem) string {
	switch it.ReviewDecision {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes"
	case "REVIEW_REQUIRED":
		return "review"
	}
	return "-"
}

func inboxRows(tab int, ts inboxTabState) ([]string, []row) {
	var cols []string
	switch tab {
	case 0:
		cols = []string{"REPO", "#", "UPDATED", "AUTHOR", "TITLE"}
	case 1, 2:
		cols = []string{"REPO", "#", "CHECKS", "REVIEW", "UPDATED", "AUTHOR", "TITLE"}
	default:
		cols = []string{"REPO", "#", "TYPE", "UPDATED", "AUTHOR", "TITLE"}
	}
	var rows []row
	for _, it := range ts.items {
		repo := it.Repo
		if it.Host != "github.com" && it.Host != "" {
			repo = it.Host + "/" + repo
		}
		if ts.fresh[inboxKey(it)] {
			repo = "● " + repo
		}
		title := it.Title
		if it.Draft {
			title = "[draft] " + title
		}
		num := fmt.Sprintf("#%d", it.Number)
		st := stOther
		var cells []string
		switch tab {
		case 0:
			cells = []string{repo, num, age(it.UpdatedAt), it.Author, title}
		case 1, 2:
			var c string
			c, st = checksCell(it.Checks)
			cells = []string{repo, num, c, reviewCell(it), age(it.UpdatedAt), it.Author, title}
		default:
			cells = []string{repo, num, it.Kind, age(it.UpdatedAt), it.Author, title}
		}
		rows = append(rows, row{id: inboxKey(it), url: it.URL, state: st, cells: cells})
	}
	return cols, rows
}

// inboxExtraRows is how many lines the inbox uses above and below its table: the tab bar, the notes
// and one for the "more on GitHub" or empty-tab sentence.
func (m model) inboxExtraRows() int {
	if m.view != viewInbox {
		return 0
	}
	st, _ := m.screenData(viewInbox).(inboxState)
	return 2 + len(st.notes)
}

func inboxBody(m model) string {
	st, _ := m.screenData(viewInbox).(inboxState)
	var b strings.Builder
	var tabs []string
	for i, name := range inboxTabNames {
		label := fmt.Sprintf(" %s (%d) ", name, st.tabs[i].total)
		if i == st.tab {
			label = ui.Sel.Render(label)
		} else {
			label = styDim.Render(label)
		}
		tabs = append(tabs, label)
	}
	b.WriteString(strings.Join(tabs, " ") + "\n")
	for _, n := range st.notes {
		b.WriteString(styErr.Render(" ⚠ "+n) + "\n")
	}
	b.WriteString(m.tableBody())
	switch tab := st.tabs[st.tab]; {
	case !st.loaded:
		b.WriteString(styDim.Render(" loading…") + "\n")
	case len(tab.items) == 0:
		b.WriteString(styDim.Render(" "+inboxEmpty[st.tab]) + "\n")
	case tab.total > len(tab.items):
		b.WriteString(styDim.Render(fmt.Sprintf(" %d more on GitHub", tab.total-len(tab.items))) + "\n")
	default:
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) switchInboxTab(delta int) model {
	st, _ := m.screenData(viewInbox).(inboxState)
	st.tabs[st.tab].cursor, st.tabs[st.tab].top = m.cursor, m.top
	st.tabs[st.tab].fresh = map[string]bool{} // leaving the tab: what was new is old now
	st.tab = (st.tab + delta + 4) % 4
	m.filter = ""
	return m.rebuildInbox(st, "")
}

func inboxKeyHandler(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st, _ := m.screenData(viewInbox).(inboxState)
	switch k.String() {
	case "tab", "l", "right":
		return m.switchInboxTab(1), nil, true
	case "shift+tab", "h", "left":
		return m.switchInboxTab(-1), nil, true
	case "R", "ctrl+r":
		if inboxRunning() {
			m.status = styDim.Render("still refreshing")
			return m, nil, true
		}
		if st.loaded && inboxNow().Sub(st.loadedAt) < inboxManual {
			m.status = styDim.Render("just refreshed")
			return m, nil, true
		}
		st.force = true
		m = m.withScreenData(viewInbox, st)
		m.loading = true
		return m, m.fetch(), true
	case "enter":
		r, ok := m.selected()
		if !ok {
			return m, nil, true
		}
		return m.openInboxItem(st, r.id)
	}
	return m, nil, false
}

// openInboxItem opens an item in the PR or issue screens, in the item's own repo and host.
func (m model) openInboxItem(st inboxState, id string) (model, tea.Cmd, bool) {
	var it *ghcli.InboxItem
	for i := range st.tabs[st.tab].items {
		if inboxKey(st.tabs[st.tab].items[i]) == id {
			it = &st.tabs[st.tab].items[i]
		}
	}
	if it == nil {
		return m, nil, true
	}
	owner, name, ok := strings.Cut(it.Repo, "/")
	if !ok || owner == "" || name == "" {
		return m, nil, true
	}
	st.tabs[st.tab].cursor, st.tabs[st.tab].top = m.cursor, m.top
	m = m.withScreenData(viewInbox, st)
	host := it.Host
	if host == "github.com" {
		host = ""
	}
	m.target = ghcli.RepoRef{Host: host, Owner: owner, Name: name}
	m.prTarget = m.target
	if it.Kind == "pr" {
		m.pr = fmt.Sprint(it.Number)
		m = m.withScreenData(viewPRDetail, prDetailState{})
		cmd := m.push(viewPRDetail)
		return m, cmd, true
	}
	m.issue = fmt.Sprint(it.Number)
	m = m.withScreenData(viewIssue, issueState{})
	cmd := m.push(viewIssue)
	return m, cmd, true
}

// rememberInboxCursor keeps the active tab's row and scroll, so coming back to the inbox lands on them.
func (m model) rememberInboxCursor() model {
	st, ok := m.screenData(viewInbox).(inboxState)
	if !ok || len(m.tbl.cols) == 0 { // not shown yet: the cursor is the reset one, not the user's
		return m
	}
	st.tabs[st.tab].cursor, st.tabs[st.tab].top = m.cursor, m.top
	return m.withScreenData(viewInbox, st)
}
