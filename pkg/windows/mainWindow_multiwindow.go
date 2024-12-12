package windows

import (
	"sync"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/multiwindow"
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
	position fyne.Position
	size     fyne.Size
}

type windowManager struct {
	open    map[string]*innerWindow
	history map[string]windowHistory
	content *multiwindow.MultipleWindows
	mu      sync.RWMutex

	openOffset fyne.Position
}

func newWindowManager() *windowManager {
	wm := &windowManager{
		open:    make(map[string]*innerWindow),
		history: make(map[string]windowHistory),
		content: multiwindow.NewMultipleWindows(),
	}
	wm.content.LockViewport = true
	return wm
}

func (wm *windowManager) Exists(title string) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	w, ok := wm.open[title]
	if ok {
		wm.content.Raise(w.InnerWindow)
	}
	return ok
}

func (wm *windowManager) Add(w *innerWindow) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.open[w.Title()] = w
	h, found := wm.history[w.Title()]
	if found {
		w.Move(h.position)
		w.Resize(h.size)
	} else {
		w.Move(wm.openOffset)
		wm.openOffset = wm.openOffset.AddXY(15, 15)
		if wm.openOffset.X > 150 {
			wm.openOffset.X = 0
			wm.openOffset.Y = 0
		}
	}
	wm.content.Add(w.InnerWindow)
}

func (wm *windowManager) Remove(w *innerWindow) {
	delete(wm.open, w.Title())
	wm.content.Remove(w.InnerWindow)
	wm.history[w.Title()] = windowHistory{
		position: w.Position(),
		size:     w.Size(),
	}
}

func (wm *windowManager) Size() fyne.Size {
	return wm.content.Size()
}
