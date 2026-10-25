package ghcli

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const discListJSON = `{"data":{"repository":{"hasDiscussionsEnabled":true,
"discussionCategories":{"nodes":[{"id":"C1","name":"Q&A\u001b[31m","isAnswerable":true},{"id":"C2","name":"Ideas","isAnswerable":false}]},
"discussions":{"totalCount":73,"nodes":[
{"id":"D1","number":42,"title":"How do I\nretry?","body":"body \u001b[1mone","url":"https://github.com/acme/app/discussions/42","updatedAt":"2026-10-01T00:00:00Z","closed":false,"locked":false,"isAnswered":true,"author":{"login":"ann"},"category":{"name":"Q&A"},"comments":{"totalCount":3}},
{"id":"D2","number":41,"title":"Idea","body":"","url":"u","updatedAt":"t","closed":true,"locked":true,"isAnswered":false,"author":null,"category":null,"comments":{"totalCount":0}},
null]}}}}`

func TestParseDiscussionList(t *testing.T) {
	l, err := ParseDiscussionList([]byte(discListJSON))
	if err != nil || !l.Enabled || l.Total != 73 || len(l.Rows) != 2 || len(l.Categories) != 2 {
		t.Fatalf("%+v %v", l, err)
	}
	if l.Categories[0].Name != "Q&A" || !l.Categories[0].Answerable || l.Categories[0].ID != "C1" {
		t.Errorf("category %+v (the escape must go)", l.Categories[0])
	}
	r := l.Rows[0]
	if r.Title != "How do I retry?" || r.Number != 42 || r.Author != "ann" || r.Category != "Q&A" || !r.Answered || r.Comments != 3 || strings.ContainsRune(r.Body, 0x1b) || r.ID != "D1" {
		t.Errorf("row %+v", r)
	}
	g := l.Rows[1]
	if g.Author != "ghost" || g.Category != "" || !g.Closed || !g.Locked {
		t.Errorf("row %+v", g)
	}
}

func TestParseDiscussionListDisabledAndErrors(t *testing.T) {
	l, err := ParseDiscussionList([]byte(`{"data":{"repository":{"hasDiscussionsEnabled":false,"discussionCategories":{"nodes":[]},"discussions":{"totalCount":0,"nodes":[]}}}}`))
	if err != nil || l.Enabled || len(l.Rows) != 0 {
		t.Errorf("%+v %v", l, err)
	}
	if _, err := ParseDiscussionList([]byte(`{"data":{"repository":null},"errors":[{"message":"Could not resolve to a Repository"}]}`)); err == nil || !strings.Contains(err.Error(), "Could not resolve") {
		t.Errorf("%v", err)
	}
	if _, err := ParseDiscussionList([]byte(`nope`)); err == nil {
		t.Error("garbage accepted")
	}
}

func TestDiscussionBodyInTheListIsCut(t *testing.T) {
	big := strings.Repeat("x", discBodyCut+500)
	l, _ := ParseDiscussionList([]byte(`{"data":{"repository":{"hasDiscussionsEnabled":true,"discussions":{"totalCount":1,"nodes":[{"id":"D","number":1,"title":"t","body":"` + big + `","comments":{"totalCount":0}}]}}}}`))
	if n := len([]rune(l.Rows[0].Body)); n != discBodyCut+1 {
		t.Errorf("body %d runes", n)
	}
}

const discJSON = `{"data":{"repository":{"hasDiscussionsEnabled":true,"discussion":{"id":"D1","number":42,"title":"Retry","body":"Please\u001b[2J help","url":"https://github.com/acme/app/discussions/42","closed":false,"locked":true,"isAnswered":true,"createdAt":"2026-10-01T00:00:00Z","author":{"login":"ann"},"category":{"name":"Q&A"},
"comments":{"totalCount":150,"nodes":[
{"id":"K1","body":"first","createdAt":"t1","isAnswer":true,"author":{"login":"bob"},"replies":{"totalCount":60,"nodes":[{"id":"R1","body":"re \u001b[1m1","createdAt":"t2","author":{"login":"cy"}},{"id":"R2","body":"re 2","createdAt":"t3","author":null},null]}},
{"id":"K2","body":"second","createdAt":"t4","isAnswer":false,"author":null,"replies":{"totalCount":0,"nodes":[]}},
null]}}}}}`

