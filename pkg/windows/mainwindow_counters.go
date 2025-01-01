package windows

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

func (mw *MainWindow) createCounters() {
	mw.counters.capturedCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.counters.captureCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.counters.captureCounter.Get(); err == nil {
			mw.counters.capturedCounterLabel.SetText(fmt.Sprintf("Cap: %d", val))
		}
	}))

	mw.counters.errorCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.counters.errorCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.counters.errorCounter.Get(); err == nil {
			mw.counters.errorCounterLabel.SetText(fmt.Sprintf("Err: %d", val))
		}
	}))

	mw.counters.fpsLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.counters.fpsCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.counters.fpsCounter.Get(); err == nil {
			mw.counters.fpsLabel.SetText(fmt.Sprintf("Fps: %d", val))
		}
	}))
}
