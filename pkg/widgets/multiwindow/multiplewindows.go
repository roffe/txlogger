package multiwindow

import (
	"log"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// MultipleWindows is a container that handles multiple `InnerWindow` containers.
type MultipleWindows struct {
	widget.BaseWidget

	LockViewport bool

	Windows []*InnerWindow

	content      *fyne.Container
	propertyLock sync.RWMutex

	openOffset fyne.Position
}

// NewMultipleWindows creates a new `MultipleWindows` container to manage many inner windows.
// The initial window list is passed optionally to this constructor function.
// You can add new more windows to this container by calling `Add` or updating the `Windows`
// field and calling `Refresh`.
func NewMultipleWindows(wins ...*InnerWindow) *MultipleWindows {
	m := &MultipleWindows{Windows: wins}
	m.ExtendBaseWidget(m)
	return m
}

func (m *MultipleWindows) CreateRenderer() fyne.WidgetRenderer {
	m.content = container.New(&multiWinLayout{})
	m.refreshChildren()
	return widget.NewSimpleRenderer(m.content)
}

func (m *MultipleWindows) Add(w *InnerWindow, startPosition ...fyne.Position) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()
	m.setupChild(w)
	m.Windows = append(m.Windows, w)
	if len(startPosition) == 0 {
		w.Move(m.openOffset)
		m.openOffset = m.openOffset.AddXY(15, 15)
		if m.openOffset.X > 150 {
			m.openOffset.X = 0
			m.openOffset.Y = 0
		}
	} else if len(startPosition) == 1 {
		w.Move(startPosition[0])
	}

	m.content.Add(w)
	m.raise(w)
}

// Remove removes the given window from the container.
func (m *MultipleWindows) Remove(w *InnerWindow) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()
	for i, ww := range m.Windows {
		if ww == w {
			m.Windows = append(m.Windows[:i], m.Windows[i+1:]...)
			m.content.Remove(w)
			m.refreshChildren()
			return
		}
	}
}

func (m *MultipleWindows) Arrange(arr Arranger) {
	arr.Layout(m.content.Size(), m.LockViewport, m.Windows)
}

func (m *MultipleWindows) Refresh() {
	m.refreshChildren()
}

func (m *MultipleWindows) Raise(w *InnerWindow) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()
	m.raise(w)
}

func (m *MultipleWindows) raise(w *InnerWindow) {
	if w.active {
		return
	}
	id := -1
	for i, ww := range m.Windows {
		if ww == w {
			id = i
			w.bgFillColor = theme.ColorNamePrimary
			w.active = true
			continue
		}
		ww.active = false
		ww.bgFillColor = theme.ColorNameOverlayBackground
	}
	if id == -1 {
		return
	}
	windows := append(m.Windows[:id], m.Windows[id+1:]...)
	m.Windows = append(windows, w)
	m.refreshChildren()
}

func (m *MultipleWindows) refreshChildren() {
	if m.content == nil {
		return
	}
	objs := make([]fyne.CanvasObject, len(m.Windows))
	for i, w := range m.Windows {
		objs[i] = w
	}
	m.content.Objects = objs
	m.content.Refresh()
}

func (m *MultipleWindows) setupChild(w *InnerWindow) {
	w.OnDragged = func(ev *fyne.DragEvent) {
		if w.maximized {
			mouseRatio := ev.Position.X / w.Size().Width
			sz := w.MinSize()
			log.Println(sz)
			w.Resize(sz)
			w.Move(fyne.NewPos(ev.AbsolutePosition.X-mouseRatio*w.MinSize().Width, w.Position().Y))
			w.maximized = false
			return
		}

		newPos := w.Position().Add(ev.Dragged)
		if m.LockViewport {
			size := w.Size()
			bounds := m.content.Size()
			newPos.X = clamp32(newPos.X, 0, bounds.Width-size.Width)
			newPos.Y = clamp32(newPos.Y, 0, bounds.Height-size.Height)
			bounds.Subtract(size).Max(newPos)
		}
		w.Move(newPos)
	}

	w.OnResized = func(ev *fyne.DragEvent) {
		var newSize fyne.Size
		minSize := w.MinSize()
		currentSize := w.Size()

		if w.leftDrag {
			actualDX := ev.Dragged.DX
			if actualDX > 0 {
				// When shrinking (dragging right), limit by remaining width
				actualDX = fyne.Min(actualDX, currentSize.Width-minSize.Width)
			} else if w.Position().X+actualDX < 0 {
				// Prevent dragging past left edge
				actualDX = -w.Position().X
			}

			newSize = fyne.NewSize(currentSize.Width-actualDX, currentSize.Height+ev.Dragged.DY)
			w.Move(w.Position().Add(fyne.NewPos(actualDX, 0)))
		} else {
			newSize = currentSize.Add(ev.Dragged)
		}

		if m.LockViewport {
			contentSize := m.content.Size()
			pos := w.Position()
			maxWidth := contentSize.Width - pos.X
			maxHeight := contentSize.Height - pos.Y
			newSize.Width = fyne.Min(newSize.Width, maxWidth)
			newSize.Height = fyne.Min(newSize.Height, maxHeight)
		}

		w.Resize(newSize.Max(minSize))
		w.maximized = false
	}

	w.OnTappedBar = func() {
		//m.Raise(w)
	}

	w.OnMouseDown = func() {
		m.Raise(w)
	}

	w.OnMaximized = func() {
		if !w.maximized {
			w.preMaximizedSize = w.Size()
			w.preMaximizedPos = w.Position()
			w.Move(fyne.NewPos(0, 0))
			w.Resize(m.Size())
		} else {
			w.Move(w.preMaximizedPos)
			if w.preMaximizedSize == w.Size() {
				w.preMaximizedSize = w.MinSize()
			}
			w.Resize(w.preMaximizedSize)
		}
		w.maximized = !w.maximized
		m.Raise(w)
	}
}

type multiWinLayout struct {
}

func (m *multiWinLayout) Layout(objects []fyne.CanvasObject, _ fyne.Size) {
	for _, w := range objects { // update the windows so they have real size
		w.Resize(w.MinSize().Max(w.Size()))
	}
}

func (m *multiWinLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.Size{}
}

func clamp32(value, min, max float32) float32 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
