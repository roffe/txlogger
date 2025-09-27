package widgets

import "image/color"

type ColorBlindMode int

const (
	ModeNormal     ColorBlindMode = iota // Green → Yellow → Red
	ModeUniversal                        // Blue → Gray → Orange
	ModeProtanopia                       // Blue → White → Brown
	ModeTritanopia                       // Red → Gray → Teal
)

func (m ColorBlindMode) String() string {
	switch m {
	case ModeNormal:
		return "Normal"
	case ModeUniversal:
		return "Universal"
	case ModeProtanopia:
		return "Protanopia"
	case ModeTritanopia:
		return "Tritanopia"
	default:
		return "Unknown"
	}
}

// getColorInterpolation returns a color interpolated on the color spectrum green to yellow to red.
// value should be between min and max.

// GetColorInterpolation returns interpolated color for a given value
func GetColorInterpolation(min, max, value float64, mode ColorBlindMode) color.RGBA {
	// Normalize to [0,1]
	t := (value - min) / (max - min)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	// pick color stops depending on mode
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
		high = color.RGBA{255, 165, 00, 255} // #FFA500

	case ModeProtanopia:
		// Blue → White → Brown
		low = color.RGBA{5, 113, 176, 255}       // #0571B0
		mid = color.RGBA{247, 247, 247, 255}     // #F7F7F7
		high = color.RGBA{0x96, 0x4B, 0x00, 255} // #964B00

	case ModeTritanopia:
		// Teal → Gray → Red
		low = color.RGBA{0x00, 0x80, 0x80, 255} // #008080
		mid = color.RGBA{247, 247, 247, 255}    // #F7F7F7
		high = color.RGBA{215, 48, 39, 255}     // #D73027

	default:
		// fallback: normal
		low = color.RGBA{0, 255, 0, 255}
		mid = color.RGBA{255, 255, 0, 255}
		high = color.RGBA{255, 0, 0, 255}
	}
	divider := .5

	// split interpolation at midpoint
	if t < divider {
		return lerpColor(low, mid, t/divider)
	}
	return lerpColor(mid, high, (t-divider)/(1-divider))
}

func GetColorInterpolationOld(min, max, value float64) color.RGBA {
	//log.Println("getColorInterpolation", min, max, value)
	// Normalize the value to a 0-1 range
	t := (value - min) / (max - min)
	divider := .6
	var r, g, b float64
	if t < divider { // Green to Yellow interpolation
		r = lerp(0, 1, t/divider)
		g = 1
	} else { // Yellow to Red interpolation
		r = 1
		g = lerp(1, 0, (t-divider)/(1-divider))
	}
	b = 0
	// Convert from 0-1 range to 0-255 for color.RGBA
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
