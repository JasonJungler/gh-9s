package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var mt0 = time.Unix(1000, 0)

func TestStartTransition(t *testing.T) {
	base := sceneKey{view: viewIssues}
	cases := []struct {
		name     string
		old, cur sceneKey
		want     string // "", "rise", or the direction
	}{
		{"same", base, base, ""},
		{"deeper", base, sceneKey{view: viewIssue, depth: 1}, "push"},
		{"back", sceneKey{view: viewIssue, depth: 1}, base, "pop"},
		{"jump", base, sceneKey{view: viewPRs}, "push"},
		{"tab right", sceneKey{view: viewFavs, tab: 0, fixedTab: 3}, sceneKey{view: viewFavs, tab: 1, fixedTab: 3}, "push"},
		{"tab left", sceneKey{view: viewFavs, tab: 2, fixedTab: 3}, sceneKey{view: viewFavs, tab: 1, fixedTab: 3}, "pop"},
		{"dialog opens", base, sceneKey{view: viewIssues, overlay: 2}, "rise"},
		{"dialog closes", sceneKey{view: viewIssues, overlay: 2}, base, ""},
	}
	for _, c := range cases {
		tr := startTransition(c.old, c.cur, "frame", mt0)
		got := ""
		switch {
		case tr == nil:
		case tr.rise:
			got = "rise"
		case tr.dir > 0:
			got = "push"
		default:
			got = "pop"
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if startTransition(base, sceneKey{view: viewPRs}, "", mt0) != nil {
		t.Error("nothing to slide from on the first frame")
	}
}

func TestSlideFrames(t *testing.T) {
	const w = 10
	old := "head\nOLDOLDOLD\nold2"
	cur := "head\nNEWNEWNEW\nnew2"
	tr := &transition{from: old, dir: 1, fixed: 1, start: mt0, dur: 200 * time.Millisecond}
	at := func(ms int) []string {
		return strings.Split(tr.apply(cur, w, mt0.Add(time.Duration(ms)*time.Millisecond)), "\n")
	}
	if got := at(0); got[1] != "OLDOLDOLD " || got[0] != "head" {
		t.Errorf("start: %q", got)
	}
	if got := at(200); got[1] != "NEWNEWNEW " {
		t.Errorf("end: %q", got)
	}
	mid := at(8)
	if !strings.Contains(mid[1], "OLD") || !strings.Contains(mid[1], "NEW") {
		t.Errorf("mid-slide should show both screens: %q", mid[1])
	}
	for _, l := range mid {
		if l != "head" && ansi.StringWidth(l) != w {
			t.Errorf("line %q is %d wide, want %d", l, ansi.StringWidth(l), w)
		}
	}
	tr.dir = -1 // popping: the new screen comes from the left, so the old one leaves to the right
	if got := at(0); got[1] != "OLDOLDOLD " {
		t.Errorf("pop start: %q", got)
	}
	if got := at(200); got[1] != "NEWNEWNEW " {
		t.Errorf("pop end: %q", got)
	}
}

func TestRiseFrames(t *testing.T) {
	cur := "h1\nh2\nA\nB\nC\n"
	tr := &transition{rise: true, fixed: 2, start: mt0, dur: 100 * time.Millisecond}
	if got := tr.apply(cur, 10, mt0); got != "h1\nh2\n\n\n\n" && !strings.HasPrefix(got, "h1\nh2\n\n") {
		t.Errorf("start should be empty below the header: %q", got)
	}
	if got := tr.apply(cur, 10, mt0.Add(time.Second)); got != cur {
		t.Errorf("end should be the dialog in place: %q", got)
	}
}

func TestSelectionFade(t *testing.T) {
	mo := newMotion()
	if mo.level("s", "a", true, mt0) != 1 || mo.level("s", "b", false, mt0) != 0 {
		t.Error("first selection is simply lit")
	}
	if mo.level("s", "b", true, mt0) != 0 || mo.level("s", "a", false, mt0) != 1 {
		t.Error("moving: the new item starts dark, the old one still lit")
	}
	mid := mt0.Add(fadeTime / 2)
	if l := mo.level("s", "b", true, mid); l <= 0 || l >= 1 {
		t.Errorf("half-way: %v", l)
	}
	end := mt0.Add(2 * fadeTime)
	if mo.level("s", "b", true, end) != 1 || mo.level("s", "a", false, end) != 0 {
		t.Error("settled")
	}
	// turning around mid-fade does not jump
	mo.level("s", "a", true, end)
	before := mo.level("s", "a", true, end.Add(fadeTime/2))
	mo.level("s", "b", true, end.Add(fadeTime/2))
	after := mo.level("s", "a", false, end.Add(fadeTime/2))
	if before != after {
		t.Errorf("a turned around at %v and jumped to %v", before, after)
	}
}

func TestTabHighlightSlides(t *testing.T) {
	motionNow = func() time.Time { return mt0 }
	defer func() { motionNow = time.Now }()
	m := model{mo: newMotion(), width: 40}
	labels := []string{" One ", " Two ", " Three "}
	still := fitTabs(labels, 0, 40)
	if got := m.tabs("x", labels, 0); got != still {
		t.Errorf("a strip at rest is the plain one: %q vs %q", got, still)
	}
	got := m.tabs("x", labels, 2) // the highlight starts to move at mt0
	if ansi.Strip(got) != ansi.Strip(fitTabs(labels, 2, 40)) {
		t.Errorf("the text never changes, only the highlight: %q", ansi.Strip(got))
	}
	motionNow = func() time.Time { return mt0.Add(time.Second) }
	if got := m.tabs("x", labels, 2); got != fitTabs(labels, 2, 40) {
		t.Errorf("after the motion the plain strip is back: %q", got)
	}
	if (model{width: 40}).tabs("x", labels, 1) != fitTabs(labels, 1, 40) {
		t.Error("no motion state: the plain strip")
	}
}

func TestMotionOffByEnv(t *testing.T) {
	m := model{mo: newMotion()}
	if !m.motionOn() {
		t.Fatal("on by default")
	}
	t.Setenv("GH9S_MOTION", "off")
	if m.motionOn() || m.spin() != "" {
		t.Error("GH9S_MOTION=off means no motion and no spinner")
	}
}
