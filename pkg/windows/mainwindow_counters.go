package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func (mw *MainWindow) createCounters() {
	mw.counters.capturedCounterLabel = &widget.Label{
		Text:      "Cap: 0",
		Alignment: fyne.TextAlignLeading,
	}
	/*
		mw.counters.captureCounter.AddListener(binding.NewDataListener(func() {
			if val, err := mw.counters.captureCounter.Get(); err == nil {
				mw.counters.capturedCounterLabel.SetText(fmt.Sprintf("Cap: %d", val))
				log.Println("Cap: ", val)
			}
		}))
	*/

	mw.counters.errorCounterLabel = &widget.Label{
		Text:      "Err: 0",
		Alignment: fyne.TextAlignLeading,
	}
	/*
		mw.counters.errorCounter.AddListener(binding.NewDataListener(func() {
			if val, err := mw.counters.errorCounter.Get(); err == nil {
				mw.counters.errorCounterLabel.SetText(fmt.Sprintf("Err: %d", val))
				log.Println("Err: ", val)
			}
		}))
	*/

	mw.counters.fpsCounterLabel = &widget.Label{
		Text:      "Fps: 0",
		Alignment: fyne.TextAlignLeading,
	}
	/*
		mw.counters.fpsCounter.AddListener(binding.NewDataListener(func() {
			if val, err := mw.counters.fpsCounter.Get(); err == nil {
				mw.counters.fpsLabel.SetText(fmt.Sprintf("Fps: %d", val))
				log.Println("Fps: ", val)
			}
		}))
	*/
}
