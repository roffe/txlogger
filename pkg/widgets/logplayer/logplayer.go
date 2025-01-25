package logplayer

import (
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/eventbus"
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

	cfg *Config

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
	closed  bool
}

type logplayerObjects struct {
	plotter           *plotter.Plotter
	restartBtn        *widget.Button
	rewindBtn         *widget.Button
	playbackToggleBtn *widget.Button
	forwardBtn        *widget.Button
	positionSlider    *slider
	timeLabel         *widget.Label
	speedSelect       *widget.Select
}

type Config struct {
	EBus       *eventbus.Controller
	Logfile    logfile.Logfile
	TimeSetter func(time.Time)
}

func New(cfg *Config) *Logplayer {
	lp := &Logplayer{
		cfg:         cfg,
		container:   container.NewWithoutLayout(),
		minsize:     fyne.Size{Width: 150, Height: 50},
		controlChan: make(chan *controlMsg, 10),
		playChan:    make(chan struct{}, 2),
		pauseChan:   make(chan struct{}, 2),
		state:       stateStopped,
		objs: &logplayerObjects{
			positionSlider: NewSlider(),
		},
		logFile: cfg.Logfile,
	}
	lp.ExtendBaseWidget(lp)

	lp.objs.positionSlider.typedKey = lp.TypedKey

	lp.render()

	return lp
}

func (l *Logplayer) Close() {
	l.closeOnce.Do(func() {
		close(l.controlChan)
		l.closed = true
		l.logFile.Close()
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
	if l.closed {
		return
	}
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
		l.control(&controlMsg{Op: OpSeek, Pos: 0})
	case fyne.KeyPageUp:
		pos := int(l.objs.positionSlider.Value) + 100
		if pos < 0 {
			pos = 0
		}
		l.control(&controlMsg{Op: OpSeek, Pos: pos})
	case fyne.KeyUp:
		pos := int(l.objs.positionSlider.Value) + 12
		if pos < 0 {
			pos = 0
		}
		l.control(&controlMsg{Op: OpSeek, Pos: pos})
	case fyne.KeyDown:
		pos := int(l.objs.positionSlider.Value) - 12
		if pos < 0 {
			pos = 0
		}
		l.control(&controlMsg{Op: OpSeek, Pos: pos})
	case fyne.KeyPageDown:
		pos := int(l.objs.positionSlider.Value) - 100
		if pos < 0 {
			pos = 0
		}
		l.control(&controlMsg{Op: OpSeek, Pos: pos})
	case fyne.KeyLeft:
		l.control(&controlMsg{Op: OpPrev})
	case fyne.KeyRight:
		l.control(&controlMsg{Op: OpNext})
	case fyne.KeySpace:
		l.objs.playbackToggleBtn.OnTapped()
	}
}

func (l *Logplayer) TypedRune(_ rune) {
}

func (l *Logplayer) control(op *controlMsg) {
	select {
	case l.controlChan <- op:
		//		log.Println("control", op.Op, op.Pos)
	default:
		fyne.LogError("Logplayer control channel full", nil)
	}
}

func (l *Logplayer) render() {
	l.objs.positionSlider.OnChanged = func(pos float64) {
		l.control(&controlMsg{Op: OpSeek, Pos: int(pos)})
	}
	l.objs.positionSlider.Step = 1
	l.objs.positionSlider.Min = 0
	l.objs.positionSlider.Max = float64(l.logFile.Len() - 1)
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

	l.objs.timeLabel = widget.NewLabel(l.logFile.Start().Format("15:04:05.00"))

	l.objs.restartBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		l.control(&controlMsg{Op: OpSeek, Pos: 0})
	})

	l.objs.rewindBtn = widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		l.control(&controlMsg{Op: OpPrev})
	})

	l.objs.playbackToggleBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		l.togglePlayback()
	})

	l.objs.forwardBtn = widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		l.control(&controlMsg{Op: OpNext})
	})

	values := make(map[string][]float64)
	for {
		if rec := l.logFile.Next(); !rec.EOF {
			for k, v := range rec.Values {
				values[k] = append(values[k], v)
			}
		} else {
			break
		}
	}
	l.logFile.Seek(-1)

	plotterOpts := []plotter.PlotterOpt{
		plotter.WithPlotResolutionFactor(1),
		//plotter.WithOrder(order),
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
	log.Println("LogplayerRenderer.Destroy")
	lr.l.Close()
}

