package colors

import (
	"image/color"
	"math"
	"testing"
)

// Every mode has a display name, and SupportedColorBlindModes lists them in
// mode order so a settings index and a ColorBlindMode agree.
func TestColorBlindModeString(t *testing.T) {
	tests := []struct {
		mode ColorBlindMode
		want string
	}{
		{ModeNormal, Normal},
		{ModeUniversal, Universal},
		{ModeProtanopia, Protanopia},
		{ModeTritanopia, Tritanopia},
		{ModeDeuteranomaly, Deuteranomaly},
		{ColorBlindMode(-1), Unknown},
		{ColorBlindMode(99), Unknown},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("ColorBlindMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
	for i, name := range SupportedColorBlindModes {
		if got := ColorBlindMode(i).String(); got != name {
			t.Errorf("SupportedColorBlindModes[%d] = %q but ColorBlindMode(%d) is %q", i, name, i, got)
		}
	}
}

// StringToColorBlindMode maps the stored preference name back to a mode,
// accepts all-lowercase names, and falls back to Normal for anything unknown.
func TestStringToColorBlindMode(t *testing.T) {
	tests := []struct {
		in   string
		want ColorBlindMode
	}{
		{"Normal", ModeNormal},
		{"Universal", ModeUniversal},
		{"Protanopia", ModeProtanopia},
		{"Tritanopia", ModeTritanopia},
		{"Deuteranomaly", ModeDeuteranomaly},
		{"universal", ModeUniversal},
		{"tritanopia", ModeTritanopia},
		{"", ModeNormal},
		{"Unknown", ModeNormal},
		{"monochrome", ModeNormal},
	}
	for _, tt := range tests {
		if got := StringToColorBlindMode(tt.in); got != tt.want {
			t.Errorf("StringToColorBlindMode(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	// Every supported name round-trips.
	for _, name := range SupportedColorBlindModes {
		if got := StringToColorBlindMode(name).String(); got != name {
			t.Errorf("round trip of %q gave %q", name, got)
		}
	}
}

// GetColorInterpolation maps min, the midpoint and max to the low, mid and
// high stops of each mode's three-color scale.
func TestGetColorInterpolationStops(t *testing.T) {
	rgb := func(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }
	tests := []struct {
		mode           ColorBlindMode
		low, mid, high color.RGBA
	}{
		{ModeNormal, rgb(0, 255, 0), rgb(255, 255, 0), rgb(255, 0, 0)},
		{ModeUniversal, rgb(0x21, 0x66, 0xAC), rgb(0xF7, 0xF7, 0xF7), rgb(0xFF, 0xA5, 0x00)},
		{ModeProtanopia, rgb(0x05, 0x71, 0xB0), rgb(0xF7, 0xF7, 0xF7), rgb(0x96, 0x4B, 0x00)},
		{ModeTritanopia, rgb(0x00, 0x80, 0x80), rgb(0xF7, 0xF7, 0xF7), rgb(0xD7, 0x30, 0x27)},
		{ModeDeuteranomaly, rgb(0x4A, 0x90, 0xE2), rgb(0xF5, 0xE6, 0xB3), rgb(0x8B, 0x45, 0x13)},
		// Unknown modes use the Normal scale.
		{ColorBlindMode(42), rgb(0, 255, 0), rgb(255, 255, 0), rgb(255, 0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			if got := GetColorInterpolation(-20, 80, -20, tt.mode); got != tt.low {
				t.Errorf("at min: got %v, want %v", got, tt.low)
			}
			if got := GetColorInterpolation(-20, 80, 30, tt.mode); got != tt.mid {
				t.Errorf("at midpoint: got %v, want %v", got, tt.mid)
			}
			if got := GetColorInterpolation(-20, 80, 80, tt.mode); got != tt.high {
				t.Errorf("at max: got %v, want %v", got, tt.high)
			}
		})
	}
}

// Between the stops the color is a per-channel linear blend, truncated to
// uint8, with alpha always opaque.
func TestGetColorInterpolationBlends(t *testing.T) {
	tests := []struct {
		value float64
		want  color.RGBA
	}{
		{25, color.RGBA{127, 255, 0, 255}}, // halfway green -> yellow
		{75, color.RGBA{255, 127, 0, 255}}, // halfway yellow -> red
		{10, color.RGBA{51, 255, 0, 255}},  // 255 * 0.2
		{90, color.RGBA{255, 51, 0, 255}},  // 255 * (1 - 0.8)
	}
	for _, tt := range tests {
		if got := GetColorInterpolation(0, 100, tt.value, ModeNormal); got != tt.want {
			t.Errorf("value %v: got %v, want %v", tt.value, got, tt.want)
		}
	}
}

// Values outside [min, max] clamp to the end stops, and an undefined position
// (NaN input, or a zero-width range hit exactly) renders neutral gray.
func TestGetColorInterpolationClampAndNaN(t *testing.T) {
	green := color.RGBA{0, 255, 0, 255}
	red := color.RGBA{255, 0, 0, 255}
	gray := color.RGBA{128, 128, 128, 255}
	tests := []struct {
		name            string
		min, max, value float64
		want            color.RGBA
	}{
		{"below min", 0, 100, -500, green},
		{"above max", 0, 100, 1e9, red},
		{"-Inf value", 0, 100, math.Inf(-1), green},
		{"+Inf value", 0, 100, math.Inf(1), red},
		{"NaN value", 0, 100, math.NaN(), gray},
		{"zero range at value", 5, 5, 5, gray},
		{"zero range above", 5, 5, 6, red},
		{"zero range below", 5, 5, 4, green},
	}
	for _, tt := range tests {
		if got := GetColorInterpolation(tt.min, tt.max, tt.value, ModeNormal); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
