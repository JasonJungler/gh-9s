package markdown

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func plain(d Doc) string {
	var out []string
	for _, l := range d.Lines {
		out = append(out, ansi.Strip(l))
	}
	return strings.Join(out, "\n")
}

func TestRenderConstructs(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string // each must appear, in order, as whole lines
	}{
		{"heading", "# Title\n\n## Sub", []string{"Title", "", "Sub"}},
		{"inline marks", "a **b** *i* `c` ~~d~~ e", []string{"a b i c d e"}},
		{"fence", "```go\nx := 1\n  y\n```", []string{"  x := 1", "    y"}},
		{"bullets", "- a\n- b", []string{"• a", "• b"}},
		{"numbers", "3. a\n4. b", []string{"3. a", "4. b"}},
		{"tasks", "- [x] done\n- [ ] todo", []string{"• ☑ done", "• ☐ todo"}},
		{"nested", "- a\n  - b", []string{"• a", "  • b"}},
		{"quote", "> hi\n> there", []string{"│ hi", "│ there"}},
		{"single newline is a line break", "a\nb", []string{"a", "b"}},
		{"link", "see [the docs](https://x.test/d) now", []string{"see the docs now"}},
		{"autolink", "<https://x.test/a>", []string{"https://x.test/a"}},
		{"rule", "a\n\n---\n\nb", []string{"a", "", "────────────────────────────────────────", "", "b"}},
		{"comment", "x\n\n<!-- hidden -->\n\ny", []string{"x", "", "y"}},
		{"inline comment", "a <!-- no --> b", []string{"a  b"}},
		{"html tags dropped", "<b>bold</b> text", []string{"bold text"}},
		{"crlf", "a\r\n\r\nb", []string{"a", "", "b"}},
		{"cjk", "你好世界", []string{"你好世界"}},
	}
	for _, c := range cases {
		got := strings.Split(plain(Render(c.in, 60)), "\n")
		i := 0
		for _, line := range got {
			if i < len(c.want) && line == c.want[i] {
				i++
			}
		}
		if i != len(c.want) {
			t.Errorf("%s: lines %q do not contain %q in order", c.name, got, c.want)
		}
	}
}

func TestStylingIsApplied(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	// lipgloss decides colour support once; this only checks that the heading text survives styling.
	if got := plain(Render("# Big", 40)); got != "Big" {
		t.Errorf("heading = %q", got)
	}
}

func TestTable(t *testing.T) {
	d := Render("| a | bb |\n|---|---|\n| 1 | 2 |\n| 333 |", 40)
	p := plain(d)
	for _, want := range []string{"a   │ bb", "333 │", "1   │ 2"} {
		if !strings.Contains(p, want) {
			t.Errorf("table lacks %q:\n%s", want, p)
		}
	}
	wide := plain(Render("| aaaaaaaaaaaaaaaaaaaa | bbbbbbbbbbbbbbbbbbbb |\n|-|-|\n| 1 | 2 |", 20))
	if !strings.Contains(wide, "…") {
		t.Errorf("a table wider than the screen should truncate cells:\n%s", wide)
	}
}

func TestImages(t *testing.T) {
	d := Render("before\n\n![shot](https://h/p.png)\n\n<img src=\"https://h/q.png\" alt=\"two\">\n\n![](https://h/r.png)", 60)
	want := []Image{{Alt: "shot", URL: "https://h/p.png"}, {Alt: "two", URL: "https://h/q.png"}, {Alt: "", URL: "https://h/r.png"}}
	if len(d.Images) != 3 {
		t.Fatalf("images = %+v", d.Images)
	}
	markers := []string{"[image: shot]", "[image: two]", "[image]"}
	for i, im := range d.Images {
		if im.Alt != want[i].Alt || im.URL != want[i].URL {
			t.Errorf("image %d = %+v; want %+v", i, im, want[i])
		}
		if got := ansi.Strip(d.Lines[im.Line]); got != markers[i] {
			t.Errorf("image %d line = %q; want %q", i, got, markers[i])
		}
	}
	linked := Render("[![badge](https://h/b.svg)](https://x.test)", 60)
	if len(linked.Images) != 1 || linked.Images[0].URL != "https://h/b.svg" {
		t.Errorf("an image inside a link: %+v", linked.Images)
	}
	mixed := Render("text ![a](https://h/1.png) more text", 60)
	if len(mixed.Images) != 1 || !strings.Contains(plain(mixed), "text") || !strings.Contains(plain(mixed), "more text") {
		t.Errorf("an image in a sentence: %+v\n%s", mixed.Images, plain(mixed))
	}
}

