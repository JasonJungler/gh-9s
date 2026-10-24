package ghcli

import (
	"strings"
	"testing"
)

func TestCleanText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"tab\there", "tab\there"},
		{"two\nlines", "two\nlines"},
		{"crlf\r\nline", "crlf\nline"},
		{"lone\rreturn", "lonereturn"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]52;c;ZXZpbA==\x07clipboard", "clipboard"},
		{"bell\x07 and \x00null and \x7fdel", "bell and null and del"},
		{"c1 \u009b31m \u009d title", "c1   title"},
		{"emoji 🚧 and 日本語 stay", "emoji 🚧 and 日本語 stay"},
		{"", ""},
	}
	for _, c := range cases {
		if got := CleanText(c.in); got != c.want {
			t.Errorf("CleanText(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestParsePRDetailStripsControlCharacters(t *testing.T) {
	d, err := ParsePRDetail([]byte(`{"number":1,"title":"fine\u001b]0;pwned\u0007","body":"a\u001b[2Jb",
		"author":{"login":"x\u001b[1m"},"comments":[{"author":{"login":"c"},"body":"hi\u001b]52;c;AAAA\u0007","createdAt":""}],
		"reviews":[{"author":{"login":"r"},"state":"COMMENTED","body":"\u001b[Hrev","submittedAt":""}],"labels":[{"name":"l\u001b[0m"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	all := d.Title + d.Body + d.Author + d.Comments[0].Body + d.Reviews[0].Body + d.Labels[0]
	if strings.ContainsAny(all, "\x1b\x07") {
		t.Errorf("control characters survived: %q", all)
	}
	if d.Body != "ab" || d.Comments[0].Body != "hi" {
		t.Errorf("text should keep its printable parts: body %q comment %q", d.Body, d.Comments[0].Body)
	}
}

func TestParseIssueDetailStripsControlCharacters(t *testing.T) {
	d, err := ParseIssueDetail([]byte(`{"number":1,"title":"t\u001b]0;x\u0007","body":"b\u001b[2J","author":{"login":"a\u001b[1m"},
		"labels":[{"name":"l\u001b[0m"}],"assignees":[{"login":"e\u001b[0m"}],"comments":[{"author":null,"body":"c\u001b]52;c;AA\u0007","createdAt":""}]}`))
	if err != nil {
		t.Fatal(err)
	}
	all := d.Title + d.Body + d.Author + d.Labels[0] + d.Assignees[0] + d.Comments[0].Body
	if strings.ContainsAny(all, "\x1b\x07") {
		t.Errorf("control characters survived: %q", all)
	}
}

func TestParseRepoInfosStripsControlCharacters(t *testing.T) {
	got, err := ParseRepoInfos([]byte(`[{"nameWithOwner":"a/b","description":"d\u001b]0;x\u0007","primaryLanguage":{"name":"Go\u001b[0m"}}]`), "")
	if err != nil || strings.ContainsAny(got[0].Description+got[0].Language, "\x1b\x07") {
		t.Errorf("got %+v err %v", got, err)
	}
}
