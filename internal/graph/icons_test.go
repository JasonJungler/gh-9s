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