func TestNoEscapeSequenceSurvives(t *testing.T) {
	in := "hi \x1b[31m\x1b]52;c;AAAA\x07 x\n\n```\n\x1b]0;title\x07code\n```\n\n| \x1b[2J |\n|-|\n| y |\n\n[\x1b]52;c;QQ\x07](https://x)"
	d := Render(in, 40)
	// tests run without colour, so the renderer's own styling adds no ESC: any ESC seen came from the text
	if raw := strings.Join(d.Lines, ""); strings.ContainsRune(raw, 0x1b) || strings.ContainsRune(raw, 0x07) {
		t.Errorf("an escape sequence from the text reached the output: %q", d.Lines)
	}
}

func TestEveryLineFitsTheWidth(t *testing.T) {
	src := "# A very long heading that must wrap or be cut somewhere reasonable\n\n" +
		"averyveryveryveryverylongwordwithoutanyspaces and 你好世界你好世界你好世界你好世界 and more\n\n" +
		"- item with a long line that needs wrapping around the edge of the screen\n\n> quote quote quote quote quote quote quote\n\n```\nthis code line is far too long for the screen so it is cut\n```\n\n| a | b |\n|-|-|\n| 123456789012345 | 123456789012345 |\n"
	for _, w := range []int{5, 20, 40, 120} {
		for _, l := range Render(src, w).Lines {
			if ansi.StringWidth(l) > w {
				t.Errorf("width %d: line %q is %d wide", w, ansi.Strip(l), ansi.StringWidth(l))
			}
		}
	}
}

func TestPathologicalInputFinishesQuickly(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		Render(strings.Repeat("> ", 5000)+"deep", 40)
		Render(strings.Repeat("- a\n  ", 3000)+"- b", 40)
		Render(strings.Repeat("x", 2<<20), 40)
		Render("```\nunclosed", 40)
		Render("", 40)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("rendering did not finish in 5 s")
	}
	if len(Render("", 40).Lines) != 0 {
		t.Error("empty input should give no lines")
	}
}

func TestTabsNeverReachTheOutput(t *testing.T) {
	for _, in := range []string{"a\tb\tc\td\te", "- a\tb", "| a\tb |\n|-|\n| c\td |", "# h\tx", "```\na\tb\n```"} {
		for _, l := range Render(in, 10).Lines {
			if strings.ContainsRune(l, '\t') {
				t.Errorf("%q: line %q contains a tab", in, l)
			}
		}
	}
}

func TestHostileNestingIsBoundedAndKeepsItsText(t *testing.T) {
	start := time.Now()
	d := Render(strings.Repeat("> ", 32000)+"x", 40)
	Render(strings.Repeat("- ", 32000)+"x", 40)
	if el := time.Since(start); el > time.Second {
		t.Errorf("hostile nesting took %v", el)
	}
	if !strings.Contains(plain(d), "x") {
		t.Errorf("the text under deep nesting was lost: %q", plain(d))
	}
}

func TestRenderingTheSameTextTwiceParsesOnce(t *testing.T) {
	parses = 0
	src := "same text " + t.Name()
	a, b := Render(src, 30), Render(src, 30)
	if parses != 1 || plain(a) != plain(b) {
		t.Errorf("parsed %d times", parses)
	}
	Render(src, 31)
	if parses != 2 {
		t.Errorf("a different width must render again; parses = %d", parses)
	}
}
