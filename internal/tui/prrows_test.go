package tui

import "testing"

func TestPRRowsShowsGhostForADeletedAuthor(t *testing.T) {
	deleted := prListItem{Number: 7, Title: "orphaned", UpdatedAt: "2026-10-20T10:00:00Z"} // author null
	named := prListItem{Number: 8, Title: "owned"}
	named.Author = &prAuthor{Login: "alice"}
	tbl := prRows([]prListItem{deleted, named}, "")
	if got := tbl.rows[0].cells[3]; got != "ghost" {
		t.Errorf("deleted author cell = %q; want ghost, like everywhere else", got)
	}
	if got := tbl.rows[1].cells[3]; got != "alice" {
		t.Errorf("author cell = %q", got)
	}
}

func TestPRRowsMarkTheLocalBranchAndDrafts(t *testing.T) {
	p := prListItem{Number: 9, Title: "wip", Head: "feature-x", Draft: true}
	tbl := prRows([]prListItem{p}, "feature-x")
	if c := tbl.rows[0].cells; c[0] != "*9" || c[2] != "[draft] wip" {
		t.Errorf("cells = %q", c)
	}
}
