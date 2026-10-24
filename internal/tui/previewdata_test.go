package tui

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

// previewFixture is a wide PR list of n rows with a fake clock and counted detail fetches.
type previewFixture struct {
	m     model
	now   time.Time
	calls *atomic.Int32
	last  *ghcli.RepoRef
	fail  *atomic.Bool
}

func newPreviewFixture(t *testing.T, n int) *previewFixture {
	t.Helper()
	f := &previewFixture{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), calls: &atomic.Int32{}, last: &ghcli.RepoRef{}, fail: &atomic.Bool{}}
	oldNow, oldPR, oldIssue := previewNow, fetchPR, fetchIssue
	previewNow = func() time.Time { return f.now }
	fetchPR = func(r ghcli.RepoRef, num string) (ghcli.PRDetail, error) {
		f.calls.Add(1)
		*f.last = r
		if f.fail.Load() {
			return ghcli.PRDetail{}, errors.New("boom")
		}
		var n int
		fmt.Sscan(num, &n)
		return ghcli.PRDetail{Number: n, Title: "pr " + num}, nil
	}
	fetchIssue = func(r ghcli.RepoRef, num string) (ghcli.IssueDetail, error) {
		f.calls.Add(1)
		*f.last = r
		return ghcli.IssueDetail{Title: "issue " + num}, nil
	}
	t.Cleanup(func() { previewNow, fetchPR, fetchIssue = oldNow, oldPR, oldIssue })
	m := paneModel(t, 140, 0)
	var prs []prListItem
	for i := 1; i <= n; i++ {
		prs = append(prs, prListItem{Number: i, Title: "t", UpdatedAt: "2026-01-01T00:00:00Z"})
	}
	m.tbl = prRows(prs, "")
	f.m, _ = m.previewSync()
	return f
}

// move selects row i and returns the cmd previewSync gave.
func (f *previewFixture) move(i int) tea.Cmd {
	f.m.cursor = i
	var cmd tea.Cmd
	f.m, cmd = f.m.previewSync()
	return cmd
}

// run feeds a tick through the model and, if it asked for a fetch, delivers the answer.
func (f *previewFixture) settle(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	for _, c := range flatten(cmd) {
		msg := c()
		if tm, ok := msg.(previewTickMsg); ok {
			next, fetch := f.m.previewTick(tm)
			f.m = next.(model)
			if fetch != nil {
				f.m = f.m.previewLoaded(fetch().(previewLoadedMsg))
			}
		}
	}
}

func flatten(c tea.Cmd) []tea.Cmd { return []tea.Cmd{c} }

func TestHoldingJFetchesOnlyTheRowItRestsOn(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 30)
	var cmds []tea.Cmd
	for i := 0; i < 20; i++ {
		cmds = append(cmds, f.move(i))
	}
	for _, c := range cmds { // every tick fires, only the last one is current
		f.settle(c)
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("20 moves made %d fetches, want 1", got)
	}
	if e, ok := f.m.previewCurrent(); !ok || e.pr == nil || e.pr.Number != 20 {
		t.Errorf("the pane should hold row 20: %+v", e)
	}
}

func TestAnOlderResultIsCachedButNotShown(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 5)
	c0 := f.move(1)
	tm := c0().(previewTickMsg)
	next, fetch := f.m.previewTick(tm)
	f.m = next.(model)
	f.move(3) // the selection moves while the fetch is in flight
	f.m = f.m.previewLoaded(fetch().(previewLoadedMsg))
	if _, shown := f.m.previewCurrent(); shown {
		t.Error("the result of row 2 must not show for row 4")
	}
	if _, cached := f.m.preview.cache[previewKey{"github.com", "acme/api", "pr", 2}]; !cached {
		t.Error("but it stays cached")
	}
	c := f.move(1) // back to row 2: shown at once, no new fetch
	if c != nil {
		t.Error("a cached row needs no tick")
	}
	if _, shown := f.m.previewCurrent(); !shown {
		t.Error("row 1 should show from the cache")
	}
}

func TestCacheLifetimeAndNewerRowsRefetch(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	f.settle(f.move(1))
	f.settle(f.move(2))
	f.now = f.now.Add(4 * time.Minute)
	if c := f.move(1); c != nil {
		t.Error("within 5 minutes the cached detail is reused")
	}
	f.move(2)
	f.now = f.now.Add(2 * time.Minute)
	if c := f.move(1); c == nil {
		t.Error("after 5 minutes it is fetched again")
	}
	f.settle(nil)
	f.move(2)
	f.m.tbl.rows[1].updated = "2026-01-02T00:00:00Z" // the list says the PR changed
	if c := f.move(1); c == nil {
		t.Error("a row updated after the cached detail is fetched again")
	}
}

func TestLeastRecentlyUsedGoesFirst(t *testing.T) {
	st := newPreviewState()
	for i := 0; i < previewCap; i++ {
		st.put(previewKey{"h", "a/b", "pr", i}, previewEntry{})
	}
	st.get(previewKey{"h", "a/b", "pr", 0}) // 0 is now the newest
	st.put(previewKey{"h", "a/b", "pr", 999}, previewEntry{})
	if len(st.cache) != previewCap {
		t.Fatalf("%d entries", len(st.cache))
	}
	if _, ok := st.cache[previewKey{"h", "a/b", "pr", 1}]; ok {
		t.Error("entry 1 was the least recently used")
	}
	if _, ok := st.cache[previewKey{"h", "a/b", "pr", 0}]; !ok {
		t.Error("entry 0 was used last and must stay")
	}
}

