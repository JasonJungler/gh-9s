package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// rowWith finds the display row whose plain text contains all of subs.
func rowWith(t *testing.T, m model, subs ...string) int {
	t.Helper()
	for i, l := range m.logs {
		ok := true
		for _, s := range subs {
			ok = ok && strings.Contains(l, s)
		}
		if ok {
			return i
		}
	}
	t.Fatalf("no row with %q in %q", subs, m.logs)
	return -1
}

func diffSt(m model) diffState { return m.screenData(viewDiff).(diffState) }

func TestRowTargetUnified(t *testing.T) {
	m := reviewModel(t, 100, reviewDiff, nil)
	cases := []struct {
		subs    []string
		oldSide bool
		want    target
		refuse  string
	}{
		{[]string{`+ import "log"`}, false, target{"main.go", 2, "RIGHT"}, ""},
		{[]string{`- import "fmt"`}, false, target{"main.go", 2, "LEFT"}, ""},
		{[]string{`- import "fmt"`}, true, target{"main.go", 2, "LEFT"}, ""},
		{[]string{"+ import"}, true, target{}, "old"},
		{[]string{"  package main"}, false, target{"main.go", 1, "RIGHT"}, ""},
		{[]string{"  package main"}, true, target{"main.go", 1, "LEFT"}, ""},
		{[]string{"+ new title"}, false, target{"README.md", 1, "RIGHT"}, ""},
		{[]string{"@@ -1,3"}, false, target{}, "hunk"},
		{[]string{"── main.go"}, false, target{}, "file"},
	}
	for _, c := range cases {
		row := rowWith(t, m, c.subs...)
		got, why := rowTarget(diffSt(m), row, c.oldSide)
		if c.refuse != "" {
			if !strings.Contains(why, c.refuse) || got != (target{}) {
				t.Errorf("%q old=%v: got %+v %q; want a refusal mentioning %q", c.subs, c.oldSide, got, why, c.refuse)
			}
			continue
		}
		if got != c.want || why != "" {
			t.Errorf("%q old=%v: got %+v %q; want %+v", c.subs, c.oldSide, got, why, c.want)
		}
	}
}

func TestRowTargetSideBySide(t *testing.T) {
	patch := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -3,3 +3,3 @@\n keep\n-gone\n+changed\n-onlyold\n@@ -20,0 +21,1 @@\n+onlynew\n"
	m := reviewModel(t, 140, patch, nil)
	st := diffSt(m)
	pair := rowWith(t, m, "gone", "changed")
	if got, why := rowTarget(st, pair, false); got != (target{"f.go", 4, "RIGHT"}) || why != "" {
		t.Errorf("pair, new side: %+v %q", got, why)
	}
	if got, why := rowTarget(st, pair, true); got != (target{"f.go", 4, "LEFT"}) || why != "" {
		t.Errorf("pair, old side: %+v %q", got, why)
	}
	del := rowWith(t, m, "onlyold")
	if got, why := rowTarget(st, del, false); got != (target{"f.go", 5, "LEFT"}) || why != "" {
		t.Errorf("a pure deletion falls back to the old side: %+v %q", got, why)
	}
	add := rowWith(t, m, "onlynew")
	if got, _ := rowTarget(st, add, false); got != (target{"f.go", 21, "RIGHT"}) {
		t.Errorf("pure addition: %+v", got)
	}
	if got, why := rowTarget(st, add, true); got != (target{}) || !strings.Contains(why, "old") {
		t.Errorf("no old line on an added row: %+v %q", got, why)
	}
	for _, subs := range [][]string{{"@@ -3,3"}, {"── f.go"}} {
		if got, why := rowTarget(st, rowWith(t, m, subs...), false); got != (target{}) || why == "" {
			t.Errorf("%q: %+v %q; want a refusal", subs, got, why)
		}
	}
}

func TestCursorMovesBeforeTheViewScrolls(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,40 +1,40 @@\n")
	for i := 0; i < 40; i++ {
		b.WriteString(" ctx\n")
	}
	m := reviewModelH(t, 100, 12, b.String(), nil) // 8 body rows
	h := m.logHeight()
	for i := 0; i < h-1; i++ {
		m = keys(m, "j")
	}
	if st := diffSt(m); st.cur != h-1 || m.top != 0 {
		t.Fatalf("cur=%d top=%d; the cursor walks down the window first", st.cur, m.top)
	}
	m = keys(m, "j")
	if st := diffSt(m); st.cur != h || m.top != 1 {
		t.Errorf("cur=%d top=%d; past the last row the view scrolls by one", st.cur, m.top)
	}
	m = keys(m, "k", "k")
	if st := diffSt(m); st.cur != h-2 || m.top != 1 {
		t.Errorf("cur=%d top=%d after k k", st.cur, m.top)
	}
	m = keys(m, "G")
	if st := diffSt(m); st.cur != len(m.logs)-1 || m.top != m.maxTop() {
		t.Errorf("G: cur=%d top=%d", st.cur, m.top)
	}
	m = keys(m, "g")
	if st := diffSt(m); st.cur != 0 || m.top != 0 {
		t.Errorf("g: cur=%d top=%d", st.cur, m.top)
	}
	m = keys(m, "ctrl+d")
	if st := diffSt(m); st.cur != h/2 {
		t.Errorf("ctrl+d moves half a page: cur=%d", st.cur)
	}
}

func TestJumpsPutTheCursorOnTheTopRow(t *testing.T) {
	m := reviewModelH(t, 140, 12, reviewDiff, nil)
	m = keys(m, "}")
	if st := diffSt(m); st.cur != m.top || m.top == 0 {
		t.Errorf("} should put the cursor on the hunk: cur=%d top=%d", st.cur, m.top)
	}
}

func TestTheCursorRowIsMarkedInBothLayouts(t *testing.T) {
	for _, w := range []int{140, 100} {
		m := reviewModelH(t, w, 12, reviewDiff, nil)
		m = keys(m, "j", "j")
		marked := 0
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, "▶") {
				marked++
			}
		}
		if marked != 1 {
			t.Errorf("width %d: %d marked rows; want exactly the cursor row\n%s", w, marked, m.View())
		}
	}
}

func TestTheCursorStaysOnTheSameSourceLineAcrossLayouts(t *testing.T) {
	m := reviewModelH(t, 100, 12, reviewDiff, nil)
	row := rowWith(t, m, `+ import "log"`)
	for i := 0; i < row; i++ {
		m = keys(m, "j")
	}
	wide := resize(m, 140)
	st := diffSt(wide)
	if got, _ := rowTarget(st, st.cur, false); got != (target{"main.go", 2, "RIGHT"}) {
		t.Errorf("after widening the cursor is on %+v (row %d)", got, st.cur)
	}
	back := resize(wide, 100)
	st = diffSt(back)
	if got, _ := rowTarget(st, st.cur, false); got != (target{"main.go", 2, "RIGHT"}) {
		t.Errorf("after narrowing the cursor is on %+v (row %d)", got, st.cur)
	}
}
