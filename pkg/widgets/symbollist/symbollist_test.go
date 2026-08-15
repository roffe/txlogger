package symbollist

import (
	"testing"

	"fyne.io/fyne/v2/test"
	symbol "github.com/roffe/ecusymbol"
)

// TestUpdateBars checks that toggling the value bars off clears them and that
// toggling them back on restores them from the values already seen, without
// waiting for the next sample.
func TestUpdateBars(t *testing.T) {
	test.NewApp()
	w := New(&Config{Symbols: []*symbol.Symbol{{Name: "Rpm", Correctionfactor: 1}}})
	w.UpdateBars(true)

	w.SetValue("Rpm", 1000) // sets the max
	w.SetValue("Rpm", 500)  // half way up the range
	if got := w.entryMap["Rpm"].valueBarFactor; got != 0.5 {
		t.Fatalf("bar factor = %v, want 0.5", got)
	}

	w.UpdateBars(false)
	if got := w.entryMap["Rpm"].valueBarFactor; got != 0 {
		t.Fatalf("bar factor after disable = %v, want 0", got)
	}

	w.UpdateBars(true)
	if got := w.entryMap["Rpm"].valueBarFactor; got != 0.5 {
		t.Fatalf("bar factor after re-enable = %v, want 0.5", got)
	}
}
