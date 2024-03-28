package plotter

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
)

func (p *Plotter) Dragged(event *fyne.DragEvent) {
	p.sel.SetValue(p.sel.Value - float64(event.Dragged.DX))
	//p.cursor.Move(fyne.NewPos(p.cursor.Position1.X, 0))

}

func (p *Plotter) DragEnd() {
}

func (p *Plotter) Scrolled(event *fyne.ScrollEvent) {
	if event.Scrolled.DY > 0 {
		val := p.zoom.Value - (p.zoom.Value * .08)
		p.zoom.SetValue(val)
	} else {
		val := p.zoom.Value + (p.zoom.Value * .08)
		p.zoom.SetValue(val)
	}
}

func (p *Plotter) onZoom(pos float64) {
	//old := p.dataPointsToShow
	p.dataPointsToShow = int(pos)

	//if old > p.dataPointsToShow {
	//	p.start = int(float64(p.start) - float64(old-(p.dataPointsToShow/2)))
	//} else {
	//	p.start = int(float64(p.start) - float64(old-(p.dataPointsToShow/2)))
	//}
	//p.zoom.Max = min(float64(p.dataLength-p.dataPointsToShow), 6000)
	p.zoom.Refresh()
	p.sel.Refresh()
	p.RefreshImages(p.plotContainer.Size())
}

func (p *Plotter) MouseIn(event *desktop.MouseEvent) {
	//log.Println("MouseIn", event.Position)
	//if event.Position.X <= p.container.Size().Width-p.legend.Size().Width-theme.Padding() {
	//	//p.cursor.Position1 = fyne.NewPos(event.Position.X, 0)
	//	//p.cursor.Position2 = fyne.NewPos(event.Position.X, p.lines.Size().Height)
	//	p.cursor.Move(fyne.NewPos(event.Position.X, 0))
	//}
}

func (p *Plotter) MouseMoved(event *desktop.MouseEvent) {
	if p.Logplayer {
		return
	}
	containerWidth := p.plotContainer.Size().Width
	//legendWidth := p.legend.Size().Width
	padding := theme.Padding()
	zw := p.zoom.Size().Width
	if event.Position.X > zw && event.Position.X <= zw+containerWidth {
		x := event.Position.X - (p.zoom.Size().Width + padding)
		widthFactor := p.plotContainer.Size().Width / float32(p.dataPointsToShow)
		pos := float64((x / widthFactor) + float32(p.start))
		if pos < 0 {
			pos = 0
		}
		p.cursor.Move(fyne.NewPos(event.Position.X, 0))
		p.updateLegend(pos)
	}
}

func (p *Plotter) MouseOut() {
	//log.Println("MouseOut")
}
