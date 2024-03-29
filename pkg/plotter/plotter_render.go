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

	pl := r.p.plotContainer.Size()
	r.p.plotMiddle = pl.Width / 2
	r.p.widthFactor = pl.Width / float32(r.p.dataPointsToShow)
	r.p.RefreshImages(pl)
}

func (r *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.container}
}
