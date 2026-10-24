// Package icons is the one table of symbols the screens draw states with. There are two sets:
// plain, which is the ordinary Unicode symbols gh-9s has always used, and nerd, which are Nerd Font
// glyphs in the style of GitHub's own icons (Octicons) as gh-dash uses them. A decoration that has no
// plain counterpart is the empty string in the plain set, so plain screens are unchanged.
package icons

import (
	"strings"
	"sync/atomic"
)

// Mode selects a set.
type Mode int

const (
	Plain Mode = iota
	Nerd
)

var active atomic.Int32

// Use switches the set every screen draws with.
func Use(m Mode) { active.Store(int32(m)) }

// Current is the set in use.
func Current() Mode { return Mode(active.Load()) }

// Parse reads "nerd" or "plain".
func Parse(s string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "nerd":
		return Nerd, true
	case "plain":
		return Plain, true
	}
	return Plain, false
}

// Name is what an icon means.
type Name int

const (
	PROpen Name = iota
	PRMerged
	PRClosed
	PRDraft
	IssueOpen
	IssueClosed
	CheckPass
	CheckFail
	CheckPending
	CheckSkipped
	ReviewApproved
	ReviewChanges
	ReviewRequired
	RunOK
	RunFail
	RunActive
	RunOther
	Branch
	Repo
	Comment
	User
	Label
	Clock
	New
	Viewed
	Selected
	TabAssigned
	TabReview
	TabMine
	TabMentions
	NameCount // not an icon: the number of names
)

// entry is one row of the table.
type entry struct {
	label string // for messages and tests
	plain string
	nerd  string
	color string // the colour that goes with the meaning ("" = the text's own)
}

// The Nerd Font glyphs are Octicons (U+F4xx) and Font Awesome (U+F0xx) code points from the Nerd Fonts
// cheat sheet, all in the BMP private use area so each is one column wide.
var table = [NameCount]entry{
	PROpen:         {"pr open", "", "", "2"},         // nf-oct-git_pull_request
	PRMerged:       {"pr merged", "", "", "5"},       // nf-oct-git_merge
	PRClosed:       {"pr closed", "", "", "1"},       // nf-oct-git_pull_request_closed
	PRDraft:        {"pr draft", "", "", "8"},        // nf-oct-git_pull_request_draft
	IssueOpen:      {"issue open", "", "", "2"},      // nf-oct-issue_opened
	IssueClosed:    {"issue closed", "", "", "5"},    // nf-oct-issue_closed
	CheckPass:      {"check passed", "✓", "", "2"},   // nf-oct-check
	CheckFail:      {"check failed", "✗", "", "1"},   // nf-oct-x
	CheckPending:   {"check pending", "●", "", "3"},  // nf-oct-dot_fill
	CheckSkipped:   {"check skipped", "○", "", "8"},  // nf-fa-ban
	ReviewApproved: {"review approved", "", "", "2"}, // nf-fa-check
	ReviewChanges:  {"review changes", "", "", "1"},  // nf-fa-exclamation_circle
	ReviewRequired: {"review required", "", "", "3"}, // nf-fa-circle_o
	RunOK:          {"run ok", "✓", "", "2"},         // nf-fa-check_circle
	RunFail:        {"run failed", "✗", "", "1"},     // nf-fa-times_circle
	RunActive:      {"run active", "●", "", "3"},     // nf-fa-circle
	RunOther:       {"run other", "○", "", "8"},      // nf-fa-dot_circle_o
	Branch:         {"branch", "", "", ""},           // nf-oct-git_branch
	Repo:           {"repo", "", "", ""},             // nf-oct-repo
	Comment:        {"comment", "", "", ""},          // nf-oct-comment
	User:           {"user", "", "", ""},             // nf-oct-person
	Label:          {"label", "", "", ""},            // nf-oct-tag
	Clock:          {"clock", "", "", "8"},           // nf-fa-clock_o
	New:            {"new", "●", "", "6"},            // nf-fa-circle (shares the glyph with a running run)
	Viewed:         {"viewed", "✓", "", "8"},         // nf-oct-check (shares the glyph with a passed check)
	Selected:       {"selected", "▸", "", ""},        // nf-fa-caret_right
	TabAssigned:    {"tab assigned", "", "", ""},     // nf-oct-issue_opened (shares the PR/issue meaning)
	TabReview:      {"tab review", "", "", ""},       // nf-oct-eye
	TabMine:        {"tab mine", "", "", ""},         // nf-oct-git_pull_request
	TabMentions:    {"tab mentions", "", "", ""},     // nf-fa-envelope
}

// names is table's labels by name, for tests and messages.
var names = func() map[Name]string {
	m := map[Name]string{}
	for i, e := range table {
		m[Name(i)] = e.label
	}
	return m
}()

// GetIn is an icon in a given set.
func GetIn(m Mode, n Name) string {
	if n < 0 || n >= NameCount {
		return ""
	}
	if m == Nerd {
		return table[n].nerd
	}
	return table[n].plain
}

// Get is an icon in the set in use.
func Get(n Name) string { return GetIn(Current(), n) }

// Color is the lipgloss colour that goes with an icon's meaning ("" when it takes the text's colour).
func Color(n Name) string {
	if n < 0 || n >= NameCount {
		return ""
	}
	return table[n].color
}

// Sample is the icons the first-run question shows.
var Sample = []Name{PROpen, PRMerged, IssueOpen, CheckPass, CheckFail, Branch, Comment, Repo}
