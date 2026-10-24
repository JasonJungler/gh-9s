package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/safe"
)

// pendingDraft is a comment being written in a dialog: where it goes and for which PR screen.
type pendingDraft struct {
	d  drafts.Draft
	id string // m.itemID of the PR the screen was opened for
}

func init() {
	dialogHandlers["diff.comment"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		pd := m.pendingDraft
		m.pendingDraft = nil
		if !r.ok || pd == nil {
			return m, nil
		}
		if m.view != viewDiff || m.itemID(m.pr) != pd.id {
			return m, nil // the screen moved on while the dialog was open
		}
		text := safe.Text(r.text) // control characters and escape sequences never make it into a draft
		if strings.TrimSpace(text) == "" {
			m.status = styDim.Render("empty comment — nothing added")
			return m, nil
		}
		pd.d.Body = text
		return m.addDraft(pd.d), nil
	}
	dialogHandlers["diff.discard"] = func(m model, r dialogResultMsg) (model, tea.Cmd) {
		if !r.ok || m.view != viewDiff {
			return m, nil
		}
		st, _ := m.screenData(viewDiff).(diffState)
		n := len(st.drafts)
		st.drafts = nil
		m = m.withScreenData(viewDiff, st)
		m = m.persistDrafts()
		m.status = styDim.Render(fmt.Sprintf("discarded %d draft comment(s)", n))
		return m.rebuildDiff(), nil
	}
}

// draftsKey names the PR this screen shows in the drafts file; ok is false when the repo is unknown.
func (m model) draftsKey() (string, bool) {
	t := m.curTarget()
	if t.Owner == "" || t.Name == "" || m.pr == "" {
		return "", false
	}
	return drafts.Key(t.Host, t.Owner, t.Name, m.pr), true
}

// persistDrafts writes the screen's drafts to the file.
func (m model) persistDrafts() model {
	st, _ := m.screenData(viewDiff).(diffState)
	key, ok := m.draftsKey()
	if !ok {
		return m
	}
	if err := m.drafts.Put(key, st.head, st.drafts); err != nil {
		m.status = styErr.Render("couldn't save drafts: " + err.Error())
	}
	return m
}

// draftThreads shows drafts as threads by "you (draft)", so every layout can draw them like comments.
func draftThreads(ds []drafts.Draft, head string) []ghcli.Thread {
	out := make([]ghcli.Thread, 0, len(ds))
	for _, d := range ds {
		who := "you (draft)"
		if staleDraft(d, head) {
			who = "you (draft, older commit)"
		}
		out = append(out, ghcli.Thread{Path: d.Path, Line: d.Line, Side: d.Side,
			Comments: []ghcli.ThreadComment{{Author: who, Body: d.Body}}})
	}
	return out
}

// staleDraft says whether a draft was written against another commit than the one on screen.
func staleDraft(d drafts.Draft, head string) bool {
	return d.Head != "" && head != "" && d.Head != head
}

// allThreads is what is drawn under the code: other people's comments, then the drafts.
func allThreads(st diffState) []ghcli.Thread {
	return append(append([]ghcli.Thread(nil), st.threads...), draftThreads(st.drafts, st.head)...)
}

// reconcileDrafts splits drafts into those whose lines are still in the diff and those that are not.
func reconcileDrafts(ds []drafts.Draft, d diff.Diff) (kept, dropped []drafts.Draft) {
	has := map[string]map[string]bool{} // path -> "SIDE:line"
	for _, f := range d.Files {
		set := map[string]bool{}
		for _, n := range f.Numbered() {
			if n.Old > 0 {
				set[fmt.Sprintf("LEFT:%d", n.Old)] = true
			}
			if n.New > 0 {
				set[fmt.Sprintf("RIGHT:%d", n.New)] = true
			}
		}
		has[f.Path] = set
	}
	for _, x := range ds {
		set := has[x.Path]
		ok := set[fmt.Sprintf("%s:%d", x.Side, x.Line)]
		if ok && x.StartLine > 0 {
			ok = set[fmt.Sprintf("%s:%d", x.Side, x.StartLine)]
		}
		if ok {
			kept = append(kept, x)
		} else {
			dropped = append(dropped, x)
		}
	}
	return kept, dropped
}

// rebuildDiff lays the diff out again (the comments under the code changed) and keeps the same lines
// under the view and the cursor.
func (m model) rebuildDiff() model {
	st, _ := m.screenData(viewDiff).(diffState)
	if len(st.d.Files) == 0 {
		return m
	}
	topKey, curKey := rowKey(st, m.top), st.curKey
	st.lines, st.starts, st.usrc = diffDisplayThreads(st.d, allThreads(st))
	if st.sb != nil {
		st.sb = buildSB(st, m.width)
		m.logs = st.sb.logs
	} else {
		m.logs = make([]string, len(st.lines))
		for i, l := range st.lines {
			m.logs[i] = l.text
		}
	}
	m.top = min(rowOfKey(st, topKey), m.maxTop())
	st.cur = rowOfKey(st, curKey)
	if m.query != "" {
		m.matches = searchMatches(m.logs, m.query)
		m.mi = min(m.mi, max(0, len(m.matches)-1))
	}
	m = m.withScreenData(viewDiff, st)
	return m.keepCursorVisible()
}

