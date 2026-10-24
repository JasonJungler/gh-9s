package tui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/pfilter"
	"github.com/MurabitoB/gh-9s/internal/pins"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// Outside call and clock of the board; tests replace them. The board only ever reads.
var (
	fetchProject = ghcli.FetchProject
	boardNow     = time.Now
)

const (
	boardEvery  = time.Minute     // coming back to the board reloads it after this long
	boardManual = 5 * time.Second // and R at most this often
)

const boardNoStatus = "No status"

// boardState is the Project on the board screen: what was read and which column is shown.
type boardState struct {
	key      string // projectKey of the Project this is about
	data     ghcli.Project
	loaded   bool
	loadedAt time.Time
	tab      int
	viewName string      // the chosen view by name; its position can change
	cols     bool        // the columns layout
	col, win int         // the selected column and the first one drawn
	curs     map[int]int // the selected card of each column
	tops     map[int]int // the first card drawn in each column
	view     int         // 0 is "All items", n is the Project's view n-1
	me       string      // the logged-in user, when a view's filter asks for @me
	force    bool        // the next fetch ignores the reuse window (R)
}

// boardPayload is one answer of fetchProject.
type boardPayload struct {
	key       string
	p         ghcli.Project
	me        string
	unchanged bool // the data is recent: nothing was asked
}

func init() {
	register(viewBoard, screen{
		name:     "board",
		identity: func(m model) string { return projectKey(m.board) },
		fetch:    boardFetchCmd,
		apply:    boardApply,
		key:      boardKeyHandler,
		body:     boardBody,
		footer:   "tab/h/l switch column  [/] switch view  enter open  s status  a assign  n add  b columns  f pin/unpin Project  o browser  / filter",
	})
}

// boardSt is the board's state, or an empty one when it is about another Project.
func (m model) boardSt() boardState {
	st, _ := m.screenData(viewBoard).(boardState)
	if st.key != projectKey(m.board) {
		return boardState{key: projectKey(m.board)}
	}
	return st
}

func boardFetchCmd(m model) tea.Cmd {
	b, key := m.board, projectKey(m.board)
	st := m.boardSt()
	if st.loaded && !st.force && boardNow().Sub(st.loadedAt) < boardEvery {
		return func() tea.Msg {
			return screenMsg{view: viewBoard, id: key, data: boardPayload{key: key, unchanged: true}}
		}
	}
	return func() tea.Msg {
		p, err := fetchProject(b.Host, b.Owner, b.Number)
		if errors.Is(err, ghcli.ErrMissingScope) {
			err = errors.New(ghcli.ScopeHint(b.Host))
		}
		me := ""
		for _, v := range p.Views {
			if err == nil && me == "" && pfilter.Parse(v.Filter).NeedsMe() {
				me = boardLogin(b.Host)
			}
		}
		return screenMsg{view: viewBoard, id: key, data: boardPayload{key: key, p: p, me: me}, err: err}
	}
}

func boardApply(m model, msg screenMsg) model {
	pl := msg.data.(boardPayload)
	st := m.boardSt()
	if pl.unchanged {
		if len(m.tbl.cols) == 0 { // coming back to the screen
			return m.rebuildBoard(st, "")
		}
		return m
	}
	keep := ""
	if r, ok := m.boardSelectedRow(); ok && m.view == viewBoard {
		keep = r.id
	}
	fresh := !st.loaded
	name := st.viewName
	if name == "" {
		name = m.boardViewMemo[st.key]
	}
	st.data, st.loaded, st.loadedAt, st.force, st.me = pl.p, true, boardNow(), false, pl.me
	if fresh { // a Project opened for the first time follows the session's choice of layout
		st.cols = m.boardColsOn
	}
	st.view, st.viewName = 0, ""
	for i, v := range pl.p.Views { // the view is found again by name: views may have been added, moved or deleted
		if name != "" && v.Name == name {
			st.view, st.viewName = i+1, name
			break
		}
	}
	return m.rebuildBoard(st, keep)
}

// boardTab is one column of the board.
type boardTab struct {
	name  string
	items []int // indices into the Project's items
}

