package plotter

import (
	"image/color"
	"math"
)

type IPlotter interface {
	SetRGBA(x int, y int, c color.RGBA)
}

const (
	Down = -1
	Zero = 0
	Up   = 1
	Two  = 2
)

// BresenhamThick draws a line with specified thickness
func BresenhamThick(p IPlotter, x1, y1, x2, y2 int, thickness int, col color.RGBA) {
	// For thickness of 1, use the simple version
	if thickness <= 1 {
		bresenhamCore(p, x1, y1, x2, y2, col)
		return
	}

	// Calculate the half-thickness
	halfThick := thickness / 2

	// Calculate the line vector
	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	length := math.Sqrt(dx*dx + dy*dy)

	// Avoid division by zero
	if length == 0 {
		fillCircle(p, x1, y1, halfThick, col)
		return
	}

	// Calculate the normalized perpendicular vector
	perpX := -dy / length
	perpY := dx / length

	// Draw filled circles at endpoints for rounded caps
	//fillCircle(p, x1, y1, halfThick, col)
	//fillCircle(p, x2, y2, halfThick, col)

	// Draw the main line body using parallel lines
	for i := -halfThick; i <= halfThick; i++ {
		offsetX := int(float64(i) * perpX)
		offsetY := int(float64(i) * perpY)
		bresenhamCore(p,
			x1+offsetX, y1+offsetY,
			x2+offsetX, y2+offsetY,
			col)
	}
}

// bresenhamCore implements the core Bresenham line algorithm
func bresenhamCore(p IPlotter, x1, y1, x2, y2 int, col color.RGBA) {
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
		if steep {
			p.SetRGBA(y, x, col)
		} else {
			p.SetRGBA(x, y, col)
		}
		err -= dy
		if err < 0 {
			y += ystep
			err += dx
		}
	}
}

// fillCircle fills a circle using the midpoint circle algorithm
func fillCircle(p IPlotter, centerX, centerY, radius int, col color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				p.SetRGBA(centerX+x, centerY+y, col)
			}
		}
	}
}

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
