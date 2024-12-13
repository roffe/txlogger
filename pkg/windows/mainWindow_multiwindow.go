package windows

import (
	"sync"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

type innerWindow struct {
	*multiwindow.InnerWindow
}

func newInnerWindow(title string, content fyne.CanvasObject) *innerWindow {
	return &innerWindow{
		InnerWindow: multiwindow.NewInnerWindow(title, content),
	}
}

type windowHistory struct {
	position         fyne.Position
	size             fyne.Size
	maximized        bool
	preMaximizedPos  fyne.Position
	preMaximizedSize fyne.Size
}

type windowManager struct {
	open    map[string]*innerWindow
	history map[string]windowHistory
	mu      sync.RWMutex

	*multiwindow.MultipleWindows
	openOffset fyne.Position
}

func newWindowManager() *windowManager {
	wm := &windowManager{
		open:            make(map[string]*innerWindow),
		history:         make(map[string]windowHistory),
		MultipleWindows: multiwindow.NewMultipleWindows(),
	}
	wm.MultipleWindows.LockViewport = true
	return wm
}

func (wm *windowManager) HasWindow(title string) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	w, ok := wm.open[title]
	if ok {
		wm.MultipleWindows.Raise(w.InnerWindow)
	}
	return ok
}

func (wm *windowManager) Add(w *innerWindow) {
	if wm.HasWindow(w.Title()) {
		return
	}
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.open[w.Title()] = w

	wm.MultipleWindows.Add(w.InnerWindow)

	h, found := wm.history[w.Title()]
	if found {
		w.Move(h.position)
		w.Resize(h.size)
		w.SetMaximized(h.maximized, h.preMaximizedPos, h.preMaximizedSize)
	} else {
		w.Move(wm.openOffset)
		wm.openOffset = wm.openOffset.AddXY(15, 15)
		if wm.openOffset.X > 150 {
			wm.openOffset.X = 0
			wm.openOffset.Y = 0
		}
	}
}

func (wm *windowManager) Remove(w *innerWindow) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	title := w.Title()
	delete(wm.open, title)
	wm.MultipleWindows.Remove(w.InnerWindow)
	wm.history[title] = windowHistory{
		position:         w.Position(),
		size:             w.Size(),
		maximized:        w.Maximized(),
		preMaximizedPos:  w.PreMaximizedPos(),
		preMaximizedSize: w.PreMaximizedSize(),
	}
}

func (wm *windowManager) Size() fyne.Size {
	return wm.MultipleWindows.Size()
}
