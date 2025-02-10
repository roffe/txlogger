package multiwindow

import (
	"math"
	"sort"

	"fyne.io/fyne/v2"
)

// Common constants
const (
	padding    float32 = 4  // Standard padding between windows
	initOffset float32 = 15 // Initial offset for floating windows
)

// Arranger defines the interface for different window arrangement strategies
type Arranger interface {
	Layout(size fyne.Size, confined bool, windows []*InnerWindow)
}

// baseArrangement contains common window arrangement utilities
type baseArrangement struct{}

func (b *baseArrangement) setWindowState(w *InnerWindow, pos fyne.Position, size fyne.Size, maximized bool) {
	w.preMaximizedPos = pos
	w.preMaximizedSize = w.MinSize()
	w.Move(pos)
	w.Resize(size)
	w.maximized = maximized
}

// GridArranger arranges windows in a grid pattern
type GridArranger struct{ baseArrangement }

func (g *GridArranger) Layout(maxSize fyne.Size, confined bool, windows []*InnerWindow) {
	numWindows := len(windows)
	if numWindows == 0 {
		return
	}

	cols := int(math.Ceil(math.Sqrt(float64(numWindows))))
	rows := (len(windows) + cols - 1) / cols
	lastRowWindows := len(windows) - (rows-1)*cols

	// Calculate cell size
	cellWidth := (maxSize.Width - padding*float32(cols+1)) / float32(cols)
	cellHeight := (maxSize.Height - padding*float32(rows+1)) / float32(rows)

	for i, window := range windows {
		row, col := i/cols, i%cols
		width := cellWidth
		if row == rows-1 && lastRowWindows < cols {
			width = (maxSize.Width - padding*float32(lastRowWindows+1)) / float32(lastRowWindows)
		}

		pos := fyne.NewPos(
			padding+float32(col)*(width+padding),
			padding+float32(row)*(cellHeight+padding),
		)
		size := fyne.NewSize(width, cellHeight).Max(window.MinSize())
		g.setWindowState(window, pos, size, true)
	}
}

// FloatingArranger arranges windows in a cascading pattern
type FloatingArranger struct{ baseArrangement }

func (f *FloatingArranger) Layout(maxSize fyne.Size, confined bool, windows []*InnerWindow) {
	numWindows := len(windows)
	if numWindows == 0 {
		return
	}

	maxSteps := int(fyne.Min(
		(maxSize.Width-initOffset)/20,
		(maxSize.Height-initOffset)/20,
	))

	for i, window := range windows {
		step := i % maxSteps
		wrap := i / maxSteps

		posX := initOffset + float32(step)*20 + float32(wrap)*40
		posY := initOffset + float32(step)*20

		if confined {
			// Clamp positions to keep windows within bounds
			maxX := maxSize.Width - window.MinSize().Width
			maxY := maxSize.Height - window.MinSize().Height
			posX = fyne.Min(posX, maxX)
			posY = fyne.Min(posY, maxY)
		}

		pos := fyne.NewPos(posX, posY)
		size := fyne.NewSize(
			fyne.Max(maxSize.Width/2, window.MinSize().Width),
			fyne.Max(maxSize.Height/2, window.MinSize().Height),
		)

		f.setWindowState(window, pos, size, false)
	}
}

// PackArranger implements an efficient packing algorithm with space-filling
type PackArranger struct{ baseArrangement }

type packNode struct {
	pos    fyne.Position
	size   fyne.Size
	used   bool
	right  *packNode
	bottom *packNode
}

type windowSpace struct {
	window *InnerWindow
	node   *packNode
}

func (p *PackArranger) Layout(maxSize fyne.Size, confined bool, windows []*InnerWindow) {
	if len(windows) == 0 {
		return
	}

	// Sort windows by area for better packing
	sorted := make([]*InnerWindow, len(windows))
	copy(sorted, windows)
	sort.Slice(sorted, func(i, j int) bool {
		areaI := sorted[i].MinSize().Width * sorted[i].MinSize().Height
		areaJ := sorted[j].MinSize().Width * sorted[j].MinSize().Height
		return areaI > areaJ
	})

	root := &packNode{
		pos:  fyne.NewPos(padding, padding),
		size: maxSize.Subtract(fyne.NewSize(padding*2, padding*2)),
	}

	// First pass: Find spaces for all windows
	spaces := make([]windowSpace, 0, len(sorted))
	for _, window := range sorted {
		size := window.MinSize().Add(fyne.NewSize(padding, padding))
		if node := p.findSpace(root, size); node != nil {
			spaces = append(spaces, windowSpace{window: window, node: node})
			node.used = true
		} else {
			// Fallback to top-left corner if no space found
			p.setWindowState(window, fyne.NewPos(padding, padding), window.MinSize(), false)
		}
	}

	// Second pass: Try to expand windows to fill gaps
	p.expandWindows(spaces, maxSize)
}

