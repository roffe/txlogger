package t7

import "testing"

// computeWriteSegments must never drop a non-0xFF byte (skipping erased flash is
// only safe if all real data is still written), must skip long 0xFF runs, and
// must absorb short 0xFF gaps rather than fragmenting.
func TestComputeWriteSegments(t *testing.T) {
	bin := make([]byte, 0x80000)
	for i := range bin {
		bin[i] = 0xFF
	}
	fill := func(a, b int) {
		for i := a; i < b; i++ {
			bin[i] = 0x5A
		}
	}
	// region 1: data, a long FF run (skipped), then data with a short FF gap (absorbed)
	fill(0x000000, 0x010000)                // data
	fill(0x010000+ffSkipMinRun, 0x020000)   // after a long FF run
	fill(0x030000, 0x030010)                // data ...
	fill(0x030010+ffSkipMinRun/2, 0x031000) // ... short gap ... more data (same segment)
	// PI area: data only at the very top
	fill(0x07FF80, 0x080000)
	// a byte in the never-written algorithm gap must be ignored even if non-FF
	bin[0x07C000] = 0x5A

	segs := computeWriteSegments(bin)

	// no non-0xFF byte inside a writable region may be dropped
	covered := make([]bool, len(bin))
	for _, s := range segs {
		if s.start >= s.end || s.start < 0 || s.end > len(bin) {
			t.Fatalf("bad segment %+v", s)
		}
		for i := s.start; i < s.end; i++ {
			covered[i] = true
		}
	}
	for _, r := range t7WriteRegions {
		for i := r.start; i < r.end; i++ {
			if bin[i] != 0xFF && !covered[i] {
				t.Fatalf("dropped data byte 0x%X", i)
			}
		}
	}
	// the algorithm-gap byte must NOT be written
	if covered[0x07C000] {
		t.Fatal("wrote into the flash-algorithm gap")
	}
	// long FF runs must actually be skipped -> real gaps between segments exist
	skipped, prev := 0, 0
	for _, s := range segs {
		skipped += s.start - prev
		prev = s.end
	}
	if skipped < ffSkipMinRun {
		t.Fatalf("expected long FF runs to be skipped, only skipped %d bytes", skipped)
	}
	t.Logf("%d segments", len(segs))
}
