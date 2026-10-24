package tui

import (
	"fmt"
	"strings"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// The pane shows a PR or an issue the way its detail screen does, shortened: what a person looks at
// when going down a list. What it renders is capped so a huge PR cannot stall the UI.
const (
	previewSplitMin   = 72 // a pane at least this wide puts the sidebar beside the text
	previewSideWidth  = 28
	previewDescLines  = 150 // lines of the description
	previewTotalLines = 400 // lines of the whole pane
	previewComments   = 5   // latest comments shown
	previewReviews    = 3   // latest reviews shown
	previewScrollStep = 3
)

// previewLines lays an entry out as lines of at most width columns.
func previewLines(e previewEntry, width int) []string {
	width = max(width, 10)
	var d detailDoc
	switch {
	case e.err != nil:
		d.add(ui.Err.Render("couldn't load: ") + ghcli.CleanText(e.err.Error()))
	case e.pr != nil:
		prPreview(&d, *e.pr, width)
	case e.issue != nil:
		issuePreview(&d, *e.issue, width)
	}
	lines := d.lines
	if len(lines) > previewTotalLines {
		lines = append(lines[:previewTotalLines:previewTotalLines], ui.Dim.Render(fmt.Sprintf("… %d more lines; enter opens the whole thing", len(d.lines)-previewTotalLines)))
	}
	return lines
}

func addWrapped(d *detailDoc, text string, width int, style func(string) string) {
	for _, l := range wrap(text, width) {
		d.add(style(l))
	}
}

func bold(s string) string { return ui.Key.Render(s) }

func prPreview(d *detailDoc, p ghcli.PRDetail, width int) {
	icon := icons.PROpen
	switch {
	case p.State == "MERGED":
		icon = icons.PRMerged
	case p.State == "CLOSED":
		icon = icons.PRClosed
	case p.IsDraft:
		icon = icons.PRDraft
	}
	addWrapped(d, withIcon(icon, fmt.Sprintf("#%d %s", p.Number, p.Title)), width, bold)
	state := p.State
	if p.IsDraft {
		state += " (draft)"
	}
	d.add(ui.Dim.Render(fmt.Sprintf("%s  %s ← %s  %s", p.Author, p.BaseRef, p.HeadRef, state)))
	split := width >= previewSplitMin
	mainW := width
	if split {
		mainW = width - previewSideWidth - 1
	}
	var body detailDoc // the part under the title; beside the sidebar when the pane is wide enough
	d, whole := &body, d
	d.add("", "Review: "+orDash(p.ReviewDecision))
	d.add(fmt.Sprintf("Checks: %s%d %s%d %s%d", icons.Get(icons.CheckPass), p.ChecksPassed, icons.Get(icons.CheckFail), p.ChecksFailed, icons.Get(icons.CheckPending), p.ChecksPending))
	for _, n := range p.FailingChecks {
		d.add(ui.Fail.Render(trunc(" "+icons.Get(icons.CheckFail)+" "+n, mainW)))
	}
	if more := p.ChecksFailed - len(p.FailingChecks); more > 0 {
		d.add(ui.Dim.Render(fmt.Sprintf("  … %d more failing", more)))
	}
	d.add(fmt.Sprintf("+%d −%d in %d files", p.Additions, p.Deletions, p.ChangedFiles))
	if !split {
		d.add("")
		d.add(prSidebar(p, width)...)
	}
	description(d, p.Body, mainW)
	comments := p.Comments[max(0, len(p.Comments)-previewComments):]
	reviews := p.Reviews[max(0, len(p.Reviews)-previewReviews):]
	discussion(d, comments, reviews, len(p.Comments)-len(comments)+len(p.Reviews)-len(reviews), mainW)
	if !split {
		whole.addDoc(body)
		return
	}
	whole.addDoc(columns(body, prSidebarColumn(p, previewSideWidth), mainW))
}

// columns puts the sidebar lines to the right of the main ones, separated by a rule.
func columns(main detailDoc, side []string, mainW int) detailDoc {
	out := detailDoc{images: main.images}
	for i := 0; i < max(len(main.lines), len(side)); i++ {
		l, r := "", ""
		if i < len(main.lines) {
			l = main.lines[i]
		}
		if i < len(side) {
			r = side[i]
		}
		out.lines = append(out.lines, fit(l, mainW)+ui.Dim.Render("│")+r)
	}
	return out
}

func issuePreview(d *detailDoc, i ghcli.IssueDetail, width int) {
	addWrapped(d, withIcon(issueIcon(i.State), fmt.Sprintf("#%d %s", i.Number, i.Title)), width, bold)
	d.add(ui.Dim.Render(fmt.Sprintf("%s  %s", i.Author, i.State)))
	if len(i.Labels) > 0 {
		addWrapped(d, withIcon(icons.Label, strings.Join(i.Labels, ", ")), width, func(s string) string { return s })
	}
	if len(i.Assignees) > 0 {
		addWrapped(d, withIcon(icons.User, strings.Join(i.Assignees, ", ")), width, func(s string) string { return s })
	}
	description(d, i.Body, width)
	comments := i.Comments[max(0, len(i.Comments)-previewComments):]
	discussion(d, comments, nil, len(i.Comments)-len(comments), width)
}

func description(d *detailDoc, body string, width int) {
	d.add("", ui.Dim.Render("── Description ──"))
	if strings.TrimSpace(body) == "" {
		d.add(ui.Dim.Render("(no description)"))
		return
	}
	var md detailDoc
	md.addMarkdown(body, width, "")
	if len(md.lines) > previewDescLines {
		md.lines = append(md.lines[:previewDescLines:previewDescLines], ui.Dim.Render(fmt.Sprintf("… %d more lines", len(md.lines)-previewDescLines)))
	}
	d.addDoc(md)
}

func discussion(d *detailDoc, comments []ghcli.Comment, reviews []ghcli.Review, skipped, width int) {
	d.add("", ui.Dim.Render("── Latest comments ──"))
	if skipped > 0 {
		d.add(ui.Dim.Render(fmt.Sprintf("%d earlier not shown", skipped)))
	}
	d.addDoc(discussionLines(comments, reviews, width))
}

// previewPaneLines are the lines of the selected row's entry at a width, laid out once per entry.
func (m model) previewPaneLines(width int) []string {
	if lines, ok := m.draftLines(width); ok {
		return lines
	}
	st := m.preview
	if st == nil || !st.has {
		return nil
	}
	e, ok := st.cache[st.sel]
	if !ok {
		return []string{ui.Dim.Render("loading…")}
	}
	if st.lines != nil && st.linesKey == st.sel && st.linesAt.Equal(e.at) && st.linesW == width {
		return st.lines
	}
	st.lines, st.linesKey, st.linesAt, st.linesW = previewLines(e, width), st.sel, e.at, width
	return st.lines
}

// previewScroll moves the pane's text by delta lines (J and K), within the text.
func (m model) previewScroll(delta int) model {
	st := m.preview
	if st == nil || (!st.has && st.draftID == "") || st.paneH == 0 {
		return m
	}
	maxTop := max(0, len(m.previewPaneLines(st.paneW))-st.paneH)
	st.top = min(max(st.top+delta, 0), maxTop)
	return m
}

// previewContent is the text of the pane, h lines of w columns: the visible part of the entry.
func (m model) previewContent(w, h int) string {
	if m.preview != nil {
		m.preview.paneW, m.preview.paneH = max(w-1, 1), h
	}
	lines := m.previewPaneLines(max(w-1, 1))
	top := 0
	if m.preview != nil {
		top = min(m.preview.top, max(0, len(lines)-h))
	}
	var b strings.Builder
	for i := top; i < len(lines) && i < top+h; i++ {
		b.WriteString(" " + lines[i] + "\n")
	}
	return b.String()
}

// draftLines is the pane for a draft card of a board, which exists only in the Project and so has
// nothing to fetch: its title and text.
func (m model) draftLines(width int) ([]string, bool) {
	if m.view != viewBoard {
		return nil, false
	}
	r, ok := m.selected()
	if !ok {
		return nil, false
	}
	it, found := boardItem(m.boardSt().data, r.id)
	if !found || it.Type != "DraftIssue" {
		return nil, false
	}
	width = max(width, 10)
	var d detailDoc
	addWrapped(&d, it.Title, width, bold)
	d.add(ui.Dim.Render("draft - only in the Project"))
	d.add("")
	body := strings.TrimSpace(it.Body)
	if body == "" {
		body = "(no description)"
	}
	for _, para := range strings.Split(body, "\n") {
		addWrapped(&d, para, width, func(s string) string { return s })
	}
	lines := d.lines
	if len(lines) > previewTotalLines {
		lines = append(lines[:previewTotalLines:previewTotalLines], ui.Dim.Render("… more text in the Project on GitHub (o)"))
	}
	return lines, true
}

// draftCardID is the id of the selected card when it is a draft on a board, else "".
func (m model) draftCardID() string {
	if m.view != viewBoard || !m.previewShown() {
		return ""
	}
	if _, ok := m.draftLines(10); !ok {
		return ""
	}
	r, _ := m.selected()
	return r.id
}
