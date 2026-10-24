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
	d.add("", "Review: "+orDash(p.ReviewDecision))
	d.add(fmt.Sprintf("Checks: %s%d %s%d %s%d", icons.Get(icons.CheckPass), p.ChecksPassed, icons.Get(icons.CheckFail), p.ChecksFailed, icons.Get(icons.CheckPending), p.ChecksPending))
	for _, n := range p.FailingChecks {
		d.add(ui.Fail.Render(trunc(" "+icons.Get(icons.CheckFail)+" "+n, width)))
	}
	if more := p.ChecksFailed - len(p.FailingChecks); more > 0 {
		d.add(ui.Dim.Render(fmt.Sprintf("  … %d more failing", more)))
	}
	d.add(fmt.Sprintf("+%d −%d in %d files", p.Additions, p.Deletions, p.ChangedFiles))
	d.add("")
	d.add(prSidebar(p, width)...)
	description(d, p.Body, width)
	var comments []ghcli.Comment
	var reviews []ghcli.Review
	comments = p.Comments[max(0, len(p.Comments)-previewComments):]
	reviews = p.Reviews[max(0, len(p.Reviews)-previewReviews):]
	discussion(d, comments, reviews, len(p.Comments)-len(comments)+len(p.Reviews)-len(reviews), width)
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
	if st == nil || !st.has || st.paneH == 0 {
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
