package multiwindow

import (
	"encoding/json"
	"errors"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
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

	content  *fyne.Container
	childBuf []fyne.CanvasObject // reused backing slice for content.Objects

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
	m.content = container.New(&multiWinLayout{mw: m})
	m.refreshChildren()
	return widget.NewSimpleRenderer(m.content)
}

func (m *MultipleWindows) Add(w *InnerWindow, startPosition ...fyne.Position) bool {
	if w := m.HasWindow(w.Title()); w != nil {
		m.Raise(w)
		return false
	}
	m.setupChild(w)

	m.windows = append(m.windows, w)

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
			startPosition[0].X = common.Clamp(startPosition[0].X, 0, bounds.Width-size.Width)
			startPosition[0].Y = common.Clamp(startPosition[0].Y, 0, bounds.Height-size.Height)
			// bounds.Subtract(size).Max(startPosition[0])
		}
		// w.Move(startPosition[0].SubtractXY(w.MinSize().Width*0.5, 80))
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
	out := make([]*InnerWindow, len(m.windows))
	copy(out, m.windows)
	return out
}

func (m *MultipleWindows) HasWindow(title string) *InnerWindow {
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

// Cycle raises the bottom-most non-minimized window. m.windows is ordered
// bottom to top, so repeated calls walk round-robin through the stack.
func (m *MultipleWindows) Cycle() {
	for _, w := range m.windows {
		if !w.minimized {
			m.Raise(w)
			return
		}
	}
}

// CycleBack is the inverse of Cycle: it sends the top window to the bottom,
// rotating the stack the other way, and raises whatever surfaces.
func (m *MultipleWindows) CycleBack() {
	for range m.windows {
		last := len(m.windows) - 1
		top := m.windows[last]
		copy(m.windows[1:], m.windows[:last])
		m.windows[0] = top
		if !m.windows[last].minimized {
			m.Raise(m.windows[last])
			return
		}
	}
}

func (m *MultipleWindows) Raise(w *InnerWindow) {
	m.raise(w)
	if obj, ok := w.content.Objects[0].(fyne.Focusable); ok {
		if c := fyne.CurrentApp().Driver().CanvasForObject(w); c != nil {
			c.Focus(obj)
		}
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

// layoutTray docks every minimized window into a row along the bottom edge,
// mimicking a taskbar/system tray.
func (m *MultipleWindows) layoutTray() {
	if m.content == nil {
		return
	}
	const margin float32 = 5
	x := margin
	bottom := m.content.Size().Height
	for _, w := range m.windows {
		if !w.minimized {
			continue
		}
		size := w.MinSize()
		w.Resize(size)
		w.Move(fyne.NewPos(x, bottom-size.Height-margin))
		x += size.Width + margin
	}
}

func (m *MultipleWindows) refreshChildren() {
	if m.content == nil {
		return
	}
	if cap(m.childBuf) < len(m.windows) {
		m.childBuf = make([]fyne.CanvasObject, len(m.windows))
	} else {
		m.childBuf = m.childBuf[:len(m.windows)]
	}
	for i, w := range m.windows {
		m.childBuf[i] = w
	}
	m.content.Objects = m.childBuf
	m.content.Refresh()
}

func (m *MultipleWindows) setupChild(w *InnerWindow) {
	w.OnDragged = func(ev *fyne.DragEvent) {
		if w.maximized {
			w.maximized = false
			w.Resize(w.preMaximizedSize)
			// Convert the cursor's canvas-relative position into a position
			// relative to our container, so this works regardless of where the
			// container sits on the canvas (e.g. below a toolbar/menu).
			local := ev.AbsolutePosition.Subtract(fyne.CurrentApp().Driver().AbsolutePositionForObject(m.content))
			barHeight := w.Theme().Size(theme.SizeNameWindowTitleBarHeight)
			w.Move(local.SubtractXY(w.preMaximizedSize.Width*0.5, barHeight*0.5))
			return
		}

		newPos := w.Position().Add(ev.Dragged)
		if m.LockViewport {
			size := w.Size()
			bounds := m.content.Size()
			newPos.X = common.Clamp(newPos.X, 0, bounds.Width-size.Width)
			newPos.Y = common.Clamp(newPos.Y, 0, bounds.Height-size.Height)
		}
		w.Move(newPos)
	}

	w.OnResized = func(direction resizeDirection, ev *fyne.DragEvent) {
		// ev.Dragged is the total delta since the drag started; the new rect
		// is computed fresh from the rect captured at drag start. Clamping
		// therefore absorbs overshoot: after hitting min size the edge stays
		// put until the mouse travels back past the point where it clamped,
		// matching native window behavior.
		minSize := w.MinSize()
		pos := w.dragStartPos
		size := w.dragStartSize
		right := pos.X + size.Width
		bottom := pos.Y + size.Height

		switch direction {
		case resizeLeft, resizeUpLeft, resizeDownLeft:
			// Moving edge is the left one: keep the right edge fixed, keep the
			// window at least minSize wide and never past the viewport's left.
			pos.X = max(0, min(pos.X+ev.Dragged.DX, right-minSize.Width))
			size.Width = right - pos.X
		case resizeRight, resizeUpRight, resizeDownRight:
			size.Width = max(minSize.Width, size.Width+ev.Dragged.DX)
		}

		switch direction {
		case resizeUp, resizeUpLeft, resizeUpRight:
			pos.Y = max(0, min(pos.Y+ev.Dragged.DY, bottom-minSize.Height))
			size.Height = bottom - pos.Y
		case resizeDown, resizeDownLeft, resizeDownRight:
			size.Height = max(minSize.Height, size.Height+ev.Dragged.DY)
		}

		if m.LockViewport {
			contentSize := m.content.Size()
			size.Width = min(size.Width, contentSize.Width-pos.X)
			size.Height = min(size.Height, contentSize.Height-pos.Y)
		}

		w.Move(pos)
		w.Resize(size.Max(minSize))
		w.maximized = false
	}

	w.OnTappedBar = func() {
		if w.minimized {
			w.OnMinimized() // single click on a tray bar restores it
		}
	}

	w.OnMinimized = func() {
		w.minimized = !w.minimized
		if w.minimized {
			w.preMinimizedSize = w.Size()
			w.preMinimizedPos = w.Position()
		} else {
			w.Resize(w.preMinimizedSize)
			w.Move(w.preMinimizedPos)
			m.Raise(w)
		}
		w.Refresh()
		m.layoutTray()
	}

	w.OnMouseDown = func() {
		if f, ok := w.content.Objects[0].(fyne.Focusable); ok {
			if c := fyne.CurrentApp().Driver().CanvasForObject(w); c != nil {
				c.Focus(f)
			}
			// c.SetOnTypedKey(f.TypedKey)
		}
		m.Raise(w)
	}

	w.OnMaximized = func() {
		if w.minimized {
			w.OnMinimized() // restore from tray instead of maximizing
			return
		}
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
	if viewportSize.Width == 0 || viewportSize.Height == 0 {
		// Viewport not laid out yet; positions/sizes would all collapse to zero.
		return nil
	}
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
	if viewportSize.Width == 0 || viewportSize.Height == 0 {
		// Avoid Inf/NaN ratios (which json.Marshal rejects) when not yet sized.
		return nil, errors.New("multiwindow: cannot save layout before viewport is sized")
	}

	for _, w := range wm.windows {
		if w.IgnoreSave {
			continue
		}
		pos := w.Position()
		size := w.Size()
		if w.minimized { // save real geometry, not the tiny tray bar
			pos = w.preMinimizedPos
			size = w.preMinimizedSize
		}
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
