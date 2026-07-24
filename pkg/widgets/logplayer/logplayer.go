package logplayer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/bus"
	"github.com/roffe/txlogger/pkg/capture"
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

var (
	_ fyne.Widget       = (*Logplayer)(nil)
	_ fyne.Focusable    = (*Logplayer)(nil)
	_ fyne.Tappable     = (*Logplayer)(nil)
	_ desktop.Mouseable = (*Logplayer)(nil)
)

type Logplayer struct {
	widget.BaseWidget

	cfg *Config

	minsize     fyne.Size
	controlChan chan *controlMsg

	container *fyne.Container
	objs      *logplayerObjects

	state playbackState

	closeOnce sync.Once

	logFile  logfile.Logfile
	playOnce sync.Once

	// uiUpdatePending coalesces per-frame slider/time updates: while a refresh
	// is queued, further frames only update slider.Value and the pending
	// refresh paints the latest position (same pattern as plotter.Seek).
	uiUpdatePending atomic.Bool

	OnMouseDown func()

	// selStart and selEnd mark the in/out points of the export selection.
	// A value of -1 means the point has not been set yet.
	selStart int
	selEnd   int

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
	setInBtn          *widget.Button
	setOutBtn         *widget.Button
	exportBtn         *widget.Button
	selectionLabel    *widget.Label
}

type Config struct {
	EBus            *bus.Controller[string, float64]
	Logfile         logfile.Logfile
	TimeSetter      func(time.Time)
	PlotterRenderer plotter.PlotBackend
	// OnExport, when set, is called with the records of the selected range when
	// the user exports a selection. When nil the selection controls are hidden.
	OnExport func(records []logfile.Record)
}

func New(cfg *Config) *Logplayer {
	lp := &Logplayer{
		cfg:         cfg,
		container:   container.NewWithoutLayout(),
		minsize:     fyne.Size{Width: 150, Height: 50},
		controlChan: make(chan *controlMsg, 10),
		state:       stateStopped,
		objs: &logplayerObjects{
			positionSlider: NewSlider(),
		},
		logFile:  cfg.Logfile,
		selStart: -1,
		selEnd:   -1,
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

func (l *Logplayer) TypedRune(r rune) {
	if l.closed {
		return
	}
	switch r {
	case 'i', 'I':
		l.setSelectionStart()
	case 'o', 'O':
		l.setSelectionEnd()
	}
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
	l.objs.timeLabel.TextStyle.Monospace = true

	l.objs.restartBtn = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), func() {
		l.control(&controlMsg{Op: OpSeek, Pos: 0})
	})
	l.objs.restartBtn.Importance = widget.LowImportance

	l.objs.rewindBtn = widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		l.control(&controlMsg{Op: OpPrev})
	})
	l.objs.rewindBtn.Importance = widget.LowImportance

	l.objs.playbackToggleBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		l.togglePlayback()
	})
	l.objs.playbackToggleBtn.Importance = widget.HighImportance

	l.objs.forwardBtn = widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		l.control(&controlMsg{Op: OpNext})
	})
	l.objs.forwardBtn.Importance = widget.LowImportance

	l.objs.setInBtn = widget.NewButton("In", l.setSelectionStart)
	l.objs.setInBtn.Importance = widget.LowImportance
	l.objs.setOutBtn = widget.NewButton("Out", l.setSelectionEnd)
	l.objs.setOutBtn.Importance = widget.LowImportance
	l.objs.exportBtn = widget.NewButtonWithIcon("Save selection", theme.DocumentSaveIcon(), l.exportSelection)
	l.objs.exportBtn.Disable()
	l.objs.selectionLabel = widget.NewLabel("")
	l.objs.selectionLabel.TextStyle.Monospace = true
	l.updateSelectionLabel()

	n := l.logFile.Len()
	values := make(map[string][]float64)
	for {
		if rec := l.logFile.Next(); !rec.EOF {
			for k, v := range rec.Values {
				if k == "Pgm_status" {
					continue
				}
				s, ok := values[k]
				if !ok {
					s = make([]float64, 0, n) // ponytail: cap once, kills append regrowth churn
				}
				values[k] = append(s, v)
			}
		} else {
			break
		}
	}
	l.logFile.Seek(-1)

	l.objs.plotter = plotter.NewPlotter(
		values,
		plotter.WithPlotResolutionFactor(1),
		plotter.WithOnDragged(func(event *fyne.DragEvent) {
			pos := l.objs.positionSlider.Value - l.objs.plotter.DragFrameDelta(event.Dragged.DX)
			if pos < 0 {
				pos = 0
			} else if pos > l.objs.positionSlider.Max {
				pos = l.objs.positionSlider.Max
			}
			l.objs.positionSlider.Value = pos
			l.objs.positionSlider.Refresh()
			l.control(&controlMsg{Op: OpSeek, Pos: int(pos)})
		}),
		plotter.WithRenderer(l.cfg.PlotterRenderer),
	)
}

