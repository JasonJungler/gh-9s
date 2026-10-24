package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

const sampleDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,4 +1,5 @@ func main() {
 package main
-import "fmt"
+import (
+	"fmt"
+)

diff --git a/README.md b/README.md
new file mode 100644
--- /dev/null
+++ b/README.md
@@ -0,0 +1,2 @@
+# Title
+text
`

type diffFakes struct {
	targets []ghcli.RepoRef
	numbers []string
}

func installDiff(t *testing.T, text string, err error) *diffFakes {
	t.Helper()
	f := &diffFakes{}
	old := fetchDiff
	fetchDiff = func(r ghcli.RepoRef, number string) (string, error) {
		f.targets = append(f.targets, r)
		f.numbers = append(f.numbers, number)
		return text, err
	}
	t.Cleanup(func() { fetchDiff = old })
	return f
}

// diffModel is a model on the diff screen with the diff already loaded.
func diffModel(t *testing.T, text string) model {
	t.Helper()
	installDiff(t, text, nil)
	m := model{view: viewDiff, pr: "482", target: acme, prTarget: acme, width: 100, height: 24,
		back: []viewKind{viewPRs, viewPRDetail}}
	next, _ := m.Update(m.fetch()())
	return next.(model)
}

func starts(m model) []int {
	st, _ := m.screenData(viewDiff).(diffState)
	_, s := diffDisplay(st.d)
	return s
}

func TestDiffShowsFileListAndColouredLines(t *testing.T) {
	m := diffModel(t, sampleDiff)
	v := m.View()
	for _, want := range []string{"main.go", "+3 −1", "README.md", "+2 −0", "+ import (", "- import \"fmt\"", "@@ -1,4 +1,5 @@"} {
		if !strings.Contains(v, want) {
			t.Errorf("diff view lacks %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "<diff>") || !strings.Contains(v, "pr=#482") {
		t.Errorf("title should name the screen and PR:\n%s", v)
	}
}

func TestBracketKeysJumpBetweenFiles(t *testing.T) {
	m := diffModel(t, sampleDiff)
	s := starts(m)
	if len(s) != 2 || s[0] != 0 {
		t.Fatalf("file starts = %v", s)
	}
	m = keys(m, "]")
	if m.top != s[1] {
		t.Errorf("] -> top %d; want the second file at %d", m.top, s[1])
	}
	m = keys(m, "]")
	if m.top != s[1] {
		t.Errorf("] at the last file should stay, top %d", m.top)
	}
	m = keys(m, "[")
	if m.top != s[0] {
		t.Errorf("[ -> top %d; want the first file", m.top)
	}
}

func TestFileListJumpsToTheChosenFile(t *testing.T) {
	m := diffModel(t, sampleDiff)
	m = keys(m, "f")
	v := m.View()
	if !strings.Contains(v, "main.go") || !strings.Contains(v, "README.md") || strings.Contains(v, "import (") {
		t.Errorf("list mode should show only the files:\n%s", v)
	}
	m = keys(m, "j", "enter")
	if m.top != starts(m)[1] {
		t.Errorf("top = %d; want the second file at %d", m.top, starts(m)[1])
	}
	if strings.Contains(m.View(), "files (") {
		t.Error("enter should leave the file list")
	}
}

func TestEscInTheFileListClosesItBeforeLeavingTheScreen(t *testing.T) {
	m := diffModel(t, sampleDiff)
	m = keys(m, "f", "esc")
	if m.view != viewDiff {
		t.Errorf("view = %v; the first esc only closes the list", m.view)
	}
	m = keys(m, "esc")
	if m.view != viewPRDetail {
		t.Errorf("view = %v; the second esc goes back to the PR", m.view)
	}
}

func TestSearchJumpsToAMatch(t *testing.T) {
	m := diffModel(t, sampleDiff)
	m = keys(m, "/", "R", "E", "A", "D", "M", "E", "enter")
	if len(m.matches) == 0 {
		t.Fatalf("no match for README; matches=%v logs=%d", m.matches, len(m.logs))
	}
	if !strings.Contains(m.logs[m.matches[0]], "README") {
		t.Errorf("match line = %q", m.logs[m.matches[0]])
	}
	if v := m.View(); !strings.Contains(v, "match 1/") {
		t.Errorf("title should show the match position:\n%s", v)
	}
	m = keys(m, "n", "N")
	if m.mi != 0 {
		t.Errorf("n then N should return to the first match, mi=%d", m.mi)
	}
}

func TestTruncatedDiffSaysSoAndOffersTheBrowser(t *testing.T) {
	old := maxDiffLines
	maxDiffLines = 3
	t.Cleanup(func() { maxDiffLines = old })
	m := diffModel(t, sampleDiff)
	if v := m.View(); !strings.Contains(v, "truncated") || !strings.Contains(v, "browser") {
		t.Errorf("a capped diff must say so and point to the browser:\n%s", v)
	}
}

func TestDiffFetchesForTheShownRepoAndPR(t *testing.T) {
	f := installDiff(t, sampleDiff, nil)
	m := model{view: viewDiff, pr: "482", target: acme, base: ghcli.RepoRef{Owner: "chosen", Name: "repo"}}
	m.fetch()()
	if len(f.targets) != 1 || f.targets[0] != acme || f.numbers[0] != "482" {
		t.Errorf("fetched %v #%v; want acme/api #482 (target, not base)", f.targets, f.numbers)
	}
}

func TestEmptyDiffSaysSo(t *testing.T) {
	m := diffModel(t, "")
	if v := m.View(); !strings.Contains(v, "no changes") {
		t.Errorf("an empty diff should say so:\n%s", v)
	}
}

func TestDiffErrorIsShown(t *testing.T) {
	installDiff(t, "", errors.New("HTTP 406: diff too large"))
	m := model{view: viewDiff, pr: "1", width: 100, height: 24}
	next, _ := m.Update(m.fetch()())
	if v := next.(model).View(); !strings.Contains(v, "diff too large") {
		t.Errorf("the error should be shown:\n%s", v)
	}
}

func TestDKeyOnTheDetailOpensTheDiff(t *testing.T) {
	installPRFetch(t, fixturePR())
	installDiff(t, sampleDiff, nil)
	m := openDetail(t, prListModel())
	m = keys(m, "d")
	if m.view != viewDiff || m.pr != "482" || m.top != 0 {
		t.Errorf("view=%v pr=%q top=%d; want the diff of PR 482 from the top", m.view, m.pr, m.top)
	}
}

func TestOKeyOpensThePRInTheBrowser(t *testing.T) {
	opened := recordOpenURL(t)
	installPRFetch(t, fixturePR())
	installDiff(t, sampleDiff, nil)
	m := openDetail(t, prListModel())
	m = keys(m, "d")
	next, _ := m.Update(m.fetch()())
	m = keys(next.(model), "o")
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/acme/api/pull/482" {
		t.Errorf("opened %v; want the PR's URL", *opened)
	}
}

func TestDiffParseFeedsDisplayCounts(t *testing.T) {
	lines, s := diffDisplay(diff.Parse(sampleDiff, 1000))
	if len(s) != 2 || len(lines) == 0 || !strings.Contains(lines[s[1]].text, "README.md") {
		t.Errorf("starts=%v lines=%d", s, len(lines))
	}
}

func TestTooLargeDiffPointsToTheBrowser(t *testing.T) {
	installDiff(t, "", errors.New("HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)"))
	m := model{view: viewDiff, pr: "1", width: 100, height: 24}
	next, _ := m.Update(m.fetch()())
	v := next.(model).View()
	if !strings.Contains(v, "too large") || !strings.Contains(v, "o ") || !strings.Contains(v, "browser") {
		t.Errorf("a diff GitHub refuses as too large should say so and offer the browser:\n%s", v)
	}
}

func TestOtherDiffErrorsDoNotClaimTheDiffIsTooLarge(t *testing.T) {
	installDiff(t, "", errors.New("HTTP 502: Bad Gateway"))
	m := model{view: viewDiff, pr: "1", width: 100, height: 24}
	next, _ := m.Update(m.fetch()())
	if v := next.(model).View(); strings.Contains(v, "too large") {
		t.Errorf("a gateway error is not a size problem:\n%s", v)
	}
}

func TestDiffLayoutIsBuiltOnceNotOnEveryKeyAndFrame(t *testing.T) {
	m := diffModel(t, sampleDiff)
	st := m.screenData(viewDiff).(diffState)
	if len(st.lines) == 0 || len(st.starts) != 2 {
		t.Fatalf("the laid-out lines should be kept in the state: lines=%d starts=%v", len(st.lines), st.starts)
	}
	st.lines = []dline{{"SENTINEL LINE", dCtx}} // if rendering rebuilt the layout this would not show
	m = m.withScreenData(viewDiff, st)
	if v := m.View(); !strings.Contains(v, "SENTINEL LINE") {
		t.Errorf("rendering must use the kept layout:\n%s", v)
	}
}

func TestDiffMarksALineEndingOnlyChange(t *testing.T) {
	text := "diff --git a/w.txt b/w.txt\n--- a/w.txt\n+++ b/w.txt\n@@ -1,2 +1,2 @@\n-same\r\n+same\n-other\n+changed\n"
	m := diffModel(t, text)
	v := m.View()
	if !strings.Contains(v, "- same ␍") || strings.Contains(v, "+ same ␍") {
		t.Errorf("the removed line had a CR and the added one did not; both look alike without a marker:\n%s", v)
	}
	if !strings.Contains(v, "line ending") {
		t.Errorf("the pair should be explained:\n%s", v)
	}
	if strings.Contains(v, "- other ␍") || strings.Contains(v, "+ changed ␍") {
		t.Errorf("ordinary changes must not be marked:\n%s", v)
	}
}
