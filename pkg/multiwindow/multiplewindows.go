package multiwindow

import (
	"log"
	"math"
	"runtime"
	"sync"
	"time"

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

	content *fyne.Container

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

func (m *MultipleWindows) ArrangeWindows(a MultipleWindowsArrangeLayout) {
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
	maxSize := m.content.Size()
	numWindows := len(m.Windows)
	if numWindows == 0 {
		return
	}

	// Calculate grid dimensions
	cols := int(math.Ceil(math.Sqrt(float64(numWindows))))
	rows := (numWindows + cols - 1) / cols

	// Calculate padding and minimum size
	padding := float32(0)
	minSize := float32(200)

	// Calculate base window dimensions
	availableWidth := maxSize.Width - (padding * float32(cols+1))
	availableHeight := maxSize.Height - (padding * float32(rows+1))
	baseWidth := max32(availableWidth/float32(cols), minSize)
	baseHeight := max32(availableHeight/float32(rows), minSize)

	// Calculate windows in last row
	windowsInLastRow := numWindows - (rows-1)*cols

	// Pre-allocate new windows slice
	newWindows := make([]*InnerWindow, numWindows)

	// Arrange windows directly in sorted order
	for i := 0; i < numWindows; i++ {
		row := i / cols
		col := i % cols
		isLastRow := row == rows-1

		width := baseWidth
		posX := padding + float32(col)*(baseWidth+padding)
		posY := padding + float32(row)*(baseHeight+padding)

		// If this is the last row and we have fewer windows than cols,
		// expand the windows to fill the space
		if isLastRow && windowsInLastRow < cols {
			totalPadding := padding * float32(windowsInLastRow+1)
			width = (maxSize.Width - totalPadding) / float32(windowsInLastRow)
			posX = padding + float32(col)*(width+padding)
		}

		window := m.Windows[i]
		window.preMaximizedPos = fyne.NewPos(posX+(width-window.Size().Width)/2, posY+(baseHeight-window.Size().Height)/2)
		window.preMaximizedSize = window.Size()
		window.Resize(fyne.NewSize(width, baseHeight))
		window.Move(fyne.NewPos(posX, posY))
		window.maximized = true

		// Store window directly in its final position
		newWindows[i] = window
	}

	// Update windows slice
	m.Windows = newWindows
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func (m *MultipleWindows) CreateRenderer() fyne.WidgetRenderer {
	m.content = container.New(&multiWinLayout{})
	m.refreshChildren()
	return widget.NewSimpleRenderer(container.NewScroll(m.content))
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
	timeStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, fullPath, line, ok := runtime.Caller(2)
	filename := fullPath
	if ok {
		log.Printf("%s %s:%d %s", timeStr, filename, line, "Refreshing children")
	} else {
		log.Println(timeStr + " Refreshing children")
	}
	if m.content == nil {
		return
	}

	objs := make([]fyne.CanvasObject, len(m.Windows))
	for i, w := range m.Windows {
		objs[i] = w
		//m.setupChild(w)
	}
	m.content.Objects = objs
	m.content.Refresh()
}

func (m *MultipleWindows) setupChild(w *InnerWindow) {
	w.OnDragged = func(ev *fyne.DragEvent) {
		if w.maximized {
			// Calculate relative mouse position as a ratio (0 to 1) of where user clicked on title bar
			mouseRatio := ev.Position.X / w.Size().Width
			w.Resize(w.MinSize())
			mouseOffset := mouseRatio * w.MinSize().Width
			newX := ev.AbsolutePosition.X - mouseOffset
			w.Move(fyne.NewPos(newX, w.Position().Y))
			w.maximized = false
			return
		}

		newPos := w.Position().Add(ev.Dragged)
		if m.LockViewport {
			// Ensure the window stays within the content bounds
			contentSize := m.content.Size()
			windowSize := w.Size()

			// Clamp X position
			if newPos.X < 0 {
				newPos.X = 0
			} else if newPos.X+windowSize.Width > contentSize.Width {
				newPos.X = contentSize.Width - windowSize.Width
			}

			// Clamp Y position
			if newPos.Y < 0 {
				newPos.Y = 0
			} else if newPos.Y+windowSize.Height > contentSize.Height {
				newPos.Y = contentSize.Height - windowSize.Height
			}
		}
		w.Move(newPos)
	}

	w.OnResized = func(ev *fyne.DragEvent) {
		newSize := w.Size().Add(ev.Dragged)
		minSize := w.MinSize()
		if m.LockViewport {
			// Ensure the window size stays within content bounds
			contentSize := m.content.Size()
			pos := w.Position()

			// Clamp width
			maxWidth := contentSize.Width - pos.X
			if newSize.Width > maxWidth {
				newSize.Width = maxWidth
			}

			// Clamp height
			maxHeight := contentSize.Height - pos.Y
			if newSize.Height > maxHeight {
				newSize.Height = maxHeight
			}
		}
		// Ensure size is not smaller than minimum size
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
			// Store current position and size before maximizing
			w.preMaximizedSize = w.Size()
			w.preMaximizedPos = w.Position()
			w.Move(fyne.NewPos(0, 0))
			w.Resize(m.Size())
			w.maximized = true
			return
		}
		w.Move(w.preMaximizedPos)
		if w.preMaximizedSize == w.Size() {
			w.preMaximizedSize = w.MinSize()
		}
		w.Resize(w.preMaximizedSize)
		w.maximized = false
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
