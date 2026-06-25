package logplayer

import (
	"testing"
	"time"
)

// TestFrameTargetNoDrift simulates a jittery scheduler (every wake-up is a few ms
// late, as on a loaded Windows box) and checks that absolute scheduling keeps the
// per-frame timing error bounded, while the old relative-reset scheme accumulates
// it into unbounded drift. Same jitter, two schemes.
func TestFrameTargetNoDrift(t *testing.T) {
	const (
		frames = 5000
		gap    = 10 * time.Millisecond // recording cadence
		jitter = 2 * time.Millisecond  // how late each wake-up fires
		speed  = 1.0                   // real time
	)

	start := time.Unix(0, 0)
	recTime := func(i int) time.Time { return start.Add(time.Duration(i) * gap) }

	// Absolute: target is recomputed from the fixed anchor every frame.
	anchorWall, anchorRec := start, recTime(0)
	now := start
	var absMax time.Duration

	// Relative: the old scheme reset the next delay from the late firing instant.
	relNow := start
	var relMax time.Duration

	for i := 1; i <= frames; i++ {
		// absolute
		now = frameTarget(anchorWall, anchorRec, recTime(i), speed).Add(jitter)
		if e := absErr(now.Sub(start), recTime(i).Sub(start)); e > absMax {
			absMax = e
		}
		// relative: fire at previous-fire + gap, then add the same jitter
		relNow = relNow.Add(gap).Add(jitter)
		if e := absErr(relNow.Sub(start), recTime(i).Sub(start)); e > relMax {
			relMax = e
		}
	}

	// Absolute error never exceeds a single wake-up's jitter, no matter how long.
	if absMax > jitter {
		t.Fatalf("absolute scheme drifted: max error %v > jitter %v", absMax, jitter)
	}
	// Relative error grows ~jitter per frame: proves the bug the rewrite fixes.
	if relMax < time.Duration(frames/2)*jitter {
		t.Fatalf("relative scheme should accumulate drift, got only %v", relMax)
	}
	t.Logf("after %d frames: absolute max error %v, relative drift %v", frames, absMax, relMax)
}

func absErr(a, b time.Duration) time.Duration {
	if a < b {
		return b - a
	}
	return a - b
}
