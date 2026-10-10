package interpolate

import (
	"math"
	"reflect"
	"testing"
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < eps }

// Every interpolator rejects an empty x axis, y axis or data slice with an
// error instead of indexing into it.
func TestEmptyInputsError(t *testing.T) {
	ax := []int{0, 10}
	ax64 := []float64{0, 10}
	ax16 := []uint16{0, 10}
	d := []int{1, 2, 3, 4}
	d64 := []float64{1, 2, 3, 4}
	calls := map[string]func() error{
		"Interpolate64/x":  func() error { _, _, _, err := Interpolate64(nil, ax64, d64, 1, 1); return err },
		"Interpolate64/y":  func() error { _, _, _, err := Interpolate64(ax64, nil, d64, 1, 1); return err },
		"Interpolate64/d":  func() error { _, _, _, err := Interpolate64(ax64, ax64, nil, 1, 1); return err },
		"Interpolate64S/x": func() error { _, _, err := Interpolate64S(nil, ax64, 1, 1); return err },
		"Interpolate64S/y": func() error { _, _, err := Interpolate64S(ax64, nil, 1, 1); return err },
		"Interpolate/x":    func() error { _, _, _, err := Interpolate(nil, ax, d, 1, 1); return err },
		"Interpolate/d":    func() error { _, _, _, err := Interpolate(ax, ax, nil, 1, 1); return err },
		"SetInterpolated":  func() error { return SetInterpolated(ax, nil, d, 1, 1, 1) },
		"SetInterpolated2": func() error { return SetInterpolated2(ax, ax, nil, 1, 1, 1) },
		"U16_u16_int2":     func() error { _, _, _, err := U16_u16_int2(ax16, nil, d, 1, 1); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: want error for empty input", name)
		}
	}
}

// bilinear is reproduced exactly by bilinear interpolation inside every
// cell, so it gives known answers for any query point.
func bilinear(x, y float64) float64 { return 3 + 2*x - y + 0.5*x*y }

// grid64 builds a row-major table (one row per y value) of bilinear over
// deliberately non-uniform axes.
func grid64() (xs, ys, data []float64) {
	xs = []float64{0, 1, 4, 10}
	ys = []float64{-5, 0, 2}
	for _, y := range ys {
		for _, x := range xs {
			data = append(data, bilinear(x, y))
		}
	}
	return xs, ys, data
}

// Interpolate64 returns the bilinear value between the four surrounding
// cells together with the fractional axis positions of the query.
func TestInterpolate64(t *testing.T) {
	xs, ys, data := grid64()
	cases := []struct {
		x, y       float64
		xPos, yPos float64
		want       float64
	}{
		{0.5, -2.5, 0.5, 0.5, bilinear(0.5, -2.5)},
		{2.5, 1, 1.5, 1.5, bilinear(2.5, 1)},
		{7, -1, 2.5, 0.8, bilinear(7, -1)},
		{4, 1, 2, 1.5, bilinear(4, 1)},   // on an interior x breakpoint
		{1, -5, 1, 0, bilinear(1, -5)},   // on a grid node in the first row
		{0, -5, 0, 0, bilinear(0, -5)},   // first corner
		{10, 2, 3, 2, bilinear(10, 2)},   // last corner
		{-3, 1, 0, 1.5, bilinear(0, 1)},  // x below the axis clamps to the first column
		{50, 1, 3, 1.5, bilinear(10, 1)}, // x above the axis clamps to the last column
		{2.5, -9, 1.5, 0, bilinear(2.5, -5)},
		{2.5, 9, 1.5, 2, bilinear(2.5, 2)},
		{-1, 99, 0, 2, bilinear(0, 2)},
	}
	for _, c := range cases {
		xPos, yPos, v, err := Interpolate64(xs, ys, data, c.x, c.y)
		if err != nil {
			t.Fatalf("Interpolate64(%v, %v): %v", c.x, c.y, err)
		}
		if !near(xPos, c.xPos) || !near(yPos, c.yPos) || !near(v, c.want) {
			t.Errorf("Interpolate64(%v, %v) = (%v, %v, %v), want (%v, %v, %v)", c.x, c.y, xPos, yPos, v, c.xPos, c.yPos, c.want)
		}
	}
}

// Interpolate64 handles one-element axes (a single column or row) by using
// that column/row only, without reading neighbouring rows.
func TestInterpolate64SingleElementAxis(t *testing.T) {
	// One column, two rows: 1 at y=0, 3 at y=10.
	for _, x := range []float64{-1, 5, 99} {
		xPos, yPos, v, err := Interpolate64([]float64{5}, []float64{0, 10}, []float64{1, 3}, x, 5)
		if err != nil {
			t.Fatal(err)
		}
		if xPos != 0 || !near(yPos, 0.5) || !near(v, 2) {
			t.Errorf("column x=%v: got (%v, %v, %v), want (0, 0.5, 2)", x, xPos, yPos, v)
		}
	}
	// One row, three columns.
	for _, y := range []float64{-1, 7, 99} {
		xPos, yPos, v, err := Interpolate64([]float64{0, 10, 20}, []float64{7}, []float64{1, 3, 7}, 15, y)
		if err != nil {
			t.Fatal(err)
		}
		if !near(xPos, 1.5) || yPos != 0 || !near(v, 5) {
			t.Errorf("row y=%v: got (%v, %v, %v), want (1.5, 0, 5)", y, xPos, yPos, v)
		}
	}
}