func (p *PackArranger) expandWindows(spaces []windowSpace, maxSize fyne.Size) {
	for i, space := range spaces {
		window := space.window
		node := space.node

		// Calculate maximum possible expansion
		maxWidth := node.size.Width
		maxHeight := node.size.Height

		// Check if expanding would overlap with other windows
		for j, otherSpace := range spaces {
			if i == j {
				continue
			}

			otherNode := otherSpace.node

			// Check horizontal overlap
			if node.pos.Y < otherNode.pos.Y+otherNode.size.Height &&
				node.pos.Y+node.size.Height > otherNode.pos.Y {
				if otherNode.pos.X > node.pos.X {
					maxWidth = fyne.Min(maxWidth, otherNode.pos.X-node.pos.X-padding)
				}
			}

			// Check vertical overlap
			if node.pos.X < otherNode.pos.X+otherNode.size.Width &&
				node.pos.X+node.size.Width > otherNode.pos.X {
				if otherNode.pos.Y > node.pos.Y {
					maxHeight = fyne.Min(maxHeight, otherNode.pos.Y-node.pos.Y-padding)
				}
			}
		}

		// Ensure we don't exceed the container bounds
		maxWidth = fyne.Min(maxWidth, maxSize.Width-node.pos.X-padding)
		maxHeight = fyne.Min(maxHeight, maxSize.Height-node.pos.Y-padding)

		// Calculate expanded size while maintaining aspect ratio
		minSize := window.MinSize()
		aspectRatio := minSize.Width / minSize.Height

		var newWidth, newHeight float32
		if maxWidth/aspectRatio <= maxHeight {
			newWidth = maxWidth
			newHeight = maxWidth / aspectRatio
		} else {
			newHeight = maxHeight
			newWidth = maxHeight * aspectRatio
		}

		// Apply the new size, ensuring it's not smaller than minimum size
		newSize := fyne.NewSize(newWidth, newHeight).Max(minSize)
		p.setWindowState(window, node.pos, newSize, false)
	}
}

func (p *PackArranger) findSpace(node *packNode, size fyne.Size) *packNode {
	if node.used {
		if right := p.findSpace(node.right, size); right != nil {
			return right
		}
		return p.findSpace(node.bottom, size)
	}

	if size.Width > node.size.Width || size.Height > node.size.Height {
		return nil
	}

	if size.Width == node.size.Width && size.Height == node.size.Height {
		return node
	}

	// Split node
	remainingHoriz := node.size.Width - size.Width
	remainingVert := node.size.Height - size.Height

	if remainingHoriz > remainingVert {
		node.right = &packNode{
			pos:  fyne.NewPos(node.pos.X+size.Width+padding, node.pos.Y),
			size: fyne.NewSize(remainingHoriz-padding, node.size.Height),
		}
		node.bottom = &packNode{
			pos:  fyne.NewPos(node.pos.X, node.pos.Y+size.Height+padding),
			size: fyne.NewSize(size.Width, remainingVert-padding),
		}
	} else {
		node.right = &packNode{
			pos:  fyne.NewPos(node.pos.X+size.Width+padding, node.pos.Y),
			size: fyne.NewSize(remainingHoriz-padding, size.Height),
		}
		node.bottom = &packNode{
			pos:  fyne.NewPos(node.pos.X, node.pos.Y+size.Height+padding),
			size: fyne.NewSize(node.size.Width, remainingVert-padding),
		}
	}

	return node
}

// PreservingArranger maintains window positions while adjusting sizes to fill space
type PreservingArranger struct{ baseArrangement }

type region struct {
	pos    fyne.Position
	size   fyne.Size
	window *InnerWindow
}

func (p *PreservingArranger) Layout(maxSize fyne.Size, confined bool, windows []*InnerWindow) {
	if len(windows) == 0 {
		return
	}

	// Convert windows to regions and sort by position (top-left to bottom-right)
	regions := make([]region, len(windows))
	for i, w := range windows {
		regions[i] = region{
			pos:    w.Position(),
			size:   w.Size(),
			window: w,
		}
	}
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].pos.Y == regions[j].pos.Y {
			return regions[i].pos.X < regions[j].pos.X
		}
		return regions[i].pos.Y < regions[j].pos.Y
	})

	// Find gaps between windows
	gaps := p.findGaps(regions, maxSize)

	// Distribute gaps to adjacent windows
	p.distributeGaps(regions, gaps)

	// Apply new sizes while preserving positions
	for _, r := range regions {
		newSize := r.size.Max(r.window.MinSize())
		if confined {
			// Ensure window stays within bounds
			maxWidth := maxSize.Width - r.pos.X
			maxHeight := maxSize.Height - r.pos.Y
			newSize.Width = fyne.Min(newSize.Width, maxWidth)
			newSize.Height = fyne.Min(newSize.Height, maxHeight)
		}
		p.setWindowState(r.window, r.pos, newSize, false)
	}
}

