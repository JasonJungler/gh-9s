package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/drafts"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

const prKey = "github.com/acme/api#482"

// draftModel is a review screen with a drafts file in a temp directory.
func draftModel(t *testing.T, width int, text string) (model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "drafts.json")
	store, err := drafts.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	installDiff(t, text, nil)
	installThreads(t, nil, nil)
	m := model{view: viewDiff, pr: "482", target: acme, prTarget: acme, width: width, height: 30,
		back: []viewKind{viewPRs, viewPRDetail}, drafts: store}
	next, _ := m.Update(m.fetch()())
	return next.(model), path
}

func goToRow(t *testing.T, m model, subs ...string) model {
	t.Helper()
	row := rowWith(t, m, subs...)
	m = keys(m, "g")
	for i := 0; i < row; i++ {
		m = keys(m, "j")
	}
	if diffSt(m).cur != row {
		t.Fatalf("cursor on row %d; wanted %d", diffSt(m).cur, row)
	}
	return m
}

func comment(m model, key, text string) model {
	m = keyAll(m, key)
	m = typeAll(m, text)
	return keyAll(m, "enter")
}

func stored(t *testing.T, path string) []drafts.Draft {
	t.Helper()
	s, err := drafts.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_, ds := s.Get(prKey)
	return ds
}

func TestCCommentsOnTheNewSideAndShowsTheDraftUnderTheLine(t *testing.T) {
	for _, width := range []int{140, 100} {
		m, path := draftModel(t, width, reviewDiff)
		m = goToRow(t, m, `import "log"`)
		m = keyAll(m, "c")
		if m.dlg == nil || !strings.Contains(m.dlg.title, "main.go:2") || !strings.Contains(m.dlg.title, "acme/api#482") || !strings.Contains(m.dlg.title, "new") {
			t.Fatalf("width %d: dialog = %+v; want one naming the file, line, side and PR", width, m.dlg)
		}
		m = typeAll(m, "why log?")
		m = keyAll(m, "enter")
		if got := stored(t, path); len(got) != 1 || got[0] != (drafts.Draft{Path: "main.go", Line: 2, Side: "RIGHT", Body: "why log?"}) {
			t.Errorf("width %d: stored %+v", width, got)
		}
		v := ansi.Strip(m.View())
		if !strings.Contains(v, "why log?") || !strings.Contains(v, "you (draft)") {
			t.Errorf("width %d: the draft should show under its line:\n%s", width, v)
		}
	}
}

func TestCapitalCUsesTheOldSide(t *testing.T) {
	m, path := draftModel(t, 140, reviewDiff)
	m = goToRow(t, m, `import "fmt"`, `import "log"`)
	m = comment(m, "C", "why remove?")
	if got := stored(t, path); len(got) != 1 || got[0].Side != "LEFT" || got[0].Line != 2 {
		t.Errorf("stored %+v; want LEFT line 2", got)
	}
}

func TestACommentOnAHunkHeaderOrAnEmptyTextAddsNothing(t *testing.T) {
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, "@@ -1,3")
	m = keyAll(m, "c")
	if m.dlg != nil || !strings.Contains(m.status, "hunk") {
		t.Errorf("dialog=%v status=%q; want a sentence and no dialog", m.dlg != nil, m.status)
	}
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "   ")
	if len(stored(t, path)) != 0 || len(diffSt(m).drafts) != 0 {
		t.Error("an empty comment must not become a draft")
	}
}

func TestVSelectsARangeAndCCommentsTheWholeRange(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,0 +10,4 @@\n")
	for i := 0; i < 4; i++ {
		b.WriteString("+line\n")
	}
	m, path := draftModel(t, 100, b.String())
	m = goToRow(t, m, "+ line")
	m = keys(m, "V", "j", "j")
	m = comment(m, "c", "these three")
	got := stored(t, path)
	if len(got) != 1 || got[0].StartLine != 10 || got[0].Line != 12 || got[0].Side != "RIGHT" {
		t.Fatalf("stored %+v; want lines 10–12", got)
	}
	if diffSt(m).anchored {
		t.Error("the range ends once the comment is written")
	}
}

func TestARangeAcrossSidesOrFilesIsRefused(t *testing.T) {
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `- import "fmt"`)
	m = keys(m, "V", "j") // deleted line to added line: LEFT to RIGHT
	m = keyAll(m, "c")
	if m.dlg != nil || !strings.Contains(m.status, "side") {
		t.Errorf("dialog=%v status=%q; want a refusal that mentions the side", m.dlg != nil, m.status)
	}
	m = keys(m, "esc")
	if diffSt(m).anchored || m.view != viewDiff {
		t.Error("esc cancels the range first and does not leave the screen")
	}
	if len(stored(t, path)) != 0 {
		t.Error("nothing stored")
	}
}

