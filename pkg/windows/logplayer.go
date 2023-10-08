package windows

import (
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type Op int

const (
	OpTogglePlayback Op = iota
	OpSeek
	OpPrev
	OpNext
	OpPlaybackSpeed
	OpExit
)

type controlMsg struct {
	Op   Op
	Pos  int
	Rate float64
}

/*
func V(width, height int32) (mjpeg.AviWriter, error) {
	filename := fmt.Sprintf("capture-%s.avi", time.Now().Format("2006-01-02-15-04-05"))
	return mjpeg.New(filename, width, height, 24)
}
*/

type slider struct {
	widget.Slider
	typedKey func(key *fyne.KeyEvent)
}

func (s *slider) TypedKey(key *fyne.KeyEvent) {
	if s.typedKey != nil {
		s.typedKey(key)
	}
}

func NewLogPlayer(a fyne.App, filename string, onClose func()) fyne.Window {
	w := a.NewWindow("LogPlayer " + filename)
	w.Resize(fyne.NewSize(1024, 600))

	controlChan := make(chan *controlMsg, 10)

	db := NewDashboard(w, true, nil, onClose)

	w.SetCloseIntercept(func() {
		controlChan <- &controlMsg{Op: OpExit}
		close(db.metricsChan)
		w.Close()
	})

	db.SetValue("CEL", 0)
	db.SetValue("CRUISE", 0)
	db.SetValue("LIMP", 0)

	logz := (logfile.Logfile)(&logfile.TxLogfile{})
	slider := &slider{}
	slider.Step = 1
	slider.Orientation = widget.Horizontal
	slider.ExtendBaseWidget(slider)

	posWidget := widget.NewLabel("")
	currLine := binding.NewFloat()

	currLine.AddListener(binding.NewDataListener(func() {
		val, err := currLine.Get()
		if err != nil {
			log.Println(err)
			return
		}
		slider.Value = val
		slider.Refresh()
		currPct := val / float64(logz.Len()) * 100
		posWidget.SetText(fmt.Sprintf("%.01f%%", currPct))
	}))

	slider.OnChanged = func(pos float64) {
		controlChan <- &controlMsg{Op: OpSeek, Pos: int(pos)}
		currPct := pos / float64(logz.Len()) * 100
		posWidget.SetText(fmt.Sprintf("%.01f%%", currPct))
	}

	playing := false
	toggleBtn := &widget.Button{
		Text: "",
		Icon: theme.MediaPlayIcon(),
	}
	toggleBtn.OnTapped = func() {
		controlChan <- &controlMsg{Op: OpTogglePlayback}
		if playing {
			toggleBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			toggleBtn.SetIcon(theme.MediaPauseIcon())
		}
		playing = !playing
	}

	prevBtn := widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		controlChan <- &controlMsg{Op: OpPrev}
	})

	nextBtn := widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		controlChan <- &controlMsg{Op: OpNext}
	})

	restartBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		controlChan <- &controlMsg{Op: OpSeek, Pos: 0}
	})

	sel := widget.NewSelect([]string{"0.1x", "0.2x", "0.5x", "1x", "2x", "4x", "8x", "16x"}, func(s string) {
		switch s {
		case "0.1x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 10}
		case "0.2x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 5}
		case "0.5x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 2}
		case "1x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 1}
		case "2x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.5}
		case "4x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.25}
		case "8x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.125}
		case "16x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.0625}
		}
	})

	sel.Selected = "1x"

	main := container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			container.NewGridWithColumns(4,
				prevBtn,
				toggleBtn,
				restartBtn,
				nextBtn,
			),
			widgets.FixedWidth(75, sel),
			container.NewBorder(nil, nil, nil, posWidget, slider),
		),
		nil,
		nil,
		nil,
		db.Content(),
	)

	handler := keyHandler(w, controlChan, slider, toggleBtn, sel)

	slider.typedKey = handler
	w.Canvas().SetOnTypedKey(handler)
	w.SetContent(main)
	w.Show()

	go func() {
		var err error
		start := time.Now()
		logz, err = logfile.NewFromTxLogfile(filename)
		if err != nil {
			log.Println(err)
			return
		}
		log.Printf("Parsed %d records in %s", logz.Len(), time.Since(start))
		slider.Max = float64(logz.Len())
		posWidget.SetText("0.0%")
		slider.Refresh()
		db.PlayLog(currLine, logz, controlChan, w)
	}()

	return w
}

func keyHandler(w fyne.Window, controlChan chan *controlMsg, slider *slider, tb *widget.Button, sel *widget.Select) func(ev *fyne.KeyEvent) {
	return func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyF12:
			capture.Screenshot(w.Canvas())
		case fyne.KeyPlus:
			sel.SetSelectedIndex(sel.SelectedIndex() + 1)

		case fyne.KeyMinus:
			sel.SetSelectedIndex(sel.SelectedIndex() - 1)
		case fyne.KeyEnter:
			sel.SetSelected("1x")
		case fyne.KeyReturn, fyne.KeyHome:
			controlChan <- &controlMsg{Op: OpSeek, Pos: 0}
		case fyne.KeyPageUp:
			pos := int(slider.Value) + 100
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyUp:
			pos := int(slider.Value) + 12
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyDown:
			pos := int(slider.Value) - 12
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyPageDown:
			pos := int(slider.Value) - 100
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyLeft:
			controlChan <- &controlMsg{Op: OpPrev}
		case fyne.KeyRight:
			controlChan <- &controlMsg{Op: OpNext}
		case fyne.KeySpace:
			//controlChan <- &controlMsg{Op: OpToggle}
			tb.Tapped(&fyne.PointEvent{
				Position:         fyne.NewPos(0, 0),
				AbsolutePosition: fyne.NewPos(0, 0),
			})
		}
	}
}

const TIME_FORMAT = "02-01-2006 15:04:05.999"
