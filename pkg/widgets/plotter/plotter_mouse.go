package plotter

import (
	"fyne.io/fyne/v2"
)

// func (p *Plotter) Tapped(event *fyne.PointEvent) {
// 	log.Println("Tapped")
// 	if f := p.OnTapped; f != nil {
// 		f(event)
// 	}
// }

func (p *Plotter) Dragged(event *fyne.DragEvent) {
	//p.sel.SetValue(p.sel.Value - float64(event.Dragged.DX))
	if f := p.OnDragged; f != nil {
		f(event)
	}

}

func (p *Plotter) DragEnd() {
}

func (p *Plotter) Scrolled(event *fyne.ScrollEvent) {
	if event.Scrolled.DY > 0 {
		val := p.zoom.Value - 2
		p.zoom.SetValue(val)
	} else {
		val := p.zoom.Value + 2
		p.zoom.SetValue(val)
	}
}

func (p *Plotter) onZoom(value float64) {
	old := p.dataPointsToShow
	p.dataPointsToShow = min(p.dataLength, 25*int(value))
	p.plotStartPos += int(float64(old-p.dataPointsToShow) * 0.5)
	if p.plotStartPos < 0 {
		p.plotStartPos = 0
	}
	p.widthFactor = p.canvasImage.Size().Width / float32(p.dataPointsToShow)
	p.updateCursor(false)
	p.refreshImage(false)
}

/*
func (p *Plotter) MouseMoved(event *desktop.MouseEvent) {
	//if p.Logplayer {
	//	return
	//}
	//containerWidth := p.plotContainer.Size().Width
	////legendWidth := p.legend.Size().Width
	//padding := theme.Padding()
	//zw := p.zoom.Size().Width
	//if event.Position.X > zw && event.Position.X <= zw+containerWidth {
	//	x := event.Position.X - (p.zoom.Size().Width + padding)
	//	widthFactor := p.plotContainer.Size().Width / float32(p.dataPointsToShow)
	//	pos := float64((x / widthFactor) + float32(p.start))
	//	if pos < 0 {
	//		pos = 0
	//	}
	//	//p.cursor.Move(fyne.NewPos(event.Position.X, 0))
	//	p.updateLegend(pos)
	//}
}

func (p *Plotter) MouseOut() {
}

func (p *Plotter) MouseIn(event *desktop.MouseEvent) {
}
*/
