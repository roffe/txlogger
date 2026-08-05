package plotter

import (
	"fyne.io/fyne/v2"
)

type plotterRenderer struct {
	PL      *Plotter
	objects []fyne.CanvasObject
}

func (p *plotterRenderer) MinSize() fyne.Size {
	return p.PL.split.MinSize()
}

func (p *plotterRenderer) Layout(size fyne.Size) {
	if p.PL.size == size {
		return
	}
	p.PL.size = size

	p.PL.split.Resize(size)
}

func (p *plotterRenderer) Refresh() {
}

func (p *plotterRenderer) Destroy() {
}

func (p *plotterRenderer) Objects() []fyne.CanvasObject {
	if p.objects == nil {
		p.objects = []fyne.CanvasObject{
			p.PL.split,
			p.PL.overlayText,
			p.PL.cursor,
		}
	}
	return p.objects
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

	t.p.plotObj.Resize(plotSize) // Calculate new plot dimensions
	t.p.plotResolution = fyne.NewSize(plotSize.Width*t.p.plotResolutionFactor, plotSize.Height*t.p.plotResolutionFactor)
	// Update width factor based on the new size
	t.p.widthFactor = plotSize.Width / float32(t.p.dataPointsToShow)
	// Refresh the image and cursor
	t.p.refreshImage(false)
	t.p.updateCursor(false)
}

func (t *plotLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 100)
}
