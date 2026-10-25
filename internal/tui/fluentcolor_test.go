package tui

import (
	"strings"
	"testing"
)

func TestFluentColorNeedsTruecolor(t *testing.T) {
	old := trueColor
	defer func() { trueColor = old }()

	trueColor = func() bool { return false }
	if got := fluentCard("x"); got != "x" {
		t.Errorf("card without truecolor = %q, want plain", got)
	}
	if got := fluentSel("x"); got != stySel.Render("x") {
		t.Errorf("selection without truecolor = %q, want reverse video", got)
	}
}

func TestFluentColorTruecolor(t *testing.T) {
	old := trueColor
	defer func() { trueColor = old }()
	trueColor = func() bool { return true }
	// the test profile strips colour codes, so only check the text survives
	for _, s := range []string{fluentSel("hello"), fluentCard("hello")} {
		if !strings.Contains(s, "hello") {
			t.Errorf("text lost: %q", s)
		}
	}
}
