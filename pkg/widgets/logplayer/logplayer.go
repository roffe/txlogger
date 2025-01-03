package logplayer

import (
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
)

type playbackState int

const (
	stateStopped playbackState = iota
	statePlaying
	statePaused
)

type controlMsg struct {
	Op   Op
	Pos  int
	Rate float64
}

var _ fyne.Widget = (*Logplayer)(nil)
var _ fyne.Focusable = (*Logplayer)(nil)
var _ fyne.Tappable = (*Logplayer)(nil)
var _ desktop.Mouseable = (*Logplayer)(nil)

type Logplayer struct {
	widget.BaseWidget

	minsize     fyne.Size
	controlChan chan *controlMsg
	playChan    chan struct{} // New channel to signal playback state changes
	pauseChan   chan struct{} // New channel to signal pause

	container *fyne.Container
	objs      *logplayerObjects

	state playbackState

	closeOnce sync.Once

	logFile  logfile.Logfile
	playOnce sync.Once

	OnMouseDown func()

	focused bool
}

type logplayerObjects struct {
	plotter           *plotter.Plotter
	restartBtn        *widget.Button
	rewindBtn         *widget.Button
	playbackToggleBtn *widget.Button
	forwardBtn        *widget.Button
	positionSlider    *widget.Slider
	timeLabel         *widget.Label
	speedSelect       *widget.Select
}

func New(logFile logfile.Logfile) *Logplayer {
	lp := &Logplayer{
		container:   container.NewWithoutLayout(),
		minsize:     fyne.NewSize(150, 50),
		controlChan: make(chan *controlMsg, 10),
		playChan:    make(chan struct{}, 1),
		pauseChan:   make(chan struct{}, 1),
		state:       stateStopped,
		objs: &logplayerObjects{
			positionSlider: widget.NewSlider(0, 100),
		},
		logFile: logFile,
	}
	lp.ExtendBaseWidget(lp)

	lp.render()

	return lp
}

func (l *Logplayer) Close() {
	l.closeOnce.Do(func() {
		close(l.controlChan)
	})
}

func (l *Logplayer) FocusGained() {
	l.focused = true
}

func (l *Logplayer) FocusLost() {
	l.focused = false
}

func (l *Logplayer) Focused() bool {
	return l.focused
}

func (l *Logplayer) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyF12:
		c := fyne.CurrentApp().Driver().CanvasForObject(l)
		capture.Screenshot(c)
	case fyne.KeyPlus:
		l.objs.speedSelect.SetSelectedIndex(l.objs.speedSelect.SelectedIndex() + 1)
	case fyne.KeyMinus:
		l.objs.speedSelect.SetSelectedIndex(l.objs.speedSelect.SelectedIndex() - 1)
	case fyne.KeyEnter:
		l.objs.speedSelect.SetSelected("1x")
	case fyne.KeyReturn, fyne.KeyHome:
		l.controlChan <- &controlMsg{Op: OpSeek, Pos: 0}
	case fyne.KeyPageUp:
		pos := int(l.objs.positionSlider.Value) + 100
		if pos < 0 {
			pos = 0
		}
		l.controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
	case fyne.KeyUp:
		pos := int(l.objs.positionSlider.Value) + 12
		if pos < 0 {
			pos = 0
		}
		l.controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
	case fyne.KeyDown:
		pos := int(l.objs.positionSlider.Value) - 12
		if pos < 0 {
			pos = 0
		}
		l.controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
	case fyne.KeyPageDown:
		pos := int(l.objs.positionSlider.Value) - 100
		if pos < 0 {
			pos = 0
		}
		l.controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
	case fyne.KeyLeft:
		l.controlChan <- &controlMsg{Op: OpPrev}
	case fyne.KeyRight:
		l.controlChan <- &controlMsg{Op: OpNext}
	case fyne.KeySpace:
		l.objs.playbackToggleBtn.OnTapped()
	}
}

func (l *Logplayer) TypedRune(_ rune) {
}

func (l *Logplayer) control(op *controlMsg) {
	select {
	case l.controlChan <- op:
	default:
		fyne.LogError("Logplayer control channel full", nil)
	}
}

