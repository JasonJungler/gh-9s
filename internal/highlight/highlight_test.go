package highlight

import (
	"strings"
	"testing"
	"time"

	"github.com/MurabitoB/gh-9s/internal/diff"
)

func fileOf(t *testing.T, path, patch string) diff.File {
	t.Helper()
	d := diff.Parse("diff --git a/"+path+" b/"+path+"\n--- a/"+path+"\n+++ b/"+path+"\n"+patch, 100000)
	return d.Files[0]
}

func join(segs []Seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func colourOf(segs []Seg, word string) string {
	for _, s := range segs {
		if strings.Contains(s.Text, word) {
			return s.FG
		}
	}
	return "?"
}

func TestGoLinesAreColouredByToken(t *testing.T) {
	f := fileOf(t, "main.go", "@@ -1,2 +1,2 @@\n func main() {\n-\tx := 1\n+\tx := \"a\"\n")
	h := ForFile(f)
	if h == nil {
		t.Fatal("Go should be known")
	}
	ctx := h.Left(0 + 1) // line index 1 is "func main() {"
	if join(ctx) != "func main() {" {
		t.Fatalf("segments %q do not add up to the line", join(ctx))
	}
	if colourOf(ctx, "func") == colourOf(ctx, "main") || colourOf(ctx, "func") == "" {
		t.Errorf("a keyword and an identifier should differ: %+v", ctx)
	}
	add := h.Right(3)
	if join(add) != "    x := \"a\"" {
		t.Errorf("tabs must be expanded to match the display: %q", join(add))
	}
	if colourOf(add, `"a"`) == colourOf(add, "x") {
		t.Errorf("a string and an identifier should differ: %+v", add)
	}
}

func TestOldAndNewSidesAreTokenisedSeparately(t *testing.T) {
	f := fileOf(t, "a.go", "@@ -1,3 +1,3 @@\n-s := `a\n+s := 1\n x := 2\n z`\n")
	h := ForFile(f)
	// on the old side the backtick opens a raw string that swallows the next line; on the new side it does not
	oldNext, newNext := h.Left(3), h.Right(3)
	if colourOf(oldNext, "x") == colourOf(newNext, "x") {
		t.Logf("old %+v new %+v", oldNext, newNext)
		t.Error("the context line should be coloured per side")
	}
}

func TestMultiLineStringInsideAHunk(t *testing.T) {
	f := fileOf(t, "a.go", "@@ -1,4 +1,4 @@\n x := `one\n two\n three`\n y := 1\n")
	h := ForFile(f)
	if h.Right(2) == nil || h.Right(1) == nil {
		t.Fatal("context lines are coloured on the new side")
	}
	if colourOf(h.Right(2), "two") != colourOf(h.Right(1), "one") {
		t.Errorf("the middle of a raw string should be string-coloured: %+v / %+v", h.Right(2), h.Right(1))
	}
}

func TestUnknownLanguageAndLimits(t *testing.T) {
	if ForFile(fileOf(t, "notes.zzzz", "@@ -1,1 +1,1 @@\n-a\n+b\n")) != nil {
		t.Error("an unknown language gets no highlighting")
	}
	long := strings.Repeat("x", 500)
	h := ForFile(fileOf(t, "a.go", "@@ -1,1 +1,1 @@\n-"+long+"\n+short := 1\n"))
	if h.Left(1) != nil {
		t.Error("a line over 400 characters is left plain")
	}
	if h.Right(2) == nil {
		t.Error("its neighbours are still coloured")
	}
	if ForFile(diff.File{Path: "a.go", Binary: true}) != nil {
		t.Error("binary files get nothing")
	}
}

func TestOutOfRangeAndHeaderLinesAreNil(t *testing.T) {
	h := ForFile(fileOf(t, "a.go", "@@ -1,1 +1,1 @@\n-a := 1\n+a := 2\n"))
	if h.Left(0) != nil || h.Right(99) != nil || h.Left(-1) != nil {
		t.Error("hunk headers and bad indexes have no segments")
	}
	var none *File
	if none.Left(1) != nil {
		t.Error("a nil File answers nil")
	}
}

func TestHostileFilesNeitherPanicNorHang(t *testing.T) {
	cases := map[string]string{
		"a.yang":     "@@ -0,0 +1,2 @@\n+/* a\n+/* b */\n",
		"m.jungle":   "@@ -0,0 +1,2 @@\n+project.manifest = manifest.xml\n+base.sourcePath = \"source\"\n",
		"p.pas":      "@@ -0,0 +1,1 @@\n+" + strings.Repeat("1", 1999) + "\n",
		"big.go":     "@@ -0,0 +1,1 @@\n+" + strings.Repeat("a+", 999) + "\n",
		"empty.go":   "@@ -0,0 +1,1 @@\n+\n",
		"quote.js":   "@@ -0,0 +1,1 @@\n+\"\\'\n",
		"weird.go":   "@@ -0,0 +1,1 @@\n+\x00\xff\xfe\n",
		"unicode.go": "@@ -0,0 +1,1 @@\n+变量 := \"你好\"\n",
	}
	for name, patch := range cases {
		start := time.Now()
		done := make(chan struct{})
		go func() {
			defer close(done)
			ForFileWithin(fileOf(t, name, patch), 300*time.Millisecond)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Errorf("%s: ForFile did not return in 3s", name)
		}
		_ = start
	}
}

func TestKnownBadLexersAreSkippedNotWaitedFor(t *testing.T) {
	start := time.Now()
	h := ForFileWithin(fileOf(t, "m.jungle", "@@ -0,0 +1,1 @@\n+project.manifest = manifest.xml\n"), 10*time.Second)
	if h != nil || time.Since(start) > 2*time.Second {
		t.Errorf("a lexer that loops forever must be skipped outright (took %v)", time.Since(start))
	}
}

func TestPlainTextKeepsTheTerminalsOwnColour(t *testing.T) {
	h := ForFile(fileOf(t, "a.go", "@@ -1,1 +1,1 @@\n x := 1\n"))
	for _, s := range h.Right(1) {
		if strings.TrimSpace(s.Text) == "" && s.FG != "" {
			t.Errorf("whitespace should use the default colour, got %q", s.FG)
		}
	}
}

func TestSizeCountsTheTextToColour(t *testing.T) {
	f := fileOf(t, "a.go", "@@ -1,1 +1,1 @@\n-abc\n+defg\n")
	if got := Size(f); got != 7 {
		t.Errorf("Size = %d; want 7", got)
	}
}