func TestParseDiscussion(t *testing.T) {
	d, err := ParseDiscussion([]byte(discJSON))
	if err != nil || d.ID != "D1" || d.Number != 42 || !d.Locked || !d.Answered || d.Category != "Q&A" || d.CommentTotal != 150 || len(d.Comments) != 2 {
		t.Fatalf("%+v %v", d, err)
	}
	if strings.ContainsRune(d.Body, 0x1b) {
		t.Errorf("escape kept in %q", d.Body)
	}
	c := d.Comments[0]
	if c.ID != "K1" || c.Author != "bob" || !c.IsAnswer || c.ReplyTotal != 60 || len(c.Replies) != 2 || c.Replies[0].ID != "R1" || c.Replies[1].Author != "ghost" || strings.ContainsRune(c.Replies[0].Body, 0x1b) {
		t.Errorf("comment %+v", c)
	}
	if d.Comments[1].Author != "ghost" {
		t.Errorf("%+v", d.Comments[1])
	}
}

func TestParseDiscussionMissing(t *testing.T) {
	_, err := ParseDiscussion([]byte(`{"data":{"repository":{"hasDiscussionsEnabled":true,"discussion":null}},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a Discussion with the number of 9."}]}`))
	if err == nil || !strings.Contains(err.Error(), "Could not resolve to a Discussion") {
		t.Errorf("%v", err)
	}
	_, err = ParseDiscussion([]byte(`{"data":{"repository":{"hasDiscussionsEnabled":false,"discussion":null}}}`))
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("%v", err)
	}
}

func TestDiscussionArgsKeepValuesApartFromTheQuery(t *testing.T) {
	a := strings.Join(DiscListArgs("ghe.corp", RepoRef{Owner: "acme", Name: "app"}, "C1"), " ")
	for _, want := range []string{"--hostname ghe.corp", "-f owner=acme", "-f name=app", "-f cat=C1"} {
		if !strings.Contains(a, want) {
			t.Errorf("%q missing from %s", want, a)
		}
	}
	if strings.Contains(strings.Join(DiscListArgs("", RepoRef{Owner: "a", Name: "b"}, ""), " "), "cat=") {
		t.Error("a category was sent for All")
	}
	d := strings.Join(DiscArgs("", RepoRef{Owner: "acme", Name: "app"}, 42), " ")
	if !strings.Contains(d, "-F n=42") || strings.Contains(discQuery, "42") {
		t.Errorf("%s", d)
	}
}

func TestFetchDiscussionRejectsBadNumbersAndRetriesATimeout(t *testing.T) {
	old := discRun
	t.Cleanup(func() { discRun = old })
	calls := 0
	discRun = func(d time.Duration, args ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("HTTP 502")
		}
		return []byte(discJSON), nil
	}
	if _, err := FetchDiscussion("", RepoRef{Owner: "a", Name: "b"}, "x"); err == nil || calls != 0 {
		t.Errorf("bad number: %v, %d calls", err, calls)
	}
	d, err := FetchDiscussion("", RepoRef{Owner: "a", Name: "b"}, "42")
	if err != nil || d.Number != 42 || calls != 2 {
		t.Errorf("%+v %v calls %d", d, err, calls)
	}
}

func TestAddDiscussionCommentSendsIdsAsVariables(t *testing.T) {
	body := captured(t, `{"data":{"addDiscussionComment":{"comment":{"id":"K"}}}}`)
	if err := AddDiscussionComment("", "D1", "Thanks -- `x`", "K1"); err != nil {
		t.Fatal(err)
	}
	b := string(*body)
	head := strings.SplitN(b, `"variables"`, 2)[0]
	for _, v := range []string{"D1", "K1", "Thanks"} {
		if strings.Contains(head, v) {
			t.Errorf("%s pasted into the query text", v)
		}
	}
	if !strings.Contains(b, `"discussion":"D1"`) || !strings.Contains(b, `"reply":"K1"`) || !strings.Contains(b, "addDiscussionComment") {
		t.Errorf("%s", b)
	}
	AddDiscussionComment("", "D1", "top level", "")
	if strings.Contains(string(*body), `"reply"`) {
		t.Errorf("a reply id was sent for a top-level comment: %s", *body)
	}
	if AddDiscussionComment("", "", "x", "") == nil || AddDiscussionComment("", "D1", "  ", "") == nil {
		t.Error("bad comment accepted")
	}
}
