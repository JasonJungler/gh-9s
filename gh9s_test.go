package main

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRunState(t *testing.T) {
	cases := []struct {
		status, conclusion string
		label              string
		st                 state
	}{
		{"completed", "success", "✓ success", stOK},
		{"completed", "failure", "✗ failure", stFail},
		{"completed", "timed_out", "✗ timed_out", stFail},
		{"completed", "cancelled", "○ cancelled", stOther},
		{"in_progress", "", "● in_progress", stRun},
		{"queued", "", "● queued", stRun},
	}
	for _, c := range cases {
		label, st := runState(c.status, c.conclusion)
		if label != c.label || st != c.st {
			t.Errorf("runState(%q,%q) = %q,%v; want %q,%v", c.status, c.conclusion, label, st, c.label, c.st)
		}
	}
}

func TestDuration(t *testing.T) {
	cases := map[[2]string]string{
		{"2026-01-01T00:00:00Z", "2026-01-01T00:00:07Z"}: "7s",
		{"2026-01-01T00:00:00Z", "2026-01-01T00:02:05Z"}: "2m05s",
		{"", ""}: "-",
	}
	for in, want := range cases {
		if got := duration(in[0], in[1]); got != want {
			t.Errorf("duration(%q,%q) = %q; want %q", in[0], in[1], got, want)
		}
	}
}

func TestCleanLog(t *testing.T) {
	in := "build\tSet up job\t2026-01-01T00:00:00.1234567Z hello\nbuild\tTest\t2026-01-01T00:00:01Z a\tb\nplain\n"
	want := []string{"Set up job  hello", "Test  a    b", "plain"}
	if got := cleanLog(in); !reflect.DeepEqual(got, want) {
		t.Errorf("cleanLog = %q; want %q", got, want)
	}
}

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
