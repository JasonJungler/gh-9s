package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/MurabitoB/gh-9s/internal/diff"
	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

func useNerd(t *testing.T) {
	t.Helper()
	old := icons.Current()
	icons.Use(icons.Nerd)
	t.Cleanup(func() { icons.Use(old) })
}

func g(n icons.Name) string { return icons.GetIn(icons.Nerd, n) }

func TestRunStatusLabelsUseTheIcons(t *testing.T) {
	cases := []struct {
		status, conclusion string
		icon               icons.Name
		text               string
	}{
		{"completed", "success", icons.RunOK, "success"}, {"completed", "failure", icons.RunFail, "failure"},
		{"in_progress", "", icons.RunActive, "in_progress"}, {"completed", "skipped", icons.RunOther, "skipped"},
	}
	for _, c := range cases {
		plain, _ := ui.RunState(c.status, c.conclusion)
		if want := icons.GetIn(icons.Plain, c.icon) + " " + c.text; plain != want {
			t.Errorf("plain %s/%s = %q; want %q", c.status, c.conclusion, plain, want)
		}
	}
	useNerd(t)
	for _, c := range cases {
		got, _ := ui.RunState(c.status, c.conclusion)
		if want := g(c.icon) + " " + c.text; got != want {
			t.Errorf("nerd %s/%s = %q; want %q", c.status, c.conclusion, got, want)
		}
	}
}

func TestPRChecksSummaryUsesTheIcons(t *testing.T) {
	ok := []rollupRef{{Status: "COMPLETED", Conclusion: "SUCCESS"}}
	bad := []rollupRef{{Status: "COMPLETED", Conclusion: "FAILURE"}}
	wait := []rollupRef{{Status: "IN_PROGRESS"}}
	if l, _ := rollupState(ok); l != "✓ 1/1" {
		t.Errorf("plain = %q", l)
	}
	useNerd(t)
	for _, c := range []struct {
		checks []rollupRef
		want   string
	}{{ok, g(icons.CheckPass) + " 1/1"}, {bad, g(icons.CheckFail) + " failing"}, {wait, g(icons.CheckPending) + " pending 0/1"}} {
		if got, _ := rollupState(c.checks); got != c.want {
			t.Errorf("nerd = %q; want %q", got, c.want)
		}
	}
}

func TestCheckRowsUseTheIcons(t *testing.T) {
	for bucket, icon := range map[string]icons.Name{"pass": icons.CheckPass, "fail": icons.CheckFail, "pending": icons.CheckPending, "skipping": icons.CheckSkipped} {
		plain, _ := checkStatus(bucket)
		if !strings.HasPrefix(plain, icons.GetIn(icons.Plain, icon)+" ") {
			t.Errorf("plain %s = %q", bucket, plain)
		}
	}
	useNerd(t)
	if got, st := checkStatus("fail"); got != g(icons.CheckFail)+" fail" || st != stFail {
		t.Errorf("nerd fail = %q %v", got, st)
	}
	if got, _ := checkStatus("cancel"); got != g(icons.CheckSkipped)+" cancel" {
		t.Errorf("an unknown bucket is shown with its name: %q", got)
	}
}

func TestPRAndIssueRowsGetAStateIconInNerdMode(t *testing.T) {
	prs := []prListItem{{Number: 12, Title: "open one", Head: "a"}, {Number: 13, Title: "draft one", Head: "b", Draft: true}}
	if got := prRows(prs, "").rows[0].cells[0]; got != "12" {
		t.Errorf("plain number cell = %q; plain screens are unchanged", got)
	}
	useNerd(t)
	rows := prRows(prs, "a").rows
	if rows[0].cells[0] != g(icons.PROpen)+" *12" || rows[1].cells[0] != g(icons.PRDraft)+" 13" {
		t.Errorf("cells = %q %q", rows[0].cells[0], rows[1].cells[0])
	}
	if issueNumberCell(5, "OPEN") != g(icons.IssueOpen)+" 5" || issueNumberCell(6, "CLOSED") != g(icons.IssueClosed)+" 6" {
		t.Errorf("issue cells = %q %q", issueNumberCell(5, "OPEN"), issueNumberCell(6, "CLOSED"))
	}
}

func TestInboxUsesTheIconsAndStaysAligned(t *testing.T) {
	f := installInbox(t)
	pr := prItem("github.com", "acme/web", 482, "Fix", "2026-10-24T10:00:00Z")
	pr.Draft = true
	f.items[inboxKeyOf("github.com", ghcli.InboxReviewRequested)] = []ghcli.InboxItem{pr, prItem("github.com", "acme/web", 7, "你好世界", "2026-10-24T09:00:00Z")}
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "an issue", "2026-10-24T10:00:00Z")}
	useNerd(t)
	m := openInbox(t)
	v := ansi.Strip(m.View())
	for _, want := range []string{g(icons.TabAssigned), g(icons.TabReview), g(icons.TabMine), g(icons.TabMentions), g(icons.IssueOpen) + " #1"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	m = keys(m, "tab")
	v = ansi.Strip(m.View())
	for _, want := range []string{g(icons.PRDraft) + " #482", g(icons.PROpen) + " #7", g(icons.CheckPass), g(icons.ReviewRequired)} {
		if !strings.Contains(v, want) {
			t.Errorf("review tab lacks %q:\n%s", want, v)
		}
	}
	widths := map[int]bool{}
	for _, r := range m.tbl.rows {
		widths[ansi.StringWidth(r.cells[0])] = true
	}
	if len(widths) != 1 {
		t.Errorf("the first column's cells have different widths: %v", widths)
	}
}

