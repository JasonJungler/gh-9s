package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// More writes of the board: remove, archive, drafts and the other fields. They share boardOp, the in-flight
// guard and the confirm-then-write shape of boardwrite.go.
var (
	removeItem  = ghcli.RemoveProjectItem
	archiveItem = ghcli.ArchiveProjectItem
	addDraft    = ghcli.AddProjectDraft
	editDraft   = ghcli.EditProjectDraft
	setField    = ghcli.SetProjectField
)

func init() {
	dialogHandlers["board.remove.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok {
			return m, nil
		}
		if len(op.bulk) > 0 {
			return m.startWriteN(len(op.bulk)+1, "removed "+cards(len(op.bulk)), bulkRun(op.bulk, func(it ghcli.ProjectItem) error { return removeItem(op.host, op.projectID, it.ID) }))
		}
		return m.startWrite("removed "+op.title, func() error { return removeItem(op.host, op.projectID, op.itemID) })
	}
	dialogHandlers["board.archive.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok {
			return m, nil
		}
		if len(op.bulk) > 0 {
			return m.startWriteN(len(op.bulk)+1, "archived "+cards(len(op.bulk)), bulkRun(op.bulk, func(it ghcli.ProjectItem) error { return archiveItem(op.host, op.projectID, it.ID) }))
		}
		return m.startWrite("archived "+op.title, func() error { return archiveItem(op.host, op.projectID, op.itemID) })
	}
	dialogHandlers["board.draft"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		op.title = strings.TrimSpace(r.text)
		op.confirming = true
		q := "Create the draft “" + op.title + "”"
		if op.inColumn != "" {
			q += " with Status " + op.inColumn
		}
		d := newConfirmDialog("board.draft.ok", q+"?", ghcli.CleanText(m.boardSt().data.Title))
		m.dlg = &d
		return m, nil
	}
	dialogHandlers["board.draft.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok {
			return m, nil
		}
		rec := &undoRec{key: op.key, host: op.host, projectID: op.projectID, title: op.title, kind: "created", what: "remove the draft “" + op.title + "” again"}
		m.undoNext = rec
		return m.startWrite("created the draft "+op.title, func() error {
			id, err := addDraft(op.host, op.projectID, op.title)
			if err != nil {
				return err
			}
			rec.itemID = id
			if op.optionID != "" {
				if err := setProjectStatus(op.host, op.projectID, id, op.statusFld, op.optionID); err != nil {
					return fmt.Errorf("created, but its Status could not be set: %w", err)
				}
			}
			return nil
		})
	}
	dialogHandlers["board.edit.title"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		op.newTitle = strings.TrimSpace(r.text)
		d := newTextDialog("board.edit.body", "Text of the draft “"+op.title+"”", "the text (ctrl+e opens your editor; - makes it empty)")
		d.text = op.oldBody
		m.dlg = &d
		return m, nil
	}
	dialogHandlers["board.edit.body"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		op.newBody = r.text
		if strings.TrimSpace(op.newBody) == "-" {
			op.newBody = ""
		}
		if op.newTitle == op.title && op.newBody == op.oldBody {
			m.boardOp = nil
			m.status = styDim.Render("no changes to the draft")
			return m, nil
		}
		op.confirming = true
		d := newConfirmDialog("board.edit.ok", "Save the changes to the draft “"+op.title+"”?", ghcli.CleanText(m.boardSt().data.Title))
		m.dlg = &d
		return m, nil
	}
	dialogHandlers["board.edit.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok {
			return m, nil
		}
		return m.startWrite("saved the draft "+op.newTitle, func() error { return editDraft(op.host, op.draftID, op.newTitle, op.newBody) })
	}
	dialogHandlers["board.field"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || len(r.values) != 1 || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		for i := range m.boardSt().data.Fields {
			if f := m.boardSt().data.Fields[i]; f.ID == r.values[0] {
				return m.askFieldValue(op, f)
			}
		}
		m.boardOp = nil
		m.status = styErr.Render("✗ that field is gone; press R")
		return m, nil
	}
	dialogHandlers["board.field.val"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.boardOp
		if !r.ok || op == nil || op.field == nil || !m.opAlive(op) {
			m.boardOp = nil
			return m, nil
		}
		f := op.field
		var v *ghcli.FieldValue
		shown := ""
		switch f.Type {
		case "SINGLE_SELECT", "ITERATION":
			if len(r.values) != 1 {
				m.boardOp = nil
				return m, nil
			}
			if r.values[0] != clearValue {
				name := ""
				for _, o := range f.Options {
					if o.ID == r.values[0] {
						name = o.Name
					}
				}
				if name == "" {
					m.boardOp = nil
					m.status = styErr.Render("✗ that option is gone; press R")
					return m, nil
				}
				v, shown = &ghcli.FieldValue{Option: r.values[0]}, name
			}
		default:
			t := strings.TrimSpace(r.text)
			if t != "-" {
				fv := ghcli.FieldValue{Text: t, Date: t}
				if f.Type == "NUMBER" {
					n, err := strconv.ParseFloat(t, 64)
					if err != nil {
						m.boardOp = nil
						m.status = styErr.Render("✗ " + ghcli.CleanText(t) + " is not a number")
						return m, nil
					}
					fv.Number = &n
				}
				v, shown = &fv, t
			}
		}
		op.value, op.shown = v, shown
		op.confirming = true
		q := "Clear " + f.Name + " of “" + op.title + "”?"
		if v != nil {
			q = "Set " + f.Name + " of “" + op.title + "” to " + shown + "?"
		}
		d := newConfirmDialog("board.field.ok", q, ghcli.CleanText(m.boardSt().data.Title))
		m.dlg = &d
		return m, nil
	}
	dialogHandlers["board.field.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op, ok := m.takeConfirm(r)
		if !ok || op.field == nil {
			return m, nil
		}
		f := *op.field
		if op.card != nil {
			m.undoNext = m.fieldUndo(op, *op.card, f, op.shown)
		}
		return m.startWrite("updated "+f.Name+" of "+op.title, func() error {
			return setField(op.host, op.projectID, op.itemID, f.ID, f.Type, op.value)
		})
	}
}