func (l *Logplayer) render() {
	l.objs.positionSlider.OnChanged = func(pos float64) {
		l.control(&controlMsg{Op: OpSeek, Pos: int(pos)})
	}
	l.objs.positionSlider.Step = 1
	l.objs.positionSlider.Max = float64(l.logFile.Len())
	l.objs.positionSlider.Value = 0

	l.objs.speedSelect = widget.NewSelect([]string{"0.1x", "0.2x", "0.5x", "1x", "2x", "4x", "8x", "16x"}, func(s string) {
		switch s {
		case "0.1x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 10})
		case "0.2x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 5})
		case "0.5x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 2})
		case "1x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 1})
		case "2x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.5})
		case "4x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.25})
		case "8x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.125})
		case "16x":
			l.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.0625})
		}
	})
	l.objs.speedSelect.Selected = "1x"

	l.objs.timeLabel = widget.NewLabel("")

	l.objs.restartBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		l.control(&controlMsg{Op: OpSeek, Pos: 0})
	})

	l.objs.rewindBtn = widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		l.control(&controlMsg{Op: OpPrev})
	})

	playing := false

	l.objs.playbackToggleBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		if playing {
			l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			l.objs.playbackToggleBtn.SetIcon(theme.MediaPauseIcon())
		}
		playing = !playing
		l.togglePlayback()
	})

	l.objs.forwardBtn = widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		l.control(&controlMsg{Op: OpNext})
	})

	values := make(map[string][]float64)
	order := make([]string, 0)
	first := true
	for {
		if rec := l.logFile.Next(); !rec.EOF {
			for k, v := range rec.Values {
				values[k] = append(values[k], v)
				if first {
					order = append(order, k)
				}
			}
			first = false
		} else {
			break
		}
	}
	l.logFile.Seek(0)

	sort.Strings(order)

	plotterOpts := []plotter.PlotterOpt{
		plotter.WithPlotResolutionFactor(1),
		plotter.WithOrder(order),
	}

	l.objs.plotter = plotter.NewPlotter(
		values,
		plotterOpts...,
	)

	l.objs.plotter.OnDragged = func(event *fyne.DragEvent) {
		pos := float64(int(l.objs.positionSlider.Value) - int(event.Dragged.DX))
		if pos < 0 {
			pos = 0
		}
		if pos > l.objs.positionSlider.Max {
			pos = l.objs.positionSlider.Max
		}
		l.control(&controlMsg{Op: OpSeek, Pos: int(pos)})
	}

	//l.objs.plotter.OnTapped = func(event *fyne.PointEvent) {
	//	log.Println("Tapped")
	//	fyne.CurrentApp().Driver().CanvasForObject(l).SetOnTypedKey(l.TypedKey)
	//}
}

func (l *Logplayer) CreateRenderer() fyne.WidgetRenderer {
	l.container = container.NewBorder(
		nil,
		container.NewBorder(
			nil,
			nil,
			container.NewGridWithColumns(4,
				l.objs.rewindBtn,
				l.objs.playbackToggleBtn,
				l.objs.forwardBtn,
				l.objs.restartBtn,
			),
			nil,
			container.NewBorder(
				nil,
				nil,
				nil,
				container.NewHBox(
					layout.NewFixedWidth(85, l.objs.timeLabel),
					layout.NewFixedWidth(75, l.objs.speedSelect),
				),
				l.objs.positionSlider,
			),
		),
		nil,
		nil,
		l.objs.plotter,
	)

	l.playOnce.Do(func() {
		go l.playLog()
	})

	return &LogplayerRenderer{
		l: l,
	}
}

type LogplayerRenderer struct {
	l *Logplayer
}

func (lr *LogplayerRenderer) Layout(space fyne.Size) {
	lr.l.container.Resize(space)
}

func (lr *LogplayerRenderer) MinSize() fyne.Size {
	return lr.l.container.MinSize()
}

func (lr *LogplayerRenderer) Refresh() {
}

func (lr *LogplayerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{lr.l.container}
}

func (lr *LogplayerRenderer) Destroy() {
	lr.l.Close()
}

type Op int

const (
	OpTogglePlayback Op = iota
	OpSeek
	OpPrev
	OpNext
	OpPlaybackSpeed
)

