package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTrunc(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abcdef", 4, "abc…"},
		{"日本語です", 3, "日本…"},
		{"abc", 0, ""},
	}
	for _, c := range cases {
		if got := trunc(c.in, c.w); got != c.want {
			t.Errorf("trunc(%q,%d) = %q; want %q", c.in, c.w, got, c.want)
		}
	}
}

func TestVisibleFilter(t *testing.T) {
	m := model{tbl: table{rows: []row{
		{cells: []string{"1", "Build", "main"}},
		{cells: []string{"2", "Deploy", "prod"}},
	}}}
	if got := len(m.visible()); got != 2 {
		t.Fatalf("unfiltered = %d rows; want 2", got)
	}
	m.filter = "DEPLOY"
	if v := m.visible(); len(v) != 1 || v[0].cells[0] != "2" {
		t.Errorf("filter is case-sensitive or wrong: %+v", v)
	}
}

func TestLogScrollBounds(t *testing.T) {
	m := model{view: viewLogs, height: 15, logs: make([]string, 100)}
	next, _, _ := m.logKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = next.(model)
	if m.top != m.maxTop() || m.top != 100-m.logHeight() {
		t.Errorf("G: top=%d maxTop=%d", m.top, m.maxTop())
	}
	next, _, _ = m.logKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(model).top != m.maxTop() {
		t.Error("j scrolled past the end")
	}
	m.top = 0
	next, _, _ = m.logKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(model).top != 0 {
		t.Error("k scrolled above the start")
	}
}
