// Package anim is the small motion toolkit of the Fluent-style animation experiment: easing curves, tweens over
// time, and colour blending. It knows nothing about Bubble Tea; callers pass the clock in.
package anim

import (
	"fmt"
	"math"
	"time"
)

// Ease maps linear progress 0..1 to eased progress 0..1.
type Ease func(t float64) float64

// Bezier is a CSS-style cubic-bezier(x1, y1, x2, y2) easing curve.
func Bezier(x1, y1, x2, y2 float64) Ease {
	coord := func(a, b, s float64) float64 {
		return 3*a*s*(1-s)*(1-s) + 3*b*s*s*(1-s) + s*s*s
	}
	return func(t float64) float64 {
		if t <= 0 {
			return 0
		}
		if t >= 1 {
			return 1
		}
		lo, hi := 0.0, 1.0
		for range 24 {
			mid := (lo + hi) / 2
			if coord(x1, x2, mid) < t {
				lo = mid
			} else {
				hi = mid
			}
		}
		return coord(y1, y2, (lo+hi)/2)
	}
}

// Fluent's motion curves: Decelerate for things entering, Accelerate for things leaving, Standard for in-place changes.
var (
	Decelerate = Bezier(0.1, 0.9, 0.2, 1)
	Accelerate = Bezier(0.9, 0.1, 1, 0.2)
	Standard   = Bezier(0.8, 0, 0.2, 1)
)

// Tween moves a value from one number to another over a duration.
type Tween struct {
	from, to float64
	start    time.Time
	dur      time.Duration
	ease     Ease
}

// Still is a tween that is already at v.
func Still(v float64) Tween { return Tween{from: v, to: v} }

// To starts a tween at now.
func To(from, to float64, now time.Time, dur time.Duration, e Ease) Tween {
	return Tween{from: from, to: to, start: now, dur: dur, ease: e}
}

// At is the value at now.
func (t Tween) At(now time.Time) float64 {
	if t.Done(now) {
		return t.to
	}
	p := float64(now.Sub(t.start)) / float64(t.dur)
	return t.from + (t.to-t.from)*t.ease(p)
}

// Done reports whether the tween has arrived.
func (t Tween) Done(now time.Time) bool {
	return t.dur <= 0 || t.ease == nil || now.Sub(t.start) >= t.dur
}

// Retarget starts a new tween from wherever this one is now, so an interrupted motion turns around smoothly
// instead of jumping.
func (t Tween) Retarget(to float64, now time.Time, dur time.Duration, e Ease) Tween {
	return To(t.At(now), to, now, dur, e)
}

// Mix blends two #rrggbb colours; p is 0 for a, 1 for b. A malformed colour is returned as a.
func Mix(a, b string, p float64) string {
	var ar, ag, ab, br, bg, bb int
	if _, err := fmt.Sscanf(a, "#%02x%02x%02x", &ar, &ag, &ab); err != nil {
		return a
	}
	if _, err := fmt.Sscanf(b, "#%02x%02x%02x", &br, &bg, &bb); err != nil {
		return a
	}
	p = math.Max(0, math.Min(1, p))
	mix := func(x, y int) int { return int(math.Round(float64(x) + float64(y-x)*p)) }
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}
