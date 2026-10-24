package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
)

func sidebarPR() ghcli.PRDetail {
	p := fullPR()
	p.Reviewers = []ghcli.Reviewer{{Name: "alice", State: "REQUESTED"}, {Name: "bob", State: "APPROVED"}}
	p.Assignees = []string{"carol"}
	p.Milestone = "v1.0"
	return p
}

func TestWidePRDetailPutsTheSidebarBesideTheText(t *testing.T) {
	p := sidebarPR()
	wide := prDetailLines(p, 140)
	var sideRows, mainRow int = 0, -1
	for i, l := range wide.lines {
		s := ansi.Strip(l)
		if w := ansi.StringWidth(l); w > 140 {
			t.Errorf("line %d is %d wide", i, w)
		}
		if strings.Contains(s, "│") && strings.TrimSpace(strings.SplitN(s, "│", 2)[1]) != "" {
			sideRows++
		}
		if strings.Contains(s, "Description") {
			mainRow = i
		}
	}
	txt := plainText(wide.lines)
	for _, want := range []string{"alice requested", "bob approved", "carol", "v1.0", "Description", "Discussion", "looks good", "#482"} {
		if !strings.Contains(txt, want) {
			t.Errorf("wide detail lacks %q:\n%s", want, txt)
		}
	}
	if sideRows < 8 || mainRow < 0 {
		t.Errorf("the sidebar should fill a column beside the description (%d rows, description at %d):\n%s", sideRows, mainRow, txt)
	}
	// an image marker keeps its row, so selecting it still highlights the right line
	for _, im := range wide.images {
		if im.Line >= len(wide.lines) || !strings.Contains(ansi.Strip(wide.lines[im.Line]), "image") {
			t.Errorf("image marker at %d points at %q", im.Line, ansi.Strip(wide.lines[min(im.Line, len(wide.lines)-1)]))
		}
	}
	narrow := plainText(prDetailLines(p, 80).lines)
	if strings.Contains(narrow, "│") || !strings.Contains(narrow, "Reviewers") {
		t.Errorf("a narrow screen keeps one column, with the sidebar above the description:\n%s", narrow)
	}
}

func TestWideIssueDetailPutsLabelsAndAssigneesBesideTheText(t *testing.T) {
	d := ghcli.IssueDetail{Number: 9, Title: "Crash", Author: "carol", State: "OPEN", Labels: []string{"bug"}, Assignees: []string{"dave"}, Body: "steps to reproduce"}
	wide := plainText(issueLines(d, 140).lines)
	if !strings.Contains(wide, "│") || !strings.Contains(wide, "Labels") || !strings.Contains(wide, "dave") || !strings.Contains(wide, "steps to reproduce") {
		t.Errorf("wide issue:\n%s", wide)
	}
	if narrow := plainText(issueLines(d, 80).lines); strings.Contains(narrow, "│") || !strings.Contains(narrow, "Labels: bug") {
		t.Errorf("narrow issue:\n%s", narrow)
	}
}
