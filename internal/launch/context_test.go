package launch

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func TestBuildContext(t *testing.T) {
	d := ghcli.IssueDetail{
		Number: 42, Title: "Fix login", URL: "https://x/42", Body: "It crashes.",
		Comments: []ghcli.Comment{{Author: "ann", Body: "Seen too"}, {Author: "", Body: "me3"}},
	}
	want := "Please solve this issue\n\n# Fix login (#42)\nhttps://x/42\n\nIt crashes.\n\n## Comments\n### ann (1/2)\nSeen too\n\n### ghost (2/2)\nme3\n"
	if got := BuildContext(d); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestBuildContextEmptyBodyAndNoComments(t *testing.T) {
	got := BuildContext(ghcli.IssueDetail{Number: 1, Title: "T", URL: "u"})
	if !strings.Contains(got, "\n(no description)\n") {
		t.Errorf("missing placeholder: %q", got)
	}
	if strings.Contains(got, "## Comments") {
		t.Errorf("unexpected comments section: %q", got)
	}
}

func TestBranchName(t *testing.T) {
	long := strings.Repeat("a", 200)
	cases := []struct {
		title string
		want  string
	}{
		{"Fix login crash!", "issue-42-fix-login-crash"},
		{"", "issue-42"},
		{"登入失敗", "issue-42"},
	}
	for _, c := range cases {
		if got := BranchName(42, c.title); got != c.want {
			t.Errorf("BranchName(42, %q) = %q, want %q", c.title, got, c.want)
		}
	}
	for _, title := range []string{long, strings.Repeat("ab ", 100), "issue-42-" + strings.Repeat("a", 30) + " b"} {
		got := BranchName(42, title)
		if len(got) > 50 || strings.HasSuffix(got, "-") {
			t.Errorf("BranchName(42, %q) = %q (len %d)", title, got, len(got))
		}
	}
}

func TestBuildContextStripsControlCharacters(t *testing.T) {
	d := ghcli.IssueDetail{
		Number: 1, Title: "t\x1b[2Jitle", URL: "u", Body: "line\ttab\r\nnext 中文 🚀 é",
		Comments: []ghcli.Comment{{Author: "a\x07", Body: "evil\x1b[201~ rm -rf \x03 \x7f \u0085 \u009b end"}},
	}
	got := BuildContext(d)
	for _, bad := range []string{"\x1b", "\x03", "\x07", "\r", "\x7f", "\u0085", "\u009b"} {
		if strings.Contains(got, bad) {
			t.Errorf("context still contains %q: %q", bad, got)
		}
	}
	if strings.Contains(got, "\x1b[201~") {
		t.Errorf("context can end a bracketed paste early: %q", got)
	}
	for _, keep := range []string{"line\ttab\nnext 中文 🚀 é", "evil[201~ rm -rf", "\n"} {
		if !strings.Contains(got, keep) {
			t.Errorf("context lost %q: %q", keep, got)
		}
	}
}

func TestCleanContextRemovesWhatBuildContextRemoves(t *testing.T) {
	for in, want := range map[string]string{
		"a\x1b[201~b":       "a[201~b", // the end of a bracketed paste cannot survive
		"a\x00b\x7fc\u0085": "abc",
		"keep\ttabs":        "keep\ttabs",
		"a\rb":              "ab", // a lone CR would submit in Claude Code
		"x\x1b[200~y":       "x[200~y",
	} {
		if got := CleanContext(in); got != want {
			t.Errorf("CleanContext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanContextKeepsTheLineBreaksOfWindowsEditors(t *testing.T) {
	if got, want := CleanContext("one\r\ntwo\r\n\r\nthree\r\n"), "one\ntwo\n\nthree"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanContextTrimsTrailingWhitespaceOnly(t *testing.T) {
	if got, want := CleanContext("  indented\n\nbody \t\n\n  \n"), "  indented\n\nbody"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	for _, empty := range []string{"", " \n\t\r\n", "\x1b\x00\r"} {
		if got := CleanContext(empty); got != "" {
			t.Errorf("CleanContext(%q) = %q, want empty", empty, got)
		}
	}
}

func TestCleanContextTreatsInvalidUTF8LikeBuildContext(t *testing.T) {
	bad := "caf\xe9 ok"
	d := ghcli.IssueDetail{Number: 1, Title: "T", URL: "u", Body: bad}
	if !strings.Contains(BuildContext(d), "caf� ok") {
		t.Fatalf("BuildContext no longer replaces invalid UTF-8: %q", BuildContext(d))
	}
	got := CleanContext(bad)
	if got != "caf� ok" || !utf8.ValidString(got) {
		t.Errorf("CleanContext(%q) = %q, want the replacement character", bad, got)
	}
}

func TestCleanContextIsIdempotentOnAGeneratedPrompt(t *testing.T) {
	d := ghcli.IssueDetail{Number: 7, Title: "T\x1b", URL: "u", Body: "b\r\nc", Comments: []ghcli.Comment{{Author: "a", Body: "x"}}}
	gen := BuildContext(d)
	if got := CleanContext(gen); got != strings.TrimRight(gen, "\n") {
		t.Errorf("CleanContext(generated) = %q, want the generated text without its final newline", got)
	}
}