// When data is shorter than the axes imply, Interpolate64 clamps cell
// offsets to the last element instead of reading past the slice.
func TestInterpolate64ShortData(t *testing.T) {
	_, _, v, err := Interpolate64([]float64{0, 10}, []float64{0, 10}, []float64{1, 2, 3}, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if v != 3 {
		t.Errorf("got %v, want the last data element 3", v)
	}
}

// Interpolate64S reports the same fractional axis positions as Interpolate64
// without needing the data table.
func TestInterpolate64S(t *testing.T) {
	xs, ys, data := grid64()
	for _, x := range []float64{-3, 0, 0.25, 1, 3, 4, 9.5, 10, 12} {
		for _, y := range []float64{-7, -5, -1, 0, 1.5, 2, 3} {
			wx, wy, _, _ := Interpolate64(xs, ys, data, x, y)
			gx, gy, err := Interpolate64S(xs, ys, x, y)
			if err != nil {
				t.Fatal(err)
			}
			if gx != wx || gy != wy {
				t.Errorf("Interpolate64S(%v, %v) = (%v, %v), want (%v, %v)", x, y, gx, gy, wx, wy)
			}
		}
	}
}

// findIndexAndFrac64 returns the upper bracketing index and the fraction of
// the way from the lower breakpoint, clamped to (1, 0) below the axis and
// (n-1, 1) above it.
func TestFindIndexAndFrac64(t *testing.T) {
	axis := []float64{10, 20, 40, 80}
	cases := []struct {
		v    float64
		idx  int
		frac float64
	}{
		{0, 1, 0},
		{10, 1, 0},
		{15, 1, 0.5},
		{20, 2, 0},
		{30, 2, 0.5},
		{70, 3, 0.75},
		{80, 3, 1},
		{1000, 3, 1},
	}
	for _, c := range cases {
		idx, frac := findIndexAndFrac64(axis, c.v)
		if idx != c.idx || !near(frac, c.frac) {
			t.Errorf("findIndexAndFrac64(%v) = (%d, %v), want (%d, %v)", c.v, idx, frac, c.idx, c.frac)
		}
	}
}

// gridInt builds a 3x3 integer table of x + 10*y over x, y = 0, 10, 20.
func gridInt() (xs, ys, data []int) {
	xs = []int{0, 10, 20}
	ys = []int{0, 10, 20}
	for _, y := range ys {
		for _, x := range xs {
			data = append(data, x+10*y)
		}
	}
	return xs, ys, data
}

// Interpolate (int axes) returns the bilinear value and fractional axis
// positions for points inside the table and on its upper breakpoints.
func TestInterpolateInt(t *testing.T) {
	xs, ys, data := gridInt()
	cases := []struct {
		x, y       int
		xPos, yPos float64
		want       float64
	}{
		{5, 15, 0.5, 1.5, 155},
		{15, 5, 1.5, 0.5, 65},
		{10, 10, 1, 1, 110},
		{20, 20, 2, 2, 220},
		{12, 18, 1.2, 1.8, 192},
	}
	for _, c := range cases {
		xPos, yPos, v, err := Interpolate(xs, ys, data, c.x, c.y)
		if err != nil {
			t.Fatal(err)
		}
		if !near(xPos, c.xPos) || !near(yPos, c.yPos) || !near(v, c.want) {
			t.Errorf("Interpolate(%d, %d) = (%v, %v, %v), want (%v, %v, %v)", c.x, c.y, xPos, yPos, v, c.xPos, c.yPos, c.want)
		}
	}
}

// A value exactly on (or below) the first axis breakpoint should interpolate
// along that first row/column and report position 0.
func TestInterpolateIntFirstBreakpoint(t *testing.T) {
	xs, ys, data := gridInt()
	cases := []struct {
		x, y       int
		xPos, yPos float64
		want       float64
	}{
		{0, 15, 0, 1.5, 150},
		{5, 0, 0.5, 0, 5},
		{0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		xPos, yPos, v, _ := Interpolate(xs, ys, data, c.x, c.y)
		if !near(xPos, c.xPos) || !near(yPos, c.yPos) || !near(v, c.want) {
			t.Errorf("Interpolate(%d, %d) = (%v, %v, %v), want (%v, %v, %v)", c.x, c.y, xPos, yPos, v, c.xPos, c.yPos, c.want)
		}
		xPos, yPos, v, _ = U16_u16_int2([]uint16{0, 10, 20}, []uint16{0, 10, 20}, data, uint16(c.x), uint16(c.y))
		if !near(xPos, c.xPos) || !near(yPos, c.yPos) || !near(v, c.want) {
			t.Errorf("U16_u16_int2(%d, %d) = (%v, %v, %v), want (%v, %v, %v)", c.x, c.y, xPos, yPos, v, c.xPos, c.yPos, c.want)
		}
	}
}

// U16_u16_int2 interpolates uint16 axes over int data the same way as
// Interpolate for points inside the table.
func TestU16Interpolate(t *testing.T) {
	_, _, data := gridInt()
	xs := []uint16{0, 10, 20}
	ys := []uint16{0, 10, 20}
	cases := []struct {
		x, y       uint16
		xPos, yPos float64
		want       float64
	}{
		{5, 15, 0.5, 1.5, 155},
		{15, 5, 1.5, 0.5, 65},
		{10, 10, 1, 1, 110},
		{20, 20, 2, 2, 220},
	}
	for _, c := range cases {
		xPos, yPos, v, err := U16_u16_int2(xs, ys, data, c.x, c.y)
		if err != nil {
			t.Fatal(err)
		}
		if !near(xPos, c.xPos) || !near(yPos, c.yPos) || !near(v, c.want) {
			t.Errorf("U16_u16_int2(%d, %d) = (%v, %v, %v), want (%v, %v, %v)", c.x, c.y, xPos, yPos, v, c.xPos, c.yPos, c.want)
		}
	}
}

// SetInterpolated adds zValue to the four cells around the point, split by
// their bilinear weights and truncated to int; on a grid node the whole
// amount lands on that node.
func TestSetInterpolated(t *testing.T) {
	xs, ys, _ := gridInt()
	cases := []struct {
		name string
		x, y int
		z    int
		want []int
	}{
		{"cell centre", 15, 15, 100, []int{0, 0, 0, 0, 25, 25, 0, 25, 25}},
		{"grid node", 10, 10, 100, []int{0, 0, 0, 0, 100, 0, 0, 0, 0}},
		{"truncates", 15, 15, 10, []int{0, 0, 0, 0, 2, 2, 0, 2, 2}},
		{"x quarter", 12, 10, 100, []int{0, 0, 0, 0, 80, 20, 0, 0, 0}},
	}
	for _, c := range cases {
		data := make([]int, 9)
		if err := SetInterpolated(xs, ys, data, c.x, c.y, c.z); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(data, c.want) {
			t.Errorf("%s: data = %v, want %v", c.name, data, c.want)
		}
	}
	// Repeated calls accumulate.
	data := make([]int, 9)
	_ = SetInterpolated(xs, ys, data, 10, 10, 7)
	_ = SetInterpolated(xs, ys, data, 10, 10, 5)
	if data[4] != 12 {
		t.Errorf("accumulated node = %d, want 12", data[4])
	}
}

// SetInterpolated2 on a grid node sets that node to zValue and leaves every
// other cell alone.
func TestSetInterpolated2GridNode(t *testing.T) {
	xs, ys, data := gridInt()
	orig := append([]int(nil), data...)
	if err := SetInterpolated2(xs, ys, data, 10, 10, 500); err != nil {
		t.Fatal(err)
	}
	for i := range data {
		want := orig[i]
		if i == 4 {
			want = 500
		}
		if data[i] != want {
			t.Errorf("data[%d] = %d, want %d", i, data[i], want)
		}
	}
	if _, _, v, _ := Interpolate(xs, ys, data, 10, 10); v != 500 {
		t.Errorf("interpolated value after set = %v, want 500", v)
	}
}

// SetInterpolated2 adjusts the surrounding cells so the interpolated value
// at the point becomes zValue, also between grid nodes.
func TestSetInterpolated2BetweenNodes(t *testing.T) {
	xs, ys, _ := gridInt()
	data := make([]int, 9)
	if err := SetInterpolated2(xs, ys, data, 15, 15, 100); err != nil {
		t.Fatal(err)
	}
	if _, _, v, _ := Interpolate(xs, ys, data, 15, 15); math.Abs(v-100) > 1 {
		t.Errorf("interpolated value after SetInterpolated2(15, 15, 100) = %v, want 100", v)
	}
}

// clamp keeps an offset within [0, max-1].
func TestClamp(t *testing.T) {
	for _, c := range []struct{ in, max, want int }{{-5, 4, 0}, {0, 4, 0}, {3, 4, 3}, {4, 4, 3}, {99, 4, 3}} {
		if got := clamp(c.in, c.max); got != c.want {
			t.Errorf("clamp(%d, %d) = %d, want %d", c.in, c.max, got, c.want)
		}
	}
}
