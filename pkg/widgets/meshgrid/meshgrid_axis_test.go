package meshgrid

import (
	"math"
	"testing"
)

// With the floor plane viewed edge-on, the X and Y edges project onto the
// same screen line. The shorter-projected axis must be lifted outward so the
// two value-label bands stack instead of overprinting — and neither axis may
// disappear.
func TestAxisEdgeOnFloorStacksBands(t *testing.T) {
	m := testGrid(t)
	// Rz(30) yaws the floor so both edges keep a real projected length,
	// Rx(90) then tilts the camera level with the floor (identity is
	// top-down); both floor edges land on one horizontal screen line.
	m.cameraRotation = RotationMatrixX(90).Multiply(RotationMatrixZ(30))
	_, labels := m.computeAxisGeometry()

	// A value can format identically on both axes (e.g. "0"); only strings
	// unique to one axis identify its band.
	xSet, ySet := map[string]bool{}, map[string]bool{}
	for _, s := range m.xLabels {
		xSet[s] = true
	}
	for _, s := range m.yLabels {
		ySet[s] = true
	}

	var xs, ys []axisLabel
	for _, l := range labels {
		switch {
		case xSet[l.text] && !ySet[l.text]:
			xs = append(xs, l)
		case ySet[l.text] && !xSet[l.text]:
			ys = append(ys, l)
		}
	}
	if len(xs) == 0 || len(ys) == 0 {
		t.Fatalf("edge-on floor: want both axes' value labels drawn, got %d x and %d y", len(xs), len(ys))
	}
	// Both bands are horizontal here, so vertical distance is band distance.
	for _, a := range xs {
		for _, b := range ys {
			d := a.y - b.y
			if d < 0 {
				d = -d
			}
			if d < 20 {
				t.Fatalf("edge-on floor: %q(y=%.0f) and %q(y=%.0f) print into the same band", a.text, a.y, b.text, b.y)
			}
		}
	}

	// The names must ride their scale's lift: each sits beyond its own value
	// band (outward is +Y here), not stranded between the two stacked scales.
	var nameX, nameY axisLabel
	var haveNX, haveNY bool
	for _, l := range labels {
		switch l.text {
		case m.xlabel:
			nameX, haveNX = l, true
		case m.ylabel:
			nameY, haveNY = l, true
		}
	}
	if !haveNX || !haveNY {
		t.Fatal("edge-on floor: missing axis name labels")
	}
	maxY := func(ls []axisLabel) float32 {
		v := float32(math.Inf(-1))
		for _, l := range ls {
			if l.y > v {
				v = l.y
			}
		}
		return v
	}
	if nameX.y <= maxY(xs) {
		t.Fatalf("x name at y=%.0f not beyond its value band (max y=%.0f)", nameX.y, maxY(xs))
	}
	if nameY.y <= maxY(ys) {
		t.Fatalf("y name at y=%.0f not beyond its value band (max y=%.0f)", nameY.y, maxY(ys))
	}
}

// Sweep the camera over a grid of angles and require that no two value labels
// of the same axis overlap. This is the regression test for the always-labeled
// last tick landing on top of its stepped neighbor ("70006500") at projected
// lengths where the thinning step exceeds one.
func TestAxisValueLabelsNeverOverlap(t *testing.T) {
	m := testGrid(t)
	names := map[string]bool{m.xlabel: true, m.ylabel: true, m.zlabel: true}
	for pitch := 0; pitch <= 90; pitch += 15 {
		for yaw := 0; yaw < 360; yaw += 15 {
			m.cameraRotation = RotationMatrixX(float64(pitch)).Multiply(RotationMatrixZ(float64(yaw)))
			_, labels := m.computeAxisGeometry()
			for i := 0; i < len(labels); i++ {
				for j := i + 1; j < len(labels); j++ {
					a, b := labels[i], labels[j]
					if a.col != b.col || names[a.text] || names[b.text] {
						continue // same-axis value labels only
					}
					w := float32(len(a.text)+len(b.text)) * axisCharW / 2
					dx, dy := a.x-b.x, a.y-b.y
					if dx < 0 {
						dx = -dx
					}
					if dy < 0 {
						dy = -dy
					}
					if dx < w*0.9 && dy < 10 {
						t.Fatalf("pitch %d yaw %d: %q(%.0f,%.0f) overlaps %q(%.0f,%.0f)",
							pitch, yaw, a.text, a.x, a.y, b.text, b.x, b.y)
					}
				}
			}
		}
	}
}

// The axis corners are picked per frame by screen-space argmax; near a tie a
// wobbly drag would flip the winner every pixel and bounce the scales between
// edges. Find every handoff angle in a full spin, then dither around each like
// an unsteady mouse: the corners may settle once but must not oscillate.
func TestAxisCornersDontBounceWhileDragging(t *testing.T) {
	m := testGrid(t)
	pick := func(yaw float64) int {
		m.cameraRotation = RotationMatrixX(60).Multiply(RotationMatrixZ(yaw))
		m.computeAxisGeometry()
		return m.frontCornerIdx*4 + m.zCornerIdx
	}

	var handoffs []float64
	prev := pick(0)
	for y := 0.5; y <= 360; y += 0.5 {
		if c := pick(y); c != prev {
			handoffs = append(handoffs, y)
			prev = c
		}
	}
	if len(handoffs) == 0 {
		t.Fatal("no corner handoffs found in a full spin")
	}

	for _, h := range handoffs {
		flips := 0
		prev := pick(h - 0.25)
		for i := range 20 {
			y := h - 0.25
			if i%2 == 0 {
				y = h + 0.25
			}
			if c := pick(y); c != prev {
				flips++
				prev = c
			}
		}
		if flips > 1 {
			t.Fatalf("axis corners bounced %d times while dithering around yaw %.2f", flips, h)
		}
	}
}

// A top-down view collapses the vertical Z edge to (nearly) a point; its tick
// labels would all stack on that point and must be skipped.
func TestAxisTopDownSkipsZTicks(t *testing.T) {
	m := testGrid(t)
	m.cameraRotation = NewMatrix3x3() // identity: looking straight down Z
	_, labels := m.computeAxisGeometry()

	if len(m.zLabels) == 0 {
		t.Fatal("no z labels cached; expected the z axis to be active")
	}
	// A z value can format to the same string as an x/y tick (e.g. "100");
	// only strings unique to the z scale prove the collapsed edge drew ticks.
	xy := map[string]bool{}
	for _, s := range m.xLabels {
		xy[s] = true
	}
	for _, s := range m.yLabels {
		xy[s] = true
	}
	zVals := map[string]bool{}
	for _, s := range m.zLabels {
		if !xy[s] {
			zVals[s] = true
		}
	}
	for _, l := range labels {
		if zVals[l.text] {
			t.Fatalf("top-down view: z tick label %q drawn on a collapsed edge", l.text)
		}
	}
}
