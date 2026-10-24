package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

const reviewDiff = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main
-import "fmt"
+import "log"
 func a() {}
@@ -20,2 +20,2 @@
 func b() {}
-var x = 1
+var x = 2
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1,1 +1,1 @@
-old title
+new title
`

type threadFakes struct {
	targets []ghcli.RepoRef
	numbers []string
}

func installThreads(t *testing.T, ts []ghcli.Thread, err error) *threadFakes {
	t.Helper()
	f := &threadFakes{}
	old := fetchThreads
	fetchThreads = func(r ghcli.RepoRef, n string) ([]ghcli.Thread, error) {
		f.targets, f.numbers = append(f.targets, r), append(f.numbers, n)
		return ts, err
	}
	t.Cleanup(func() { fetchThreads = old })
	return f
}

func reviewModel(t *testing.T, width int, text string, ts []ghcli.Thread) model {
	t.Helper()
	return reviewModelH(t, width, 24, text, ts)
}

// bodyLine is the i-th row of the screen's body (the title and hint line come first).
func bodyLine(m model, i int) string { return strings.Split(ansi.Strip(m.View()), "\n")[2+i] }

// sideBySideShown reports whether some row of the view holds both the old and the new line.
func sideBySideShown(v string) bool {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, `import "fmt"`) && strings.Contains(l, `import "log"`) {
			return true
		}
	}
	return false
}

func reviewModelH(t *testing.T, width, height int, text string, ts []ghcli.Thread) model {
	t.Helper()
	installDiff(t, text, nil)
	installThreads(t, ts, nil)
	m := model{view: viewDiff, pr: "482", target: acme, prTarget: acme, width: width, height: height,
		back: []viewKind{viewPRs, viewPRDetail}}
	next, _ := m.Update(m.fetch()())
	return next.(model)
}

func resize(m model, w int) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: m.height})
	return next.(model)
}

func TestWideTerminalsGetFileListAndSideBySide(t *testing.T) {
	m := reviewModel(t, 140, reviewDiff, nil)
	v := ansi.Strip(m.View())
	var pairRow string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, `import "fmt"`) {
			pairRow = l
		}
	}
	if !strings.Contains(pairRow, `import "log"`) {
		t.Errorf("old and new should share a row:\n%s", v)
	}
	for _, want := range []string{"main.go", "README.md", "+1 −1"} {
		if !strings.Contains(v, want) {
			t.Errorf("file list lacks %q:\n%s", want, v)
		}
	}
	narrow := ansi.Strip(resize(m, 100).View())
	if !strings.Contains(narrow, `- import "fmt"`) || !strings.Contains(narrow, `+ import "log"`) {
		t.Errorf("below 120 columns the unified view is used:\n%s", narrow)
	}
}

func TestSTogglesAndSticksAcrossResizes(t *testing.T) {
	m := reviewModel(t, 100, reviewDiff, nil)
	if sideBySideShown(ansi.Strip(m.View())) {
		t.Fatal("100 columns start unified")
	}
	m = keys(m, "s")
	if v := ansi.Strip(m.View()); !sideBySideShown(v) {
		t.Errorf("s should force side-by-side at 100 columns:\n%s", v)
	}
	m = resize(resize(m, 150), 90)
	if v := ansi.Strip(m.View()); !sideBySideShown(v) {
		t.Errorf("the choice must survive resizes:\n%s", v)
	}
	m = keys(m, "s")
	if v := ansi.Strip(m.View()); sideBySideShown(v) {
		t.Errorf("s again should return to unified:\n%s", v)
	}
	wide := keys(reviewModel(t, 140, reviewDiff, nil), "s")
	if v := ansi.Strip(wide.View()); sideBySideShown(v) {
		t.Errorf("s at 140 columns forces unified:\n%s", v)
	}
}

func TestTabMovesBetweenTheListAndTheDiff(t *testing.T) {
	m := reviewModelH(t, 140, 12, reviewDiff, nil)
	m = keys(m, "tab", "j") // list focused: go to the second file
	if m.top == 0 || !strings.Contains(ansi.Strip(m.View()), "── README.md") {
		t.Errorf("the diff should have jumped to README.md (top=%d):\n%s", m.top, m.View())
	}
	before := m.top
	m = keys(m, "tab", "k") // back to the diff: k scrolls the diff
	if m.top >= before {
		t.Errorf("k in the diff should scroll it up: top %d -> %d", before, m.top)
	}
}

func TestVMarksAFileViewedAndTheMarkSurvivesARefetchOfTheSameHead(t *testing.T) {
	m := reviewModel(t, 140, reviewDiff, nil)
	m = m.withScreenData(viewPRDetail, prDetailState{d: ghcli.PRDetail{HeadSHA: "abc"}})
	next, _ := m.Update(m.fetch()())
	m = next.(model)
	m = keys(m, "v")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✓ main.go") && !strings.Contains(v, "✓main.go") {
		t.Errorf("main.go should be marked viewed:\n%s", v)
	}
	next, _ = m.Update(m.fetch()())
	if v := ansi.Strip(next.(model).View()); !strings.Contains(v, "✓") {
		t.Errorf("a refetch at the same head keeps the marks:\n%s", v)
	}
	m = m.withScreenData(viewPRDetail, prDetailState{d: ghcli.PRDetail{HeadSHA: "def"}})
	next, _ = m.Update(m.fetch()())
	if v := ansi.Strip(next.(model).View()); strings.Contains(v, "✓") {
		t.Errorf("a new head commit clears the marks:\n%s", v)
	}
	m2 := keys(m, "v")
	if v := ansi.Strip(m2.View()); strings.Contains(v, "✓ main.go") {
		t.Errorf("v again clears the mark:\n%s", v)
	}
}

func TestBracesJumpBetweenHunks(t *testing.T) {
	m := reviewModelH(t, 140, 12, reviewDiff, nil)
	m = keys(m, "}")
	first := m.top
	m = keys(m, "}")
	if m.top <= first {
		t.Errorf("} should move to the next hunk: %d -> %d", first, m.top)
	}
	m = keys(m, "{")
	if m.top != first {
		t.Errorf("{ should come back: %d", m.top)
	}
	u := reviewModelH(t, 100, 12, reviewDiff, nil)
	u = keys(u, "}")
	if u.top == 0 {
		t.Error("} works in the unified view too")
	}
}

func TestListShowsCommentCounts(t *testing.T) {
	ts := []ghcli.Thread{{Path: "main.go", Line: 2, Side: "RIGHT", Comments: []ghcli.ThreadComment{{Author: "bob", Body: "why log?"}, {Author: "amy", Body: "because"}}}}
	m := reviewModel(t, 140, reviewDiff, ts)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "◆1") {
		t.Errorf("main.go has one thread:\n%s", v)
	}
	if !strings.Contains(v, "why log?") || !strings.Contains(v, "because") {
		t.Errorf("the thread should be shown under its line:\n%s", v)
	}
}

func TestThreadsAreFetchedForTheShownPRAndAFailureKeepsTheDiff(t *testing.T) {
	installDiff(t, reviewDiff, nil)
	f := installThreads(t, nil, errors.New("rate limited"))
	m := model{view: viewDiff, pr: "482", target: acme, base: ghcli.RepoRef{Owner: "chosen", Name: "repo"}, width: 140, height: 24}
	next, _ := m.Update(m.fetch()())
	m = next.(model)
	if len(f.targets) != 1 || f.targets[0] != acme || f.numbers[0] != "482" {
		t.Errorf("threads fetched for %v #%v; want acme/api #482", f.targets, f.numbers)
	}
	if !strings.Contains(m.status, "couldn't load review comments") || !strings.Contains(m.status, "rate limited") {
		t.Errorf("status = %q", m.status)
	}
	if !strings.Contains(ansi.Strip(m.View()), "main.go") {
		t.Error("the diff must still show")
	}
}

func TestSearchFindsTextOnBothSides(t *testing.T) {
	m := reviewModel(t, 140, reviewDiff, nil)
	m = keys(m, "/", "l", "o", "g")
	next, _ := m.Update(k("enter"))
	m = next.(model)
	if len(m.matches) == 0 {
		t.Fatalf("no match for log (new side)")
	}
	m = keys(m, "/")
	for i := 0; i < 3; i++ {
		m = keys(m, "backspace")
	}
	m = keys(m, "f", "m", "t")
	next, _ = m.Update(k("enter"))
	if len(next.(model).matches) == 0 {
		t.Errorf("no match for fmt (old side)")
	}
}

func TestResizingAcrossTheThresholdKeepsTheSameLineOnTop(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,60 +1,60 @@\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "-old%03d\n+new%03d\n", i, i)
	}
	m := reviewModelH(t, 140, 12, b.String(), nil)
	for i := 0; i < 20; i++ {
		m = keys(m, "j")
	}
	token := regexp.MustCompile(`old\d{3}`).FindString(bodyLine(m, 0))
	if token == "" {
		t.Fatalf("setup: no old line on top: %q", bodyLine(m, 0))
	}
	narrow := resize(m, 100)
	if got := bodyLine(narrow, 0); !strings.Contains(got, token) {
		t.Errorf("after narrowing the top line is %q; want it to stay at %s", got, token)
	}
	wide := resize(narrow, 140)
	if got := bodyLine(wide, 0); !strings.Contains(got, token) || wide.top != m.top {
		t.Errorf("after widening the top is %q (row %d); want %s at row %d", got, wide.top, token, m.top)
	}
}

func TestTheDiffCapIsFiftyThousandLines(t *testing.T) {
	if maxDiffLines != 50000 {
		t.Errorf("maxDiffLines = %d", maxDiffLines)
	}
}

func TestSideModeShowsLoadingAndErrorsInsteadOfAStaleDiff(t *testing.T) {
	m := reviewModel(t, 150, reviewDiff, nil)
	m.err = errors.New("HTTP 406: diff too large")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "too large") {
		t.Errorf("a failed fetch must be visible in side-by-side mode:\n%s", v)
	}
	m.err, m.loading, m.logs = nil, true, nil
	if v := ansi.Strip(m.View()); !strings.Contains(v, "loading diff") {
		t.Errorf("a loading diff must say so:\n%s", v)
	}
	m.err = errors.New("boom")
	if next, _, ok := reviewKey(m, k("v")); ok {
		t.Errorf("review keys must not act on a stale diff while an error is shown: %v", next.screenData(viewDiff))
	}
}

func TestWideningOutOfUnifiedListModeDoesNotLeaveAnInvisibleList(t *testing.T) {
	m := reviewModel(t, 100, reviewDiff, nil)
	m = keys(m, "f")
	m = resize(m, 150)
	if st := m.screenData(viewDiff).(diffState); st.listMode {
		t.Fatal("the unified file list must close when the layout becomes side-by-side")
	}
	m = keys(m, "s")
	if sideBySideShown(ansi.Strip(m.View())) {
		t.Error("s should still toggle")
	}
}

func TestRefetchKeepsTheLineOnTop(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,300 +1,300 @@\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "-old%03d\n", i)
	}
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "+new%03d\n", i)
	}
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, " ctx%03d\n", i)
	}
	m := reviewModelH(t, 140, 12, b.String(), nil)
	for i := 0; i < 130; i++ {
		m = keys(m, "j")
	}
	want := bodyLine(m, 0)
	next, _ := m.Update(m.fetch()())
	if got := bodyLine(next.(model), 0); got != want {
		t.Errorf("R changed the top line: %q -> %q", want, got)
	}
}

func TestSwitchingToSideBySideKeepsAnAddedLineOnTop(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,100 +1,100 @@\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "-old%03d\n", i)
	}
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "+new%03d\n", i)
	}
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, " ctx%03d\n", i)
	}
	m := reviewModelH(t, 100, 12, b.String(), nil)
	for i := 0; i < 50+3+48; i++ { // header, hunk, olds, then new000..new047
		m = keys(m, "j")
	}
	top := regexp.MustCompile(`new\d{3}`).FindString(bodyLine(m, 0))
	if top == "" {
		t.Fatalf("setup: %q", bodyLine(m, 0))
	}
	wide := resize(m, 150)
	if got := bodyLine(wide, 0); !strings.Contains(got, top) {
		t.Errorf("after widening the top line is %q; want it to hold %s", got, top)
	}
}

func TestSideBySideIsRefusedWhereItCannotFit(t *testing.T) {
	m := reviewModel(t, 50, reviewDiff, nil)
	m = keys(m, "s")
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n")[2 : 2+m.logHeight()] {
		if ansi.StringWidth(l) > 50 {
			t.Errorf("a %d-wide row on a 50-column terminal: %q", ansi.StringWidth(l), l)
		}
	}
	if !strings.Contains(m.status, "columns") {
		t.Errorf("status = %q; s should say why nothing changed", m.status)
	}
}

func TestOutdatedThreadRowsKeepTheKeysInOrder(t *testing.T) {
	ts := []ghcli.Thread{{Path: "main.go", Line: 99, Side: "RIGHT", Outdated: true, Comments: []ghcli.ThreadComment{{Author: "bob", Body: "stale"}}}}
	m := reviewModel(t, 140, reviewDiff, ts)
	l := m.screenData(viewDiff).(diffState).sb
	for i := 1; i < len(l.src); i++ {
		if l.src[i] < l.src[i-1] {
			t.Fatalf("keys go backwards at row %d: %d after %d", i, l.src[i], l.src[i-1])
		}
	}
}
