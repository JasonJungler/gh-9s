package graph

import (
	"testing"
	"time"

	"github.com/MurabitoB/gh-9s/internal/icons"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

func TestNodesUseTheIcons(t *testing.T) {
	old := icons.Current()
	t.Cleanup(func() { icons.Use(old) })
	ok := Node{Name: "build", State: ui.StOK}
	queued := Node{Name: "queued", State: ui.StRun}
	icons.Use(icons.Plain)
	if sym, _ := nodeParts(ok, time.Now()); sym != "✓" {
		t.Errorf("plain = %q", sym)
	}
	if sym, _ := nodeParts(queued, time.Now()); sym != "○" {
		t.Errorf("plain queued = %q", sym)
	}
	icons.Use(icons.Nerd)
	if sym, _ := nodeParts(ok, time.Now()); sym != icons.GetIn(icons.Nerd, icons.RunOK) {
		t.Errorf("nerd = %q", sym)
	}
	if sym, _ := nodeParts(queued, time.Now()); sym != icons.GetIn(icons.Nerd, icons.RunOther) {
		t.Errorf("nerd queued = %q", sym)
	}
}

func TestGroupCountsUseTheIcons(t *testing.T) {
	old := icons.Current()
	t.Cleanup(func() { icons.Use(old) })
	jobs := []Job{{ID: "1", Name: "a", Status: "completed", Conclusion: "success"}, {ID: "2", Name: "b", Status: "completed", Conclusion: "failure"},
		{ID: "3", Name: "c", Status: "in_progress"}, {ID: "4", Name: "d", Status: "queued"}}
	icons.Use(icons.Plain)
	if got := countLabel(jobs); got != "1✓ 1✗ 1● 1○" {
		t.Errorf("plain = %q", got)
	}
	icons.Use(icons.Nerd)
	want := "1" + icons.GetIn(icons.Nerd, icons.RunOK) + " 1" + icons.GetIn(icons.Nerd, icons.RunFail) + " 1" + icons.GetIn(icons.Nerd, icons.RunActive) + " 1" + icons.GetIn(icons.Nerd, icons.RunOther)
	if got := countLabel(jobs); got != want {
		t.Errorf("nerd = %q; want %q", got, want)
	}
}
