package ghcli

import (
	"reflect"
	"strings"
	"testing"
)

const threadsJSON = `[
 {"id":1,"path":"main.go","line":42,"original_line":40,"side":"RIGHT","user":{"login":"bob"},"body":"why 3?","created_at":"2026-10-20T10:00:00Z"},
 {"id":2,"in_reply_to_id":1,"path":"main.go","line":42,"side":"RIGHT","user":{"login":"alice"},"body":"magic number","created_at":"2026-10-20T11:00:00Z"},
 {"id":3,"path":"old.go","line":null,"original_line":7,"side":"LEFT","user":{"login":"carol"},"body":"gone now","created_at":"2026-10-19T09:00:00Z"},
 {"id":4,"path":"x.go","line":1,"user":{"login":"d\u001b[31mave"},"body":"hi \u001b]52;c;AAAA\u0007 there","created_at":"2026-10-21T09:00:00Z"}
]`

func TestParseThreads(t *testing.T) {
	ts, err := ParseThreads([]byte(threadsJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 3 {
		t.Fatalf("%d threads; want 3: %+v", len(ts), ts)
	}
	first := ts[0]
	if first.Path != "main.go" || first.Line != 42 || first.Side != "RIGHT" || first.Outdated || len(first.Comments) != 2 {
		t.Errorf("first thread = %+v", first)
	}
	if first.Comments[0].Author != "bob" || first.Comments[1].Author != "alice" {
		t.Errorf("reply order: %+v", first.Comments)
	}
	old := ts[1]
	if !old.Outdated || old.Line != 7 || old.Side != "LEFT" {
		t.Errorf("outdated thread = %+v", old)
	}
	if ts[2].Side != "RIGHT" {
		t.Errorf("default side = %q", ts[2].Side)
	}
	if strings.ContainsRune(ts[2].Comments[0].Body, 0x1b) || strings.ContainsRune(ts[2].Comments[0].Author, 0x1b) || strings.ContainsRune(ts[2].Comments[0].Body, 7) {
		t.Errorf("escape sequences survived: %+v", ts[2].Comments[0])
	}
}

func TestParseThreadsAcceptsPagesAndEmpty(t *testing.T) {
	pages := `[[{"id":1,"path":"a","line":1,"user":{"login":"x"},"body":"b","created_at":"t"}],[{"id":2,"path":"b","line":2,"user":{"login":"y"},"body":"c","created_at":"t"}]]`
	ts, err := ParseThreads([]byte(pages))
	if err != nil || len(ts) != 2 {
		t.Errorf("pages: %v %+v", err, ts)
	}
	if ts, err := ParseThreads([]byte(`[]`)); err != nil || len(ts) != 0 {
		t.Errorf("empty: %v %+v", err, ts)
	}
	if _, err := ParseThreads([]byte(`{nope`)); err == nil {
		t.Error("malformed JSON should be an error")
	}
}

func TestThreadsArgs(t *testing.T) {
	got := ThreadsArgs(RepoRef{Host: "github.com", Owner: "acme", Name: "api"}, "482")
	want := []string{"api", "--paginate", "--slurp", "repos/acme/api/pulls/482/comments?per_page=100"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q; want %q", got, want)
	}
	got = ThreadsArgs(RepoRef{Host: "ghe.corp", Owner: "acme", Name: "api"}, "7")
	want = []string{"api", "--hostname", "ghe.corp", "--paginate", "--slurp", "repos/acme/api/pulls/7/comments?per_page=100"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enterprise args = %q; want %q", got, want)
	}
}