// boardTabs are All, then the Status options in the Project's order, then statuses the options do not
// list (a renamed option), then No status when some item has none.
func boardTabs(p ghcli.Project, idx []int) []boardTab {
	tabs := []boardTab{{name: "All"}}
	at := map[string]int{}
	for _, s := range p.StatusOrder {
		if _, dup := at[s]; !dup {
			at[s] = len(tabs)
			tabs = append(tabs, boardTab{name: s})
		}
	}
	none := -1
	for _, i := range idx {
		it := p.Items[i]
		tabs[0].items = append(tabs[0].items, i)
		if it.Status == "" {
			if none < 0 {
				none = len(tabs)
				tabs = append(tabs, boardTab{name: boardNoStatus})
			}
			tabs[none].items = append(tabs[none].items, i)
			continue
		}
		j, ok := at[it.Status]
		if !ok {
			j = len(tabs)
			at[it.Status] = j
			tabs = append(tabs, boardTab{name: it.Status})
		}
		tabs[j].items = append(tabs[j].items, i)
	}
	return tabs
}

func boardRowID(p ghcli.Project, i int) string {
	it := p.Items[i]
	if it.Type == "Issue" || it.Type == "PullRequest" {
		return fmt.Sprintf("%s#%d", it.Repo, it.Number)
	}
	return fmt.Sprintf("i%d", i)
}

// boardItem finds the item a row stands for.
func boardItem(p ghcli.Project, id string) (ghcli.ProjectItem, bool) {
	for i := range p.Items {
		if boardRowID(p, i) == id {
			return p.Items[i], true
		}
	}
	return ghcli.ProjectItem{}, false
}

// boardLook is the icon, the state word and the colour class of an item.
func boardLook(it ghcli.ProjectItem) (icons.Name, string, state) {
	switch {
	case it.Type == "":
		return icons.IssueOpen, "-", stOther
	case it.Type == "DraftIssue":
		return icons.PRDraft, "draft", stOther
	case it.Type == "PullRequest" && it.State == "MERGED":
		return icons.PRMerged, "merged", stOther
	case it.Type == "PullRequest" && it.Draft:
		return icons.PRDraft, "draft", stOther
	case it.Type == "PullRequest" && it.State == "CLOSED":
		return icons.PRClosed, "closed", stFail
	case it.Type == "PullRequest":
		return icons.PROpen, "open", stOK
	case it.State == "CLOSED":
		return icons.IssueClosed, "closed", stFail
	}
	return icons.IssueOpen, "open", stOK
}

func boardRows(p ghcli.Project, t boardTab) []row {
	var rows []row
	for _, i := range t.items {
		it := p.Items[i]
		ic, word, st := boardLook(it)
		ref := "-"
		upd := "-"
		switch it.Type {
		case "Issue", "PullRequest":
			ref, upd = fmt.Sprintf("%s #%d", it.Repo, it.Number), age(it.Updated)
		case "DraftIssue":
			ref = "(draft)"
		}
		join := func(l []string) string {
			if len(l) == 0 {
				return "-"
			}
			return strings.Join(l, ",")
		}
		rows = append(rows, row{id: boardRowID(p, i), url: it.URL, updated: it.Updated, state: st, tint: tintFor(0, ic),
			cells: []string{withIcon(ic, ref), it.Title, word, join(it.Assignees), join(it.Labels), upd}})
	}
	return rows
}

func (m model) rebuildBoard(st boardState, keepID string) model {
	idx, _ := st.viewItems()
	tabs := boardTabs(st.data, idx)
	st.tab = min(max(st.tab, 0), len(tabs)-1)
	m = m.withScreenData(viewBoard, st)
	m.tbl = table{cols: []string{"ITEM", "TITLE", "STATE", "ASSIGNEES", "LABELS", "UPDATED"}, rows: boardRows(st.data, tabs[st.tab])}
	prev := m.cursor
	m.cursor, m.top = 0, 0
	found := false
	for i, r := range m.visible() {
		if keepID != "" && r.id == keepID {
			m.cursor, found = i, true
		}
	}
	if keepID != "" && !found { // the card left this tab: stay near where it was
		m.cursor = max(0, min(prev, len(m.visible())-1))
	}
	m.scroll()
	if st.cols {
		m = m.colsReselect(st, keepID)
	}
	return m
}

// boardExtraRows is the tab bar above the board's table and the sentence under it.
func (m model) boardExtraRows() int {
	if m.view != viewBoard {
		return 0
	}
	st := m.boardSt()
	n := 2
	if st.hasViews() {
		n++
	}
	return n
}

func boardCount(p ghcli.Project, n int) string {
	if p.Total > len(p.Items) {
		return fmt.Sprintf("%d+", n)
	}
	return fmt.Sprint(n)
}

