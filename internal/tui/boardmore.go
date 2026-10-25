package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// Marks, writes on several cards, reordering and undo.

const maxMarked = 50

var moveItem = ghcli.MoveProjectItem

// marked are the marked cards that are still on the board, in the Project's order.
func (st boardState) marked() []ghcli.ProjectItem {
	var out []ghcli.ProjectItem
	for _, it := range st.data.Items {
		if it.ID != "" && st.marks[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// markedHere are the marked cards the user can see now: those of the list or tab on screen, or of any column.
// Cards marked on another tab are not acted on.
func (m model) markedHere() []ghcli.ProjectItem {
	st := m.boardSt()
	shown := map[string]bool{}
	if st.colsActive(m) {
		for _, c := range m.boardColumns(st) {
			for _, i := range c.cards {
				shown[st.data.Items[i].ID] = true
			}
		}
	} else {
		for _, r := range m.visible() {
			if it, ok := boardItem(st.data, r.id); ok {
				shown[it.ID] = true
			}
		}
	}
	var out []ghcli.ProjectItem
	for _, it := range st.marked() {
		if shown[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// withMarks returns st with the mark of id switched; the map is copied, never changed in place.
func (st boardState) withMarks(id string, on bool) boardState {
	marks := make(map[string]bool, len(st.marks)+1)
	for k, v := range st.marks {
		marks[k] = v
	}
	if on {
		marks[id] = true
	} else {
		delete(marks, id)
	}
	st.marks = marks
	return st
}

// toggleMark is space: mark or unmark the selected card.
func (m model) toggleMark() model {
	st := m.boardSt()
	it, ok := m.selectedCard()
	if !ok || it.ID == "" {
		return m
	}
	on := !st.marks[it.ID]
	if on && len(st.marks) >= maxMarked {
		m.status = styErr.Render(fmt.Sprintf("at most %d cards can be marked", maxMarked))
		return m
	}
	r, _ := m.boardSelectedRow()
	m = m.rebuildBoard(st.withMarks(it.ID, on), r.id)
	m.status = styDim.Render(fmt.Sprintf("%d marked (esc clears the marks)", len(m.boardSt().marked())))
	return m
}

func (m model) clearMarks() model {
	st := m.boardSt()
	if len(st.marks) == 0 {
		return m
	}
	r, _ := m.boardSelectedRow()
	st.marks = nil
	m = m.rebuildBoard(st, r.id)
	m.status = ""
	return m
}

// namesOf says which cards a bulk question is about: the first three titles and how many more.
func namesOf(items []ghcli.ProjectItem) string {
	var t []string
	for i, it := range items {
		if i == 3 {
			t = append(t, fmt.Sprintf("and %d more", len(items)-3))
			break
		}
		t = append(t, "“"+it.Title+"”")
	}
	return strings.Join(t, ", ")
}

func cards(n int) string {
	if n == 1 {
		return "1 card"
	}
	return fmt.Sprintf("%d cards", n)
}

// bulkRun runs do on each card in turn and stops at the first failure.
func bulkRun(items []ghcli.ProjectItem, do func(it ghcli.ProjectItem) error) func() error {
	return func() error {
		for i, it := range items {
			if err := runWithin(writeTimeout, func() error { return do(it) }); err != nil {
				return fmt.Errorf("stopped after %d of %d cards: %w", i, len(items), err)
			}
		}
		return nil
	}
}

// startBulkRemoval is x / X with cards marked.
func (m model) startBulkRemoval(items []ghcli.ProjectItem, archive bool) (model, tea.Cmd) {
	op := m.newOp()
	op.bulk = items
	drafts := 0
	for _, it := range items {
		if it.Type == "DraftIssue" {
			drafts++
		}
	}
	id, q, detail := "board.remove.ok", "Remove "+cards(len(items))+" from the Project?", namesOf(items)+". Issues and pull requests are not changed."
	if drafts > 0 {
		detail += fmt.Sprintf(" %d draft(s) are gone for good.", drafts)
	}
	if archive {
		id, q, detail = "board.archive.ok", "Archive "+cards(len(items))+"?", namesOf(items)+". Archived cards are not shown here; un-archive them in the browser."
	}
	op.confirming = true
	m.boardOp = op
	d := newConfirmDialog(id, q, detail)
	m.dlg = &d
	return m, nil
}

// startBulkStatus is s with cards marked.
func (m model) startBulkStatus(items []ghcli.ProjectItem) (model, tea.Cmd) {
	st := m.boardSt()
	if len(st.data.StatusIDs) == 0 {
		m.status = styDim.Render("this Project has no Status to set")
		return m, nil
	}
	op := m.newOp()
	op.bulk = items
	m.boardOp = op
	var cs []choice
	for _, name := range st.data.StatusOrder {
		cs = append(cs, choice{label: name, value: name})
	}
	m = m.withDialog(newChoiceDialog("board.status", "Move "+cards(len(items))+" to…", cs, false, nil))
	return m, nil
}

// startBulkAssign is a with cards marked: add people to cards of one repo.
func (m model) startBulkAssign(items []ghcli.ProjectItem) (model, tea.Cmd) {
	repo := ""
	for _, it := range items {
		if (it.Type != "Issue" && it.Type != "PullRequest") || it.Repo == "" {
			m.status = styErr.Render("only issues and pull requests have assignees; unmark the others")
			return m, nil
		}
		if repo != "" && it.Repo != repo {
			m.status = styErr.Render("the marked cards are in different repos; assignees are changed one repo at a time")
			return m, nil
		}
		repo = it.Repo
	}
	owner, name, cut := strings.Cut(repo, "/")
	host := m.board.Host
	if host == "github.com" {
		host = ""
	}
	ref := ghcli.RepoRef{Host: host, Owner: owner, Name: name}
	if !cut || !validRepoName(ref) {
		m.status = styErr.Render("✗ these cards name a repo gh-9s will not use")
		return m, nil
	}
	op := m.newOp()
	op.bulk, op.target = items, ref
	m.boardOp = op
	m.status = styDim.Render("loading who can be assigned…")
	return m, func() tea.Msg {
		names, err := listAssignees(ref)
		return boardAssigneesMsg{op, names, err}
	}
}

// bulkAssignRun adds the people to every card.
func bulkAssignRun(op *boardOp) func() error {
	return bulkRun(op.bulk, func(it ghcli.ProjectItem) error {
		n := strconv.Itoa(it.Number)
		if it.Type == "PullRequest" {
			return editPRAssignees(op.target, n, op.add, nil)
		}
		return editIssue(op.target, n, nil, nil, op.add, nil)
	})
}

// cardList is the cards of the list or column the cursor is in, in the order shown, and the selected position.
func (m model) cardList() ([]ghcli.ProjectItem, int) {
	st := m.boardSt()
	var out []ghcli.ProjectItem
	if st.colsActive(m) {
		cols := m.boardColumns(st)
		st = m.colFit(st, cols)
		if st.col >= len(cols) {
			return nil, 0
		}
		for _, i := range cols[st.col].cards {
			out = append(out, st.data.Items[i])
		}
		return out, min(st.curs[st.col], len(out)-1)
	}
	for _, r := range m.visible() {
		if it, ok := boardItem(st.data, r.id); ok {
			out = append(out, it)
		}
	}
	return out, m.cursor
}

// startReorder is ctrl+up / ctrl+down: put the selected card one place up or down in its list or column.
func (m model) startReorder(delta int) (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	st := m.boardSt()
	if st.view > 0 {
		v := st.data.Views[st.view-1]
		if v.Layout == "ROADMAP_LAYOUT" || v.SortField != "" {
			m.status = styDim.Render("this view is sorted; switch to All items to change the order")
			return m, nil
		}
	}
	list, sel := m.cardList()
	to := sel + delta
	if len(list) == 0 || sel < 0 || to < 0 || to >= len(list) {
		return m, nil
	}
	it := list[sel]
	if it.ID == "" {
		return m, nil
	}
	after := "" // the card the moved one will follow; "" is the top
	switch {
	case delta < 0 && to > 0:
		after = list[to-1].ID
	case delta > 0:
		after = list[to].ID
	}
	if delta > 0 && after == "" || (delta < 0 && to > 0 && after == "") {
		return m, nil
	}
	op := m.newOp()
	op.itemID, op.title, op.afterID = it.ID, it.Title, after
	where := "up"
	if delta > 0 {
		where = "down"
	}
	op.optionName = where
	op.confirming = true
	m.boardOp = op
	d := newConfirmDialog("board.move.ok", "Move “"+it.Title+"” "+where+" one place?", ghcli.CleanText(st.data.Title))
	m.dlg = &d
	return m, nil
}

func init() {
	dialogHandlers["board.move.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok {
			return m, nil
		}
		return m.startWrite("moved "+op.title+" "+op.optionName, func() error { return moveItem(op.host, op.projectID, op.itemID, op.afterID) })
	}
}

// undoRec is what it takes to take the last change back.
type undoRec struct {
	key, host, projectID, title string
	what                        string // the question: "move “x” back to Todo"
	kind                        string // status, field, created
	itemID                      string // the card changed; for created, filled when the write is done
	fieldID, fieldType          string
	prev                        *ghcli.FieldValue // the value before (nil: it had none)
	wasName, nowName            string            // the value before and after as shown, to see whether it changed since
	statusFld                   string
}

// statusUndo is the record for moving one card to Status now.
func (m model) statusUndo(op *boardOp, it ghcli.ProjectItem, now string) *undoRec {
	st := m.boardSt()
	r := &undoRec{key: op.key, host: op.host, projectID: op.projectID, title: it.Title, kind: "status", itemID: it.ID, statusFld: op.statusFld, wasName: it.Status, nowName: now}
	if it.Status == "" {
		r.what = "clear the Status of “" + it.Title + "”"
		return r
	}
	id, ok := st.data.StatusIDs[it.Status]
	if !ok {
		return nil
	}
	r.prev = &ghcli.FieldValue{Option: id}
	r.what = "move “" + it.Title + "” back to " + it.Status
	return r
}

// fieldUndo is the record for setting a field of one card to now (empty: cleared).
func (m model) fieldUndo(op *boardOp, it ghcli.ProjectItem, f ghcli.ProjectField, now string) *undoRec {
	was := it.Values[f.ID]
	r := &undoRec{key: op.key, host: op.host, projectID: op.projectID, title: it.Title, kind: "field", itemID: it.ID, fieldID: f.ID, fieldType: f.Type, wasName: was, nowName: now}
	if was == "" {
		r.what = "clear " + f.Name + " of “" + it.Title + "”"
		return r
	}
	switch f.Type {
	case "SINGLE_SELECT", "ITERATION":
		for _, o := range f.Options {
			if o.Name == was {
				r.prev = &ghcli.FieldValue{Option: o.ID}
			}
		}
		if r.prev == nil {
			return nil
		}
	case "NUMBER":
		n, err := strconv.ParseFloat(was, 64)
		if err != nil {
			return nil
		}
		r.prev = &ghcli.FieldValue{Number: &n}
	default:
		r.prev = &ghcli.FieldValue{Text: was, Date: was}
	}
	r.what = "set " + f.Name + " of “" + it.Title + "” back to " + was
	return r
}

// startUndo is u.
func (m model) startUndo() (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	if m.loading {
		m.status = styDim.Render("still refreshing the board; try again in a moment")
		return m, nil
	}
	r := m.undo
	if r == nil || r.key != projectKey(m.board) {
		if m.undoWhy != "" && m.undoKey == projectKey(m.board) {
			m.status = styDim.Render(m.undoWhy)
		} else {
			m.status = styDim.Render("nothing to undo here (only a Status or field change, an added card or a new draft can be)")
		}
		return m, nil
	}
	if r.kind != "created" {
		cur, ok := boardItem2(m.boardSt().data, r.itemID)
		switch {
		case !ok:
			m.status = styErr.Render("that card is not on the board any more; not undone")
			return m, nil
		case r.kind == "status" && cur.Status != r.nowName, r.kind == "field" && cur.Values[r.fieldID] != r.nowName:
			m.status = styErr.Render("changed since; not undone")
			return m, nil
		}
	} else if r.itemID == "" {
		m.status = styDim.Render("nothing to undo here")
		return m, nil
	}
	op := m.newOp()
	op.undoRec = r
	op.confirming = true
	m.boardOp = op
	what := r.what
	d := newConfirmDialog("board.undo.ok", "Undo: "+what+"?", ghcli.CleanText(m.boardSt().data.Title))
	m.dlg = &d
	return m, nil
}

func boardItem2(p ghcli.Project, itemID string) (ghcli.ProjectItem, bool) {
	for _, it := range p.Items {
		if it.ID == itemID {
			return it, true
		}
	}
	return ghcli.ProjectItem{}, false
}

func init() {
	dialogHandlers["board.undo.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok || op.undoRec == nil {
			return m, nil
		}
		u := op.undoRec
		return m.startWrite("undone: "+u.what, func() error { // the record is dropped when it worked (onBoardWriteDone)
			switch u.kind {
			case "status":
				if u.prev == nil {
					return setField(u.host, u.projectID, u.itemID, u.statusFld, "SINGLE_SELECT", nil)
				}
				return setProjectStatus(u.host, u.projectID, u.itemID, u.statusFld, u.prev.Option)
			case "field":
				return setField(u.host, u.projectID, u.itemID, u.fieldID, u.fieldType, u.prev)
			case "created":
				return removeItem(u.host, u.projectID, u.itemID)
			}
			return fmt.Errorf("nothing to undo")
		})
	}
}
