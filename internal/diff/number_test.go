package diff

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNumbered(t *testing.T) {
	d := Parse(twoFiles, 1000)
	got := d.Files[0].Numbered()
	type on struct{ old, new int }
	var want = []on{{0, 0}, {1, 1}, {2, 0}, {0, 2}, {0, 3}, {0, 4}, {3, 5}, {0, 0}, {20, 21}, {21, 0}, {0, 22}}
	if len(got) != len(want) {
		t.Fatalf("%d lines; want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Old != w.old || got[i].New != w.new {
			t.Errorf("line %d (%v %q): old/new = %d/%d; want %d/%d", i, got[i].Kind, got[i].Text, got[i].Old, got[i].New, w.old, w.new)
		}
	}
	nf := d.Files[1].Numbered() // a new file: @@ -0,0 +1,2 @@
	if nf[1].New != 1 || nf[2].New != 2 || nf[1].Old != 0 {
		t.Errorf("new file numbers: %+v", nf)
	}
}

func rowText(r Row) string {
	side := func(n *NumLine) string {
		if n == nil {
			return "-"
		}
		return n.Text
	}
	return side(r.Old) + "|" + side(r.New)
}

func TestRowsPairDeletionsWithAdditions(t *testing.T) {
	cases := []struct {
		name, diff string
		want       []string
	}{
		{"3 del 1 add", "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,3 +1,1 @@\n-a\n-b\n-c\n+X\n", []string{"@@ -1,3 +1,1 @@|@@ -1,3 +1,1 @@", "a|X", "b|-", "c|-"}},
		{"1 del 3 add", "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,1 +1,3 @@\n-a\n+X\n+Y\n+Z\n", []string{"@@ -1,1 +1,3 @@|@@ -1,1 +1,3 @@", "a|X", "-|Y", "-|Z"}},
		{"add only", "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -0,0 +1,2 @@\n+X\n+Y\n", []string{"@@ -0,0 +1,2 @@|@@ -0,0 +1,2 @@", "-|X", "-|Y"}},
		{"context both sides", "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n", []string{"@@ -1,2 +1,2 @@|@@ -1,2 +1,2 @@", "a|a", "b|c"}},
		{"separated runs stay separate", "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n-a\n k\n+b\n", []string{"@@ -1,3 +1,3 @@|@@ -1,3 +1,3 @@", "a|-", "k|k", "-|b"}},
	}
	for _, c := range cases {
		var got []string
		for _, r := range Parse(c.diff, 100).Files[0].Rows() {
			got = append(got, rowText(r))
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: rows %q; want %q", c.name, got, c.want)
		}
	}
}

func TestRowsKeepLineNumbers(t *testing.T) {
	rows := Parse(twoFiles, 1000).Files[0].Rows()
	r := rows[2] // the pair "import "fmt"" / "import ("
	if r.Old == nil || r.New == nil || r.Old.Old != 2 || r.New.New != 2 {
		t.Errorf("pair numbers: %+v %+v", r.Old, r.New)
	}
}

func spans(ss []Span) [][2]int {
	var out [][2]int
	for _, s := range ss {
		out = append(out, [2]int{s.Start, s.End})
	}
	return out
}

func TestWordDiff(t *testing.T) {
	cases := []struct {
		name, a, b string
		wa, wb     [][2]int
	}{
		{"one word", "for i := 0; i < 3; i++ {", "for i := 0; i < n; i++ {", [][2]int{{16, 17}}, [][2]int{{16, 17}}},
		{"identical", "same", "same", nil, nil},
		{"all different", "abc", "xyz", [][2]int{{0, 3}}, [][2]int{{0, 3}}},
		{"whitespace only", "a b", "a  b", nil, [][2]int{{2, 3}}},
		{"cjk by runes", "你好世界", "你好地球", [][2]int{{2, 4}}, [][2]int{{2, 4}}},
		{"insertion", "f(a)", "f(a, b)", nil, [][2]int{{3, 6}}},
	}
	for _, c := range cases {
		sa, sb := WordDiff(c.a, c.b)
		if !reflect.DeepEqual(spans(sa), c.wa) || !reflect.DeepEqual(spans(sb), c.wb) {
			t.Errorf("%s: %v / %v; want %v / %v", c.name, spans(sa), spans(sb), c.wa, c.wb)
		}
	}
}

func TestWordDiffOfHugeLinesIsFast(t *testing.T) {
	a := strings.Repeat("word ", 1000) + "OLD" + strings.Repeat(" tail", 1000)
	b := strings.Repeat("word ", 1000) + "NEW" + strings.Repeat(" tail", 1000)
	start := time.Now()
	sa, sb := WordDiff(a, b)
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("took %v", time.Since(start))
	}
	if len(sa) != 1 || len(sb) != 1 || sa[0].Start != 5000 || sa[0].End != 5003 {
		t.Errorf("spans %v %v", sa, sb)
	}
}
