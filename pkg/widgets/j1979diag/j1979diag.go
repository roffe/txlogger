// Package j1979diag is a Fyne widget exposing every SAE J1979 (OBD-II)
// service the Trionic 7 implements (modes $01-$09) through pkg/j1979.
// The CANbus adapter comes from the txlogger settings via the injected
// getAdapter callback; a fresh bus + diagnostic session is opened per
// operation, dtcreader-style.
package j1979diag

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/j1979"
)

var _ fyne.Widget = (*Widget)(nil)

type Widget struct {
	widget.BaseWidget

	getAdapter func() (gocan.Adapter, error)
	log        func(string)
	err        func(error)

	tabs *container.AppTabs

	liveOut, dtcOut, ffOut, testOut, infoOut *widget.Label

	liveBtn *widget.Button
	btns    []*widget.Button

	// liveCancel/opCancel are non-nil while live polling / a one-shot
	// operation runs. Only touched on the fyne main thread (button taps
	// and the goroutines' fyne.Do teardowns).
	liveCancel context.CancelFunc
	opCancel   context.CancelFunc
}

func New(getAdapter func() (gocan.Adapter, error), logFn func(string), errFn func(error)) *Widget {
	w := &Widget{
		getAdapter: getAdapter,
		log:        logFn,
		err:        errFn,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *Widget) render() {
	w.liveOut = newOutput("Read polls the supported mode $01 PIDs once; Live polls continuously.")
	w.dtcOut = newOutput("Read fetches stored (mode $03) and pending (mode $07) trouble codes.")
	w.ffOut = newOutput("Reads the freeze frame captured when a DTC was stored (mode $02).")
	w.testOut = newOutput("O2 sensor tests (mode $05), on-board monitor results (mode $06) and the EVAP purge valve control test (mode $08).")
	w.infoOut = newOutput("VIN, calibration ID and CVN (mode $09).")

	readBtn := w.button("Read", theme.SearchIcon(), func() {
		w.run(30*time.Second, "reading current data", w.readCurrentData)
	})
	w.liveBtn = widget.NewButtonWithIcon("Live", theme.MediaPlayIcon(), w.toggleLive)
	w.btns = append(w.btns, w.liveBtn)

	readDTCBtn := w.button("Read DTCs", theme.SearchIcon(), func() {
		w.run(30*time.Second, "reading DTCs", w.readDTCs)
	})
	clearDTCBtn := w.button("Clear DTCs", theme.DeleteIcon(), func() {
		confirm("Clear diagnostic information",
			"Mode $04 clears DTCs, freeze frames and readiness results, and also\n"+
				"resets the fuel trim adaptation. Do not do this casually on a tuned car.\n\nClear?",
			func() { w.run(20*time.Second, "clearing diagnostic information", w.clearDTCs) })
	})

	ffBtn := w.button("Read freeze frame", theme.SearchIcon(), func() {
		w.run(30*time.Second, "reading freeze frame", w.readFreezeFrame)
	})

	o2Btn := w.button("O2 sensor tests", theme.SearchIcon(), func() {
		w.run(30*time.Second, "reading O2 sensor tests", w.readO2Tests)
	})
	monBtn := w.button("Monitor results", theme.SearchIcon(), func() {
		w.run(30*time.Second, "reading monitor test results", w.readTestResults)
	})
	evapBtn := w.button("EVAP valve test", theme.WarningIcon(), func() {
		confirm("EVAP purge valve test",
			"Mode $08 closes the EVAP purge valve for leak testing.\n"+
				"Engine must be off, ignition on and the vehicle stationary.\n\nStart the test?",
			func() { w.run(20*time.Second, "running EVAP valve test", w.runEvapTest) })
	})

	infoBtn := w.button("Read", theme.SearchIcon(), func() {
		w.run(30*time.Second, "reading vehicle information", w.readVehicleInfo)
	})

	w.tabs = container.NewAppTabs(
		container.NewTabItem("Live data", tab(w.liveOut, readBtn, w.liveBtn)),
		container.NewTabItem("DTCs", tab(w.dtcOut, readDTCBtn, clearDTCBtn)),
		container.NewTabItem("Freeze frame", tab(w.ffOut, ffBtn)),
		container.NewTabItem("Tests", tab(w.testOut, o2Btn, monBtn, evapBtn)),
		container.NewTabItem("Vehicle info", tab(w.infoOut, infoBtn)),
	)
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	w.render()
	return widget.NewSimpleRenderer(w.tabs)
}

func (w *Widget) Enable() {
	for _, b := range w.btns {
		b.Enable()
	}
}

func (w *Widget) Disable() {
	for _, b := range w.btns {
		b.Disable()
	}
}

// Close stops live polling and aborts any in-flight operation, so the
// adapter is released when the inner window is closed.
func (w *Widget) Close() {
	if w.liveCancel != nil {
		w.liveCancel()
	}
	if w.opCancel != nil {
		w.opCancel()
	}
}

// run executes op against a fresh bus + session in the background, with all
// buttons disabled until it finishes.
func (w *Widget) run(timeout time.Duration, what string, op func(context.Context, *j1979.Client) error) {
	w.Disable()
	w.log("J1979: " + what)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	w.opCancel = cancel
	go func() {
		defer fyne.Do(func() {
			w.opCancel = nil
			w.Enable()
		})
		defer cancel()
		err := w.withSession(ctx, op)
		// a cancel means the window was closed; nobody is left to tell
		if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
			w.err(fmt.Errorf("J1979 %s: %w", what, err))
		}
	}()
}

