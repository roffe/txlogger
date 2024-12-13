package multiwindow

import (
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// MultipleWindowsArrangeLayout is an enumeration of the possible layout arrangements for the `MultipleWindows.Arrange()` container.
type MultipleWindowsArrangeLayout int

const (
	// MultipleWindowsArrangementLayoutHorizontal will arrange the windows horizontally.
	MultipleWindowsArrangeLayoutHorizontal MultipleWindowsArrangeLayout = iota
	// MultipleWindowsArrangementLayoutVertical will arrange the windows vertically.
	MultipleWindowsArrangeLayoutVertical
	// MultipleWindowsArrangementLayoutGrid will arrange the windows in a grid.
	MultipleWindowsArrangeLayoutGrid
)

// MultipleWindows is a container that handles multiple `InnerWindow` containers.
type MultipleWindows struct {
	widget.BaseWidget

	// LockViewport determines if the windows can be moved or resized outside the viewport.
	// If set to true, the windows size & position will be clamped to the viewport bounds.
	LockViewport bool

	Windows []*InnerWindow

	content      *fyne.Container
	propertyLock sync.RWMutex
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

func (m *MultipleWindows) Add(w *InnerWindow) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()
	m.setupChild(w)
	m.Windows = append(m.Windows, w)
	m.refreshChildren()
}

// Remove removes the given window from the container.
func (m *MultipleWindows) Remove(w *InnerWindow) {
	m.propertyLock.Lock()
	defer m.propertyLock.Unlock()

	id := -1
	for i, ww := range m.Windows {
		if ww == w {
			id = i
			break
		}
	}
	if id == -1 {
		return
	}
	m.Windows = append(m.Windows[:id], m.Windows[id+1:]...)
	m.refreshChildren()
}

func (m *MultipleWindows) Arrange(a MultipleWindowsArrangeLayout) {
	switch a {
	case MultipleWindowsArrangeLayoutHorizontal:
		for i, w := range m.Windows {
			w.Move(fyne.NewPos(float32(i)*w.Size().Width, 0))
		}
	case MultipleWindowsArrangeLayoutVertical:
		for i, w := range m.Windows {
			w.Move(fyne.NewPos(0, float32(i)*w.Size().Height))
		}
	case MultipleWindowsArrangeLayoutGrid:
		m.arrangeSquare()
	default:
	}
}

func (m *MultipleWindows) arrangeSquare() {
	if len(m.Windows) == 0 {
		return
	}

	maxSize := m.content.Size()
	numWindows := len(m.Windows)

	// Calculate grid dimensions - store as constants to avoid recalculation
	cols := int(math.Ceil(math.Sqrt(float64(numWindows))))
	rows := (numWindows + cols - 1) / cols
	windowsInLastRow := numWindows - (rows-1)*cols

	const (
		padding float32 = 0
		minSize float32 = 200
	)

	// Pre-calculate dimensions once
	availWidth := maxSize.Width - (padding * float32(cols+1))
	availHeight := maxSize.Height - (padding * float32(rows+1))
	baseWidth := fyne.Max(availWidth/float32(cols), minSize)
	baseHeight := fyne.Max(availHeight/float32(rows), minSize)

	// Pre-calculate last row width if needed
	var lastRowWidth float32
	if windowsInLastRow < cols {
		totalPadding := padding * float32(windowsInLastRow+1)
		lastRowWidth = (maxSize.Width - totalPadding) / float32(windowsInLastRow)
	}

	// Process windows in place to avoid allocation
	for i, window := range m.Windows {
		row := i / cols
		col := i % cols

		width := baseWidth
		posX := padding + float32(col)*(baseWidth+padding)

		// Only adjust width for last row
		if row == rows-1 && windowsInLastRow < cols {
			width = lastRowWidth
			posX = padding + float32(col)*(width+padding)
		}

		posY := padding + float32(row)*(baseHeight+padding)

		// Set window properties directly
		window.preMaximizedPos = fyne.NewPos(
			posX+(width-window.Size().Width)/2,
			posY+(baseHeight-window.Size().Height)/2,
		)
		window.preMaximizedSize = window.Size()
		window.Resize(fyne.NewSize(width, baseHeight))
		window.Move(fyne.NewPos(posX, posY))
		window.maximized = true
	}
}

func (m *MultipleWindows) CreateRenderer() fyne.WidgetRenderer {
	m.content = container.New(&multiWinLayout{})
	m.refreshChildren()
	return widget.NewSimpleRenderer(m.content)
}

func (m *MultipleWindows) Refresh() {
	m.refreshChildren()
}

func (m *MultipleWindows) Raise(w *InnerWindow) {
	if w.active {
		return
	}
	m.propertyLock.RLock()
	defer m.propertyLock.RUnlock()

	id := -1
	for i, ww := range m.Windows {
		if ww == w {
			id = i
			w.bgFillColor = theme.ColorNamePrimary
			w.active = true
			continue
		}
		ww.bgFillColor = theme.ColorNameOverlayBackground
		ww.active = false
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

	objects := make([]fyne.CanvasObject, len(m.Windows))
	for i, w := range m.Windows {
		objects[i] = w
	}
	m.content.Objects = objects
	m.content.Refresh()
}

func (m *MultipleWindows) setupChild(w *InnerWindow) {
	w.OnDragged = func(ev *fyne.DragEvent) {
		if w.maximized {
			mouseRatio := ev.Position.X / w.Size().Width
			w.Resize(w.MinSize())
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
		m.Raise(w)
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
