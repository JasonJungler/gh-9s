package safe

import "testing"

func TestText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "plain text", "plain text"},
		{"tab and newline stay", "a\tb\nc", "a\tb\nc"},
		{"CRLF becomes LF", "one\r\ntwo", "one\ntwo"},
		{"lone CR is dropped", "over\rwrite", "overwrite"},
		{"colour codes vanish whole", "\x1b[31mred\x1b[0m and \x1b[1;32mgreen\x1b[m", "red and green"},
		{"cursor and erase", "a\x1b[2J\x1b[Hb\x1b[10;20Hc", "abc"},
		{"OSC title ended by BEL", "x\x1b]0;pwned\x07y", "xy"},
		{"OSC clipboard ended by ST", "x\x1b]52;c;ZXZpbA==\x1b\\y", "xy"},
		{"two-character escape", "a\x1bcb", "ab"},
		{"C1 CSI and OSC", "a\u009b31mb\u009d0;t\u0007c", "abc"},
		{"bell, NUL and DEL", "a\x07b\x00c\x7fd", "abcd"},
		{"emoji and CJK stay", "🚧 日本語 ✓", "🚧 日本語 ✓"},
		{"truncated CSI at the end", "abc\x1b[", "abc"},
		{"lone ESC at the end", "abc\x1b", "abc"},
		{"unterminated OSC keeps the text after it", "a\x1b]0;never ends but text continues", "a0;never ends but text continues"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := Text(c.in); got != c.want {
			t.Errorf("%s: Text(%q) = %q; want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestNoBidiDropsOverridesAndTextKeepsThem(t *testing.T) {
	in := "a\u202eb\u202ac\u2066d\u2069e"
	if got := NoBidi(in); got != "abcde" {
		t.Errorf("NoBidi(%q) = %q", in, got)
	}
	if got := Text(in); got != in {
		t.Errorf("Text decides validity of stored text and must not change it: %q", got)
	}
	if got := NoBidi("שלום"); got != "שלום" {
		t.Errorf("Hebrew letters must stay: %q", got)
	}
}
