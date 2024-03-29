package plotter

import (
	"log"

	"fyne.io/fyne/v2"
)

type plotterRenderer struct {
	plotter *Plotter
	size    fyne.Size
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
	r.size = size
	log.Println("plotter.Layout", size.Width, size.Height)
	r.plotter.container.Resize(size)
	pl := r.plotter.plotContainer.Size()
	r.plotter.cursor.Position2 = fyne.NewPos(r.plotter.cursor.Position1.X, pl.Height)
	r.plotter.cursor.Refresh()
	r.plotter.RefreshImages(pl)
}

func (r *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.plotter.container}
}
