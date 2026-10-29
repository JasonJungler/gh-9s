package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestShortcutHintsHighlightKeysAndDimDescriptions(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	got := shortcutHints("enter open  j/k move  ? help")
	if plain := ansi.Strip(got); plain != "enter: open  j/k: move  ?: help" {
		t.Fatalf("plain hints = %q", plain)
	}
	for _, want := range []string{styKey.Render("enter:"), styKey.Render("j/k:"), styDim.Render("open"), styDim.Render("move")} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered hints lack %q: %q", want, got)
		}
	}
}

func TestLabelValueHighlightsOnlyTheLabel(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	got := labelValue("Review", styOK.Render("approved"))
	if ansi.Strip(got) != "Review: approved" {
		t.Fatalf("plain label = %q", ansi.Strip(got))
	}
	if !strings.HasPrefix(got, styKey.Render("Review:")) || !strings.Contains(got, styOK.Render("approved")) {
		t.Errorf("label or value style was lost: %q", got)
	}
}

func TestHintsRemainReadableWithoutColour(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	if got := shortcutHints("enter open  esc back"); got != "enter: open  esc: back" {
		t.Errorf("plain terminal hints = %q", got)
	}
	if got := labelValue("Checks", "3 passed"); got != "Checks: 3 passed" {
		t.Errorf("plain terminal label = %q", got)
	}
}

func TestHintAndMetadataViewsUseTheHighlightRenderer(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	footer := (model{view: viewWorkflows}).footer()
	if !strings.Contains(footer, styKey.Render("enter:")) || !strings.Contains(footer, styDim.Render("runs")) {
		t.Errorf("footer is not rendered as highlighted key and dim description: %q", footer)
	}
	help := strings.Join((model{view: viewWorkflows}).helpLines(80), "\n")
	if !strings.Contains(help, styKey.Render("j k:")) {
		t.Errorf("help does not highlight the key label: %q", help)
	}
	detail := strings.Join(prDetailLines(fullPR(), 100).lines, "\n")
	for _, label := range []string{"Review:", "Merge:", "Checks:"} {
		if !strings.Contains(detail, styKey.Render(label)) {
			t.Errorf("detail does not highlight %q: %q", label, detail)
		}
	}
}

func TestFunctionBarHighlightsKeysAndDimsLabels(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	got := functionBar(0)
	if plain := ansi.Strip(got); plain != "<1> pulls <2> issues <3> runs <4> workflows <5> disc <6> wiki <0> inbox <:fav> favs <:repo> repo <P> current-PR <:> cmd </> filter" {
		t.Fatalf("plain function bar = %q", plain)
	}
	for _, want := range []string{styNumKey.Render("<1>"), styKey.Render("<:fav>"), styKey.Render("</>"), styDim.Render("pulls")} {
		if !strings.Contains(got, want) {
			t.Errorf("function bar lacks %q: %q", want, got)
		}
	}
	if styNumKey.Render("<1>") == styKey.Render("<1>") {
		t.Error("numbered function keys and ordinary keywords must use different colors")
	}
	if got := functionBar(40); ansi.StringWidth(got) > 40 {
		t.Errorf("narrow function bar is %d columns: %q", ansi.StringWidth(got), got)
	}
}
