package t5cli

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestHistoryEntry(t *testing.T) {
	e := newHistoryEntry()
	win := test.NewWindow(e)
	defer win.Close()

	e.record("one")
	e.record("one") // immediate repeat: skipped
	e.record("")    // blank: skipped
	e.record("two")
	if len(e.history) != 2 {
		t.Fatalf("history = %v, want [one two]", e.history)
	}

	up := &fyne.KeyEvent{Name: fyne.KeyUp}
	down := &fyne.KeyEvent{Name: fyne.KeyDown}

	e.TypedKey(up)
	if e.Text != "two" {
		t.Fatalf("after up: %q, want two", e.Text)
	}
	e.TypedKey(up)
	e.TypedKey(up) // at oldest: stays
	if e.Text != "one" {
		t.Fatalf("after up x3: %q, want one", e.Text)
	}
	e.TypedKey(down)
	e.TypedKey(down) // past newest: fresh empty line
	if e.Text != "" {
		t.Fatalf("after down past end: %q, want empty", e.Text)
	}

	// only the last historySize commands are kept
	for i := range historySize * 2 {
		e.record(fmt.Sprintf("cmd%d", i))
	}
	if len(e.history) != historySize {
		t.Fatalf("history len = %d, want %d", len(e.history), historySize)
	}
	e.TypedKey(up)
	if want := fmt.Sprintf("cmd%d", historySize*2-1); e.Text != want {
		t.Fatalf("after up: %q, want %s", e.Text, want)
	}
}
