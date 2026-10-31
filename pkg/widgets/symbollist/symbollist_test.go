package symbollist

import (
	"strconv"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
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

// TestSetValueConcurrent feeds values from several goroutines, as the ebus
// does, while the UI goroutine toggles the bars, recolours and resizes, then
// checks that every label ends on the last value. Run it with -race.
func TestSetValueConcurrent(t *testing.T) {
	onUI := uitest.Start(t)

	syms := make([]*symbol.Symbol, 20)
	for i := range syms {
		syms[i] = &symbol.Symbol{Name: "race" + strconv.Itoa(i), Correctionfactor: 0.1}
	}
	var (
		w      *Widget
		win    fyne.Window
		labels []*widget.Label
	)
	onUI(func() {
		w = New(&Config{Symbols: syms})
		win = test.NewWindow(w)
		win.Resize(fyne.NewSize(400, 900))
		w.UpdateBars(true)
		for _, s := range syms {
			labels = append(labels, w.entryMap[s.Name].symbolValue)
		}
	})

	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 300 {
				for _, s := range syms {
					w.SetValue(s.Name, float64(g*1000+i))
				}
			}
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
churn:
	for i := 0; ; i++ {
		select {
		case <-done:
			break churn
		default:
		}
		onUI(func() {
			w.UpdateBars(i%2 == 0)
			w.SetColorBlindMode(colors.ColorBlindMode(i % 5))
			w.Refresh()
			win.Resize(fyne.NewSize(float32(300+i%5*40), 900))
		})
	}

	// values no publisher sent, so each label's last update is known
	for i, s := range syms {
		w.SetValue(s.Name, -float64(i)-0.5)
	}
	onUI(func() { // runs after everything already queued
		for i, l := range labels {
			if want := strconv.FormatFloat(-float64(i)-0.5, 'f', 1, 64); l.Text != want {
				t.Errorf("%s shows %q, want %q", syms[i].Name, l.Text, want)
			}
		}
	})
}
