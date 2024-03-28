package plotter

import "image/color"

type IPlotter interface {
	SetRGBA(x int, y int, c color.RGBA)
}

func Bresenham(p IPlotter, x1, y1, x2, y2 int, col color.RGBA) {
	dx, dy := x2-x1, y2-y1
	absDx, absDy := abs(dx), abs(dy)

	// Determine the direction of increment along x and y
	xInc, yInc := sign(dx), sign(dy)

	// Is line a point?
	if absDx == 0 && absDy == 0 {
		p.SetRGBA(x1, y1, col)
		return
	}

	// Initialize decision variables
	var d, dInc1, dInc2 int
	isXDominant := absDx > absDy
	if isXDominant {
		d, dInc1, dInc2 = 2*absDy-absDx, 2*absDy, 2*(absDy-absDx)
	} else {
		d, dInc1, dInc2 = 2*absDx-absDy, 2*absDx, 2*(absDx-absDy)
	}

	// Draw the line
	for {
		p.SetRGBA(x1, y1, col)
		if x1 == x2 && y1 == y2 {
			break
		}
		if isXDominant {
			if d < 0 {
				d += dInc1
			} else {
				y1 += yInc
				d += dInc2
			}
			x1 += xInc
		} else {
			if d < 0 {
				d += dInc1
			} else {
				x1 += xInc
				d += dInc2
			}
			y1 += yInc
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	if n < 0 {
		return -1
	} else if n > 0 {
		return 1
	}
	return 0
}
