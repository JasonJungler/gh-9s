package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func paneModel(t *testing.T, width int, pct int) model {
	t.Helper()
	m, _ := settingsModel(t, "", nil)
	m.view, m.width, m.height, m.target = viewPRs, width, 30, acme
	if pct > 0 {
		if _, err := m.settings.Set("preview_width", itoa(pct)); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func itoa(n int) string { return fmt.Sprint(n) }

func TestPaneWidths(t *testing.T) {
	for _, w := range []int{80, 100, 119, 120, 160, 200} {
		for _, pct := range []int{25, 40, 60} {
			m := paneModel(t, w, pct)
			lw, rw := m.paneWidths()
			if lw == 0 && rw == 0 {
				if w >= paneMinList+paneMinPane {
					t.Errorf("w=%d pct=%d: pane dropped though it fits", w, pct)
				}
				continue
			}
			if lw < paneMinList || rw < paneMinPane || lw+rw != w {
				t.Errorf("w=%d pct=%d: list %d pane %d", w, pct, lw, rw)
			}
		}
	}
	if lw, rw := paneModel(t, 79, 40).paneWidths(); lw != 0 || rw != 0 {
		t.Errorf("79 columns cannot hold both: %d %d", lw, rw)
	}
}

func TestPreviewShownByWidthSettingToggleAndScreen(t *testing.T) {
	m := paneModel(t, 140, 0)
	if !m.previewShown() {
		t.Fatal("140 columns on a PR list shows the pane")
	}
	if narrow := paneModel(t, 119, 0); narrow.previewShown() {
		t.Error("below preview_min_columns there is no pane")
	}
	runs := m
	runs.view = viewRuns
	if runs.previewShown() {
		t.Error("only the PR list, issue list and inbox have a pane")
	}
	m.settings.Set("preview", "off")
	if m.previewShown() {
		t.Error("the setting turns it off")
	}
	m.settings.Set("preview", "on")
	m = keys(m, "p")
	if !m.previewOff || m.previewShown() {
		t.Error("p switches the pane off for the session")
	}
	m = keys(m, "p")
	if m.previewOff {
		t.Error("p switches it back on")
	}
	if !m.cfg().Bool("preview") {
		t.Error("the toggle must not touch the setting")
	}
}

func TestJoinPanesIsExactlyTheRequestedSize(t *testing.T) {
	left := "plain\n\x1b[31mred 文件 text\x1b[0m\n" + strings.Repeat("x", 300) + "\ttab"
	right := "short"
	for _, c := range []struct{ lw, rw, h int }{{60, 40, 8}, {50, 30, 3}, {101, 59, 12}} {
		out := joinPanes(left, right, c.lw, c.rw, c.h)
		lines := strings.Split(out, "\n")
		if len(lines) != c.h {
			t.Errorf("%+v: %d lines", c, len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != c.lw+c.rw {
				t.Errorf("%+v line %d is %d wide", c, i, w)
			}
		}
		if !strings.HasPrefix(ansi.Strip(lines[0]), "╭") || !strings.HasSuffix(ansi.Strip(lines[0]), "╮") {
			t.Errorf("rounded corners expected: %q", ansi.Strip(lines[0]))
		}
	}
}

func TestListWithPaneFillsTheScreenAndTheSelectionTheListWidth(t *testing.T) {
	m := paneModel(t, 140, 0)
	m.tbl = prRows([]prListItem{{Number: 1, Title: "one"}, {Number: 2, Title: "two"}}, "")
	lw, rw := m.paneWidths()
	out := m.withPane(func(x model) string { return x.tableBody() })
	for i, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := ansi.StringWidth(l); w != lw+rw {
			t.Errorf("line %d is %d wide, want %d", i, w, lw+rw)
		}
	}
	inner := m
	inner.width, inner.paneList = lw-2, true
	var sel string
	for _, l := range strings.Split(inner.tableBody(), "\n") {
		if strings.Contains(l, "one") {
			sel = l
		}
	}
	if ansi.StringWidth(sel) != lw-2 {
		t.Errorf("the selected row is %d wide, want the list width %d", ansi.StringWidth(sel), lw-2)
	}
}

func TestBelowTheThresholdTheListIsAsBefore(t *testing.T) {
	m := paneModel(t, 100, 0)
	m.tbl = prRows([]prListItem{{Number: 1, Title: "one"}}, "")
	if strings.Contains(ansi.Strip(m.View()), "╭") {
		t.Error("no borders below the threshold")
	}
}
