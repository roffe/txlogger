package plotter

import (
	"fyne.io/fyne/v2"
)

type plotterRenderer struct {
	p    *Plotter
	size fyne.Size
}

func (r *plotterRenderer) MinSize() fyne.Size {
	return fyne.NewSize(500, 200)
}

func (r *plotterRenderer) Destroy() {}

func (r *plotterRenderer) Refresh() {}

func (r *plotterRenderer) Layout(size fyne.Size) {
	if r.size == size {
		return
	}
	//log.Println("Layout", size)
	if r.size.Width == size.Width && r.size.Height == size.Height {
		return
	}
	r.size = size

	// Resize the container first
	r.p.container.Resize(size)
	/*
		// Calculate new plot dimensions
		plotSize := r.p.canvasImage.Size()
		r.p.plotResolution = fyne.NewSize(plotSize.Width*r.p.plotResolutionFactor, plotSize.Height*r.p.plotResolutionFactor)

		// Update width factor based on the new size
		r.p.widthFactor = plotSize.Width / float32(r.p.dataPointsToShow)

		// Refresh the image and cursor
		r.p.RefreshImage()
		r.p.updateCursor()
	*/
}

// Updated cursor positioning method
func (p *Plotter) updateCursor() {
	var x float32
	halfDataPointsToShow := int(float64(p.dataPointsToShow) * .5)
	plotSize := p.canvasImage.Size()

	if p.cursorPos >= p.dataLength-halfDataPointsToShow {
		// Handle cursor position near the end of data
		x = float32(p.dataLength-p.cursorPos) * p.widthFactor
		x = plotSize.Width - x
	} else {
		// Calculate x position based on current view
		x = float32(p.cursorPos-max(p.plotStartPos, 0)) * p.widthFactor
	}

	// Account for zoom slider width and ensure cursor stays within plot bounds
	xOffset := p.zoom.Size().Width + x
	xOffset = min32(xOffset, plotSize.Width+p.zoom.Size().Width)
	xOffset = max32(xOffset, p.zoom.Size().Width)

	p.cursor.Position1 = fyne.NewPos(xOffset, 0)
	p.cursor.Position2 = fyne.NewPos(xOffset+1, plotSize.Height)
	p.cursor.Refresh()
}

// Helper functions
func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func (r *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.canvas}
}
