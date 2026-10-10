package datalogger

import (
	"math"
	"testing"
)

// NewWBLInterpolator clamps to the first/last lambda outside the support
// points and interpolates linearly between them.
func TestWBLInterpolator(t *testing.T) {
	conv := NewWBLInterpolator(WidebandConfig{
		SupportPoints: []int{0, 100, 200},
		LambdaValues:  []float64{0.5, 1.0, 2.0},
	})
	tests := []struct {
		ad   int
		want float64
	}{
		{-10, 0.5},
		{0, 0.5},
		{50, 0.75},
		{100, 1.0},
		{125, 1.25},
		{200, 2.0},
		{1023, 2.0},
	}
	for _, tt := range tests {
		if got := conv(tt.ad); math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("conv(%d) = %v, want %v", tt.ad, got, tt.want)
		}
	}
}

// A support point / lambda list length mismatch is an invalid table and
// yields -1 instead of a lambda.
func TestWBLInterpolatorMismatch(t *testing.T) {
	conv := NewWBLInterpolator(WidebandConfig{SupportPoints: []int{0, 100}, LambdaValues: []float64{1}})
	if got := conv(50); got != -1 {
		t.Fatalf("conv = %v, want -1", got)
	}
}

// An empty table is as invalid as a mismatched one and must yield -1, the
// same as NewWBLInterpolatorFixed does, rather than panic inside the logging
// loop that calls it for every AD scanner sample.
func TestWBLInterpolatorEmptyTable(t *testing.T) {
	conv := NewWBLInterpolator(WidebandConfig{})
	var got float64
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("conv panicked: %v", r)
			}
		}()
		got = conv(10)
	}()
	if got != -1 {
		t.Fatalf("conv = %v, want -1", got)
	}
}

// NewWBLInterpolatorFixed reproduces the 68k TBLS.W routine: clamp below the
// first point, return the last lambda at or above the last point, and round
// the 8-bit-fraction interpolation half away from zero.
func TestWBLInterpolatorFixed(t *testing.T) {
	up := NewWBLInterpolatorFixed(WidebandConfig2{
		SupportPoints: []int16{0, 100, 200},
		LambdaValues:  []int16{50, 100, 200},
	})
	down := NewWBLInterpolatorFixed(WidebandConfig2{
		SupportPoints: []int16{0, 100},
		LambdaValues:  []int16{100, 50},
	})
	tests := []struct {
		name string
		conv func(int16) int16
		in   int16
		want int16
	}{
		{"below first point", up, -5, 50},
		{"first point", up, 0, 50},
		{"midpoint, truncated fraction", up, 50, 75}, // fraction 128: 50*128/256 = 25
		{"non-exact fraction", up, 33, 66},           // fraction 84: (4200+128)/256 = 16
		{"second segment", up, 150, 150},             // fraction 128: (12800+128)/256 = 50
		{"last point", up, 200, 200},                 // max clamp
		{"above last point", up, 1000, 200},          // max clamp
		{"falling table midpoint", down, 50, 75},     // (-6400-128)/256 = -25
		{"falling table non-exact", down, 33, 84},    // (-4200-128)/256 = -16
		{"falling table below first", down, -1, 100}, // min clamp
		{"falling table above last", down, 500, 50},  // max clamp
		{"falling table last point", down, 100, 50},  // max clamp
		{"near the top", up, 199, 199},               // fraction 253: (100*253+128)/256 = 99
		{"just above bottom", up, 1, 50},             // fraction 2: (50*2+128)/256 = 0
	}
	for _, tt := range tests {
		if got := tt.conv(tt.in); got != tt.want {
			t.Errorf("%s: conv(%d) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
}

// Empty or mismatched fixed-point tables yield -1.
func TestWBLInterpolatorFixedInvalid(t *testing.T) {
	if got := NewWBLInterpolatorFixed(WidebandConfig2{})(10); got != -1 {
		t.Errorf("empty table = %d, want -1", got)
	}
	if got := NewWBLInterpolatorFixed(WidebandConfig2{SupportPoints: []int16{0, 1}, LambdaValues: []int16{1}})(0); got != -1 {
		t.Errorf("mismatched table = %d, want -1", got)
	}
}

// Over a realistic AD scanner table the fixed-point routine never strays more
// than one lambda hundredth from the float interpolator.
func TestWBLInterpolatorFixedTracksFloat(t *testing.T) {
	fixed := NewWBLInterpolatorFixed(WidebandConfig2{
		SupportPoints: []int16{0, 256, 512, 1023},
		LambdaValues:  []int16{50, 80, 100, 150},
	})
	float := NewWBLInterpolator(WidebandConfig{
		SupportPoints: []int{0, 256, 512, 1023},
		LambdaValues:  []float64{0.5, 0.8, 1.0, 1.5},
	})
	for ad := -20; ad <= 1100; ad++ {
		f := float(ad) * 100
		x := float64(fixed(int16(ad)))
		if math.Abs(f-x) > 1 {
			t.Fatalf("ad %d: fixed %v, float %v", ad, x, f)
		}
	}
}
