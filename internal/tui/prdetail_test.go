package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func fixturePR() ghcli.PRDetail {
	return ghcli.PRDetail{
		Number: 482, Title: "Fix flaky retry in deploy", Author: "alice", State: "OPEN",
		BaseRef: "main", HeadRef: "fix/retry", URL: "https://github.com/acme/api/pull/482",
		ReviewDecision: "REVIEW_REQUIRED", Mergeable: "MERGEABLE", MergeState: "BLOCKED",
		Additions: 40, Deletions: 7, ChangedFiles: 3, Labels: []string{"bug"},
		ChecksPassed: 3, ChecksFailed: 1, ChecksPending: 2,
		Body:     "Retries the deploy step.\n\nCloses #12",
		Comments: []ghcli.Comment{{Author: "bob", Body: "Looks good to me", CreatedAt: "2026-10-20T10:00:00Z"}},
		Reviews:  []ghcli.Review{{Author: "carol", State: "CHANGES_REQUESTED", Body: "Please add a test", CreatedAt: "2026-10-21T09:00:00Z"}},
	}
}

type prFakes struct{ targets []ghcli.RepoRef }

func installPRFetch(t *testing.T, d ghcli.PRDetail) *prFakes {
	t.Helper()
	f := &prFakes{}
	old := fetchPR
	fetchPR = func(r ghcli.RepoRef, number string) (ghcli.PRDetail, error) {
		f.targets = append(f.targets, r)
		return d, nil
	}
	t.Cleanup(func() { fetchPR = old })
	return f
}

func prListModel() model {
	return model{view: viewPRs, width: 120, height: 30,
		tbl: table{cols: []string{"#", "CHECKS", "TITLE"}, rows: []row{
			{id: "482", cells: []string{"482", "✓ 3/3", "Fix flaky retry in deploy"}},
		}}}
}

// openDetail presses enter on the first PR and delivers the fetched detail.
func openDetail(t *testing.T, m model) model {
	t.Helper()
	m = keys(m, "enter")
	if m.view != viewPRDetail {
		t.Fatalf("view = %v after enter on a PR; want the detail screen", m.view)
	}
	next, _ := m.Update(m.fetch()())
	return next.(model)
}

func TestPRListFilterKeysSetTheFilterAndRefetch(t *testing.T) {
	m := prListModel()
	for key, want := range map[string]ghcli.PRFilter{
		"u": {Mine: true}, "e": {ReviewRequested: true}, "a": {},
	} {
		next, cmd := m.Update(k(key))
		m = next.(model)
		if m.prFilter != want || cmd == nil || !m.loading {
			t.Errorf("key %q: filter=%+v refetch=%v loading=%v; want %+v with a refetch", key, m.prFilter, cmd != nil, m.loading, want)
		}
	}
}

func TestPRListArgsIncludeTheFilter(t *testing.T) {
	args := prListArgs(acme, ghcli.PRFilter{ReviewRequested: true})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--search review-requested:@me") || !strings.Contains(joined, "-R acme/api") || args[0] != "pr" {
		t.Errorf("args = %v", args)
	}
	if got := prListArgs(ghcli.RepoRef{}, ghcli.PRFilter{}); strings.Contains(strings.Join(got, " "), "--search") || strings.Contains(strings.Join(got, " "), "--author") {
		t.Errorf("no filter should add no flags: %v", got)
	}
}

func TestTitleShowsTheActiveFilter(t *testing.T) {
	m := prListModel()
	m.prFilter = ghcli.PRFilter{ReviewRequested: true}
	if v := m.View(); !strings.Contains(v, "review requested") {
		t.Errorf("title should say which filter is active:\n%s", v)
	}
	m.prFilter = ghcli.PRFilter{}
	if v := m.View(); strings.Contains(v, "review requested") || strings.Contains(v, "opened by me") {
		t.Errorf("no filter, no label:\n%s", v)
	}
}

func TestEnterOnAPRFetchesAndShowsItsDetail(t *testing.T) {
	installPRFetch(t, fixturePR())
	m := openDetail(t, prListModel())
	v := m.View()
	for _, want := range []string{"#482", "Fix flaky retry in deploy", "alice", "main", "fix/retry", "REVIEW_REQUIRED",
		"BLOCKED", "3", "Retries the deploy step.", "Looks good to me", "bob", "carol", "Please add a test", "changes requested"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail lacks %q:\n%s", want, v)
		}
	}
}

func TestDetailWithNoBodyOrCommentsRenders(t *testing.T) {
	installPRFetch(t, ghcli.PRDetail{Number: 7, Title: "Tiny", Author: "ghost", State: "OPEN"})
	m := openDetail(t, prListModel())
	v := m.View()
	if !strings.Contains(v, "Tiny") || !strings.Contains(v, "no description") || !strings.Contains(v, "no comments") {
		t.Errorf("empty PR should still render with placeholders:\n%s", v)
	}
}

func TestDetailCKeyOpensChecksForThatPR(t *testing.T) {
	installPRFetch(t, fixturePR())
	m := openDetail(t, prListModel())
	m = keys(m, "c")
	if m.view != viewChecks || m.pr != "482" {
		t.Errorf("view=%v pr=%q; want the checks of PR 482", m.view, m.pr)
	}
}

func TestEscFromChecksReturnsToTheDetail(t *testing.T) {
	installPRFetch(t, fixturePR())
	m := openDetail(t, prListModel())
	m = keys(m, "c", "esc")
	if m.view != viewPRDetail {
		t.Errorf("view = %v; want the PR detail", m.view)
	}
}

func TestDetailUsesTargetNotBase(t *testing.T) {
	f := installPRFetch(t, fixturePR())
	m := prListModel()
	m.base = ghcli.RepoRef{Owner: "chosen", Name: "repo"}
	m.target = acme // a favorite from another repo is open
	openDetail(t, m)
	if len(f.targets) != 1 || f.targets[0] != acme {
		t.Errorf("fetched %v; want the repo the view shows (target), not base", f.targets)
	}
}

func TestDetailScrollsAndClamps(t *testing.T) {
	d := fixturePR()
	d.Body = strings.Repeat("a line of the description\n", 80)
	installPRFetch(t, d)
	m := openDetail(t, prListModel())
	m = keys(m, "k")
	if s := m.screenData(viewPRDetail).(prDetailState); s.scroll != 0 {
		t.Errorf("scroll = %d; cannot go above the top", s.scroll)
	}
	m = keys(m, "G")
	end := m.screenData(viewPRDetail).(prDetailState).scroll
	if end == 0 {
		t.Fatal("G should scroll to the end of a long description")
	}
	m = keys(m, "j", "j")
	if s := m.screenData(viewPRDetail).(prDetailState); s.scroll != end {
		t.Errorf("scroll = %d; cannot go past the end (%d)", s.scroll, end)
	}
	m = keys(m, "g")
	if s := m.screenData(viewPRDetail).(prDetailState); s.scroll != 0 {
		t.Errorf("g should return to the top, scroll = %d", s.scroll)
	}
}

func TestWrap(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want []string
	}{
		{"hello world foo", 11, []string{"hello world", "foo"}},
		{"a\n\nb", 20, []string{"a", "", "b"}},
		{"averyveryverylongword", 8, []string{"averyver", "yverylon", "gword"}},
		{"", 10, []string{""}},
		{"crlf\r\nline", 20, []string{"crlf", "line"}},
	}
	for _, c := range cases {
		if got := wrap(c.in, c.w); !reflect.DeepEqual(got, c.want) {
			t.Errorf("wrap(%q, %d) = %q; want %q", c.in, c.w, got, c.want)
		}
	}
}