// toggleLive starts or stops continuous mode $01 polling.
func (w *Widget) toggleLive() {
	if w.liveCancel != nil {
		w.liveCancel()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.liveCancel = cancel
	w.Disable()
	w.liveBtn.SetText("Stop")
	w.liveBtn.SetIcon(theme.MediaStopIcon())
	w.liveBtn.Enable()
	w.log("J1979: live data started")
	go func() {
		defer fyne.Do(func() {
			w.liveCancel = nil
			w.liveBtn.SetText("Live")
			w.liveBtn.SetIcon(theme.MediaPlayIcon())
			w.Enable()
		})
		defer cancel()
		err := w.withSession(ctx, w.liveLoop)
		if err != nil && ctx.Err() == nil {
			w.err(fmt.Errorf("J1979 live data: %w", err))
		}
		w.log("J1979: live data stopped")
	}()
}

// withSession opens the adapter from settings, starts a diagnostic session
// and runs op. The session is stopped and the bus closed when op returns.
func (w *Widget) withSession(ctx context.Context, op func(context.Context, *j1979.Client) error) error {
	dev, err := w.getAdapter()
	if err != nil {
		return err
	}
	w.log("Connecting to device " + gocan.AdapterName(dev))

	// Events (incl. the final fatal) stream to the log; a fatal adapter
	// failure also aborts any in-flight call below with that error.
	bus, err := gocan.OpenAdapter(ctx, dev, gocan.WithEventFunc(func(e gocan.Event) {
		w.log(e.String())
	}))
	if err != nil {
		return err
	}
	defer bus.Close()

	cl := j1979.New(bus)
	if err := cl.StartSession(ctx); err != nil {
		// The T7 silently ignores startCommunication while a session is
		// already active (e.g. live polling stopped without a clean
		// stopCommunication and the ~6 s diagnostic timeout has not expired
		// yet). Probe for such a session on the default response channel
		// and reuse it if it answers.
		if terr := cl.TesterPresent(ctx); terr != nil {
			return err
		}
		w.log("J1979: reusing already active diagnostic session")
	}
	defer func() {
		// The bus dies with ctx (live stop / op timeout); the T7 drops the
		// session by itself after ~6 s, so only stop it while we still can.
		if bus.Context().Err() != nil {
			return
		}
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := cl.StopSession(sctx); err != nil {
			w.log("J1979 stop session: " + err.Error())
		}
		time.Sleep(75 * time.Millisecond)
	}()
	return op(ctx, cl)
}

func (w *Widget) button(label string, icon fyne.Resource, tapped func()) *widget.Button {
	b := widget.NewButtonWithIcon(label, icon, tapped)
	w.btns = append(w.btns, b)
	return b
}

// setText updates an output label from an operation goroutine.
func (w *Widget) setText(l *widget.Label, s string) {
	fyne.Do(func() { l.SetText(s) })
}

func newOutput(hint string) *widget.Label {
	l := widget.NewLabel(hint)
	l.Wrapping = fyne.TextWrapWord
	l.TextStyle = fyne.TextStyle{Monospace: true}
	l.Selectable = true
	return l
}

func tab(out *widget.Label, buttons ...fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(container.NewHBox(buttons...), nil, nil, nil, container.NewVScroll(out))
}

func confirm(title, msg string, onConfirm func()) {
	win := fyne.CurrentApp().Driver().AllWindows()[0]
	dialog.ShowConfirm(title, msg, func(ok bool) {
		if ok {
			onConfirm()
		}
	}, win)
}
