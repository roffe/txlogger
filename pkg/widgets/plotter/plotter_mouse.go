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

// dragZoomThreshold is the zoom slider value below which dragging stays a
// strict 1 frame per pixel for fine, precise scrubbing. At or above it the
// drag distance scales with how many data points each pixel covers, so the
// plot tracks the cursor and feels natural when zoomed out.
const dragZoomThreshold = 20

// DragFrameDelta converts a horizontal drag distance (in pixels) into the
// number of log frames to move. Below dragZoomThreshold it is 1:1; when zoomed
// out it scales by the data-points-per-pixel of the current view.
func (p *Plotter) DragFrameDelta(dx float32) float64 {
	scale := 1.0
	// widthFactor is pixels-per-point; <1 means each pixel spans more than one
	// data point, i.e. we're zoomed out far enough for scaling to add frames.
	if p.zoom.Value >= dragZoomThreshold && p.widthFactor > 0 && p.widthFactor < 1 {
		scale = 1.0 / float64(p.widthFactor)
	}
	return float64(dx) * scale
}

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
	p.widthFactor = p.plotObj.Size().Width / float32(p.dataPointsToShow)
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