func (l *Logplayer) togglePlayback() {
	switch l.state {
	case stateStopped, statePaused:
		l.objs.playbackToggleBtn.SetIcon(theme.MediaPauseIcon())
		l.state = statePlaying
		select {
		case l.playChan <- struct{}{}: // Signal to start/resume playback
		default:
		}
	case statePlaying:
		l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
		l.state = statePaused
		select {
		case l.pauseChan <- struct{}{}: // Signal to pause playback
		default:
		}
	}
}

func (l *Logplayer) playLog() {
	speedMultiplier := 1.0
	logLen := l.logFile.Len() - 1
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case op := <-l.controlChan:
			if op == nil {
				return
			}
			switch op.Op {
			case OpPlaybackSpeed:
				speedMultiplier = op.Rate
			case OpSeek:
				l.logFile.Seek(op.Pos)
				if rec := l.logFile.Next(); !rec.EOF {
					for k, v := range rec.Values {
						ebus.Publish(k, v)
					}
					l.objs.positionSlider.Value = float64(op.Pos)
					l.objs.positionSlider.Refresh()
					l.objs.timeLabel.SetText(rec.Time.Format("15:04:05.00"))
					l.objs.plotter.Seek(op.Pos)
					// Seek back one position since we just read the record
					l.logFile.Seek(op.Pos)
				}
				if l.state == statePlaying {
					timer.Reset(0) // Trigger immediate playback
				}
			case OpPrev:
				pos := l.logFile.Pos() - 2
				if pos < 0 {
					pos = 0
				}
				l.logFile.Seek(pos)
				// Update plotter and UI immediately regardless of playback state
				if rec := l.logFile.Next(); !rec.EOF {
					for k, v := range rec.Values {
						ebus.Publish(k, v)
					}
					l.objs.positionSlider.Value = float64(pos)
					l.objs.positionSlider.Refresh()
					l.objs.timeLabel.SetText(rec.Time.Format("15:04:05.00"))
					l.objs.plotter.Seek(pos)
					// Seek back one position since we just read the record
					l.logFile.Seek(pos)
				}
				if l.state == statePlaying {
					timer.Reset(0)
				}
			case OpNext:
				pos := l.logFile.Pos()
				if pos < logLen {
					l.logFile.Seek(pos)
					if rec := l.logFile.Next(); !rec.EOF {
						for k, v := range rec.Values {
							ebus.Publish(k, v)
						}
						l.objs.positionSlider.Value = float64(pos + 1)
						l.objs.positionSlider.Refresh()
						l.objs.timeLabel.SetText(rec.Time.Format("15:04:05.00"))
						l.objs.plotter.Seek(pos + 1)
					}
				}
				if l.state == statePlaying {
					timer.Reset(0)
				}
			}

		case <-l.playChan:
			timer.Reset(0) // Start playback immediately

		case <-l.pauseChan:
			continue // Wait for next control message

		case <-timer.C:
			if l.state != statePlaying {
				continue
			}

			currentPos := l.logFile.Pos()
			if currentPos >= logLen {
				l.state = stateStopped
				l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
				continue
			}

			if rec := l.logFile.Next(); !rec.EOF {
				// Update all values atomically
				for k, v := range rec.Values {
					ebus.Publish(k, v)
				}

				l.objs.positionSlider.Value = float64(currentPos)
				l.objs.positionSlider.Refresh()
				l.objs.timeLabel.SetText(rec.Time.Format("15:04:05.00"))
				l.objs.plotter.Seek(currentPos)

				// Schedule next frame
				nextDelay := time.Duration(float64(rec.DelayTillNext)*speedMultiplier) * time.Millisecond
				timer.Reset(nextDelay)
			} else {
				l.state = stateStopped
				l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
			}
		}
	}
}

type slider struct {
	widget.Slider
	typedKey func(key *fyne.KeyEvent)
}

func NewSlider() *slider {
	s := &slider{}
	s.ExtendBaseWidget(s)
	return s
}

func (s *slider) TypedKey(key *fyne.KeyEvent) {
	if s.typedKey != nil {
		s.typedKey(key)
	}
}
