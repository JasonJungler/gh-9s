package diff

import (
	"strings"
	"testing"
)

const twoFiles = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,4 +1,5 @@ func main() {
 package main
-import "fmt"
+import (
+	"fmt"
+)

@@ -20,3 +21,3 @@ func other() {
 a
-b
+c
diff --git a/README.md b/README.md
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/README.md
@@ -0,0 +1,2 @@
+# Title
+text
`

func kinds(f File) string {
	var b strings.Builder
	for _, l := range f.Lines {
		b.WriteByte("cadhm"[l.Kind])
	}
	return b.String()
}

func TestParseTwoFilesWithHunks(t *testing.T) {
	d := Parse(twoFiles, 1000)
	if len(d.Files) != 2 || d.Truncated {
		t.Fatalf("files=%d truncated=%v", len(d.Files), d.Truncated)
	}
	a, b := d.Files[0], d.Files[1]
	if a.Path != "main.go" || b.Path != "README.md" {
		t.Errorf("paths = %q, %q", a.Path, b.Path)
	}
	// hunk, ctx, del, add, add, add, ctx, hunk, ctx, del, add  (c=context a=add d=del h=hunk m=meta)
	if got := kinds(a); got != "hcdaaachcda" {
		t.Errorf("main.go kinds = %q; want hcdaaachcda", got)
	}
	if got := kinds(b); got != "haa" {
		t.Errorf("README.md kinds = %q; want haa", got)
	}
	if a.Lines[0].Text != "@@ -1,4 +1,5 @@ func main() {" || a.Lines[2].Text != `import "fmt"` {
		t.Errorf("text kept wrongly: %q / %q", a.Lines[0].Text, a.Lines[2].Text)
	}
}

func TestParseCountsAddedAndRemoved(t *testing.T) {
	d := Parse(twoFiles, 1000)
	if d.Files[0].Added != 4 || d.Files[0].Removed != 2 || d.Files[1].Added != 2 || d.Files[1].Removed != 0 {
		t.Errorf("counts = +%d -%d and +%d -%d", d.Files[0].Added, d.Files[0].Removed, d.Files[1].Added, d.Files[1].Removed)
	}
}

func TestParseBinaryFile(t *testing.T) {
	d := Parse("diff --git a/logo.png b/logo.png\nindex 1..2 100644\nBinary files a/logo.png and b/logo.png differ\n", 100)
	if len(d.Files) != 1 || !d.Files[0].Binary || d.Files[0].Path != "logo.png" || d.Files[0].Added != 0 {
		t.Fatalf("files = %+v", d.Files)
	}
	if len(d.Files[0].Lines) == 0 || d.Files[0].Lines[0].Kind != Meta {
		t.Errorf("the 'Binary files differ' note should be kept: %+v", d.Files[0].Lines)
	}
}

func TestParseRename(t *testing.T) {
	d := Parse("diff --git a/old/name.go b/new/name.go\nsimilarity index 100%\nrename from old/name.go\nrename to new/name.go\n", 100)
	f := d.Files[0]
	if len(d.Files) != 1 || !f.Renamed || f.OldPath != "old/name.go" || f.Path != "new/name.go" || f.Added != 0 {
		t.Errorf("file = %+v", f)
	}
}

func TestParseDeletedAndNewFilePaths(t *testing.T) {
	d := Parse("diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\nindex 1..0\n--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n", 100)
	if d.Files[0].Path != "gone.txt" || d.Files[0].Removed != 1 {
		t.Errorf("deleted file = %+v", d.Files[0])
	}
}

func TestParseNoNewlineAtEndOfFile(t *testing.T) {
	d := Parse("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n", 100)
	if got := kinds(d.Files[0]); got != "hdmam" {
		t.Errorf("kinds = %q; want the marker lines as meta (hdmam)", got)
	}
	if d.Files[0].Added != 1 || d.Files[0].Removed != 1 {
		t.Errorf("the marker must not be counted: +%d -%d", d.Files[0].Added, d.Files[0].Removed)
	}
}

func TestParseEmptyInput(t *testing.T) {
	for _, in := range []string{"", "\n", "   \n\n"} {
		d := Parse(in, 100)
		if len(d.Files) != 0 || d.Truncated || d.TotalLines != 0 {
			t.Errorf("Parse(%q) = %+v; want an empty diff", in, d)
		}
	}
}

func TestParseCRLFKeepsContentButNotTheCR(t *testing.T) {
	d := Parse("diff --git a/w.txt b/w.txt\r\n--- a/w.txt\r\n+++ b/w.txt\r\n@@ -1 +1 @@\r\n-old\r\n+new\r\n", 100)
	f := d.Files[0]
	if f.Path != "w.txt" || len(f.Lines) != 3 || f.Lines[1].Text != "old" || f.Lines[2].Text != "new" {
		t.Errorf("file = %+v", f)
	}
}

func TestParseCapsAtMaxLinesAndMarksTruncated(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big b/big\n--- a/big\n+++ b/big\n@@ -0,0 +1,50 @@\n")
	for i := 0; i < 50; i++ {
		b.WriteString("+line\n")
	}
	b.WriteString("diff --git a/after b/after\n--- a/after\n+++ b/after\n@@ -0,0 +1 @@\n+x\n")
	d := Parse(b.String(), 10)
	if !d.Truncated {
		t.Error("Truncated should be set")
	}
	stored := 0
	for _, f := range d.Files {
		stored += len(f.Lines)
	}
	if stored > 10 {
		t.Errorf("stored %d lines; the cap is 10", stored)
	}
	if len(d.Files) != 2 || d.Files[1].Path != "after" {
		t.Errorf("files after the cap must still be listed: %+v", d.Files)
	}
	if d.Files[0].Added != 50 {
		t.Errorf("counts must cover the whole diff, got +%d", d.Files[0].Added)
	}
	if d.TotalLines < 50 {
		t.Errorf("TotalLines = %d; want the full size of the diff", d.TotalLines)
	}
}

func TestParseTextBeforeAnyFileIsIgnored(t *testing.T) {
	d := Parse("warning: something from gh\n"+twoFiles, 1000)
	if len(d.Files) != 2 {
		t.Errorf("files = %d; stray leading text must not create a file", len(d.Files))
	}
}

func TestParseStripsTerminalControlSequences(t *testing.T) {
	d := Parse("diff --git a/x\x1b[31m b/x\x1b[31m\n--- a/x\n+++ b/x\n@@ -0,0 +1,2 @@\n+evil \x1b]52;c;ZXZpbA==\x07 clipboard\n+ok\ttab\n", 100)
	f := d.Files[0]
	for _, l := range f.Lines {
		if strings.ContainsAny(l.Text, "\x1b\x07") {
			t.Errorf("line %q kept a control character", l.Text)
		}
	}
	if strings.ContainsAny(f.Path, "\x1b\x07") {
		t.Errorf("path %q kept a control character", f.Path)
	}
	if f.Lines[1].Text != "evil  clipboard" || f.Lines[2].Text != "ok\ttab" {
		t.Errorf("printable text and tabs must survive: %q / %q", f.Lines[1].Text, f.Lines[2].Text)
	}
}
