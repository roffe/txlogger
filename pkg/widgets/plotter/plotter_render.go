package plotter

import (
	"fyne.io/fyne/v2"
)

type plotterRenderer struct {
	*Plotter
}

func (p *plotterRenderer) MinSize() fyne.Size {
	return p.split.MinSize()
}

func (p *plotterRenderer) Layout(size fyne.Size) {
	if p.size == size {
		return
	}
	p.size = size

	p.split.Resize(size)
}

func (p *plotterRenderer) Refresh() {
}

func (p *plotterRenderer) Destroy() {
}

func (p *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{p.split,
		p.overlayText,
		p.cursor,
	}
}

type plotLayout struct {
	p       *Plotter
	oldSize fyne.Size
}

func (t *plotLayout) Layout(_ []fyne.CanvasObject, plotSize fyne.Size) {
	if t.oldSize == plotSize {
		return
	}
	t.oldSize = plotSize

	t.p.overlayText.Move(fyne.NewPos(t.p.zoom.Size().Width, 20))

	t.p.canvasImage.Resize(plotSize) // Calculate new plot dimensions
	t.p.plotResolution = fyne.NewSize(plotSize.Width*t.p.plotResolutionFactor, plotSize.Height*t.p.plotResolutionFactor)
	// Update width factor based on the new size
	t.p.widthFactor = plotSize.Width / float32(t.p.dataPointsToShow)
	// Refresh the image and cursor
	t.p.refreshImage(false)
	t.p.updateCursor()
}

func (t *plotLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 100)
}
