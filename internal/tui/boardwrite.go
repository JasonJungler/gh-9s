package tui

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// The writes of the board: change a card's Status, its assignees, add an item. Each asks first, with the
// change in words, and takes what it needs from the screen at key-press time.
var (
	setProjectStatus = ghcli.SetProjectStatus
	resolveContent   = ghcli.ResolveContent
	addProjectItem   = ghcli.AddProjectItem
	editPRAssignees  = ghcli.EditPRAssignees
)

// boardOp is a write being collected through dialogs.
type boardOp struct {
	key        string // projectKey of the board it was started on
	host       string
	projectID  string
	statusFld  string
	itemID     string
	title      string // the card, for the question
	optionName string
	optionID   string
	content    ghcli.Content
	target     ghcli.RepoRef
	number     string
	isPR       bool
	current    []string
	add, drop  []string
	inColumn   string // add: the Status of the column the cursor was in
	confirming bool   // its confirm question is on screen or its answer is on the way
}

// boardWriteDoneMsg is the outcome of a write.
type boardWriteDoneMsg struct {
	key  string
	text string
	err  error
}

// boardAssigneesMsg carries the names that can be assigned in a card's repo.
type boardAssigneesMsg struct {
	op    *boardOp
	names []string
	err   error
}

// boardResolvedMsg is the issue or PR to add, found.
type boardResolvedMsg struct {
	op  *boardOp
	err error
}

func init() {
	dialogHandlers["board.status"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || len(r.values) != 1 || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		return m.askStatus(op, r.values[0])
	}
	dialogHandlers["board.status.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		m.boardOp = nil
		if !r.ok || op == nil || !op.confirming || !m.opAlive(op) {
			return m, nil
		}
		return m.startWrite("moved "+op.title+" to "+op.optionName, func() error {
			return setProjectStatus(op.host, op.projectID, op.itemID, op.statusFld, op.optionID)
		})
	}
	dialogHandlers["board.assignees"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		op.add, op.drop = diffNames(op.current, r.values)
		if len(op.add)+len(op.drop) == 0 {
			m.boardOp = nil
			m.status = styDim.Render("no assignee changes")
			return m, nil
		}
		var parts []string
		if len(op.add) > 0 {
			parts = append(parts, "assign "+strings.Join(op.add, ", "))
		}
		if len(op.drop) > 0 {
			parts = append(parts, "unassign "+strings.Join(op.drop, ", "))
		}
		op.confirming = true
		d := newConfirmDialog("board.assignees.ok", strings.ToUpper(parts[0][:1])+strings.Join(parts, " and ")[1:]+" on "+displayRef(op.target)+"#"+op.number+"?", op.title)
		m.dlg = &d
		return m, nil
	}
	dialogHandlers["board.assignees.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		m.boardOp = nil
		if !r.ok || op == nil || !op.confirming || !m.opAlive(op) {
			return m, nil
		}
		return m.startWrite("updated assignees on "+displayRef(op.target)+"#"+op.number, func() error {
			if op.isPR {
				return editPRAssignees(op.target, op.number, op.add, op.drop)
			}
			return editIssue(op.target, op.number, nil, nil, op.add, op.drop)
		})
	}
	dialogHandlers["board.add"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		owner, name, n, err := parseContentRef(r.text, op.host)
		if err != nil {
			m.boardOp = nil
			m.status = styErr.Render("✗ " + ghcli.CleanText(err.Error()))
			return m, nil
		}
		host := op.host
		m.status = styDim.Render("looking up " + owner + "/" + name + "#" + strconv.Itoa(n) + "…")
		return m, func() tea.Msg {
			c, err := resolveContent(host, owner, name, n)
			op.content = c
			op.target = ghcli.RepoRef{Host: host, Owner: owner, Name: name}
			op.number = strconv.Itoa(n)
			return boardResolvedMsg{op, err}
		}
	}
	dialogHandlers["board.add.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		m.boardOp = nil
		if !r.ok || op == nil || !op.confirming || !m.opAlive(op) {
			return m, nil
		}
		return m.startWrite("added "+op.content.Title, func() error {
			id, err := addProjectItem(op.host, op.projectID, op.content.ID)
			if err != nil {
				return err
			}
			if op.optionID != "" {
				if err := setProjectStatus(op.host, op.projectID, id, op.statusFld, op.optionID); err != nil {
					return fmt.Errorf("added, but its Status could not be set: %w", err)
				}
			}
			return nil
		})
	}
}

// opAlive says whether the write still belongs to the board on screen.
func (m model) opAlive(op *boardOp) bool { return m.view == viewBoard && op.key == projectKey(m.board) }

