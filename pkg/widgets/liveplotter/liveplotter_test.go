package liveplotter

import "testing"

// newBare builds a Widget with only the fields the data path needs, so the
// windowing/retention logic can be exercised without a Fyne app.
func newBare(order []string, windowMillis int64) *Widget {
	p := &Widget{
		order:        order,
		windowMillis: windowMillis,
		follow:       true,
		latest:       map[string]float64{},
		values:       map[string][]float64{},
	}
	for _, n := range order {
		p.values[n] = nil
	}
	return p
}

// span at full zoom equals the window.
func fullSpan(p *Widget) int64 { return p.windowMillis }

func TestFollowWindowSlides(t *testing.T) {
	const window = 120_000 // 120s
	p := newBare([]string{"a"}, window)

	// 300s of frames at 10Hz.
	const hz = 10
	const dur = 300_000
	for ms := int64(0); ms <= dur; ms += 1000 / hz {
		p.latest["a"] = float64(ms)
		p.ingest(ms, false /*not paused*/)
	}

	newest := p.times[len(p.times)-1]
	oldest := p.times[0]

	// Retention: compaction keeps at most ~2x the window, and never less than
	// the window once full.
	span := newest - oldest
	if span > 2*window+1000 {
		t.Fatalf("retained span %dms exceeds 2x window", span)
	}
	if span < window-1000 {
		t.Fatalf("retained span %dms dropped below window", span)
	}

	// Visible window in follow mode: right edge at newest, ~window wide, and the
	// oldest visible sample is well after the very first frame (old data phased
	// out).
	start, end, rightT := viewRange(p.times, true, 0, fullSpan(p))
	if rightT != newest {
		t.Fatalf("follow right edge = %d, want newest %d", rightT, newest)
	}
	if start == 0 {
		t.Fatalf("visible window still starts at index 0; old entries never phased out")
	}
	visibleSpan := p.times[end-1] - p.times[start]
	if visibleSpan > window+1000 || visibleSpan < window-2000 {
		t.Fatalf("visible span %dms, want ~%dms", visibleSpan, window)
	}
}

func TestPausedFreezesAndRetains(t *testing.T) {
	const window = 120_000
	p := newBare([]string{"a"}, window)

	for ms := int64(0); ms <= 200_000; ms += 100 {
		p.latest["a"] = float64(ms)
		p.ingest(ms, false)
	}

	// Pause: anchor at the current newest, then keep ingesting without compaction.
	p.follow = false
	anchor := p.times[len(p.times)-1]
	preLen := len(p.times)
	for ms := int64(200_100); ms <= 600_000; ms += 100 {
		p.latest["a"] = float64(ms)
		p.ingest(ms, true /*paused*/)
	}

	// Frozen view: right edge stays at the anchor regardless of new data.
	start, end, rightT := viewRange(p.times, false, anchor, fullSpan(p))
	if rightT != anchor {
		t.Fatalf("paused right edge = %d, want anchor %d", rightT, anchor)
	}
	if got := p.times[end-1]; got > anchor {
		t.Fatalf("frozen view includes sample at %d past anchor %d", got, anchor)
	}
	visibleSpan := p.times[end-1] - p.times[start]
	if visibleSpan < window-2000 {
		t.Fatalf("frozen visible span %dms collapsed below window", visibleSpan)
	}

	// Retention halted: nothing was trimmed while paused.
	if len(p.times) <= preLen {
		t.Fatalf("paused buffer did not grow: pre=%d post=%d", preLen, len(p.times))
	}
	if p.times[0] != 0 {
		t.Fatalf("paused buffer trimmed old data: oldest=%d, want 0", p.times[0])
	}
}