// addDraft keeps a new draft, saves it, and shows it.
func (m model) addDraft(d drafts.Draft) model {
	st, _ := m.screenData(viewDiff).(diffState)
	d.Head = st.head
	st.drafts = append(append([]drafts.Draft(nil), st.drafts...), d)
	m = m.withScreenData(viewDiff, st)
	m = m.persistDrafts()
	if !strings.Contains(m.status, "couldn't save") {
		m.status = styOK.Render(fmt.Sprintf("✓ draft added (%d pending) — r sends them with a review", len(st.drafts)))
	}
	return m.rebuildDiff()
}

func lineSpan(d drafts.Draft) string {
	if d.StartLine > 0 {
		return fmt.Sprintf("%d–%d", d.StartLine, d.Line)
	}
	return fmt.Sprint(d.Line)
}

// draftKeys handles c, C, V, x, X and esc (which ends a range before it leaves the screen).
func draftKeys(m model, st diffState, key string) (model, tea.Cmd, bool) {
	switch key {
	case "V":
		if st.anchored {
			st.anchored = false
			return m.withScreenData(viewDiff, st), nil, true
		}
		st.anchored, st.anchor = true, st.cur
		m.status = styDim.Render("range: move to the last line, then c (esc cancels)")
		return m.withScreenData(viewDiff, st), nil, true
	case "esc":
		if !st.anchored {
			return m, nil, false
		}
		st.anchored = false
		m.status = ""
		return m.withScreenData(viewDiff, st), nil, true
	case "c", "C":
		return m.startDraft(st, key == "C"), nil, true
	case "x":
		return m.removeDraftAtCursor(st), nil, true
	case "r":
		return m.startDiffReview(st), nil, true
	case "A":
		return m.adoptDrafts(st), nil, true
	case "X":
		if len(st.drafts) == 0 {
			m.status = styDim.Render("no drafts")
			return m, nil, true
		}
		return m.withDialog(newConfirmDialog("diff.discard", fmt.Sprintf("Discard all %d draft comments on %s?", len(st.drafts), m.prLabel()), "They are removed from disk too.")), nil, true
	}
	return m, nil, false
}

func (m model) startDraft(st diffState, oldSide bool) model {
	rows := [2]int{st.cur, st.cur}
	if st.anchored {
		rows = [2]int{min(st.anchor, st.cur), max(st.anchor, st.cur)}
	}
	first, why := rowTarget(st, rows[0], oldSide)
	if why != "" {
		m.status = styErr.Render(why)
		return m
	}
	last, why := rowTarget(st, rows[1], oldSide)
	if why != "" {
		m.status = styErr.Render(why)
		return m
	}
	if first.Path != last.Path || first.Side != last.Side {
		m.status = styErr.Render("a range has to stay on one side of one file: pick lines on the same side (C for the old side)")
		return m
	}
	if hunkOf(st, rows[0]) != hunkOf(st, rows[1]) {
		m.status = styErr.Render("a range can't cross hunks: GitHub only takes lines of one hunk")
		return m
	}
	d := drafts.Draft{Path: last.Path, Line: last.Line, Side: last.Side}
	switch {
	case first.Line < last.Line:
		d.StartLine = first.Line
	case first.Line > last.Line:
		d.StartLine, d.Line = last.Line, first.Line
	}
	st.anchored = false
	m = m.withScreenData(viewDiff, st)
	m.pendingDraft = &pendingDraft{d: d, id: m.itemID(m.pr)}
	side := "new"
	if d.Side == "LEFT" {
		side = "old"
	}
	title := fmt.Sprintf("Comment on %s:%s (%s side) of %s", d.Path, lineSpan(d), side, m.prLabel())
	return m.withDialog(newTextDialog("diff.comment", title, "Your comment"))
}

func (m model) removeDraftAtCursor(st diffState) model {
	var want []target
	for _, old := range []bool{false, true} {
		if t, why := rowTarget(st, st.cur, old); why == "" {
			want = append(want, t)
		}
	}
	for i := len(st.drafts) - 1; i >= 0; i-- {
		d := st.drafts[i]
		for _, t := range want {
			if d.Path == t.Path && d.Side == t.Side && d.Line == t.Line {
				st.drafts = append(append([]drafts.Draft(nil), st.drafts[:i]...), st.drafts[i+1:]...)
				m = m.withScreenData(viewDiff, st)
				m = m.persistDrafts()
				m.status = styDim.Render(fmt.Sprintf("draft removed (%d left)", len(st.drafts)))
				return m.rebuildDiff()
			}
		}
	}
	m.status = styDim.Render("no draft on this line")
	return m
}

// inRange says whether display row i is part of the range being selected.
func inRange(st diffState, i int) bool {
	return st.anchored && i >= min(st.anchor, st.cur) && i <= max(st.anchor, st.cur)
}