func boardBody(m model) string {
	st := m.boardSt()
	if st.colsActive(m) {
		return boardColumnsBody(m, st)
	}
	var b strings.Builder
	var labels []string
	if st.loaded {
		idx, _ := st.viewItems()
		for i, t := range boardTabs(st.data, idx) {
			l := fmt.Sprintf(" %s (%s) ", t.name, boardCount(st.data, len(t.items)))
			if i == st.tab {
				l = ui.Sel.Render(l)
			} else {
				l = styDim.Render(l)
			}
			labels = append(labels, l)
		}
	}
	if st.hasViews() {
		b.WriteString(fitTabs(st.viewLabels(), st.view, m.width) + "\n")
	}
	b.WriteString(fitTabs(labels, st.tab, m.width) + "\n")
	b.WriteString(m.tableBody())
	_, note := st.viewItems()
	switch {
	case note != "":
		b.WriteString(styErr.Render(" "+trunc(note, max(m.width-2, 10))) + "\n")
	case !st.loaded && m.err == nil:
		b.WriteString(styDim.Render(" loading…") + "\n")
	case st.loaded && st.data.Total > len(st.data.Items):
		b.WriteString(styDim.Render(fmt.Sprintf(" %d more in the Project (the first %d are shown)", st.data.Total-len(st.data.Items), len(st.data.Items))) + "\n")
	case st.loaded && len(m.tbl.rows) == 0:
		b.WriteString(styDim.Render(" nothing in this column") + "\n")
	default:
		b.WriteString("\n")
	}
	return b.String()
}

// boardTitle is what the header adds for the board.
func (m model) boardTitle() string {
	t := projectKey(m.board)
	if st := m.boardSt(); st.loaded && st.data.Title != "" {
		t = ghcli.CleanText(st.data.Title) + "  " + t
		if st.data.Closed {
			t += " (closed)"
		}
	}
	return t
}

func (m model) switchBoardTab(delta int) model {
	st := m.boardSt()
	if !st.loaded {
		return m
	}
	idx, _ := st.viewItems()
	n := len(boardTabs(st.data, idx))
	st.tab = (st.tab + delta + n) % n
	m.filter = ""
	return m.rebuildBoard(st, "")
}

func boardKeyHandler(m model, k tea.KeyMsg) (model, tea.Cmd, bool) {
	st := m.boardSt()
	if st.colsActive(m) {
		if next, cmd, ok := boardColsKey(m, st, k); ok {
			return next, cmd, true
		}
	}
	switch k.String() {
	case "b":
		return m.toggleBoardColumns(), nil, true
	case "s":
		next, cmd := m.startStatus()
		return next, cmd, true
	case "a":
		next, cmd := m.startAssign()
		return next, cmd, true
	case "n":
		next, cmd := m.startAdd()
		return next, cmd, true
	case "H", "L":
		if !st.colsActive(m) {
			m.status = styDim.Render("H and L move a card between columns; b shows the columns, s picks a Status")
			return m, nil, true
		}
		delta := 1
		if k.String() == "H" {
			delta = -1
		}
		next, cmd := m.moveCardSideways(delta)
		return next, cmd, true
	case "tab", "l", "right":
		return m.switchBoardTab(1), nil, true
	case "shift+tab", "h", "left":
		return m.switchBoardTab(-1), nil, true
	case "]":
		return m.switchBoardView(1), nil, true
	case "[":
		return m.switchBoardView(-1), nil, true
	case "R", "ctrl+r":
		if st.loaded && boardNow().Sub(st.loadedAt) < boardManual {
			m.status = styDim.Render("just refreshed")
			return m, nil, true
		}
		st.force = true
		m = m.withScreenData(viewBoard, st)
		m.loading = true
		return m, m.fetch(), true
	case "f":
		p := m.board
		if p.Title == "" || p.URL == "" {
			p.Title, p.URL = st.data.Title, st.data.URL
		}
		if !pins.WebURL(p.URL) {
			p.URL = ""
		}
		if p.Title == "" {
			p.Title = projectKey(p)
		}
		return m.togglePinnedProjectKeys(p)
	case "o":
		r, ok := m.boardSelectedRow()
		if !ok {
			return m, nil, true
		}
		url := r.url
		if url == "" {
			url = st.data.URL
		}
		if url != "" && !pins.WebURL(url) {
			url = ""
		}
		if url == "" {
			m.status = styDim.Render("no address known for this item")
			return m, nil, true
		}
		m = m.open(url)
		if m.status == "" {
			m.status = styDim.Render("opened " + url)
		}
		return m, nil, true
	case "enter":
		r, ok := m.boardSelectedRow()
		if !ok {
			return m, nil, true
		}
		return m.openBoardItem(st, r.id)
	}
	return m, nil, false
}

