package tui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

type inboxFakes struct {
	hosts   []string
	items   map[string][]ghcli.InboxItem // key: host/tab
	totals  map[string]int
	failing map[string]error // host -> error
	calls   int
	now     time.Time
	mu      sync.Mutex // the fetches run on several goroutines
}

func inboxKeyOf(host string, tab ghcli.InboxTab) string { return fmt.Sprintf("%s/%d", host, tab) }

func installInbox(t *testing.T) *inboxFakes {
	t.Helper()
	f := &inboxFakes{hosts: []string{"github.com"}, items: map[string][]ghcli.InboxItem{}, totals: map[string]int{},
		failing: map[string]error{}, now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	oldH, oldL, oldF, oldN := inboxHosts, inboxLogin, inboxFetch, inboxNow
	inboxHosts = func() ([]string, error) { return f.hosts, nil }
	inboxLogin = func(host string) (string, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if err := f.failing[host]; err != nil {
			return "", err
		}
		return "dana", nil
	}
	inboxFetch = func(host string, tab ghcli.InboxTab, login string) ([]ghcli.InboxItem, int, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if err := f.failing[host]; err != nil {
			return nil, 0, err
		}
		items := f.items[inboxKeyOf(host, tab)]
		total := f.totals[inboxKeyOf(host, tab)]
		if total == 0 {
			total = len(items)
		}
		return items, total, nil
	}
	inboxNow = func() time.Time { return f.now }
	t.Cleanup(func() { inboxHosts, inboxLogin, inboxFetch, inboxNow = oldH, oldL, oldF, oldN })
	return f
}

func issueItem(host, repo string, n int, title, updated string) ghcli.InboxItem {
	return ghcli.InboxItem{Host: host, Repo: repo, Number: n, Kind: "issue", Title: title, Author: "erin", UpdatedAt: updated,
		URL: fmt.Sprintf("https://%s/%s/issues/%d", host, repo, n)}
}

func prItem(host, repo string, n int, title, updated string) ghcli.InboxItem {
	return ghcli.InboxItem{Host: host, Repo: repo, Number: n, Kind: "pr", Title: title, Author: "alice", UpdatedAt: updated,
		Checks: "SUCCESS", ReviewDecision: "REVIEW_REQUIRED", URL: fmt.Sprintf("https://%s/%s/pull/%d", host, repo, n)}
}

func openInbox(t *testing.T) model {
	t.Helper()
	m := model{view: viewRuns, width: 140, height: 30, target: acme, base: acme}
	return keyAll(m, "0")
}

// poll does what a tick does for a screen that polls: fetch again if the screen asks to be polled.
func poll(t *testing.T, m model) model {
	t.Helper()
	if !m.shouldPoll() {
		t.Fatal("the inbox must ask to be polled")
	}
	return pump(m, m.fetch())
}

func inboxSt(m model) inboxState { return m.screenData(viewInbox).(inboxState) }

func TestZeroAndTheCommandOpenTheInboxOnAssignedIssues(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 14812, "auth login hangs", "2026-10-24T10:00:00Z")}
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = []ghcli.InboxItem{prItem("github.com", "acme/web", 482, "Fix retry", "2026-10-24T09:00:00Z"), prItem("github.com", "acme/web", 7, "Other", "2026-10-23T09:00:00Z")}
	m := openInbox(t)
	v := ansi.Strip(m.View())
	for _, want := range []string{"Assigned (1)", "Review requested (2)", "My PRs (0)", "Mentions (0)", "acme/api", "14812", "auth login hangs"} {
		if !strings.Contains(v, want) {
			t.Errorf("0 should open the inbox on the assigned issues; lacks %q:\n%s", want, v)
		}
	}
	if m.view != viewInbox || inboxSt(m).tab != 0 {
		t.Errorf("view=%v tab=%d", m.view, inboxSt(m).tab)
	}
	m2 := keyAll(model{view: viewRuns, width: 140, height: 30, target: acme, base: acme}, ":")
	m2 = typeAll(m2, "inbox")
	m2 = keyAll(m2, "enter")
	if m2.view != viewInbox {
		t.Errorf(":inbox opened %v", m2.view)
	}
}

