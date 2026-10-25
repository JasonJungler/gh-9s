package tui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// P-less entry to the Projects of an issue or PR: M on the issue and PR screens and lists adds the item to
// Projects and takes it out of others, after one question that names each change.
var removeFromProject = ghcli.RemoveProjectItem

// projOp is a Projects change being collected through dialogs, from the item it was started on.
type projOp struct {
	host, label string
	repo        ghcli.RepoRef
	content     ghcli.Content
	names       map[string]string // Project id to its name
	add         []string          // Project ids
	remove      []ghcli.Membership
	confirming  bool
}

type projLoadedMsg struct {
	op    *projOp
	cands []ghcli.ProjectRef
	err   error
}

type projDoneMsg struct {
	op   *projOp
	text string
	err  error
}

func init() {
	dialogHandlers["proj.assign"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.projOp
		if !r.ok || op == nil {
			m.projOp = nil
			return m, nil
		}
		want := map[string]bool{}
		for _, v := range r.values {
			want[v] = true
		}
		had := map[string]bool{}
		for _, ms := range op.content.Memberships {
			had[ms.ProjectID] = true
			if !want[ms.ProjectID] {
				op.remove = append(op.remove, ms)
			}
		}
		for _, v := range r.values {
			if !had[v] {
				op.add = append(op.add, v)
			}
		}
		if len(op.add)+len(op.remove) == 0 {
			m.projOp = nil
			m.status = styDim.Render("no Project changes")
			return m, nil
		}
		var parts []string
		if len(op.add) > 0 {
			var n []string
			for _, id := range op.add {
				n = append(n, op.names[id])
			}
			parts = append(parts, "Add to "+strings.Join(n, ", "))
		}
		if len(op.remove) > 0 {
			var n []string
			for _, ms := range op.remove {
				n = append(n, ms.Title)
			}
			parts = append(parts, "remove from "+strings.Join(n, ", "))
		}
		q := strings.Join(parts, " and ")
		q = strings.ToUpper(q[:1]) + q[1:] + " — " + op.label + "?"
		op.confirming = true
		d := newConfirmDialog("proj.assign.ok", q, op.content.Title)
		m.dlg = &d
		return m, nil
	}
	dialogHandlers["proj.assign.ok"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		op := m.projOp
		m.projOp = nil
		if !r.ok || op == nil || !op.confirming {
			return m, nil
		}
		if m.projBusy {
			m.status = styDim.Render("still working on the last change")
			return m, nil
		}
		m.projBusy = true
		m.status = styDim.Render("working…")
		return m, func() tea.Msg {
			// each call has its own timeout, so the run stops at the first one that does not answer
			done := 0
			total := len(op.add) + len(op.remove)
			err := runWithin(writeTimeout*time.Duration(total+2), func() error {
				for _, id := range op.add {
					if err := runWithin(writeTimeout, func() error {
						_, err := addProjectItem(op.host, id, op.content.ID)
						return err
					}); err != nil {
						return fmt.Errorf("adding to %s: %w", op.names[id], err)
					}
					done++
				}
				for _, ms := range op.remove {
					if err := runWithin(writeTimeout, func() error { return removeFromProject(op.host, ms.ProjectID, ms.ItemID) }); err != nil {
						return fmt.Errorf("removing from %s: %w", ms.Title, err)
					}
					done++
				}
				return nil
			})
			if err != nil && done > 0 {
				err = fmt.Errorf("%d of %d changes made, then: %w", done, total, err)
			}
			text := "updated the Projects of " + op.label
			if err == nil && len(op.add) > 0 {
				text += "; set the Status of a new card from its board"
			}
			if errors.Is(err, ghcli.ErrNeedsProjectScope) {
				hint := ghcli.WriteScopeHint(op.host)
				if done > 0 {
					hint = fmt.Sprintf("%d of %d changes made, then: %s", done, total, hint)
				}
				err = errors.New(hint)
			}
			return projDoneMsg{op, text, err}
		}
	}
}