func TestNewMarkAndTheDiffListUseTheIcons(t *testing.T) {
	useNerd(t)
	st := diffState{d: diff.Diff{Files: []diff.File{{Path: "a.go", Added: 1}}}}
	line := treeLine(buildTree(st.d.Files, map[string]bool{"a.go": true}, nil, nil, nil)[0], false, true, 40)
	if !strings.Contains(line, g(icons.Selected)) || !strings.Contains(line, g(icons.Viewed)) {
		t.Errorf("selected and viewed marks: %q", line)
	}
	if w := ansi.StringWidth(line); w != 40 {
		t.Errorf("the list line is %d wide; want exactly 40", w)
	}
}

func TestNewRowsGetTheNewIconInNerdMode(t *testing.T) {
	f := installInbox(t)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	useNerd(t)
	m := openInbox(t)
	f.now = f.now.Add(2 * time.Minute)
	f.items[inboxKeyOf("github.com", ghcli.InboxAssigned)] = []ghcli.InboxItem{issueItem("github.com", "acme/api", 2, "two", "2026-10-24T11:00:00Z"), issueItem("github.com", "acme/api", 1, "one", "2026-10-24T10:00:00Z")}
	m = poll(t, m)
	if v := ansi.Strip(m.View()); !strings.Contains(v, g(icons.New)+" acme/api") {
		t.Errorf("the newcomer is marked with the new icon:\n%s", v)
	}
}

func TestIconsCarryTheirMeaningsColour(t *testing.T) {
	plain := prRows([]prListItem{{Number: 1, Title: "t"}}, "").rows[0]
	if len(plain.tint) != 0 {
		t.Errorf("plain rows have no tint: %v", plain.tint)
	}
	useNerd(t)
	rows := prRows([]prListItem{{Number: 1, Title: "open"}, {Number: 2, Title: "draft", Draft: true}}, "").rows
	if rows[0].tint[0] != icons.Color(icons.PROpen) || rows[1].tint[0] != icons.Color(icons.PRDraft) {
		t.Errorf("tints = %v %v", rows[0].tint, rows[1].tint)
	}
	if issueTint("OPEN") != icons.Color(icons.IssueOpen) || issueTint("CLOSED") != icons.Color(icons.IssueClosed) {
		t.Error("issue tints")
	}
}

func TestTintedCellsAreDrawnInTheirColourExceptOnTheSelectedRow(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	useNerd(t)
	m := model{view: viewPRs, width: 120, height: 20, tbl: prRows([]prListItem{{Number: 1, Title: "a"}, {Number: 2, Title: "b", Draft: true}, {Number: 3, Title: "c"}}, "")}
	body := m.tableBody()
	start := func(c string) string { // the escape code lipgloss opens a colour with
		return strings.Split(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("x"), "x")[0]
	}
	if !strings.Contains(body, start("8")) {
		t.Errorf("the draft's number cell should be grey:\n%q", body)
	}
	lines := strings.Split(body, "\n")
	if !strings.Contains(lines[3], start("2")) { // an open PR's number is green, and nothing else on its row is
		t.Errorf("the open PR's number cell should be green:\n%q", lines[3])
	}
	if strings.Contains(lines[1], start("2")) { // line 1 is the selected row: reverse video only
		t.Errorf("the selected row keeps one style:\n%q", lines[1])
	}
}

func TestSwitchingIconsDropsTheFavoritesStatusCache(t *testing.T) {
	m := model{view: viewFavs, latest: map[string]latestEntry{"x": {}}}
	m, _ = m.afterIconsChanged()
	if m.latest != nil {
		t.Error("the cached labels hold the old symbols: forget them")
	}
}

func TestPRDetailChecksLineUsesTheIcons(t *testing.T) {
	d := ghcli.PRDetail{Number: 1, Title: "t", ChecksPassed: 3, ChecksFailed: 1, ChecksPending: 2}
	if got := strings.Join(prDetailLines(d, 100).lines, "\n"); !strings.Contains(got, "Checks: ✓3 ✗1 ●2") {
		t.Errorf("plain:\n%s", got)
	}
	useNerd(t)
	got := strings.Join(prDetailLines(d, 100).lines, "\n")
	want := "Checks: " + g(icons.CheckPass) + "3 " + g(icons.CheckFail) + "1 " + g(icons.CheckPending) + "2"
	if !strings.Contains(got, want) {
		t.Errorf("nerd:\n%s", got)
	}
}