func TestTabsSwitchAndKeepTheirOwnRows(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{
		issueItem("github.com", "acme/api", 1, "first", "2026-10-24T10:00:00Z"), issueItem("github.com", "acme/api", 2, "second", "2026-10-24T09:00:00Z")}
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = []ghcli.InboxItem{prItem("github.com", "acme/web", 482, "Fix retry", "2026-10-24T09:00:00Z")}
	m := openInbox(t)
	m = keys(m, "j") // second issue
	m = keys(m, "tab")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Fix retry") || strings.Contains(v, "first") {
		t.Errorf("tab should show the review-requested PRs:\n%s", v)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "SUCCESS") && !strings.Contains(v, "✓") {
		t.Errorf("PR rows show their checks:\n%s", v)
	}
	m = keys(m, "shift+tab")
	if r, _ := m.selected(); r.cells[1] != "2" && !strings.Contains(strings.Join(r.cells, " "), "second") {
		t.Errorf("the first tab remembers its cursor: %+v", r)
	}
	m = keys(m, "l", "l", "h")
	if inboxSt(m).tab != 1 {
		t.Errorf("l l h should end on tab 1, got %d", inboxSt(m).tab)
	}
}

func TestHostsAreMergedNewestFirstAndAFailingHostIsANoteNotAnError(t *testing.T) {
	f := installInbox(t)
	f.hosts = []string{"github.com", "ghe.corp"}
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "older on github", "2026-10-24T08:00:00Z")}
	f.items[inboxKeyOf("ghe.corp", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("ghe.corp", "acme/api", 1, "newer on ghe", "2026-10-24T11:00:00Z")}
	m := openInbox(t)
	v := ansi.Strip(m.View())
	if strings.Index(v, "newer on ghe") > strings.Index(v, "older on github") || strings.Index(v, "newer on ghe") < 0 {
		t.Errorf("rows are merged newest first:\n%s", v)
	}
	if !strings.Contains(v, "ghe.corp/acme/api") {
		t.Errorf("an Enterprise row shows its host:\n%s", v)
	}
	if strings.Contains(v, "github.com/acme/api") {
		t.Errorf("github.com rows do not show the host:\n%s", v)
	}
	f.failing["ghe.corp"] = errors.New("HTTP 401: Bad credentials")
	f.now = f.now.Add(2 * time.Minute)
	m = poll(t, m)
	m = keyAll(m, "R")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "ghe.corp") || !strings.Contains(v, "Bad credentials") || !strings.Contains(v, "older on github") {
		t.Errorf("the failing host gives a note and the other host still shows:\n%s", v)
	}
}

func TestEveryHostFailingShowsNotesAndAnEmptyList(t *testing.T) {
	f := installInbox(t)
	f.failing["github.com"] = errors.New("dial tcp: no such host")
	m := openInbox(t)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "no such host") || !strings.Contains(v, "nothing assigned to you") {
		t.Errorf("view:\n%s", v)
	}
}

func TestEmptyTabsSaySoAndTheCapIsExplained(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	f.totals[inboxKeyOf("github.com", ghcli.InboxAssigned)] = 130
	m := openInbox(t)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "129 more on GitHub") {
		t.Errorf("a capped tab says how many more there are:\n%s", v)
	}
	m = keys(m, "tab", "tab", "tab")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "no mentions") && !strings.Contains(v, "nothing") {
		t.Errorf("an empty tab says so:\n%s", v)
	}
}

func TestFilterWorksOnTheLoadedRows(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{
		issueItem("github.com", "acme/api", 1, "auth hangs", "2026-10-24T10:00:00Z"), issueItem("github.com", "acme/api", 2, "typo in docs", "2026-10-24T09:00:00Z")}
	m := openInbox(t)
	calls := f.calls
	m = keys(m, "/", "t", "y", "p", "o")
	next, _ := m.Update(k("enter"))
	if v := ansi.Strip(next.(model).View()); strings.Contains(v, "auth hangs") || !strings.Contains(v, "typo in docs") {
		t.Errorf("filter:\n%s", v)
	}
	if f.calls != calls {
		t.Error("filtering must not ask GitHub again")
	}
}