// startProjectAssign is M: load the item's Projects and the ones it could be in, then ask.
func (m model) startProjectAssign(number string) (model, tea.Cmd) {
	if m.projOp != nil && m.projOp.confirming {
		m.status = styDim.Render("answer the question first")
		return m, nil
	}
	if m.projBusy {
		m.status = styDim.Render("still working on the last change")
		return m, nil
	}
	n, err := strconv.Atoi(number)
	repo := m.curTarget()
	if err != nil || n <= 0 || !validRepoName(repo) {
		m.status = styDim.Render("no issue or pull request selected")
		return m, nil
	}
	host := hostOrDefault(repo.Host) // one named host for every call, so the ids all come from the same server
	op := &projOp{host: host, repo: repo, label: displayRef(repo) + "#" + number}
	m.projOp = op
	m.status = styDim.Render("loading Projects…")
	return m, func() tea.Msg {
		c, cerr := resolveContent(host, repo.Owner, repo.Name, n)
		if cerr != nil {
			return projLoadedMsg{op: op, err: cerr}
		}
		op.content = c
		cands, lerr := listProjects(host)
		return projLoadedMsg{op: op, cands: cands, err: lerr}
	}
}

func hostOrDefault(h string) string {
	if h == "" {
		return "github.com"
	}
	return h
}

func (m model) onProjLoaded(msg projLoadedMsg) (model, tea.Cmd) {
	op := msg.op
	if m.projOp != op || m.dlg != nil {
		if m.projOp == op {
			m.projOp = nil
		}
		return m, nil
	}
	if msg.err != nil {
		m.projOp = nil
		if errors.Is(msg.err, ghcli.ErrMissingScope) || errors.Is(msg.err, ghcli.ErrNeedsProjectScope) {
			m.status = styErr.Render("✗ " + ghcli.WriteScopeHint(op.host))
		} else {
			m.status = styErr.Render("✗ " + ghcli.CleanText(msg.err.Error()))
		}
		return m, nil
	}
	op.names = map[string]string{}
	member := map[string]bool{}
	var checked []string
	for _, ms := range op.content.Memberships {
		member[ms.ProjectID] = true
		op.names[ms.ProjectID] = ms.Title
		checked = append(checked, ms.ProjectID)
	}
	var cs []choice
	seen := map[string]bool{}
	for _, p := range msg.cands {
		if p.ID == "" || seen[p.ID] || (p.Closed && !member[p.ID]) {
			continue
		}
		seen[p.ID] = true
		name := p.Owner + "/" + strconv.Itoa(p.Number) + " " + p.Title
		op.names[p.ID] = name
		cs = append(cs, choice{label: name, value: p.ID})
	}
	for _, ms := range op.content.Memberships { // a Project the list did not show can still be left
		if !seen[ms.ProjectID] {
			seen[ms.ProjectID] = true
			cs = append(cs, choice{label: ms.Title, value: ms.ProjectID})
		}
	}
	if len(cs) == 0 {
		m.projOp = nil
		m.status = styDim.Render("no Projects found for you on this host")
		return m, nil
	}
	slices.SortStableFunc(cs, func(a, b choice) int { return strings.Compare(strings.ToLower(a.label), strings.ToLower(b.label)) })
	m.status = ""
	title := "Projects of " + op.label + " — " + op.content.Title
	if len(op.content.Memberships) >= 50 || len(msg.cands) >= 50 {
		title += " (the list may be incomplete)"
	}
	return m.withDialog(newChoiceDialog("proj.assign", title, cs, true, checked)), nil
}

func (m model) onProjDone(msg projDoneMsg) (model, tea.Cmd) {
	m.projBusy = false
	if st, ok := m.screenData(viewBoard).(boardState); ok && st.data.ID != "" { // a board of a Project that was changed is read again
		changed := slices.Contains(msg.op.add, st.data.ID)
		for _, ms := range msg.op.remove {
			changed = changed || ms.ProjectID == st.data.ID
		}
		if changed {
			st.force = true
			m = m.withScreenData(viewBoard, st)
		}
	}
	if msg.err != nil {
		m.status = styErr.Render("✗ " + ghcli.CleanText(msg.err.Error()) + " (open the Project to see what it looks like now)")
	} else {
		m.status = styOK.Render("✓ " + msg.text)
	}
	return m, nil
}