func TestXRemovesTheDraftOnTheLineAndBigXClearsThemAll(t *testing.T) {
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "one")
	m = comment(m, "c", "two")
	if len(stored(t, path)) != 2 {
		t.Fatalf("stored %+v", stored(t, path))
	}
	m = keys(m, "x")
	if got := stored(t, path); len(got) != 1 || got[0].Body != "one" {
		t.Errorf("x removes the newest draft on the line: %+v", got)
	}
	m = keyAll(m, "X")
	if m.dlg == nil || !strings.Contains(m.dlg.title, "1") {
		t.Fatalf("X asks first: %+v", m.dlg)
	}
	m = keyAll(m, "y")
	if len(stored(t, path)) != 0 || len(diffSt(m).drafts) != 0 {
		t.Error("everything is gone from memory and disk")
	}
}

func TestDraftsSurviveARestart(t *testing.T) {
	m, path := draftModel(t, 140, reviewDiff)
	m = goToRow(t, m, `import "log"`)
	_ = comment(m, "c", "remember me")
	again, _ := draftModelAt(t, path, 140, reviewDiff)
	if v := ansi.Strip(again.View()); !strings.Contains(v, "remember me") {
		t.Errorf("a new session should show the saved draft:\n%s", v)
	}
}

// draftModelAt is draftModel over an existing drafts file.
func draftModelAt(t *testing.T, path string, width int, text string) (model, string) {
	t.Helper()
	store, err := drafts.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	installDiff(t, text, nil)
	installThreads(t, nil, nil)
	m := model{view: viewDiff, pr: "482", target: acme, prTarget: acme, width: width, height: 30, drafts: store}
	next, _ := m.Update(m.fetch()())
	return next.(model), path
}

func TestARefetchKeepsDraftsWhoseLineSurvivesAndReportsTheRest(t *testing.T) {
	m, path := draftModel(t, 100, reviewDiff)
	m = goToRow(t, m, `+ import "log"`)
	m = comment(m, "c", "stays")
	m = goToRow(t, m, "+ var x = 2")
	m = comment(m, "c", "goes")
	changed := strings.Replace(reviewDiff, "@@ -20,2 +20,2 @@\n func b() {}\n-var x = 1\n+var x = 2\n", "", 1)
	installDiff(t, changed, nil)
	next, _ := m.Update(m.fetch()())
	m = next.(model)
	got := stored(t, path)
	if len(got) != 1 || got[0].Body != "stays" {
		t.Errorf("stored %+v; want only the draft whose line is still in the diff", got)
	}
	if !strings.Contains(m.status, "1 draft") || !strings.Contains(m.status, "dropped") {
		t.Errorf("status = %q; the dropped draft must be reported", m.status)
	}
}

func TestDraftsOfAnotherPRNeverAppear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	store, _ := drafts.Load(path)
	_ = store.Put("github.com/acme/api#999", "abc1234", []drafts.Draft{{Path: "main.go", Line: 2, Side: "RIGHT", Body: "not yours"}})
	installDiff(t, reviewDiff, nil)
	installThreads(t, nil, nil)
	m := model{view: viewDiff, pr: "482", target: acme, width: 140, height: 30, drafts: store}
	next, _ := m.Update(m.fetch()())
	if v := ansi.Strip(next.(model).View()); strings.Contains(v, "not yours") {
		t.Errorf("another PR's draft leaked in:\n%s", v)
	}
}

func TestDraftsWorkWithoutAStore(t *testing.T) {
	installDiff(t, reviewDiff, nil)
	installThreads(t, nil, nil)
	m := model{view: viewDiff, pr: "482", target: acme, width: 100, height: 30}
	next, _ := m.Update(m.fetch()())
	m = goToRow(t, next.(model), `+ import "log"`)
	m = comment(m, "c", "in memory only")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "in memory only") {
		t.Errorf("a draft is still shown with no store:\n%s", v)
	}
}

func TestTheFileListCountsDrafts(t *testing.T) {
	m, _ := draftModel(t, 140, reviewDiff)
	m = goToRow(t, m, `import "log"`)
	m = comment(m, "c", "x")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✎1") {
		t.Errorf("main.go has one draft:\n%s", v)
	}
}

func TestExistingThreadsShowInTheUnifiedViewToo(t *testing.T) {
	installDiff(t, reviewDiff, nil)
	installThreads(t, []ghcli.Thread{{Path: "main.go", Line: 2, Side: "RIGHT", Comments: []ghcli.ThreadComment{{Author: "bob", Body: "why log?"}}}}, nil)
	m := model{view: viewDiff, pr: "482", target: acme, width: 100, height: 30}
	next, _ := m.Update(m.fetch()())
	if v := ansi.Strip(next.(model).View()); !strings.Contains(v, "bob") || !strings.Contains(v, "why log?") {
		t.Errorf("threads belong in the unified view too:\n%s", v)
	}
}
