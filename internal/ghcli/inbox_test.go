package ghcli

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseAuthHosts(t *testing.T) {
	b := []byte(`{"hosts":{"ghe.corp":[{"state":"success","active":true,"host":"ghe.corp","login":"me"}],
		"github.com":[{"state":"success","active":true,"host":"github.com","login":"me"}],
		"another.example":[{"state":"error","active":true,"host":"another.example","login":"me"}]}}`)
	got := ParseAuthHosts(b)
	want := []string{"github.com", "another.example", "ghe.corp"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hosts = %q; want github.com first, then the rest in order: %q", got, want)
	}
	if got := ParseAuthHosts([]byte(`{nope`)); got != nil {
		t.Errorf("garbage gives no hosts: %q", got)
	}
	if got := ParseAuthHosts([]byte(`{"hosts":{"bad host!":[{}],"-x":[{}],"ok.com":[{}]}}`)); !reflect.DeepEqual(got, []string{"ok.com"}) {
		t.Errorf("a host name that is not a host name is dropped: %q", got)
	}
}

func TestInboxSearch(t *testing.T) {
	cases := map[InboxTab]string{
		InboxAssigned:        "is:issue is:open assignee:dana archived:false sort:updated-desc",
		InboxReviewRequested: "is:pr is:open review-requested:dana archived:false sort:updated-desc",
		InboxMine:            "is:pr is:open author:dana archived:false sort:updated-desc",
		InboxMentions:        "is:open mentions:dana -author:dana archived:false sort:updated-desc",
	}
	for tab, want := range cases {
		if got := InboxSearch(tab, "dana"); got != want {
			t.Errorf("tab %d: %q; want %q", tab, got, want)
		}
	}
	for _, bad := range []string{"", "da na", "dana archived:true", "-dana", "d;rm", "dana\n"} {
		if got := InboxSearch(InboxAssigned, bad); got != "" {
			t.Errorf("login %q must not make a query, got %q", bad, got)
		}
	}
	if got := InboxSearch(InboxMine, "dependabot[bot]"); !strings.Contains(got, "author:dependabot[bot]") {
		t.Errorf("a bot login is a login: %q", got)
	}
}

func TestInboxArgs(t *testing.T) {
	got := InboxArgs("github.com", "is:issue assignee:dana")
	if got[0] != "api" || got[1] != "graphql" {
		t.Fatalf("args = %q", got)
	}
	if !reflect.DeepEqual(got[2:4], []string{"--hostname", "github.com"}) {
		t.Errorf("github.com is named too: a GH_HOST in the environment must not redirect the search (%q)", got)
	}
	has := func(args []string, s string) bool {
		for i, a := range args {
			if a == "-f" && i+1 < len(args) && args[i+1] == s {
				return true
			}
		}
		return false
	}
	if !has(got, "q=is:issue assignee:dana") {
		t.Errorf("the search travels as its own -f argument: %q", got)
	}
	ghe := InboxArgs("ghe.corp", "x")
	if !reflect.DeepEqual(ghe[2:4], []string{"--hostname", "ghe.corp"}) {
		t.Errorf("enterprise args = %q", ghe)
	}
}

const inboxJSON = `{"data":{"search":{"issueCount":130,"nodes":[
 {"__typename":"Issue","number":14812,"title":"auth login hangs","url":"https://github.com/acme/api/issues/14812","updatedAt":"2026-10-24T10:00:00Z","author":{"login":"dana"},"repository":{"nameWithOwner":"acme/api"}},
 {"__typename":"PullRequest","number":482,"title":"Fix \u001b]52;c;AAAA\u0007 retry\nsecond line","url":"https://github.com/acme/web/pull/482","updatedAt":"2026-10-24T09:00:00Z","isDraft":true,"reviewDecision":"REVIEW_REQUIRED","author":{"login":"alice"},"repository":{"nameWithOwner":"acme/web"},"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},
 {"__typename":"PullRequest","number":7,"title":"ghost's work","url":"https://github.com/acme/web/pull/7","updatedAt":"2026-10-23T09:00:00Z","author":null,"repository":{"nameWithOwner":"acme/web"},"commits":{"nodes":[]}},
 null,
 {"__typename":"Issue"}
]}}}`

