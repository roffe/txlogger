package plotter

import "fyne.io/fyne/v2"

type plotterRenderer struct {
	*Plotter
}

func (p *plotterRenderer) MinSize() fyne.Size {
	return p.container.MinSize()
}

func (p *plotterRenderer) Layout(size fyne.Size) {
}

func (p *plotterRenderer) Refresh() {
}

func (p *plotterRenderer) Destroy() {
}

func (p *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{p.container,
		p.overlayText,
		p.cursor,
	}
}