type Op int

func (o Op) String() string {
	switch o {
	case OpTogglePlayback:
		return "TogglePlayback"
	case OpSeek:
		return "Seek"
	case OpPrev:
		return "Prev"
	case OpNext:
		return "Next"
	case OpPlaybackSpeed:
		return "PlaybackSpeed"
	}
	return "Unknown"
}

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
		select {
		case l.playChan <- struct{}{}: // Signal to start/resume playback
		default:
		}
	case statePlaying:
		l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
		select {
		case l.pauseChan <- struct{}{}: // Signal to pause playback
		default:
		}
	}
}

func (l *Logplayer) playLog() {
	speedMultiplier := 1.0
	timer := time.NewTimer(0)
	defer timer.Stop()

	var nextDelay time.Duration

	timeSetter := func(t time.Time) {
		timeText := t.Format("15:04:05.00")
		fyne.Do(func() {
			l.objs.timeLabel.SetText(timeText)
		})
	}

	for {
		select {
		case op, ok := <-l.controlChan:
			if !ok {
				return
			}
			switch op.Op {
			case OpPlaybackSpeed:
				speedMultiplier = op.Rate
			case OpSeek:
				// log.Println("Seeking to", op.Pos)
				l.logFile.Seek(op.Pos)
				if rec := l.logFile.Get(); !rec.EOF {
					if f := l.cfg.TimeSetter; f != nil {
						f(rec.Time)
					}
					if l.state == statePlaying {
						timer.Reset(10 * time.Millisecond)
					} else {
						for k, v := range rec.Values {
							if err := l.cfg.EBus.Publish(k, v); err != nil {
								log.Println("Error publishing to eventbus:", err)
							}
						}
						timeSetter(rec.Time)
						timer.Stop()
					}
					l.objs.plotter.Seek(op.Pos)
				}
			case OpPrev:
				if rec := l.logFile.Prev(); !rec.EOF {
					pos := l.logFile.Pos()
					l.objs.positionSlider.Value = float64(pos)
					timeSetter(rec.Time)
					fyne.Do(func() {
						l.objs.positionSlider.Refresh()
					})

					if l.state == statePlaying {
						timer.Reset(0)
					} else {
						for k, v := range rec.Values {
							if err := l.cfg.EBus.Publish(k, v); err != nil {
								log.Println("Error publishing to eventbus:", err)
							}
						}
					}

					l.objs.plotter.Seek(pos)
					if f := l.cfg.TimeSetter; f != nil {
						f(rec.Time)
					}
				}

			case OpNext:
				if rec := l.logFile.Next(); !rec.EOF {
					pos := l.logFile.Pos()
					l.objs.positionSlider.Value = float64(pos)
					timeSetter(rec.Time)
					fyne.Do(func() {

					})
					if l.state == statePlaying {
						timer.Reset(0)
					} else {
						for k, v := range rec.Values {
							if err := l.cfg.EBus.Publish(k, v); err != nil {
								log.Println("Error publishing to eventbus:", err)
							}
						}
					}
					if f := l.cfg.TimeSetter; f != nil {
						f(rec.Time)
					}
					l.objs.plotter.Seek(pos)
				}

			}

		case <-l.playChan:
			l.state = statePlaying
			timer.Reset(0) // Start playback immediately

		case <-l.pauseChan:
			l.state = statePaused
			timer.Stop()
			continue

		case <-timer.C:
			if l.state != statePlaying {
				timer.Stop()
				continue
			}
			if rec := l.logFile.Next(); !rec.EOF {
				currentPos := l.logFile.Pos()
				nextDelay = time.Duration(float64(rec.DelayTillNext)*speedMultiplier) * time.Millisecond
				timer.Reset(nextDelay)

				l.objs.positionSlider.Value = float64(currentPos)
				timeText := rec.Time.Format("15:04:05.00")
				fyne.Do(func() {
					l.objs.positionSlider.Refresh()
					l.objs.timeLabel.SetText(timeText)
				})
				for k, v := range rec.Values {
					if err := l.cfg.EBus.Publish(k, v); err != nil {
						log.Println("Error publishing to eventbus:", err)
					}
				}
				if f := l.cfg.TimeSetter; f != nil {
					f(rec.Time)
				}
				l.objs.plotter.Seek(currentPos)
			} else {
				l.state = stateStopped
				fyne.Do(func() {
					l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
				})
				timer.Reset(100 * time.Millisecond)
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
