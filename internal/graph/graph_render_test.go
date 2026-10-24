package graph

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/MurabitoB/gh-9s/internal/workflow"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii) // goldens must not contain escape codes
	os.Exit(m.Run())
}

var base = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func at(s int) time.Time { return base.Add(time.Duration(s) * time.Second) }

// fixture: lint,test -> build -> deploy-staging,e2e -> release (the spec's example run).
func fixture() Graph {
	mk := func(name, status, concl, step string, start, end int) Job {
		j := Job{ID: name, Name: name, Status: status, Conclusion: concl, Step: step}
		if start >= 0 {
			j.Start = at(start)
		}
		if end >= 0 {
			j.End = at(end)
		}
		return j
	}
	jobs := []Job{
		mk("lint", "completed", "success", "", 0, 12),
		mk("test", "completed", "success", "", 0, 48),
		mk("build", "completed", "success", "", 48, 112),
		mk("deploy-staging", "in_progress", "", "Apply manifests", 112, -1),
		mk("e2e", "in_progress", "", "Run playwright", 112, -1),
		mk("release", "queued", "", "", -1, -1),
	}
	specs := []workflow.JobSpec{
		{ID: "lint", Name: "lint"}, {ID: "test", Name: "test"},
		{ID: "build", Name: "build", Needs: []string{"lint", "test"}},
		{ID: "deploy-staging", Name: "deploy-staging", Needs: []string{"build"}},
		{ID: "e2e", Name: "e2e", Needs: []string{"build"}},
		{ID: "release", Name: "release", Needs: []string{"deploy-staging", "e2e"}},
	}
	return Build(jobs, specs)
}

func golden(t *testing.T, name string, lines []string) {
	t.Helper()
	got := strings.Join(lines, "\n") + "\n"
	path := filepath.Join("testdata", name+".golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden (run with -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from golden.\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

func TestRenderBoxes(t *testing.T) {
	lines, used := Render(fixture(), StyleBoxes, 2, 120, at(153))
	if used != StyleBoxes {
		t.Fatalf("used = %v; want boxes", used)
	}
	golden(t, "boxes", lines)
}

func TestRenderTree(t *testing.T) {
	lines, used := Render(fixture(), StyleTree, 3, 100, at(153))
	if used != StyleTree {
		t.Fatalf("used = %v", used)
	}
	golden(t, "tree", lines)
}

func TestRenderLanes(t *testing.T) {
	lines, used := Render(fixture(), StyleLanes, 4, 80, at(153))
	if used != StyleLanes {
		t.Fatalf("used = %v", used)
	}
	golden(t, "lanes", lines)
}

func TestBoxesFallBackToTree(t *testing.T) {
	lines, used := Render(fixture(), StyleBoxes, 0, 20, at(153))
	if used != StyleTree || len(lines) == 0 {
		t.Errorf("used=%v lines=%d; want tree fallback", used, len(lines))
	}
}

func TestRenderStaysWithinWidth(t *testing.T) {
	for _, style := range []Style{StyleBoxes, StyleTree, StyleLanes} {
		for _, w := range []int{30, 40, 60, 80, 120} {
			lines, _ := Render(fixture(), style, 1, w, at(153))
			for _, l := range lines {
				if n := utf8.RuneCountInString(l); n > w {
					t.Errorf("style %v width %d: line is %d wide: %q", style, w, n, l)
				}
			}
		}
	}
}

func TestRenderTinyAndEmpty(t *testing.T) {
	one := Build([]Job{{ID: "a", Name: "a", Status: "completed", Conclusion: "success"}}, []workflow.JobSpec{{ID: "a", Name: "a"}})
	flat := FlatGraph([]Job{{ID: "a", Name: "a", Status: "queued"}}, "why")
	for _, style := range []Style{StyleBoxes, StyleTree, StyleLanes} {
		for _, w := range []int{-1, 0, 1, 5, 80} {
			for _, g := range []Graph{{}, one, flat, fixture()} {
				lines, _ := Render(g, style, 0, w, at(10))
				if len(lines) == 0 {
					t.Errorf("style %v width %d Graph %d Nodes: no lines", style, w, len(g.Nodes))
				}
			}
		}
	}
}

func TestNextCycles(t *testing.T) {
	s := StyleBoxes
	for _, want := range []Style{StyleTree, StyleLanes, StyleBoxes} {
		if s = s.Next(); s != want {
			t.Fatalf("next = %v; want %v", s, want)
		}
	}
}
