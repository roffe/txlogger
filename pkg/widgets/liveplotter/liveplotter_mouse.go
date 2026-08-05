package liveplotter

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

var _ desktop.Hoverable = (*Widget)(nil)

// Scrolled zooms the visible time span: scrolling up zooms in (shorter span),
// down zooms out toward the full window.
func (p *Widget) Scrolled(event *fyne.ScrollEvent) {
	if event.Scrolled.DY > 0 {
		p.zoom.SetValue(p.zoom.Value - 5)
	} else {
		p.zoom.SetValue(p.zoom.Value + 5)
	}
}

// Dragged pans the view back in time, pausing the live follow. Dragging right
// reveals older data; the anchored right edge is clamped to the retained window
// in refresh.
func (p *Widget) Dragged(event *fyne.DragEvent) {
	plotW := p.plotObj.Size().Width
	if plotW <= 0 {
		return
	}
	if p.follow {
		p.setFollow(false)
	}
	deltaMillis := int64(float64(event.Dragged.DX) / float64(plotW) * float64(p.spanMillis()))
	p.anchorMillis -= deltaMillis
	p.scheduleRefresh()
}

func (p *Widget) DragEnd() {}

// MouseIn / MouseMoved track the pointer over the plot so the cursor line and
// legend follow it. MouseOut returns the cursor to the right edge and the legend
// to the newest/frozen value.
func (p *Widget) MouseIn(event *desktop.MouseEvent) { p.onMouse(event.Position) }

func (p *Widget) MouseMoved(event *desktop.MouseEvent) { p.onMouse(event.Position) }

func (p *Widget) MouseOut() {
	p.mouseInPlot = false
	p.refreshCursorAndLegend()
}

// onMouse maps a pointer position (widget-local) onto the plot and updates the
// cursor + legend. Runs on the UI goroutine.
func (p *Widget) onMouse(pos fyne.Position) {
	plotW := p.plotObj.Size().Width
	relX := pos.X - p.zoom.Size().Width
	if relX < 0 {
		relX = 0
	}
	if relX > plotW {
		relX = plotW
	}
	p.cursorX = relX
	p.mouseInPlot = true
	p.refreshCursorAndLegend()
}

// refreshCursorAndLegend repositions the cursor and updates the legend readout
// without redrawing the trace (the samples are unchanged on a mouse move).
func (p *Widget) refreshCursorAndLegend() {
	p.computeLegendVals()
	p.updateLegendValues()
	p.layoutCursor()
	p.cursor.Refresh()
}