// openBoardItem opens an item in the PR or issue screens, in the item's own repo on the Project's host.
func (m model) openBoardItem(st boardState, id string) (model, tea.Cmd, bool) {
	it, ok := boardItem(st.data, id)
	if !ok {
		return m, nil, true
	}
	if it.Type != "Issue" && it.Type != "PullRequest" {
		m.status = styDim.Render("this card is only in the Project (a draft or not visible to you); o opens the Project")
		return m, nil, true
	}
	owner, name, found := strings.Cut(it.Repo, "/")
	if !found || owner == "" || name == "" {
		return m, nil, true
	}
	host := m.board.Host
	if host == "github.com" {
		host = ""
	}
	ref := ghcli.RepoRef{Host: host, Owner: owner, Name: name}
	if !validRepoName(ref) {
		m.status = styErr.Render("✗ this card names a repo gh-9s will not use")
		return m, nil, true
	}
	m.target = ref
	m.prTarget = m.target
	if it.Type == "PullRequest" {
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

// openBoard shows a pinned Project's board.
func (m model) openBoard(p pins.Project) (model, tea.Cmd) {
	m.board = p
	cmd := m.push(viewBoard)
	return m, cmd
}

func (m model) togglePinnedProjectKeys(p pins.Project) (model, tea.Cmd, bool) {
	m, cmd := m.togglePinnedProject(p)
	return m, cmd, true
}

func (st boardState) hasViews() bool { return st.loaded && len(st.data.Views) > 0 }

func (st boardState) viewLabels() []string {
	labels := []string{" All items "}
	for _, v := range st.data.Views {
		l := " " + v.Name + " "
		if v.Layout == "ROADMAP_LAYOUT" {
			l = " " + v.Name + " (roadmap) "
		}
		labels = append(labels, l)
	}
	for i := range labels {
		if i == st.view {
			labels[i] = ui.Sel.Render(labels[i])
		} else {
			labels[i] = styDim.Render(labels[i])
		}
	}
	return labels
}

// viewItems are the items of the chosen view, in its order, and a note when the view could not be applied exactly.
func (st boardState) viewItems() (idx []int, note string) {
	p := st.data
	for i := range p.Items {
		idx = append(idx, i)
	}
	if st.view == 0 || st.view > len(p.Views) {
		return idx, ""
	}
	v := p.Views[st.view-1]
	if v.Layout == "ROADMAP_LAYOUT" {
		return idx, "a roadmap view is not drawn here; showing all items"
	}
	f := pfilter.Parse(v.Filter)
	var notes []string
	if len(f.Unsupported) > 0 {
		notes = append(notes, "filter not fully applied: "+strings.Join(f.Unsupported, " "))
	}
	if f.NeedsMe() && st.me == "" {
		return idx, "this view's filter needs your login, which could not be read; showing all items"
	}
	var kept []int
	for _, i := range idx {
		if f.Match(p.Items[i], st.me) {
			kept = append(kept, i)
		}
	}
	idx = kept
	if v.SortField != "" && !pfilter.Sort(idx, p.Items, p.StatusOrder, v.SortField, v.SortDesc) {
		notes = append(notes, "sort by "+v.SortField+" not applied")
	}
	return idx, strings.Join(notes, "; ")
}

func (m model) switchBoardView(delta int) model {
	st := m.boardSt()
	if !st.hasViews() {
		return m
	}
	n := len(st.data.Views) + 1
	st.view = (st.view + delta + n) % n
	st.viewName = ""
	if st.view > 0 {
		st.viewName = st.data.Views[st.view-1].Name
	}
	memo := make(map[string]string, len(m.boardViewMemo)+1)
	for k, v := range m.boardViewMemo {
		memo[k] = v
	}
	memo[st.key] = st.viewName
	m.boardViewMemo = memo
	st.tab, st.col, st.curs, st.tops, st.win = 0, 0, nil, nil, 0
	m.filter = ""
	return m.rebuildBoard(st, "")
}

// The login is asked once per host for the session; a failed lookup is not remembered and does not
// replace a login that worked.
var (
	boardLogins   = map[string]string{}
	boardLoginsMu sync.Mutex
)

func boardLogin(host string) string {
	if host == "" {
		host = "github.com"
	}
	boardLoginsMu.Lock()
	defer boardLoginsMu.Unlock()
	if me, ok := boardLogins[host]; ok {
		return me
	}
	me, err := inboxLogin(host)
	if err != nil || me == "" {
		return ""
	}
	boardLogins[host] = me
	return me
}
