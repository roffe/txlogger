package plotter

import (
	"log"

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
	if r.size.Width == size.Width && r.size.Height == size.Height {
		return
	}
	r.size = size

	log.Println("plotter.Layout", size.Width, size.Height)
	r.p.container.Resize(size)
	pl := r.p.canvasImage.Size()
	r.p.plotResolution = fyne.NewSize(pl.Width*r.p.plotResolutionFactor, pl.Height*r.p.plotResolutionFactor)
	r.p.widthFactor = pl.Width / float32(r.p.dataPointsToShow)
	//r.p.canvasImage.Image = image.NewRGBA(image.Rect(0, 0, int(r.p.plotResolution.Width), int(r.p.plotResolution.Height)))
	r.p.RefreshImage()
	r.p.updateCursor()
}

func (r *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.canvas}
}
