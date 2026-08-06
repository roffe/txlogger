package multiwindow

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// Cycle and CycleBack must be exact inverses, or Ctrl+Tab / Ctrl+Shift+Tab
// won't retrace the same path through the stack.
func TestCycleRoundTrip(t *testing.T) {
	test.NewTempApp(t)

	m := NewMultipleWindows()
	m.CreateRenderer()
	for _, title := range []string{"a", "b", "c"} {
		m.Add(NewInnerWindow(title, widget.NewLabel(title)))
	}

	order := func() string {
		var s string
		for _, w := range m.windows {
			s += w.Title()
		}
		return s
	}

	start := order()
	for range m.windows {
		m.Cycle()
	}
	if got := order(); got != start {
		t.Fatalf("full forward cycle: got %q, want %q", got, start)
	}

	m.Cycle()
	mid := order()
	m.CycleBack()
	if got := order(); got != start {
		t.Fatalf("CycleBack did not undo Cycle: %q -> %q -> %q", start, mid, got)
	}
}

// Ctrl+W must close the window that has focus, i.e. the top of the stack.
func TestCloseActive(t *testing.T) {
	test.NewTempApp(t)

	m := NewMultipleWindows()
	m.CreateRenderer()
	for _, title := range []string{"a", "b", "c"} {
		m.Add(NewInnerWindow(title, widget.NewLabel(title)))
	}

	m.Raise(m.windows[0]) // "a" to the top
	m.CloseActive()
	if len(m.windows) != 2 || m.HasWindow("a") != nil {
		t.Fatalf("CloseActive did not close the raised window, left %d", len(m.windows))
	}
	if top := m.windows[len(m.windows)-1]; !top.active {
		t.Fatalf("window %q left unfocused after the close", top.Title())
	}

	m.CloseActive()
	m.CloseActive()
	m.CloseActive() // must not panic on an empty stack
}
