package logplayer

import (
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/logfile"
)

type controlMsg struct {
	Op   Op
	Pos  int
	Rate float64
}

var _ fyne.Widget = (*Logplayer)(nil)

type Logplayer struct {
	widget.BaseWidget

	minsize     fyne.Size
	controlChan chan *controlMsg

	container *fyne.Container

	objs *logplayerObjects

	playing bool

	closed    bool
	closeOnce sync.Once

	logFile logfile.Logfile

	playOnce             sync.Once
	ebusTopicPublishTime bool
	ebusTopic            string
}

type logplayerObjects struct {
	restartBtn        *widget.Button
	rewindBtn         *widget.Button
	playbackToggleBtn *widget.Button
	forwardBtn        *widget.Button
	slider            *widget.Slider
	posLabel          *widget.Label
	speedSelect       *widget.Select
}

func New(logFile logfile.Logfile, topic string) *Logplayer {
	lp := &Logplayer{
		container:   container.NewWithoutLayout(),
		minsize:     fyne.NewSize(150, 50),
		controlChan: make(chan *controlMsg, 10),
		objs: &logplayerObjects{
			slider: widget.NewSlider(0, 100),
		},
		logFile:   logFile,
		ebusTopic: topic,
	}
	lp.ExtendBaseWidget(lp)

	if topic != "" {
		lp.ebusTopicPublishTime = true
	}

	lp.objs.slider.OnChanged = func(pos float64) {
		lp.control(&controlMsg{Op: OpSeek, Pos: int(pos)})
		log.Println("Seek pos", pos)
	}
	lp.objs.slider.Step = 1
	lp.objs.slider.Max = float64(logFile.Len())
	lp.objs.slider.Value = 0

	lp.objs.speedSelect = widget.NewSelect([]string{"0.1x", "0.2x", "0.5x", "1x", "2x", "4x", "8x", "16x"}, func(s string) {
		switch s {
		case "0.1x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 10})
		case "0.2x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 5})
		case "0.5x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 2})
		case "1x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 1})
		case "2x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.5})
		case "4x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.25})
		case "8x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.125})
		case "16x":
			lp.control(&controlMsg{Op: OpPlaybackSpeed, Rate: 0.0625})
		}
	})
	lp.objs.speedSelect.Selected = "1x"

	lp.objs.posLabel = widget.NewLabel("")

	lp.render()

	return lp
}

func (l *Logplayer) Close() {
	l.closed = true
	l.closeOnce.Do(func() {
		close(l.controlChan)
	})
}

func (l *Logplayer) control(op *controlMsg) {
	select {
	case l.controlChan <- op:
	default:
		fyne.LogError("Logplayer control channel full", nil)
	}
}

func (l *Logplayer) render() {
	l.objs.restartBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		l.control(&controlMsg{Op: OpSeek, Pos: 0})
	})

	l.objs.rewindBtn = widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		l.control(&controlMsg{Op: OpPrev})
	})

	l.objs.playbackToggleBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		if l.playing {
			l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			l.objs.playbackToggleBtn.SetIcon(theme.MediaPauseIcon())
		}
		l.playing = !l.playing
		l.playOnce.Do(func() {
			go l.playLog()
		})
	})

	l.objs.forwardBtn = widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		l.control(&controlMsg{Op: OpNext})
	})
}

func (l *Logplayer) CreateRenderer() fyne.WidgetRenderer {
	l.container = container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			container.NewGridWithColumns(4,
				l.objs.rewindBtn,
				l.objs.playbackToggleBtn,
				l.objs.forwardBtn,
				l.objs.restartBtn,
			),
			layout.NewFixedWidth(75, l.objs.speedSelect),
			layout.NewFixedWidth(80, l.objs.posLabel),
		),
		nil,
		nil,
		nil,
		l.objs.slider,
	)
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
	OpExit
)

func (l *Logplayer) playLog() {
	var playonce bool
	speedMultiplier := 1.0

	// Control channel handler
	go func() {
		for op := range l.controlChan {
			switch op.Op {
			case OpPlaybackSpeed:
				speedMultiplier = op.Rate
			case OpSeek:
				l.logFile.Seek(op.Pos)
				playonce = true
			case OpPrev:
				pos := l.logFile.Pos() - 2
				if pos < 0 {
					pos = 0
				}
				playonce = true
				l.logFile.Seek(pos)
			case OpNext:
				playonce = true
			case OpExit:
				l.closed = true
				return
			}
		}
	}()

	// Playback loop with precise timing
	targetFrameTime := time.Now()

	for !l.closed {
		if l.logFile.Pos() >= l.logFile.Len() || (!l.playing && !playonce) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		if rec := l.logFile.Next(); !rec.EOF {
			// Calculate when this frame should be displayed
			delayTilNext := time.Duration(float64(rec.DelayTillNext)*speedMultiplier) * time.Millisecond

			// Wait until it's time to display this frame
			now := time.Now()
			if now.Before(targetFrameTime) {
				time.Sleep(targetFrameTime.Sub(now))
			}

			// Update target time for next frame
			targetFrameTime = targetFrameTime.Add(delayTilNext)

			// Update all values atomically
			for k, v := range rec.Values {
				ebus.Publish(k, v)
			}

			currentPos := float64(l.logFile.Pos())

			l.objs.slider.Value = currentPos
			l.objs.slider.Refresh()
			//l.objs.posLabel.SetText(strconv.Itoa(l.logFile.Pos()) + "/" + strconv.Itoa(l.logFile.Len()))

			l.objs.posLabel.SetText(rec.Time.Format("15:04:05.00"))

			if l.ebusTopicPublishTime {
				ebus.Publish(l.ebusTopic, currentPos)
			}

			// Reset target time if we're getting too far behind
			if time.Since(targetFrameTime) > 100*time.Millisecond {
				targetFrameTime = time.Now()
			}
		}

		if playonce {
			playonce = false
		}
	}
}