// startWrite runs a write unless one is already running.
func (m model) startWrite(done string, fn func() error) (model, tea.Cmd) {
	if m.boardBusy {
		m.status = styDim.Render("still working on the last change")
		return m, nil
	}
	m.boardBusy = true
	m.status = styDim.Render("working…")
	key := projectKey(m.board)
	return m, func() tea.Msg {
		err := runWithin(writeTimeout, fn)
		if errors.Is(err, ghcli.ErrNeedsProjectScope) {
			err = errors.New(ghcli.WriteScopeHint(hostOfKey(key)))
		}
		return boardWriteDoneMsg{key: key, text: done, err: err}
	}
}

// writeTimeout is how long a write may take before the screen stops waiting for it.
var writeTimeout = 90 * time.Second

// runWithin runs fn and gives up waiting after d; a write that gives up may still have happened.
func runWithin(d time.Duration, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return errors.New("no answer from gh; the change may or may not have happened")
	}
}

func hostOfKey(key string) string {
	if parts := strings.Split(key, "/"); len(parts) == 3 {
		return parts[0]
	}
	return ""
}

func (m model) onBoardWriteDone(msg boardWriteDoneMsg) (model, tea.Cmd) {
	m.boardBusy = false
	if msg.err != nil {
		m.status = styErr.Render("✗ " + ghcli.CleanText(msg.err.Error()) + " (R shows what the Project looks like now)")
	} else {
		m.status = styOK.Render("✓ " + msg.text)
	}
	if m.view == viewBoard && msg.key == projectKey(m.board) {
		st := m.boardSt()
		st.force = true
		m = m.withScreenData(viewBoard, st)
		m.loading = true
		return m, m.fetch()
	}
	return m, nil
}

// newOp captures what a write needs from the screen now.
func (m model) newOp() *boardOp {
	st := m.boardSt()
	return &boardOp{key: projectKey(m.board), host: m.board.Host, projectID: st.data.ID, statusFld: st.data.StatusField}
}

func (m model) selectedCard() (ghcli.ProjectItem, bool) {
	r, ok := m.boardSelectedRow()
	if !ok {
		return ghcli.ProjectItem{}, false
	}
	return boardItem(m.boardSt().data, r.id)
}

// startStatus is s: pick the Status to move the selected card to.
func (m model) startStatus() (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	st := m.boardSt()
	it, ok := m.selectedCard()
	if !ok || len(st.data.StatusIDs) == 0 {
		m.status = styDim.Render("nothing to move here")
		return m, nil
	}
	if it.ID == "" {
		m.status = styErr.Render("this card has no id yet; press R and try again")
		return m, nil
	}
	op := m.newOp()
	op.itemID, op.title = it.ID, it.Title
	m.boardOp = op
	var choices []choice
	for _, name := range st.data.StatusOrder {
		c := choice{label: name, value: name}
		if name == it.Status {
			c.disabled, c.note = true, "current"
		}
		choices = append(choices, c)
	}
	d := newChoiceDialog("board.status", "Move “"+it.Title+"” to…", choices, false, nil)
	m.dlg = &d
	return m, nil
}

// askStatus asks to confirm moving the card to Status name.
func (m model) askStatus(op *boardOp, name string) (model, tea.Cmd) {
	st := m.boardSt()
	id, ok := st.data.StatusIDs[name]
	if !ok {
		m.boardOp = nil
		m.status = styErr.Render("✗ Status " + name + " is not one of the Project's options any more; press R")
		return m, nil
	}
	op.optionName, op.optionID = name, id
	op.confirming = true
	m.boardOp = op
	d := newConfirmDialog("board.status.ok", "Move “"+op.title+"” to "+name+"?", ghcli.CleanText(st.data.Title))
	m.dlg = &d
	return m, nil
}

// moveCardSideways is H and L in the columns: move the selected card one column left or right.
func (m model) moveCardSideways(delta int) (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	st := m.boardSt()
	it, ok := m.selectedCard()
	cols := m.boardColumns(st)
	if !ok || st.col+delta < 0 || st.col+delta >= len(cols) {
		return m, nil
	}
	name := cols[st.col+delta].name
	if _, real := st.data.StatusIDs[name]; !real {
		m.status = styDim.Render("a card cannot be moved to “" + name + "” from here")
		return m, nil
	}
	if it.ID == "" {
		m.status = styErr.Render("this card has no id yet; press R and try again")
		return m, nil
	}
	op := m.newOp()
	op.itemID, op.title = it.ID, it.Title
	return m.askStatus(op, name)
}