func TestRefreshIsThrottledAndTheSelectionStaysOnTheSameItem(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{
		issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z"), issueItem("github.com", "acme/api", 2, "two", "2026-10-24T09:00:00Z")}
	m := openInbox(t)
	first := f.calls
	if first != 4 {
		t.Fatalf("one host, four tabs = 4 searches, got %d", first)
	}
	m = poll(t, m) // a tick right after: nothing to ask
	if f.calls != first {
		t.Errorf("a tick within a minute asked GitHub again (%d calls)", f.calls)
	}
	m = keys(m, "j") // on "two"
	f.now = f.now.Add(61 * time.Second)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{
		issueItem("github.com", "acme/api", 3, "brand new", "2026-10-24T12:00:00Z"),
		issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z"), issueItem("github.com", "acme/api", 2, "two", "2026-10-24T09:00:00Z")}
	m = poll(t, m)
	if f.calls != first+4 {
		t.Errorf("a minute later the tick refreshes (%d calls)", f.calls)
	}
	if r, _ := m.selected(); !strings.Contains(strings.Join(r.cells, " "), "two") {
		t.Errorf("selection = %+v; it stays on the same issue even though a new one came in above it", r)
	}
	// a manual refresh straight after is held back
	before := f.calls
	m = keyAll(m, "R")
	if f.calls != before {
		t.Errorf("R within 5 s of a refresh must not search again")
	}
	f.now = f.now.Add(6 * time.Second)
	m = keyAll(m, "R")
	if f.calls != before+4 {
		t.Errorf("R after 5 s refreshes (%d calls)", f.calls)
	}
}

func TestNewRowsAreMarkedUntilTheTabIsLeft(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	m := openInbox(t)
	if v := ansi.Strip(m.View()); strings.Contains(v, "●") {
		t.Errorf("nothing is new on the first load:\n%s", v)
	}
	f.now = f.now.Add(61 * time.Second)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{
		issueItem("github.com", "acme/api", 2, "newcomer", "2026-10-24T12:00:00Z"), issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	m = poll(t, m)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	marked := 0
	for _, l := range lines {
		if strings.Contains(l, "●") {
			marked++
			if !strings.Contains(l, "newcomer") {
				t.Errorf("the mark is on the wrong row: %q", l)
			}
		}
	}
	if marked != 1 {
		t.Errorf("%d marked rows; want only the newcomer", marked)
	}
	m = keys(m, "tab", "shift+tab")
	if v := ansi.Strip(m.View()); strings.Contains(v, "●") {
		t.Errorf("leaving the tab clears the mark:\n%s", v)
	}
}

func TestARowThatMovesBetweenTabsIsNotNew(t *testing.T) {
	f := installInbox(t)
	pr := prItem("github.com", "acme/web", 9, "moves", "2026-10-24T10:00:00Z")
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = []ghcli.InboxItem{pr}
	m := openInbox(t)
	f.now = f.now.Add(61 * time.Second)
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = nil
	f.items[inboxKeyOf("github.com", ghcli.InboxMine)] = []ghcli.InboxItem{pr}
	m = poll(t, m)
	m = keys(m, "tab", "tab")
	if v := ansi.Strip(m.View()); strings.Contains(v, "●") || !strings.Contains(v, "moves") {
		t.Errorf("a PR that moved from one tab to another is not a newcomer:\n%s", v)
	}
}

func TestALateResultIsDropped(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	m := openInbox(t)
	late := screenMsg{view: viewInbox, id: "something else", data: inboxPayload{}}
	next, _ := m.Update(late)
	if len(inboxSt(next.(model)).tabs[0].items) != 1 {
		t.Error("a result for another identity replaced the inbox")
	}
	left := m
	left.view = viewRuns
	next, _ = left.Update(screenMsg{view: viewInbox, id: "inbox", data: inboxPayload{}})
	if next.(model).view != viewRuns {
		t.Error("a late inbox result must not pull the user back")
	}
}

func TestSwitchingRepoDoesNotChangeTheInbox(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	m := openInbox(t)
	before := f.calls
	m.base, m.target = ghcli.RepoRef{Owner: "other", Name: "repo"}, ghcli.RepoRef{Owner: "other", Name: "repo"}
	m = poll(t, m)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "one") || f.calls != before {
		t.Errorf("the inbox is not about one repo; calls %d -> %d\n%s", before, f.calls, v)
	}
}

func TestAHostThatFailsKeepsItsLastRowsWithANote(t *testing.T) {
	f := installInbox(t)
	f.hosts = []string{"github.com", "ghe.corp"}
	f.items[inboxKeyOf("ghe.corp", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("ghe.corp", "acme/api", 1, "from ghe", "2026-10-24T11:00:00Z")}
	m := openInbox(t)
	f.failing["ghe.corp"] = errors.New("i/o timeout")
	f.now = f.now.Add(2 * time.Minute)
	m = poll(t, m)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "from ghe") || !strings.Contains(v, "i/o timeout") {
		t.Errorf("a transient failure keeps the last answer and says what happened:\n%s", v)
	}
}

func TestEnterOpensAPRInItsOwnRepoAndEscComesBackToTheSameRow(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = []ghcli.InboxItem{
		prItem("github.com", "acme/web", 482, "Fix retry", "2026-10-24T10:00:00Z"), prItem("github.com", "other/lib", 7, "Second", "2026-10-24T09:00:00Z")}
	installPRFetch(t, fixturePR())
	m := openInbox(t)
	m = keys(m, "tab", "j") // second row of the review-requested tab
	m = keys(m, "enter")
	if m.view != viewPRDetail || m.pr != "7" || m.target != (ghcli.RepoRef{Owner: "other", Name: "lib"}) || m.prTarget != m.target {
		t.Fatalf("view=%v pr=%q target=%+v prTarget=%+v; want PR 7 of other/lib", m.view, m.pr, m.target, m.prTarget)
	}
	m = keys(m, "esc")
	if m.view != viewInbox || m.target != m.base {
		t.Fatalf("view=%v target=%+v; back in the inbox with the repo we came from", m.view, m.target)
	}
	m = pump(m, m.fetch())
	if st := inboxSt(m); st.tab != 1 {
		t.Errorf("tab = %d; want the same tab", st.tab)
	}
	if r, _ := m.selected(); r.id != "github.com/other/lib#7" {
		t.Errorf("selected %q; want the same row", r.id)
	}
}

func TestEnterpriseItemsOpenInTheirHostAndNothingCarriesOver(t *testing.T) {
	f := installInbox(t)
	f.hosts = []string{"github.com", "ghe.corp"}
	f.items[inboxKeyOf("ghe.corp", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("ghe.corp", "corp/tool", 3, "on ghe", "2026-10-24T11:00:00Z")}
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 9, "on github", "2026-10-24T10:00:00Z")}
	fk := installIssues(t, fixtureIssue())
	m := openInbox(t)
	m = keys(m, "enter") // newest first: the Enterprise issue
	next, _ := m.Update(m.fetch()())
	m = next.(model)
	want := ghcli.RepoRef{Host: "ghe.corp", Owner: "corp", Name: "tool"}
	if m.view != viewIssue || m.issue != "3" || m.target != want || len(fk.viewTargets) != 1 || fk.viewTargets[0] != want {
		t.Fatalf("view=%v issue=%q target=%+v fetched=%v; want issue 3 of ghe.corp/corp/tool", m.view, m.issue, m.target, fk.viewTargets)
	}
	m = keys(m, "esc")
	m = pump(m, m.fetch())
	m = keys(m, "j", "enter") // the github.com issue
	next, _ = m.Update(m.fetch()())
	m = next.(model)
	if m.target != (ghcli.RepoRef{Owner: "acme", Name: "api"}) || len(fk.viewTargets) != 2 || fk.viewTargets[1].Host != "" {
		t.Errorf("target=%+v fetched=%v; the first item's host must not carry over", m.target, fk.viewTargets)
	}
}

func TestPRActionsFromTheInboxActOnTheItemsRepo(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = []ghcli.InboxItem{prItem("github.com", "other/lib", 7, "Second", "2026-10-24T09:00:00Z")}
	installPRFetch(t, fixturePR())
	comments := installPRActions(t)
	m := openInbox(t)
	m = keys(m, "tab", "enter")
	m = openDetail2(t, m)
	m = keyAll(m, "C")
	m = typeAll(m, "hello")
	m = keyAll(m, "enter")
	if len(comments.comments) != 1 || comments.comments[0].repo != (ghcli.RepoRef{Owner: "other", Name: "lib"}) || comments.comments[0].number != "7" {
		t.Errorf("comments = %+v; want a comment on other/lib#7", comments.comments)
	}
}

// openDetail2 delivers the detail the PR screen fetched.
func openDetail2(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.Update(m.fetch()())
	return next.(model)
}

func TestOOpensTheItemInTheBrowserAndAVanishedRowDoesNothing(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	opened := recordOpenURL(t)
	m := openInbox(t)
	m = keys(m, "o")
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/acme/api/issues/1" {
		t.Errorf("opened %v", *opened)
	}
	st := inboxSt(m)
	st.tabs[0].items = nil // refreshed away while the table still shows it
	m = m.withScreenData(viewInbox, st)
	m = keys(m, "enter")
	if m.view != viewInbox {
		t.Errorf("view = %v; a row that is gone opens nothing", m.view)
	}
}