func TestErrorsAreCachedForThirtySecondsOnly(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	f.fail.Store(true)
	f.settle(f.move(1))
	if e, ok := f.m.previewCurrent(); !ok || e.err == nil {
		t.Fatal("the error should show")
	}
	f.move(2)
	f.now = f.now.Add(10 * time.Second)
	if c := f.move(1); c != nil {
		t.Error("an error is reused for 30 s")
	}
	f.move(2)
	f.now = f.now.Add(time.Minute)
	f.fail.Store(false)
	if c := f.move(1); c == nil {
		t.Error("an old error is retried")
	}
}

func TestHostsAreDifferentKeysAndInboxRowsUseTheirOwnRepo(t *testing.T) {
	a := previewKey{normHost(""), "acme/api", "pr", 1}
	b := previewKey{normHost("ghe.example.com"), "acme/api", "pr", 1}
	if a == b {
		t.Error("the same repo and number on two hosts are different entries")
	}
	m := paneModel(t, 140, 0)
	m.view = viewInbox
	items := []ghcli.InboxItem{{Host: "ghe.example.com", Repo: "corp/tool", Number: 7, Kind: "issue", UpdatedAt: "2026-01-01T00:00:00Z"}}
	st := inboxState{}
	st.tabs[0].items = items
	_, rows := inboxRows(0, st.tabs[0])
	m.tbl = table{cols: []string{"a"}, rows: rows}
	m = m.withScreenData(viewInbox, st)
	key, tg, _, ok := m.previewTarget()
	if !ok || key.host != "ghe.example.com" || key.repo != "corp/tool" || key.kind != "issue" || key.number != 7 {
		t.Errorf("key %+v ok=%v", key, ok)
	}
	if tg != (ghcli.RepoRef{Host: "ghe.example.com", Owner: "corp", Name: "tool"}) {
		t.Errorf("fetch target %+v: it must be the item's own repo, not the current directory's", tg)
	}
}

func TestLeavingTheListDropsPendingTicks(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	tm := f.move(1)().(previewTickMsg)
	f.m.view = viewRuns // left the screen
	f.m, _ = f.m.previewSync()
	if _, fetch := f.m.previewTick(tm); fetch != nil {
		t.Error("a tick that outlived the screen must do nothing")
	}
	if f.calls.Load() != 0 {
		t.Error("no fetch")
	}
}

func TestMovingAwayAndBackWithinThePauseFetchesOnce(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 5)
	first := f.move(2)
	f.move(3)
	second := f.move(2) // back on row 3 before the first tick fired
	f.settle(first)
	f.settle(second)
	if got := f.calls.Load(); got != 1 {
		t.Errorf("%d fetches for one row, want 1", got)
	}
}

func TestStayingOnARowRefreshesItWhenTheListSaysItChanged(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	f.settle(f.move(1))
	f.m.tbl.rows[1].updated = "2026-02-01T00:00:00Z" // a refresh brought a newer updatedAt
	var cmd tea.Cmd
	f.m, cmd = f.m.previewSync()
	if cmd == nil {
		t.Fatal("the pane must refetch the row it stays on")
	}
	f.settle(cmd)
	if f.calls.Load() != 2 {
		t.Errorf("%d fetches, want 2", f.calls.Load())
	}
	if _, cmd = f.m.previewSync(); cmd != nil {
		t.Error("and not again once it is fresh")
	}
	// the cache lifetime ends while the row stays selected
	f.now = f.now.Add(6 * time.Minute)
	if _, cmd = f.m.previewSync(); cmd == nil {
		t.Error("an old entry on the selected row is fetched again")
	}
}

func TestAHungFetchDoesNotBlockTheRowForever(t *testing.T) {
	old := previewDelay
	previewDelay = time.Millisecond
	defer func() { previewDelay = old }()
	f := newPreviewFixture(t, 3)
	tm := f.move(1)().(previewTickMsg)
	next, fetch := f.m.previewTick(tm) // the fetch is started and never answers
	f.m = next.(model)
	_ = fetch
	f.move(2)
	if c := f.move(1); c != nil {
		t.Error("while the fetch may still answer there is nothing to start")
	}
	f.now = f.now.Add(inflightFor + time.Second)
	f.move(2)
	if c := f.move(1); c == nil {
		t.Error("a fetch that never answered must not block the row for the rest of the session")
	}
}

func TestEveryRowOfAScrolledListIsVisibleWithThePane(t *testing.T) {
	f := newPreviewFixture(t, 60)
	m := f.m
	m.width, m.height = 160, 30
	m.tbl = prRows(func() []prListItem {
		var out []prListItem
		for i := 1; i <= 60; i++ {
			out = append(out, prListItem{Number: i, Title: fmt.Sprintf("title-%03d", i)})
		}
		return out
	}(), "")
	for i := 0; i < 60; i++ {
		m.cursor = i
		m.scroll()
		if v := ansi.Strip(m.View()); !strings.Contains(v, fmt.Sprintf("title-%03d", i+1)) {
			t.Fatalf("cursor on row %d: its row is not on screen (top=%d)", i, m.top)
		}
	}
}