// startAssign is a: list who can be assigned in the card's repo.
func (m model) startAssign() (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	it, ok := m.selectedCard()
	if !ok || (it.Type != "Issue" && it.Type != "PullRequest") {
		m.status = styDim.Render("only issues and pull requests have assignees here")
		return m, nil
	}
	owner, name, cut := strings.Cut(it.Repo, "/")
	host := m.board.Host
	if host == "github.com" {
		host = ""
	}
	ref := ghcli.RepoRef{Host: host, Owner: owner, Name: name}
	if !cut || !validRepoName(ref) {
		m.status = styErr.Render("✗ this card names a repo gh-9s will not use")
		return m, nil
	}
	op := m.newOp()
	op.title, op.target, op.number, op.isPR = it.Title, ref, strconv.Itoa(it.Number), it.Type == "PullRequest"
	op.current = append([]string(nil), it.Assignees...)
	m.boardOp = op
	m.status = styDim.Render("loading who can be assigned…")
	return m, func() tea.Msg {
		names, err := listAssignees(ref)
		return boardAssigneesMsg{op, names, err}
	}
}

func (m model) onBoardAssignees(msg boardAssigneesMsg) (model, tea.Cmd) {
	if m.boardOp != msg.op || !m.opAlive(msg.op) || m.dlg != nil {
		return m, nil
	}
	if msg.err != nil {
		m.boardOp = nil
		m.status = styErr.Render("✗ " + ghcli.CleanText(msg.err.Error()))
		return m, nil
	}
	seen := map[string]bool{}
	var names []string
	for _, n := range append(append([]string(nil), msg.names...), msg.op.current...) {
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
	m.status = ""
	return m.withDialog(newChoiceDialog("board.assignees", "Assignees for "+displayRef(msg.op.target)+"#"+msg.op.number, choices, true, msg.op.current)), nil
}

// startAdd is n: ask for the issue or PR to add.
func (m model) startAdd() (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	st := m.boardSt()
	if st.data.ID == "" {
		m.status = styErr.Render("this Project was read without its id; press R and try again")
		return m, nil
	}
	op := m.newOp()
	if st.colsActive(m) {
		cols := m.boardColumns(st)
		if st.col < len(cols) {
			if id, ok := st.data.StatusIDs[cols[st.col].name]; ok {
				op.inColumn, op.optionName, op.optionID = cols[st.col].name, cols[st.col].name, id
			}
		}
	} else if tabs := boardTabsFor(st); st.tab > 0 && st.tab < len(tabs) {
		if id, ok := st.data.StatusIDs[tabs[st.tab].name]; ok {
			op.inColumn, op.optionName, op.optionID = tabs[st.tab].name, tabs[st.tab].name, id
		}
	}
	m.boardOp = op
	m.dlg = &dialog{id: "board.add", title: "Add to " + ghcli.CleanText(st.data.Title), prompt: "OWNER/REPO#N or the address of an issue or PR", kind: dialogText}
	return m, nil
}

func (m model) onBoardResolved(msg boardResolvedMsg) (model, tea.Cmd) {
	if m.boardOp != msg.op || !m.opAlive(msg.op) || m.dlg != nil {
		return m, nil
	}
	if msg.err != nil {
		m.boardOp = nil
		m.status = styErr.Render("✗ " + ghcli.CleanText(msg.err.Error()))
		return m, nil
	}
	op := msg.op
	if slices.Contains(op.content.OnProjects, op.projectID) {
		m.boardOp = nil
		m.status = styErr.Render("✗ " + ghcli.CleanText(op.content.Title) + " is already on this Project; find it and press s to change its Status")
		return m, nil
	}
	op.confirming = true
	q := "Add “" + op.content.Title + "” (" + displayRef(op.target) + "#" + op.number + ") to the Project"
	if op.inColumn != "" {
		q += ", Status " + op.inColumn
	}
	d := newConfirmDialog("board.add.ok", q+"?", ghcli.CleanText(m.boardSt().data.Title))
	m.dlg = &d
	m.status = ""
	return m, nil
}

var (
	contentShort = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)/([A-Za-z0-9][A-Za-z0-9._-]*)#([0-9]{1,9})$`)
	contentPath  = regexp.MustCompile(`^/([A-Za-z0-9][A-Za-z0-9._-]*)/([A-Za-z0-9][A-Za-z0-9._-]*)/(?:issues|pull)/([0-9]{1,9})/?$`)
)

// parseContentRef reads OWNER/REPO#N or a github address of an issue or PR on the Project's host.
func parseContentRef(s, host string) (owner, name string, n int, err error) {
	s = strings.TrimSpace(s)
	if m := contentShort.FindStringSubmatch(s); m != nil {
		n, _ = strconv.Atoi(m[3])
		return m[1], m[2], n, nil
	}
	u, perr := url.Parse(s)
	want := host
	if want == "" {
		want = "github.com"
	}
	if perr == nil && u.Scheme == "https" && strings.EqualFold(u.Host, want) {
		if m := contentPath.FindStringSubmatch(u.Path); m != nil {
			n, _ = strconv.Atoi(m[3])
			return m[1], m[2], n, nil
		}
	}
	return "", "", 0, fmt.Errorf("want OWNER/REPO#N or an issue or PR address on %s", want)
}
