package colors

import (
	"image/color"
	"math"
	"strings"
)

type ColorBlindMode int

var SupportedColorBlindModes = [...]string{
	Normal,
	Universal,
	Protanopia,
	Tritanopia,
	Deuteranomaly,
}

const (
	Normal        = "Normal"
	Universal     = "Universal"
	Protanopia    = "Protanopia"
	Tritanopia    = "Tritanopia"
	Deuteranomaly = "Deuteranomaly"
	Unknown       = "Unknown"
)

const (
	ModeNormal        ColorBlindMode = iota // Green → Yellow → Red
	ModeUniversal                           // Blue → Gray → Orange
	ModeProtanopia                          // Blue → White → Brown
	ModeTritanopia                          // Red → Gray → Teal
	ModeDeuteranomaly                       // Blue → Beige → Brown
)

func (m ColorBlindMode) String() string {
	switch m {
	case ModeNormal:
		return Normal
	case ModeUniversal:
		return Universal
	case ModeProtanopia:
		return Protanopia
	case ModeTritanopia:
		return Tritanopia
	case ModeDeuteranomaly:
		return Deuteranomaly
	default:
		return Unknown
	}
}

func StringToColorBlindMode(s string) ColorBlindMode {
	switch strings.Title(s) {
	case Normal:
		return ModeNormal
	case Universal:
		return ModeUniversal
	case Protanopia:
		return ModeProtanopia
	case Tritanopia:
		return ModeTritanopia
	case Deuteranomaly:
		return ModeDeuteranomaly
	default:
		return ModeNormal
	}
}

// getColorInterpolation returns a color interpolated on the color spectrum green to yellow to red.
// value should be between min and max.

// GetColorInterpolation returns interpolated color for a given value
func GetColorInterpolation(min, max, value float64, mode ColorBlindMode) color.RGBA {
	t := (value - min) / (max - min)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	if math.IsNaN(t) {
		return color.RGBA{128, 128, 128, 255}
	}

	var low, mid, high color.RGBA
	switch mode {
	case ModeNormal:
		// Green → Yellow → Red
		low = color.RGBA{0, 255, 0, 255}
		mid = color.RGBA{255, 255, 0, 255}
		high = color.RGBA{255, 0, 0, 255}

	case ModeUniversal:
		// Blue → Gray → Orange
		low = color.RGBA{33, 102, 172, 255}  // #2166AC
		mid = color.RGBA{247, 247, 247, 255} // #F7F7F7
		high = color.RGBA{255, 165, 0, 255}  // #FFA500

	case ModeProtanopia:
		// Blue → White → Brown
		low = color.RGBA{5, 113, 176, 255}   // #0571B0
		mid = color.RGBA{247, 247, 247, 255} // #F7F7F7
		high = color.RGBA{150, 75, 0, 255}   // #964B00

	case ModeTritanopia:
		// Teal → Gray → Red
		low = color.RGBA{0, 128, 128, 255}   // #008080
		mid = color.RGBA{247, 247, 247, 255} // #F7F7F7
		high = color.RGBA{215, 48, 39, 255}  // #D73027

	case ModeDeuteranomaly:
		// Blue → Beige → Brown
		low = color.RGBA{0x4A, 0x90, 0xE2, 255}  // #4A90E2 — medium blue
		mid = color.RGBA{0xF5, 0xE6, 0xB3, 255}  // #F5E6B3 — beige
		high = color.RGBA{0x8B, 0x45, 0x13, 255} // #8B4513 — dark brown

	default:
		low = color.RGBA{0, 255, 0, 255}
		mid = color.RGBA{255, 255, 0, 255}
		high = color.RGBA{255, 0, 0, 255}
	}

	const divider = 0.5
	if t < divider {
		return lerpColor(low, mid, t/divider)
	}
	return lerpColor(mid, high, (t-divider)/(1-divider))
}

func GetColorInterpolationOld(min, max, value float64) color.RGBA {
	t := (value - min) / (max - min)
	const divider = 0.6
	var r, g, b float64
	if t < divider {
		r = lerp(0, 1, t/divider)
		g = 1
	} else {
		r = 1
		g = lerp(1, 0, (t-divider)/(1-divider))
	}
	b = 0
	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}

// lerp helper
func lerp(a, b, t float64) float64 {
	return a + t*(b-a)
}

// linear interpolation between two colors
func lerpColor(c1, c2 color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(lerp(float64(c1.R), float64(c2.R), t)),
		G: uint8(lerp(float64(c1.G), float64(c2.G), t)),
		B: uint8(lerp(float64(c1.B), float64(c2.B), t)),
		A: 255,
	}
}
