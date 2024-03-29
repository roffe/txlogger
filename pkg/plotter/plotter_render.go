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
	//if r.size.Width == size.Width && r.size.Height == size.Height {
	//	return
	//}

	r.p.plotMiddle = size.Width / 2
	r.size = size
	log.Println("plotter.Layout", size.Width, size.Height)
	r.p.container.Resize(size)
	pl := r.p.plotContainer.Size()
	r.p.cursor.Position2 = fyne.NewPos(r.p.cursor.Position1.X, pl.Height)
	r.p.cursor.Refresh()
	r.p.RefreshImages(pl)
}

func (r *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.container}
}
