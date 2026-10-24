package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
)

func fullPR() ghcli.PRDetail {
	return ghcli.PRDetail{
		Number: 482, Title: "Fix flaky retry", Author: "alice", State: "OPEN", BaseRef: "main", HeadRef: "fix/retry",
		Labels: []string{"bug", "ci"}, ReviewDecision: "APPROVED", ChecksPassed: 3, ChecksFailed: 2, ChecksPending: 1,
		FailingChecks: []string{"lint", "e2e"}, Additions: 10, Deletions: 4, ChangedFiles: 3,
		Body:     "# Why\n\n- [x] done\n- [ ] todo\n\n```go\nfunc a() {}\n```\n\n![shot](https://x/y.png)",
		Comments: []ghcli.Comment{{Author: "bob", Body: "looks **good**", CreatedAt: "2026-01-01T00:00:00Z"}},
	}
}

func plainText(lines []string) string { return ansi.Strip(strings.Join(lines, "\n")) }

func TestPRPreviewShowsEveryField(t *testing.T) {
	p := fullPR()
	txt := plainText(previewLines(previewEntry{pr: &p}, 60))
	for _, want := range []string{"#482 Fix flaky retry", "alice", "main ← fix/retry", "OPEN", "bug, ci", "Review: APPROVED",
		"3", "lint", "e2e", "+10 −4 in 3 files", "Why", "func a() {}", "bob commented", "looks good"} {
		if !strings.Contains(txt, want) {
			t.Errorf("preview lacks %q:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "\uf407") { // the Octicons PR glyph
		t.Error("plain mode has no icons")
	}
}

func TestPreviewIconsOnlyInTheNerdSet(t *testing.T) {
	p := fullPR()
	useNerd(t)
	txt := plainText(previewLines(previewEntry{pr: &p}, 60))
	for _, n := range []icons.Name{icons.PROpen, icons.Label, icons.CheckFail} {
		if g := icons.GetIn(icons.Nerd, n); !strings.Contains(txt, g) {
			t.Errorf("nerd preview lacks the %q icon:\n%s", g, txt)
		}
	}
}

func TestIssuePreview(t *testing.T) {
	i := ghcli.IssueDetail{Number: 9, Title: "Crash", Author: "carol", State: "OPEN", Labels: []string{"bug"}, Assignees: []string{"dave"}, Body: "steps", Comments: []ghcli.Comment{{Author: "e", Body: "me too", CreatedAt: "2026-01-01T00:00:00Z"}}}
	txt := plainText(previewLines(previewEntry{issue: &i}, 50))
	for _, want := range []string{"#9 Crash", "carol", "bug", "dave", "steps", "me too"} {
		if !strings.Contains(txt, want) {
			t.Errorf("issue preview lacks %q:\n%s", want, txt)
		}
	}
}

func TestPreviewCapsChecksAndComments(t *testing.T) {
	p := fullPR()
	p.ChecksFailed = 500
	for i := 0; i < 20; i++ {
		p.FailingChecks = append(p.FailingChecks, fmt.Sprintf("job-%d", i))
	}
	for i := 0; i < 200; i++ {
		p.Comments = append(p.Comments, ghcli.Comment{Author: "u", Body: fmt.Sprint("c", i), CreatedAt: fmt.Sprintf("2026-02-%02dT00:00:00Z", i%28+1)})
	}
	txt := plainText(previewLines(previewEntry{pr: &p}, 60))
	if !strings.Contains(txt, "more failing") || !strings.Contains(txt, "earlier not shown") {
		t.Errorf("caps must be announced:\n%s", txt)
	}
	if n := strings.Count(txt, " commented"); n > previewComments {
		t.Errorf("%d comments shown, cap is %d", n, previewComments)
	}
}

func TestPreviewTextIsSanitised(t *testing.T) {
	raw := `{"number":1,"title":"t\u001b[2Jitle\u202e","body":"b\u001b]0;pwn\u0007ody","author":{"login":"a\u001b[31mlice"},"state":"OPEN",
		"headRefName":"h\u001b[2Jead","labels":[{"name":"l\u001b[2Jab"}],"comments":[{"author":{"login":"x"},"body":"c\u001b[2Jc","createdAt":"2026-01-01T00:00:00Z"}]}`
	d, err := ghcli.ParsePRDetail([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(previewLines(previewEntry{pr: &d}, 60), "\n")
	for _, bad := range []string{"\x1b[2J", "\x1b]0;", "\u202e", "\a"} {
		if strings.Contains(joined, bad) {
			t.Errorf("preview kept %q", bad)
		}
	}
}

func TestPreviewLoadingAndError(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	f.move(1)
	if txt := ansi.Strip(f.m.previewContent(60, 10)); !strings.Contains(txt, "loading…") {
		t.Errorf("a row that is loading says so:\n%s", txt)
	}
	f.fail.Store(true)
	f.settle(f.move(2))
	if txt := ansi.Strip(f.m.previewContent(60, 10)); !strings.Contains(txt, "boom") {
		t.Errorf("a failed fetch shows its error:\n%s", txt)
	}
	f.m.preview.has = false
	if txt := strings.TrimSpace(ansi.Strip(f.m.previewContent(60, 10))); txt != "" {
		t.Errorf("nothing selected shows nothing, got %q", txt)
	}
}

func TestPaneScrollsAndClamps(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	f.settle(f.move(1))
	p := fullPR()
	p.Body = strings.Repeat("line\n\n", 60)
	f.m.preview.put(f.m.preview.sel, previewEntry{pr: &p, at: previewNow(), rowUpdated: "9"})
	f.m.previewContent(60, 10) // drawn once, so scrolling knows the size
	f.m = f.m.previewScroll(-5)
	if f.m.preview.top != 0 {
		t.Error("cannot scroll above the top")
	}
	f.m = f.m.previewScroll(previewScrollStep)
	if f.m.preview.top != previewScrollStep {
		t.Errorf("top = %d", f.m.preview.top)
	}
	f.m = f.m.previewScroll(100000)
	lines := f.m.previewPaneLines(59)
	if f.m.preview.top != len(lines)-10 {
		t.Errorf("top %d, want the last page of %d lines", f.m.preview.top, len(lines))
	}
	// the entry changes (a new load): back to the top
	f.m = f.m.previewLoaded(previewLoadedMsg{key: f.m.preview.sel, entry: previewEntry{pr: &p, rowUpdated: "9"}})
	if f.m.preview.top != 0 {
		t.Error("a freshly loaded entry starts at its top")
	}
	// J and K work on the real keys
	f.m.previewContent(60, 10)
	if got := keys(f.m, "J"); got.preview.top != previewScrollStep {
		t.Errorf("J scrolls, top = %d", got.preview.top)
	}
}

func TestHugeBodyRendersFastAndOnce(t *testing.T) {
	p := fullPR()
	p.Body = strings.Repeat("some **bold** text with `code` and a [link](http://x) here. ", 1000) // ~60,000 characters
	start := time.Now()
	lines := previewLines(previewEntry{pr: &p}, 60)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("rendering took %s", d)
	}
	if len(lines) > previewTotalLines+1 {
		t.Errorf("%d lines; the pane caps at %d", len(lines), previewTotalLines)
	}
	m := paneModel(t, 140, 0)
	m.preview = newPreviewState()
	m.preview.has, m.preview.sel = true, previewKey{"h", "a/b", "pr", 1}
	m.preview.cache[m.preview.sel] = previewEntry{pr: &p, at: time.Now()}
	a := m.previewPaneLines(60)
	b := m.previewPaneLines(60)
	if &a[0] != &b[0] {
		t.Error("the laid-out lines are kept for the same entry and width")
	}
}

func TestPaneIsDrawnBesideTheListEndToEnd(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	f.settle(f.move(1))
	v := ansi.Strip(f.m.View())
	if !strings.Contains(v, "pr 2") || !strings.Contains(v, "╭") {
		t.Errorf("the pane should show the selected PR:\n%s", v)
	}
	for i, l := range strings.Split(strings.TrimRight(v, "\n"), "\n") {
		if w := ansi.StringWidth(l); w > f.m.width {
			t.Errorf("line %d is %d wide, terminal is %d", i, w, f.m.width)
		}
	}
	_ = errors.New
}