// clearValue is the choice that empties a single-select or iteration field.
const clearValue = "\x00clear"

// takeConfirm is the common start of a confirm answer: the op it was about, if it is still wanted.
func (m *model) takeConfirm(r dialogResultMsg) (*boardOp, bool) {
	op := m.boardOp
	m.boardOp = nil
	if !r.ok || op == nil || !op.confirming || !m.opAlive(op) {
		return nil, false
	}
	return op, true
}

// cardOp starts an op on the selected card, refusing while another question is open.
func (m model) cardOp() (*boardOp, ghcli.ProjectItem, bool) {
	it, ok := m.selectedCard()
	if !ok {
		return nil, it, false
	}
	if it.ID == "" {
		return nil, it, false
	}
	op := m.newOp()
	op.itemID, op.title, op.card = it.ID, it.Title, &it
	return op, it, true
}

// startRemove is x.
func (m model) startRemove() (model, tea.Cmd) { return m.startRemoval(false) }

// startArchive is X.
func (m model) startArchive() (model, tea.Cmd) { return m.startRemoval(true) }

func (m model) startRemoval(archive bool) (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	if marked := m.markedHere(); len(marked) > 0 {
		return m.startBulkRemoval(marked, archive)
	}
	op, it, ok := m.cardOp()
	if !ok {
		m.status = styDim.Render("nothing to " + map[bool]string{true: "archive", false: "remove"}[archive] + " here")
		return m, nil
	}
	id, q, detail := "board.remove.ok", "Remove “"+it.Title+"” from the Project?", "The "+strings.ToLower(it.Type)+" itself is not changed."
	switch {
	case archive:
		id, q, detail = "board.archive.ok", "Archive “"+it.Title+"”?", "Archived cards are not shown here; un-archive it in the browser."
	case it.Draft && it.Type == "DraftIssue":
		detail = "This draft exists only in the Project and cannot be restored."
	case it.Type != "Issue" && it.Type != "PullRequest":
		detail = "Only the card is removed."
	case it.Type == "PullRequest":
		detail = "The pull request itself is not changed."
	default:
		detail = "The issue itself is not changed."
	}
	op.confirming = true
	m.boardOp = op
	d := newConfirmDialog(id, q, detail)
	m.dlg = &d
	return m, nil
}