// loadDrafts is the drafts to show for a freshly fetched diff: the saved ones plus the ones only
// kept in memory (a save may have failed) of this same PR, those whose lines are still in the diff,
// and a note when some had to be dropped. Drafts of another PR never come along.
func (m model) loadDrafts(old, st diffState) ([]drafts.Draft, string) {
	key, ok := m.draftsKey()
	var saved []drafts.Draft
	if ok && m.drafts != nil {
		_, saved = m.drafts.Get(key)
	}
	merged := append([]drafts.Draft(nil), saved...)
	if old.id == st.id {
		for _, d := range old.drafts {
			if !hasDraft(merged, d) {
				merged = append(merged, d)
			}
		}
	}
	kept, dropped := reconcileDrafts(merged, st.d)
	if ok && m.drafts != nil && (len(dropped) > 0 || len(kept) != len(saved)) {
		_ = m.drafts.Put(key, st.head, kept) // also heals a save that failed earlier
	}
	if len(dropped) == 0 {
		return kept, ""
	}
	return kept, fmt.Sprintf("%d draft(s) dropped: their lines are no longer in the diff", len(dropped))
}

func hasDraft(ds []drafts.Draft, d drafts.Draft) bool {
	for _, x := range ds {
		if x == d {
			return true
		}
	}
	return false
}

// threadTextRows draws a thread as plain rows for the single-column view.
func threadTextRows(t ghcli.Thread) []string {
	var out []string
	if t.Outdated {
		out = append(out, fmt.Sprintf("    ┃ (outdated) %s:%d", t.Path, t.Line))
	}
	for _, c := range t.Comments {
		lines := strings.Split(strings.TrimRight(c.Body, "\n"), "\n")
		out = append(out, "    ┃ "+c.Author+": "+lines[0])
		for _, l := range lines[1:] {
			out = append(out, "    ┃   "+l)
		}
	}
	return out
}

// startDiffReview opens the review dialogs from the diff screen; the drafts go with the review.
func (m model) startDiffReview(st diffState) model {
	t := m.curTarget()
	switch {
	case m.sendingKey != "":
		m.status = styDim.Render("a review is being sent: wait for it to finish")
		return m
	case t.Owner == "" || t.Name == "":
		m.status = styErr.Render("don't know which repo this PR is in: nothing was sent")
		return m
	case st.head == "":
		m.status = styErr.Render("can't tell which commit this diff is of: reload it (R) before reviewing")
		return m
	}
	stale := 0
	for _, d := range st.drafts {
		if staleDraft(d, st.head) {
			stale++
		}
	}
	if stale > 0 {
		m.status = styErr.Render(fmt.Sprintf("%d draft(s) were written for an older commit: A keeps them for this one, x or X removes them", stale))
		return m
	}
	m, p := m.startPRAction()
	p.target = t
	p.drafts = append([]drafts.Draft(nil), st.drafts...)
	p.head = st.head
	p.draftsKey, _ = m.draftsKey()
	return m.withDialog(newChoiceDialog("pr.review.kind", "Review "+p.label, []choice{
		{label: "Approve", value: string(ghcli.ReviewApprove)},
		{label: "Request changes", value: string(ghcli.ReviewRequestChanges)},
		{label: "Comment", value: string(ghcli.ReviewComment)},
	}, false, nil))
}

// adoptDrafts keeps the drafts written for an older commit, now for the one on screen.
func (m model) adoptDrafts(st diffState) model {
	n := 0
	next := make([]drafts.Draft, len(st.drafts))
	for i, d := range st.drafts {
		if staleDraft(d, st.head) {
			d.Head = st.head
			n++
		}
		next[i] = d
	}
	st.drafts = next
	m = m.withScreenData(viewDiff, st)
	m = m.persistDrafts()
	m.status = styDim.Render(fmt.Sprintf("%d draft(s) kept for this commit — check them before sending", n))
	return m.rebuildDiff()
}

// clearSentDrafts forgets the drafts that went out with a review (and only those: ones written
// while it was on its way stay).
func (m model) clearSentDrafts(key string, sent []drafts.Draft) model {
	without := func(ds []drafts.Draft) []drafts.Draft {
		var rest []drafts.Draft
		for _, d := range ds {
			if !hasDraft(sent, d) {
				rest = append(rest, d)
			}
		}
		return rest
	}
	if cur, ok := m.draftsKey(); ok && cur == key {
		if st, ok := m.screenData(viewDiff).(diffState); ok {
			st.drafts = without(st.drafts)
			m = m.withScreenData(viewDiff, st)
		}
	}
	if m.drafts != nil {
		head, saved := m.drafts.Get(key)
		_ = m.drafts.Put(key, head, without(saved))
	}
	if cur, ok := m.draftsKey(); ok && cur == key && m.view == viewDiff {
		m = m.rebuildDiff()
	}
	return m
}

// hunkOf is the display row of the hunk header a row belongs to, or -1.
func hunkOf(st diffState, row int) int {
	for i := row; i >= 0; i-- {
		if st.sb != nil && i < len(st.sb.lines) && st.sb.lines[i].hunk {
			return i
		}
		if st.sb == nil && i < len(st.lines) && st.lines[i].kind == dHunk {
			return i
		}
	}
	return -1
}
