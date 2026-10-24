package icons

import (
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func restore(t *testing.T) {
	t.Helper()
	old := Current()
	t.Cleanup(func() { Use(old) })
}

func TestEveryNameIsDefinedInBothSets(t *testing.T) {
	for n := Name(0); n < NameCount; n++ {
		if _, ok := names[n]; !ok {
			t.Errorf("name %d has no entry in the table", n)
		}
	}
	for n := Name(0); n < NameCount; n++ {
		if GetIn(Nerd, n) == "" {
			t.Errorf("%s has no Nerd Font glyph", names[n])
		}
	}
}

func TestPlainIsExactlyTodaysSymbols(t *testing.T) {
	pinned := map[Name]string{
		CheckPass: "✓", CheckFail: "✗", CheckPending: "●", CheckSkipped: "○",
		RunOK: "✓", RunFail: "✗", RunActive: "●", RunOther: "○",
		Selected: "▸", New: "●", Viewed: "✓",
	}
	for n, want := range pinned {
		if got := GetIn(Plain, n); got != want {
			t.Errorf("plain %s = %q; want today's %q", names[n], got, want)
		}
	}
	for _, n := range []Name{PROpen, PRMerged, PRClosed, PRDraft, IssueOpen, IssueClosed, Branch, Repo, Comment, User, Label, Clock,
		TabAssigned, TabReview, TabMine, TabMentions, ReviewApproved, ReviewChanges, ReviewRequired} {
		if got := GetIn(Plain, n); got != "" {
			t.Errorf("plain %s = %q; a decoration that does not exist today stays empty", names[n], got)
		}
	}
}

func TestNerdGlyphsAreOneColumnPrivateUseRunes(t *testing.T) {
	seen := map[string]Name{}
	shared := map[[2]Name]bool{{Viewed, CheckPass}: true, {New, RunActive}: true, {TabAssigned, IssueOpen}: true, {TabMine, PROpen}: true} // deliberate: same meaning
	for n := Name(0); n < NameCount; n++ {
		g := GetIn(Nerd, n)
		r, size := utf8.DecodeRuneInString(g)
		if size != len(g) || r < 0xE000 || r > 0xF8FF {
			t.Errorf("%s = %U: want a single rune in the BMP private use area", names[n], r)
		}
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%s is %d columns wide; the layout counts one", names[n], w)
		}
		if other, dup := seen[g]; dup && !shared[[2]Name{n, other}] && !shared[[2]Name{other, n}] {
			t.Errorf("%s and %s share a glyph", names[n], names[other])
		}
		seen[g] = n
	}
}

func TestColoursFollowTheMeaning(t *testing.T) {
	want := map[Name]string{PROpen: "2", PRMerged: "5", PRClosed: "1", PRDraft: "8", IssueOpen: "2", IssueClosed: "5",
		CheckPass: "2", CheckFail: "1", CheckPending: "3", ReviewApproved: "2", ReviewChanges: "1", ReviewRequired: "3"}
	for n, c := range want {
		if got := Color(n); got != c {
			t.Errorf("%s colour = %q; want %q", names[n], got, c)
		}
	}
}

func TestUseSwitchesGet(t *testing.T) {
	restore(t)
	Use(Plain)
	if Get(CheckPass) != "✓" || Current() != Plain {
		t.Error("plain")
	}
	Use(Nerd)
	if Get(CheckPass) == "✓" || Current() != Nerd {
		t.Error("nerd")
	}
}

func TestConcurrentReadersAndAWriter(t *testing.T) {
	restore(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_ = Get(PROpen)
				_ = Current()
			}
		}()
	}
	for j := 0; j < 100; j++ {
		Use(Mode(j % 2))
	}
	wg.Wait()
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Mode{"nerd": Nerd, "plain": Plain, " NERD ": Nerd} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %v %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "ask", "emoji"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestSampleIsAFewRealIcons(t *testing.T) {
	if len(Sample) < 4 {
		t.Fatalf("the question shows %d glyphs", len(Sample))
	}
	for _, n := range Sample {
		if GetIn(Nerd, n) == "" {
			t.Errorf("sample %s has no glyph", names[n])
		}
	}
}