// startBoardDraft is N: ask for the title of a new draft card.
func (m model) startBoardDraft() (model, tea.Cmd) {
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
	op.inColumn, op.optionName, op.optionID = m.currentColumnStatus(st)
	m.boardOp = op
	d := newTextDialog("board.draft", "New draft in "+ghcli.CleanText(st.data.Title), "title of the draft card")
	m.dlg = &d
	return m, nil
}

// currentColumnStatus is the real Status of the column or tab the cursor is in, with its option id.
func (m model) currentColumnStatus(st boardState) (name, optName, optID string) {
	var n string
	if st.colsActive(m) {
		if cols := m.boardColumns(st); st.col < len(cols) {
			n = cols[st.col].name
		}
	} else if tabs := boardTabsFor(st); st.tab > 0 && st.tab < len(tabs) {
		n = tabs[st.tab].name
	}
	if id, ok := st.data.StatusIDs[n]; ok {
		return n, n, id
	}
	return "", "", ""
}

// startEditDraft is e: change a draft card's title and text.
func (m model) startEditDraft() (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	op, it, ok := m.cardOp()
	if !ok || it.Type != "DraftIssue" || it.DraftID == "" {
		m.status = styDim.Render("e edits a draft card; this one is not a draft")
		return m, nil
	}
	op.draftID, op.oldBody = it.DraftID, it.Body
	m.boardOp = op
	d := newTextDialog("board.edit.title", "Edit the draft “"+it.Title+"”", "title")
	d.text = it.Title
	m.dlg = &d
	return m, nil
}

// startField is v: pick a field of the Project and give the selected card a value for it.
func (m model) startField() (model, tea.Cmd) {
	if m.boardOp != nil && m.boardOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	op, _, ok := m.cardOp()
	if !ok {
		m.status = styDim.Render("no card selected")
		return m, nil
	}
	fields := m.boardSt().data.Fields
	if len(fields) == 0 {
		m.status = styDim.Render("this Project has no other fields that can be changed here")
		return m, nil
	}
	m.boardOp = op
	var cs []choice
	for _, f := range fields {
		cs = append(cs, choice{label: f.Name + "  (" + strings.ToLower(strings.ReplaceAll(f.Type, "_", " ")) + ")", value: f.ID})
	}
	m = m.withDialog(newChoiceDialog("board.field", "Change a field of “"+op.title+"”", cs, false, nil))
	return m, nil
}

func (m model) askFieldValue(op *boardOp, f ghcli.ProjectField) (model, tea.Cmd) {
	op.field = &f
	m.boardOp = op
	title := "Set " + f.Name
	switch f.Type {
	case "SINGLE_SELECT", "ITERATION":
		cs := []choice{{label: "(clear)", value: clearValue}}
		for _, o := range f.Options {
			cs = append(cs, choice{label: o.Name, value: o.ID})
		}
		return m.withDialog(newChoiceDialog("board.field.val", title, cs, false, nil)), nil
	}
	prompt := map[string]string{"TEXT": "text", "NUMBER": "a number", "DATE": "a date, YYYY-MM-DD"}[f.Type] + " (- clears it)"
	d := newTextDialog("board.field.val", title, prompt)
	if it, ok := m.selectedCard(); ok {
		d.text = it.Values[f.ID]
	}
	m.dlg = &d
	return m, nil
}
