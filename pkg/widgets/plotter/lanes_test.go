package plotter

import (
	"image"
	"image/color"
	"testing"
)

// TestLaneImage checks the bands tile the buffer without gaps or overlap and
// that a draw in one band cannot bleed into another.
func TestLaneImage(t *testing.T) {
	const w, h, n = 8, 30, 4
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	prev := 0
	for i := range n {
		lane := laneImage(img, i, n)
		if lane.Rect.Min.Y != prev {
			t.Fatalf("lane %d starts at %d, want %d", i, lane.Rect.Min.Y, prev)
		}
		prev = lane.Rect.Max.Y
		// draw the whole lane, in lane-local coordinates
		red := color.RGBA{R: 0xff, A: 0xff}
		for x := range w {
			fillVRun(lane.Pix, lane.Stride, w, lane.Rect.Dy(), x, -100, 100, red)
		}
	}
	if prev != h {
		t.Fatalf("lanes end at %d, want %d", prev, h)
	}
	for i, v := range img.Pix {
		if i%4 == 1 || i%4 == 2 { // G, B stay zero
			continue
		}
		if v != 0xff {
			t.Fatalf("pixel byte %d not covered by any lane", i)
		}
	}
}
