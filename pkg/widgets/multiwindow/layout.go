package multiwindow

import "fyne.io/fyne/v2"

type multiWinLayout struct {
	mw *MultipleWindows
}

func (m *multiWinLayout) Layout(objects []fyne.CanvasObject, _ fyne.Size) {
	for _, w := range objects { // update the windows so they have real size
		// not Size.Max: its Vector2 arg escapes to the heap on every layout pass
		minSize, size := w.MinSize(), w.Size()
		w.Resize(fyne.NewSize(max(minSize.Width, size.Width), max(minSize.Height, size.Height)))
	}
	if m.mw != nil {
		m.mw.layoutTray() // keep minimized windows docked to the bottom on resize
	}
}

func (m *multiWinLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.Size{Width: 700, Height: 400}
}
