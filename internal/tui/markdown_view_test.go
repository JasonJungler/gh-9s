package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func prWithBody(t *testing.T, body string, comments ...ghcli.Comment) model {
	t.Helper()
	d := fixturePR()
	d.Body, d.Comments = body, comments
	installPRFetch(t, d)
	m := prListModel()
	m.target, m.prTarget = acme, acme
	return openDetail(t, m)
}

type shown struct{ urls []string }

func recordShowImage(t *testing.T) *shown {
	t.Helper()
	s := &shown{}
	old := showImage
	showImage = func(m model, url string) (model, tea.Cmd) { s.urls = append(s.urls, url); return m, nil }
	t.Cleanup(func() { showImage = old })
	return s
}

func TestDescriptionIsFormattedMarkdown(t *testing.T) {
	m := prWithBody(t, "## Plan\n\n- [x] a\n\n![shot](https://h/p.png)", ghcli.Comment{Author: "bob", Body: "**ok** then", CreatedAt: "2026-10-20T10:00:00Z"})
	v := m.View()
	for _, want := range []string{"Plan", "☑ a", "[image: shot]", "ok then"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	for _, bad := range []string{"##", "**", "- [x]"} {
		if strings.Contains(v, bad) {
			t.Errorf("view still shows raw markdown %q:\n%s", bad, v)
		}
	}
}

func TestTabSelectsImageMarkersAndEnterShowsThem(t *testing.T) {
	s := recordShowImage(t)
	m := prWithBody(t, "one ![a](https://h/1.png) two\n\n![b](https://h/2.png)")
	m = keys(m, "enter")
	if len(s.urls) != 0 {
		t.Fatalf("enter with no marker selected showed %v", s.urls)
	}
	m = keys(m, "tab", "enter")
	if len(s.urls) != 1 || s.urls[0] != "https://h/1.png" {
		t.Fatalf("shown %v; want the first image", s.urls)
	}
	m = keys(m, "tab", "enter")
	m = keys(m, "tab", "enter") // wraps round to the first
	m = keys(m, "shift+tab", "enter")
	if want := []string{"https://h/1.png", "https://h/2.png", "https://h/1.png", "https://h/2.png"}; strings.Join(s.urls, " ") != strings.Join(want, " ") {
		t.Errorf("shown %v; want %v", s.urls, want)
	}
}

func TestSelectedMarkerScrollsIntoView(t *testing.T) {
	body := strings.Repeat("filler line\n\n", 60) + "![far](https://h/far.png)"
	m := prWithBody(t, body)
	m = keys(m, "tab")
	if v := m.View(); !strings.Contains(v, "[image: far]") {
		t.Errorf("the selected marker is not on screen:\n%s", v)
	}
}

func TestOOnASelectedMarkerOpensTheImageInTheBrowser(t *testing.T) {
	opened := recordOpenURL(t)
	m := prWithBody(t, "![a](https://h/1.png)")
	m = keys(m, "tab", "o")
	if len(*opened) != 1 || (*opened)[0] != "https://h/1.png" {
		t.Errorf("opened %v; want the image URL", *opened)
	}
	_ = keys(m, "esc")
	opened2 := recordOpenURL(t)
	m = prWithBody(t, "![a](https://h/1.png)")
	keys(m, "o")
	if len(*opened2) != 1 || (*opened2)[0] != "https://github.com/acme/api/pull/482" {
		t.Errorf("with no marker, o opens the PR; opened %v", *opened2)
	}
}

func TestIssueDescriptionAndMarkersToo(t *testing.T) {
	s := recordShowImage(t)
	d := fixtureIssue()
	d.Body = "### Steps\n\n1. run it\n\n![log](https://h/l.png)"
	installIssues(t, d)
	m := issuesLoaded(t)
	m = keyAll(m, "enter")
	v := m.View()
	if !strings.Contains(v, "Steps") || strings.Contains(v, "###") || !strings.Contains(v, "[image: log]") {
		t.Errorf("issue not formatted:\n%s", v)
	}
	keys(keys(m, "tab"), "enter")
	if len(s.urls) != 1 || s.urls[0] != "https://h/l.png" {
		t.Errorf("shown %v", s.urls)
	}
}

func TestDetailLinesStayWithinTheWidth(t *testing.T) {
	d := fixturePR()
	d.Body = "| a | b |\n|-|-|\n| " + strings.Repeat("x", 80) + " | y |\n\n" + strings.Repeat("word ", 60)
	lines := prDetailLines(d, 40).lines
	for i, l := range lines {
		if i < 5 { // the header lines are not markdown and have never been wrapped
			continue
		}
		if ansi.StringWidth(l) > 40 {
			t.Errorf("line %q is %d wide; the screen is 40", ansi.Strip(l), ansi.StringWidth(l))
		}
	}
}

func TestOnlyAFetchableLinkIsEverOpenedFromAMarker(t *testing.T) {
	for _, src := range []string{"-aTerminal", `\\attacker.example\share\x.exe`, "file:///System/Applications/Calculator.app"} {
		opened := recordOpenURL(t)
		m := prWithBody(t, "<img src=\""+src+"\" alt=\"x\">")
		m = keys(m, "tab", "o")
		if len(*opened) != 0 || !strings.Contains(m.status, "can't open") {
			t.Errorf("%q: opened=%v status=%q; want nothing opened and a refusal", src, *opened, m.status)
		}
	}
	opened := recordOpenURL(t)
	m := prWithBody(t, "![x](/org/repo/raw/p.png)")
	keys(m, "tab", "o")
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/org/repo/raw/p.png" {
		t.Errorf("a relative link is opened as the absolute https link: %v", *opened)
	}
}

func TestScrollingAwayFromTheMarkerReleasesO(t *testing.T) {
	opened := recordOpenURL(t)
	m := prWithBody(t, "![a](https://h/1.png)\n\n"+strings.Repeat("filler\n\n", 80))
	m = keys(m, "tab", "G", "o")
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/acme/api/pull/482" {
		t.Errorf("with the marker scrolled out of view, o must open the PR; opened %v", *opened)
	}
}

func TestShiftTabWithNothingSelectedGoesToTheLastMarker(t *testing.T) {
	s := recordShowImage(t)
	m := prWithBody(t, "![a](https://h/1.png)\n\n![b](https://h/2.png)\n\n![c](https://h/3.png)")
	keys(keys(m, "shift+tab"), "enter")
	if len(s.urls) != 1 || s.urls[0] != "https://h/3.png" {
		t.Errorf("shown %v; want the last image", s.urls)
	}
}