func (p *PreservingArranger) findGaps(regions []region, maxSize fyne.Size) []region {
	gaps := make([]region, 0)

	// Helper to check if a point is covered by any region
	isCovered := func(x, y float32) bool {
		for _, r := range regions {
			if x >= r.pos.X && x < r.pos.X+r.size.Width &&
				y >= r.pos.Y && y < r.pos.Y+r.size.Height {
				return true
			}
		}
		return false
	}

	// Scan horizontally for gaps
	for _, r := range regions {
		// Check right gap
		if r.pos.X+r.size.Width < maxSize.Width {
			gapStart := r.pos.X + r.size.Width + padding
			hasGap := false
			var gapEnd float32

			// Find the next window in this row
			for _, other := range regions {
				if other.pos.Y <= r.pos.Y+r.size.Height &&
					other.pos.Y+other.size.Height > r.pos.Y &&
					other.pos.X > r.pos.X {
					if !hasGap || other.pos.X < gapEnd {
						hasGap = true
						gapEnd = other.pos.X - padding
					}
				}
			}

			if !hasGap {
				gapEnd = maxSize.Width - padding
			}

			if gapEnd > gapStart && !isCovered(gapStart+(gapEnd-gapStart)/2, r.pos.Y+r.size.Height/2) {
				gaps = append(gaps, region{
					pos:  fyne.NewPos(gapStart, r.pos.Y),
					size: fyne.NewSize(gapEnd-gapStart, r.size.Height),
				})
			}
		}

		// Check bottom gap
		if r.pos.Y+r.size.Height < maxSize.Height {
			gapStart := r.pos.Y + r.size.Height + padding
			hasGap := false
			var gapEnd float32

			// Find the next window in this column
			for _, other := range regions {
				if other.pos.X <= r.pos.X+r.size.Width &&
					other.pos.X+other.size.Width > r.pos.X &&
					other.pos.Y > r.pos.Y {
					if !hasGap || other.pos.Y < gapEnd {
						hasGap = true
						gapEnd = other.pos.Y - padding
					}
				}
			}

			if !hasGap {
				gapEnd = maxSize.Height - padding
			}

			if gapEnd > gapStart && !isCovered(r.pos.X+r.size.Width/2, gapStart+(gapEnd-gapStart)/2) {
				gaps = append(gaps, region{
					pos:  fyne.NewPos(r.pos.X, gapStart),
					size: fyne.NewSize(r.size.Width, gapEnd-gapStart),
				})
			}
		}
	}

	return gaps
}

func (p *PreservingArranger) distributeGaps(regions []region, gaps []region) {
	for _, gap := range gaps {
		// Find adjacent windows
		adjacentWindows := make([]int, 0)

		for i, r := range regions {
			// Check if window is adjacent horizontally or vertically
			horizontallyAdjacent := (r.pos.Y <= gap.pos.Y+gap.size.Height &&
				r.pos.Y+r.size.Height > gap.pos.Y) &&
				(r.pos.X+r.size.Width == gap.pos.X-padding ||
					r.pos.X == gap.pos.X+gap.size.Width+padding)

			verticallyAdjacent := (r.pos.X <= gap.pos.X+gap.size.Width &&
				r.pos.X+r.size.Width > gap.pos.X) &&
				(r.pos.Y+r.size.Height == gap.pos.Y-padding ||
					r.pos.Y == gap.pos.Y+gap.size.Height+padding)

			if horizontallyAdjacent || verticallyAdjacent {
				adjacentWindows = append(adjacentWindows, i)
			}
		}

		// Distribute gap space among adjacent windows
		if len(adjacentWindows) > 0 {
			extraSpace := gap.size.Width / float32(len(adjacentWindows))
			extraHeight := gap.size.Height / float32(len(adjacentWindows))

			for _, idx := range adjacentWindows {
				r := &regions[idx]
				// Grow window based on its position relative to the gap
				if r.pos.X+r.size.Width <= gap.pos.X+padding {
					// Window is to the left of gap
					r.size.Width += extraSpace
				} else if r.pos.X >= gap.pos.X+gap.size.Width-padding {
					// Window is to the right of gap
					r.pos.X -= extraSpace
					r.size.Width += extraSpace
				}

				if r.pos.Y+r.size.Height <= gap.pos.Y+padding {
					// Window is above gap
					r.size.Height += extraHeight
				} else if r.pos.Y >= gap.pos.Y+gap.size.Height-padding {
					// Window is below gap
					r.pos.Y -= extraHeight
					r.size.Height += extraHeight
				}
			}
		}
	}
}