func (l *Logplayer) CreateRenderer() fyne.WidgetRenderer {
	controls := container.NewBorder(
		nil,
		nil,
		container.NewGridWithColumns(4,
			l.objs.restartBtn,
			l.objs.rewindBtn,
			l.objs.playbackToggleBtn,
			l.objs.forwardBtn,
		),
		nil,
		container.NewBorder(
			nil,
			nil,
			nil,
			container.NewHBox(
				layout.NewFixedWidth(100, l.objs.timeLabel),
				layout.NewFixedWidth(75, l.objs.speedSelect),
			),
			l.objs.positionSlider,
		),
	)

	var bottom fyne.CanvasObject = controls
	if l.cfg.OnExport != nil {
		selection := container.NewBorder(
			nil,
			nil,
			container.NewHBox(
				l.objs.setInBtn,
				l.objs.setOutBtn,
				l.objs.exportBtn,
			),
			nil,
			l.objs.selectionLabel,
		)
		bottom = container.NewVBox(selection)
	}

	l.container = container.NewBorder(
		bottom,
		controls,
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
	l       *Logplayer
	objects []fyne.CanvasObject
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
	if lr.objects == nil {
		lr.objects = []fyne.CanvasObject{lr.l.container}
	}
	return lr.objects
}

func (lr *LogplayerRenderer) Destroy() {
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
	OpPlay
	OpPause
)

func (l *Logplayer) togglePlayback() {
	switch l.state {
	case stateStopped, statePaused:
		l.objs.playbackToggleBtn.SetIcon(theme.MediaPauseIcon())
		l.control(&controlMsg{Op: OpPlay})
	case statePlaying:
		l.objs.playbackToggleBtn.SetIcon(theme.MediaPlayIcon())
		l.control(&controlMsg{Op: OpPause})
	}
}

// setSelectionStart marks the in point of the export selection at the current
// playback position.
func (l *Logplayer) setSelectionStart() {
	l.selStart = int(l.objs.positionSlider.Value)
	l.updateSelectionLabel()
}

// setSelectionEnd marks the out point of the export selection at the current
// playback position.
func (l *Logplayer) setSelectionEnd() {
	l.selEnd = int(l.objs.positionSlider.Value)
	l.updateSelectionLabel()
}

// selectionRange returns the normalized [lo, hi] record indices of the current
// selection. An unset in point defaults to the start of the log and an unset
// out point to the end. The bounds are clamped to the log and swapped if the
// out point was set before the in point.
func (l *Logplayer) selectionRange() (int, int) {
	last := l.logFile.Len() - 1
	lo, hi := l.selStart, l.selEnd
	if lo == -1 {
		lo = 0
	}
	if hi == -1 {
		hi = last
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > last {
		hi = last
	}
	return lo, hi
}

func (l *Logplayer) updateSelectionLabel() {
	if l.objs.selectionLabel == nil {
		return
	}
	inTxt, outTxt := "—", "—"
	if l.selStart != -1 {
		inTxt = l.logFile.RecordAt(l.selStart).Time.Format("15:04:05.00")
	}
	if l.selEnd != -1 {
		outTxt = l.logFile.RecordAt(l.selEnd).Time.Format("15:04:05.00")
	}

	if l.selStart == -1 && l.selEnd == -1 {
		l.objs.selectionLabel.SetText("In —  Out —")
		l.objs.exportBtn.Disable()
		return
	}

	lo, hi := l.selectionRange()
	l.objs.selectionLabel.SetText(fmt.Sprintf("In %s  Out %s  (%d samples)", inTxt, outTxt, hi-lo+1))
	l.objs.exportBtn.Enable()
}

// exportSelection collects the records of the current selection and hands them
// to the OnExport callback so they can be written to a new logfile.
func (l *Logplayer) exportSelection() {
	if l.cfg.OnExport == nil || l.logFile.Len() == 0 {
		return
	}
	lo, hi := l.selectionRange()
	records := make([]logfile.Record, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		records = append(records, l.logFile.RecordAt(i))
	}
	l.cfg.OnExport(records)
}

// frameTarget returns the wall-clock instant a record should play at, given an
// anchor (anchorWall pinned to anchorRec) and a speed multiplier where larger
// means slower (1 = real time). Because the target is computed from the record's
// own timestamp relative to the fixed anchor, scheduling jitter on earlier
// frames does not shift it — error stays bounded instead of accumulating.
func frameTarget(anchorWall, anchorRec, recTime time.Time, speed float64) time.Time {
	return anchorWall.Add(time.Duration(float64(recTime.Sub(anchorRec)) * speed))
}

func (l *Logplayer) playLog() {
	speedMultiplier := 1.0
	timer := time.NewTimer(0)
	defer timer.Stop()

	// Absolute scheduling: anchorWall maps to anchorRec (the wall-clock instant
	// the current record's timeline position was pinned). Every frame is
	// scheduled against its record's true timestamp relative to this anchor, so
	// per-frame scheduling jitter cannot accumulate into drift — a late frame
	// fires immediately and re-syncs instead of pushing the whole timeline back.
	var anchorWall, anchorRec time.Time

	// reanchor pins the timeline to the current record at the current instant.
	// Called whenever playback (re)starts or jumps: play, seek, step, speed change.
	reanchor := func() {
		anchorWall = time.Now()
		anchorRec = l.logFile.Get().Time
	}

	// schedule arms the timer for the next record using its real timestamp.
	// time.Until goes negative when we're behind, firing immediately to catch up.
	schedule := func() {
		next := l.logFile.RecordAt(l.logFile.Pos() + 1)
		timer.Reset(time.Until(frameTarget(anchorWall, anchorRec, next.Time, speedMultiplier)))
	}

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
				if l.state == statePlaying {
					reanchor()
					schedule()
				}
			case OpSeek:
				l.logFile.Seek(op.Pos)
				if rec := l.logFile.Get(); !rec.EOF {
					l.objs.positionSlider.Value = float64(op.Pos)

					if f := l.cfg.TimeSetter; f != nil {
						f(rec.Time)
					}
					for k, v := range rec.Values {
						l.cfg.EBus.Publish(k, v)
					}
					timeSetter(rec.Time)
					l.cfg.EBus.Publish("__frame__", float64(rec.Time.UnixMilli()))
					if l.state == statePlaying {
						reanchor()
						schedule()
					} else {
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
						reanchor()
						schedule()
					} else {
						for k, v := range rec.Values {
							l.cfg.EBus.Publish(k, v)
						}
						l.cfg.EBus.Publish("__frame__", float64(rec.Time.UnixMilli()))
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
					if l.state == statePlaying {
						reanchor()
						schedule()
					} else {
						for k, v := range rec.Values {
							l.cfg.EBus.Publish(k, v)
						}
						l.cfg.EBus.Publish("__frame__", float64(rec.Time.UnixMilli()))
					}
					if f := l.cfg.TimeSetter; f != nil {
						f(rec.Time)
					}
					l.objs.plotter.Seek(pos)
				}
			case OpPlay:
				l.state = statePlaying
				reanchor()
				schedule()
			case OpPause:
				l.state = statePaused
				timer.Stop()
			}
		case <-timer.C:
			if l.state != statePlaying {
				timer.Stop()
				continue
			}
			if rec := l.logFile.Next(); !rec.EOF {
				currentPos := l.logFile.Pos()
				schedule()

				l.objs.positionSlider.Value = float64(currentPos)
				if l.uiUpdatePending.CompareAndSwap(false, true) {
					timeText := rec.Time.Format("15:04:05.00")
					fyne.Do(func() {
						l.uiUpdatePending.Store(false)
						l.objs.positionSlider.Refresh()
						l.objs.timeLabel.SetText(timeText)
					})
				}
				for k, v := range rec.Values {
					l.cfg.EBus.Publish(k, v)
				}
				l.cfg.EBus.Publish("__frame__", float64(rec.Time.UnixMilli()))
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
