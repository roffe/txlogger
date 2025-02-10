package multiwindow

import (
	"encoding/json"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type WindowRatio struct {
	X, Y float64 // Position ratios
	W, H float64 // Size ratios
}

type WindowProperties struct {
	Title            string
	Ratios           WindowRatio
	Maximized        bool
	PreMaximizedPos  WindowRatio
	PreMaximizedSize WindowRatio
}

// MultipleWindows is a container that handles multiple `InnerWindow` containers.
type MultipleWindows struct {
	widget.BaseWidget

	LockViewport bool

	windows []*InnerWindow

	content      *fyne.Container
	propertyLock sync.RWMutex

	openOffset fyne.Position

	OnError func(error)
}

// NewMultipleWindows creates a new `MultipleWindows` container to manage many inner windows.
// The initial window list is passed optionally to this constructor function.
// You can add new more windows to this container by calling `Add` or updating the `Windows`
// field and calling `Refresh`.
func NewMultipleWindows(wins ...*InnerWindow) *MultipleWindows {
	m := &MultipleWindows{}
	m.ExtendBaseWidget(m)
	return m
}

func (m *MultipleWindows) CreateRenderer() fyne.WidgetRenderer {
	m.content = container.New(&multiWinLayout{})
	m.refreshChildren()
	return widget.NewSimpleRenderer(m.content)
}

func (m *MultipleWindows) Add(w *InnerWindow, startPosition ...fyne.Position) bool {
	if w := m.HasWindow(w.Title()); w != nil {
		m.Raise(w)
		return false
	}

	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()

	m.windows = append(m.windows, w)
	m.setupChild(w)
	if len(startPosition) == 0 {
		w.Move(m.openOffset)
		m.openOffset = m.openOffset.AddXY(15, 15)
		if m.openOffset.X > 150 {
			m.openOffset.X = 0
			m.openOffset.Y = 0
		}
	} else if len(startPosition) == 1 {
		if m.LockViewport {
			size := w.MinSize()
			bounds := m.content.Size()
			startPosition[0].X = clamp32(startPosition[0].X, 0, bounds.Width-size.Width)
			startPosition[0].Y = clamp32(startPosition[0].Y, 0, bounds.Height-size.Height)
			bounds.Subtract(size).Max(startPosition[0])
		}
		w.Move(startPosition[0])
	}

	m.content.Add(w)
	m.raise(w)
	if c := fyne.CurrentApp().Driver().CanvasForObject(m.content); c != nil {
		if focusable, ok := w.content.Objects[0].(fyne.Focusable); ok {
			c.Focus(focusable)
		}
	}
	return true
}

func (m *MultipleWindows) Windows() []*InnerWindow {
	m.propertyLock.RLock()
	defer m.propertyLock.RUnlock()
	out := make([]*InnerWindow, len(m.windows))
	copy(out, m.windows)
	return out
}

func (m *MultipleWindows) HasWindow(title string) *InnerWindow {
	//m.propertyLock.RLock()
	//defer m.propertyLock.RUnlock()
	for _, w := range m.windows {
		if w.Title() == title {
			return w
		}
	}
	return nil
}

func (m *MultipleWindows) CloseAll() {
	windows := make([]*InnerWindow, len(m.windows))
	copy(windows, m.windows)
	for _, w := range windows {
		if w.Persist {
			// log.Println("Persist", w.Title())
			continue
		}
		// log.Println("Close", w.Title())
		w.Close()
	}
}

/*
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
*/

func (m *MultipleWindows) remove(w *InnerWindow) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()
	for i, ww := range m.windows {
		if ww == w {
			m.windows = append(m.windows[:i], m.windows[i+1:]...)
			m.content.Remove(w)
			m.refreshChildren()
			return
		}
	}
}

func (m *MultipleWindows) Arrange(arr Arranger) {
	arr.Layout(m.content.Size(), m.LockViewport, m.windows)
}

func (m *MultipleWindows) Refresh() {
	m.refreshChildren()
}

func (m *MultipleWindows) Raise(w *InnerWindow) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()
	m.raise(w)
	if obj, ok := w.content.Objects[0].(fyne.Focusable); ok {
		fyne.CurrentApp().Driver().CanvasForObject(w).Focus(obj)
	}
}

