package anim

import (
	"testing"
	"time"
)

func TestBezierEndpointsAndMonotone(t *testing.T) {
	for name, e := range map[string]Ease{"dec": Decelerate, "acc": Accelerate, "std": Standard} {
		if e(0) != 0 || e(1) != 1 {
			t.Errorf("%s: endpoints %v %v", name, e(0), e(1))
		}
		prev := 0.0
		for i := 1; i <= 100; i++ {
			v := e(float64(i) / 100)
			if v < prev-1e-9 {
				t.Fatalf("%s not monotone at %d: %v < %v", name, i, v, prev)
			}
			prev = v
		}
	}
	if Decelerate(0.3) <= 0.3 {
		t.Error("decelerate should be ahead of linear early on")
	}
}

func TestTweenRetargetDoesNotJump(t *testing.T) {
	t0 := time.Unix(0, 0)
	tw := To(0, 10, t0, 200*time.Millisecond, Standard)
	mid := t0.Add(80 * time.Millisecond)
	before := tw.At(mid)
	after := tw.Retarget(0, mid, 200*time.Millisecond, Standard).At(mid)
	if before != after {
		t.Errorf("jumped: %v -> %v", before, after)
	}
	if !tw.Done(t0.Add(time.Second)) || tw.At(t0.Add(time.Second)) != 10 {
		t.Error("should finish at 10")
	}
}

func TestMix(t *testing.T) {
	if got := Mix("#000000", "#ffffff", 0.5); got != "#808080" {
		t.Errorf("got %s", got)
	}
	if Mix("#102030", "#ffffff", 0) != "#102030" || Mix("#102030", "#ffffff", 1) != "#ffffff" {
		t.Error("endpoints")
	}
	if Mix("nope", "#ffffff", 0.5) != "nope" {
		t.Error("malformed should pass through")
	}
}