func TestParseInbox(t *testing.T) {
	items, total, err := ParseInbox("github.com", []byte(inboxJSON))
	if err != nil {
		t.Fatal(err)
	}
	if total != 130 {
		t.Errorf("total = %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("%d items; the null node and the empty one are skipped: %+v", len(items), items)
	}
	i := items[0]
	if i.Kind != "issue" || i.Repo != "acme/api" || i.Number != 14812 || i.Author != "dana" || i.Host != "github.com" || i.URL == "" {
		t.Errorf("issue = %+v", i)
	}
	p := items[1]
	if p.Kind != "pr" || !p.Draft || p.ReviewDecision != "REVIEW_REQUIRED" || p.Checks != "FAILURE" {
		t.Errorf("pr = %+v", p)
	}
	if strings.ContainsAny(p.Title, "\x1b\x07\n") {
		t.Errorf("the title must be clean: %q", p.Title)
	}
	if g := items[2]; g.Author != "ghost" || g.Checks != "" {
		t.Errorf("a deleted author is 'ghost' and no checks is empty: %+v", g)
	}
	e, _, _ := ParseInbox("ghe.corp", []byte(inboxJSON))
	if e[0].Host != "ghe.corp" {
		t.Errorf("host = %q", e[0].Host)
	}
}

func TestParseInboxErrors(t *testing.T) {
	if _, _, err := ParseInbox("h", []byte(`{"errors":[{"message":"API rate limit exceeded"}]}`)); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("errors without data are an error carrying GitHub's words: %v", err)
	}
	items, _, err := ParseInbox("h", []byte(`{"data":{"search":{"issueCount":1,"nodes":[{"__typename":"Issue","number":1,"title":"t","url":"u","updatedAt":"x","author":{"login":"a"},"repository":{"nameWithOwner":"o/r"}}]}},"errors":[{"message":"Could not resolve to a Repository"}]}`))
	if err != nil || len(items) != 1 {
		t.Errorf("partial data with errors still shows what it has: %v %+v", err, items)
	}
	if items, total, err := ParseInbox("h", []byte(`{"data":{"search":{"issueCount":0,"nodes":[]}}}`)); err != nil || len(items) != 0 || total != 0 {
		t.Errorf("empty: %v %v %v", items, total, err)
	}
	if _, _, err := ParseInbox("h", []byte(`<html>`)); err == nil {
		t.Error("not JSON is an error")
	}
}

func TestLoginArgs(t *testing.T) {
	got := LoginArgs("ghe.corp")
	if got[0] != "api" || got[1] != "graphql" || !reflect.DeepEqual(got[2:4], []string{"--hostname", "ghe.corp"}) {
		t.Errorf("args = %q", got)
	}
	if login, err := ParseLogin([]byte(`{"data":{"viewer":{"login":"dana"}}}`)); err != nil || login != "dana" {
		t.Errorf("login = %q %v", login, err)
	}
	if _, err := ParseLogin([]byte(`{"data":{"viewer":{"login":"bad login"}}}`)); err == nil {
		t.Error("a login that is not a login is an error")
	}
	if _, err := ParseLogin([]byte(`{"errors":[{"message":"Bad credentials"}]}`)); err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("err = %v", err)
	}
}

func TestRunWithTimeoutGivesUp(t *testing.T) {
	start := time.Now()
	if _, err := RunWithTimeout(time.Nanosecond, "version"); err == nil {
		t.Error("a command that cannot finish in time is an error")
	}
	if time.Since(start) > 5*time.Second {
		t.Error("it must not wait")
	}
}
