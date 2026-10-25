package tui

import (
	"testing"
	"time"
)

func TestCardFxLevel(t *testing.T) {
	t0 := time.Unix(100, 0)
	fx := cardFx{prev: "a", cur: "b", start: t0}
	if fx.level("b", true, t0) != 0 || fx.level("a", false, t0) != 1 {
		t.Error("at the start the old card is still lit and the new one dark")
	}
	end := t0.Add(cardFade)
	if fx.level("b", true, end) != 1 || fx.level("a", false, end) != 0 {
		t.Error("at the end the new card is lit and the old one dark")
	}
	mid := t0.Add(cardFade / 2)
	if l := fx.level("b", true, mid); l <= 0 || l >= 1 {
		t.Errorf("half-way should be between, got %v", l)
	}
	if fx.level("c", false, mid) != 0 || fx.level("c", true, mid) != 1 {
		t.Error("other cards are not touched by the fade")
	}
	if (cardFx{}).level("x", true, t0) != 1 || (cardFx{}).active(t0) {
		t.Error("no fade: the selected card is simply lit")
	}
}