func (m *MultipleWindows) raise(w *InnerWindow) {
	if w.active {
		return
	}
	id := -1
	for i, ww := range m.windows {
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
	windows := append(m.windows[:id], m.windows[id+1:]...)
	m.windows = append(windows, w)
	m.refreshChildren()
}

func (m *MultipleWindows) refreshChildren() {
	if m.content == nil {
		return
	}
	objs := make([]fyne.CanvasObject, len(m.windows))
	for i, w := range m.windows {
		objs[i] = w
	}
	m.content.Objects = objs
	m.content.Refresh()
}

func (m *MultipleWindows) setupChild(w *InnerWindow) {
	w.OnDragged = func(ev *fyne.DragEvent) {
		//S		log.Println("Dragged", ev.Dragged)
		// log.Println(m.content.Size())
		if w.maximized {
			//mouseRatio := ev.Position.X / w.Size().Width
			//w.Resize(w.MinSize())
			//w.Move(fyne.NewPos(ev.AbsolutePosition.X-mouseRatio*w.MinSize().Width, w.Position().Y))
			w.maximized = false

			w.Move(ev.Position.SubtractXY(w.preMaximizedSize.Width*0.5, 5))
			w.Resize(w.MinSize())

			return
		}

		newPos := w.Position().Add(ev.Dragged)
		if m.LockViewport {
			size := w.Size()
			bounds := m.content.Size()
			newPos.X = clamp32(newPos.X, 0, bounds.Width-size.Width)
			newPos.Y = clamp32(newPos.Y, 0, bounds.Height-size.Height)
			//bounds.Subtract(size).Max(newPos)
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
		if f, ok := w.content.Objects[0].(fyne.Focusable); ok {
			c := fyne.CurrentApp().Driver().CanvasForObject(w)
			c.Focus(f)
			//c.SetOnTypedKey(f.TypedKey)
		}
		m.Raise(w)
	}

	w.OnMaximized = func() {
		if !w.maximized {
			w.preMaximizedSize = w.Size()
			w.preMaximizedPos = w.Position()
			am := canvas.NewPositionAnimation(w.Position(), fyne.NewPos(0, 0), 250*time.Millisecond, func(pos fyne.Position) {
				w.Move(pos)
			})
			am.Start()
			rm := canvas.NewSizeAnimation(w.Size(), m.content.Size(), 250*time.Millisecond, func(sz fyne.Size) {
				w.Resize(sz)
			})
			rm.Start()
		} else {
			am := canvas.NewPositionAnimation(w.Position(), w.preMaximizedPos, 250*time.Millisecond, func(pos fyne.Position) {
				w.Move(pos)
			})
			am.Start()
			if w.preMaximizedSize == w.Size() {
				w.preMaximizedSize = w.MinSize()
			}
			rm := canvas.NewSizeAnimation(w.Size(), w.preMaximizedSize, 250*time.Millisecond, func(sz fyne.Size) {
				w.Resize(sz)
			})
			rm.Start()
		}
		w.maximized = !w.maximized
		m.Raise(w)
	}

	w.onClose = func() {
		m.remove(w)
	}
}

func (m *MultipleWindows) LoadLayout(windows []WindowProperties) error {
	viewportSize := m.Size()
	for _, h := range windows {
		for _, w := range m.windows {
			if w.Title() == h.Title {
				// Convert ratios to absolute positions and sizes
				position := fyne.NewPos(
					float32(h.Ratios.X*float64(viewportSize.Width)),
					float32(h.Ratios.Y*float64(viewportSize.Height)),
				)
				size := fyne.NewSize(
					float32(h.Ratios.W*float64(viewportSize.Width)),
					float32(h.Ratios.H*float64(viewportSize.Height)),
				)

				preMaxPos := fyne.NewPos(
					float32(h.PreMaximizedPos.X*float64(viewportSize.Width)),
					float32(h.PreMaximizedPos.Y*float64(viewportSize.Height)),
				)
				preMaxSize := fyne.NewSize(
					float32(h.PreMaximizedSize.W*float64(viewportSize.Width)),
					float32(h.PreMaximizedSize.H*float64(viewportSize.Height)),
				)
				w.Move(position)
				w.Resize(size)
				w.SetMaximized(h.Maximized, preMaxPos, preMaxSize)
				break
			}
		}
	}
	return nil
}

func (wm *MultipleWindows) JsonLayout() ([]byte, error) {
	var history []WindowProperties
	viewportSize := wm.Size()

	for _, w := range wm.windows {
		if w.IgnoreSave {
			continue
		}
		pos := w.Position()
		size := w.Size()
		preMaxPos := w.PreMaximizedPos()
		preMaxSize := w.PreMaximizedSize()

		entry := WindowProperties{
			Title: w.Title(),
			Ratios: WindowRatio{
				X: float64(pos.X) / float64(viewportSize.Width),
				Y: float64(pos.Y) / float64(viewportSize.Height),
				W: float64(size.Width) / float64(viewportSize.Width),
				H: float64(size.Height) / float64(viewportSize.Height),
			},
			Maximized: w.Maximized(),
			PreMaximizedPos: WindowRatio{
				X: float64(preMaxPos.X) / float64(viewportSize.Width),
				Y: float64(preMaxPos.Y) / float64(viewportSize.Height),
			},
			PreMaximizedSize: WindowRatio{
				W: float64(preMaxSize.Width) / float64(viewportSize.Width),
				H: float64(preMaxSize.Height) / float64(viewportSize.Height),
			},
		}

		history = append(history, entry)
	}

	b, err := json.Marshal(history)
	if err != nil {
		return nil, err
	}
	return b, nil
}
