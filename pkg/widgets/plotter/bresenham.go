package plotter

import (
	"image"
	"image/color"
	"math"
)

// BresenhamThick draws a line of given thickness directly into img.Pix,
// bypassing image.RGBA.SetRGBA and its interface dispatch.
func BresenhamThick(img *image.RGBA, x1, y1, x2, y2 int, thickness int, col color.RGBA) {
	pix := img.Pix
	stride := img.Stride
	w := img.Rect.Dx()
	h := img.Rect.Dy()

	if thickness <= 1 {
		bresenhamCore(pix, stride, w, h, x1, y1, x2, y2, col)
		return
	}

	halfThick := thickness / 2

	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	length := math.Sqrt(dx*dx + dy*dy)

	if length == 0 {
		fillCircle(pix, stride, w, h, x1, y1, halfThick, col)
		return
	}

	perpX := -dy / length
	perpY := dx / length

	for i := -halfThick; i <= halfThick; i++ {
		offsetX := int(float64(i) * perpX)
		offsetY := int(float64(i) * perpY)
		bresenhamCore(pix, stride, w, h,
			x1+offsetX, y1+offsetY,
			x2+offsetX, y2+offsetY,
			col)
	}
}

func bresenhamCore(pix []uint8, stride, w, h, x1, y1, x2, y2 int, col color.RGBA) {
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	steep := dy > dx

	if steep {
		x1, y1 = y1, x1
		x2, y2 = y2, x2
	}
	if x1 > x2 {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}

	dx = abs(x2 - x1)
	dy = abs(y2 - y1)
	err := dx / 2
	y := y1
	ystep := 1
	if y1 >= y2 {
		ystep = -1
	}

	for x := x1; x <= x2; x++ {
		var px, py int
		if steep {
			px, py = y, x
		} else {
			px, py = x, y
		}
		// Single combined bounds check (negative values wrap to large uints).
		if uint(px) < uint(w) && uint(py) < uint(h) {
			i := py*stride + px*4
			// Max-blend so overlapping lines render order-independently —
			// avoids per-frame z-flicker when two series share pixels.
			pix[i+0] = max(pix[i+0], col.R)
			pix[i+1] = max(pix[i+1], col.G)
			pix[i+2] = max(pix[i+2], col.B)
			pix[i+3] = max(pix[i+3], col.A)
		}
		err -= dy
		if err < 0 {
			y += ystep
			err += dx
		}
	}
}

func fillCircle(pix []uint8, stride, w, h, centerX, centerY, radius int, col color.RGBA) {
	rr := radius * radius
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y > rr {
				continue
			}
			px := centerX + x
			py := centerY + y
			if uint(px) < uint(w) && uint(py) < uint(h) {
				i := py*stride + px*4
				pix[i+0] = max(pix[i+0], col.R)
				pix[i+1] = max(pix[i+1], col.G)
				pix[i+2] = max(pix[i+2], col.B)
				pix[i+3] = max(pix[i+3], col.A)
			}
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
