package plotter

import "image/color"

type IPlotter interface {
	SetRGBA(x int, y int, c color.RGBA)
}

const (
	Down = -1
	Zero = 0
	Up   = 1
	Two  = 2
)

func Bresenham(p IPlotter, x1, y1, x2, y2 int, col color.RGBA) {
	dx, dy := x2-x1, y2-y1
	absDx, absDy := abs(dx), abs(dy)

	// Is line a point?
	if absDx == Zero && absDy == Zero {
		p.SetRGBA(x1, y1, col)
		return
	}

	// Determine the direction of increment along x and y
	xInc, yInc := sign(dx), sign(dy)

	// Initialize decision variables
	isXDominant := absDx > absDy

	doubleAbsDy := Two * absDy
	doubleAbsDx := Two * absDx

	var direction, dInc1, dInc2 int
	if isXDominant {
		direction, dInc1, dInc2 = doubleAbsDy-absDx, doubleAbsDy, Two*(absDy-absDx)
	} else {
		direction, dInc1, dInc2 = doubleAbsDx-absDy, doubleAbsDx, Two*(absDx-absDy)
	}

	// Draw the line
	for {
		p.SetRGBA(x1, y1, col)
		if x1 == x2 && y1 == y2 {
			break
		}
		if isXDominant {
			if direction < Zero {
				direction += dInc1
			} else {
				y1 += yInc
				direction += dInc2
			}
			x1 += xInc
			continue
		}
		if direction < Zero {
			direction += dInc1
			y1 += yInc
			continue
		}
		x1 += xInc
		direction += dInc2
		y1 += yInc
	}
}

func abs(n int) int {
	if n < Zero {
		return -n
	}
	return n
}

func sign(n int) int {
	if n < Zero {
		return Down
	} else if n > Zero {
		return Up
	}
	return Zero
}
