package windows

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
)

const TIME_FORMAT = "02-01-2006 15:04:05.999"

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

type slider struct {
	widget.Slider
	typedKey func(key *fyne.KeyEvent)
}

func (s *slider) TypedKey(key *fyne.KeyEvent) {
	if s.typedKey != nil {
		s.typedKey(key)
	}
}

type LogPlayer struct {
	app fyne.App

	prevBtn     *widget.Button
	toggleBtn   *widget.Button
	restartBtn  *widget.Button
	nextBtn     *widget.Button
	currLine    binding.Float
	posLabel    *widget.Label
	speedSelect *widget.Select

	slider *slider

	db          *widgets.Dashboard
	controlChan chan *controlMsg

	symbols symbol.SymbolCollection

	logType string

	openMaps map[string]*MapViewerWindow

	mvh *MapViewerHandler

	handler func(ev *fyne.KeyEvent)

	closed bool

	fyne.Window
}

func NewLogPlayer(a fyne.App, filename string, symbols symbol.SymbolCollection, onClose func()) *LogPlayer {
	w := a.NewWindow("LogPlayer " + filename)
	w.Resize(fyne.NewSize(1024, 530))

	lp := &LogPlayer{
		app: a,
		db:  widgets.NewDashboard(a, w, true, nil, onClose),

		controlChan: make(chan *controlMsg, 10),

		openMaps: make(map[string]*MapViewerWindow),
		symbols:  symbols,
		mvh:      NewMapViewerHandler(),

		logType: strings.ToUpper(filepath.Ext(filename)),
		closed:  false,

		Window: w,
	}

	w.SetCloseIntercept(func() {
		lp.controlChan <- &controlMsg{Op: OpExit}
		if lp.db != nil {
			lp.db.Close()
			lp.closed = true
		}
		for _, ma := range lp.openMaps {
			ma.Close()
		}
		w.Close()
	})

	lp.db.SetValue("CEL", 0)
	lp.db.SetValue("CRUISE", 0)
	lp.db.SetValue("LIMP", 0)

	logz := (logfile.Logfile)(&logfile.TxLogfile{})

	lp.slider = &slider{}
	lp.slider.Step = 1
	lp.slider.Orientation = widget.Horizontal
	lp.slider.ExtendBaseWidget(lp.slider)

	lp.posLabel = widget.NewLabel("")

	lp.currLine = binding.NewFloat()

	lp.currLine.AddListener(binding.NewDataListener(func() {
		val, err := lp.currLine.Get()
		if err != nil {
			log.Println(err)
			return
		}
		lp.slider.Value = val
		lp.slider.Refresh()
		currPct := val / float64(logz.Len()) * 100
		lp.posLabel.SetText(fmt.Sprintf("%.01f%%", currPct))
	}))

	lp.slider.OnChanged = func(pos float64) {
		lp.controlChan <- &controlMsg{Op: OpSeek, Pos: int(pos)}
		currPct := pos / float64(logz.Len()) * 100
		lp.posLabel.SetText(fmt.Sprintf("%.01f%%", currPct))
	}

	playing := false
	lp.toggleBtn = &widget.Button{
		Text: "",
		Icon: theme.MediaPlayIcon(),
	}
	lp.toggleBtn.OnTapped = func() {
		lp.controlChan <- &controlMsg{Op: OpTogglePlayback}
		if playing {
			lp.toggleBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			lp.toggleBtn.SetIcon(theme.MediaPauseIcon())
		}
		playing = !playing
	}

	lp.prevBtn = widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		lp.controlChan <- &controlMsg{Op: OpPrev}
	})

	lp.nextBtn = widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		lp.controlChan <- &controlMsg{Op: OpNext}
	})

	lp.restartBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		lp.controlChan <- &controlMsg{Op: OpSeek, Pos: 0}
	})

	lp.speedSelect = widget.NewSelect([]string{"0.1x", "0.2x", "0.5x", "1x", "2x", "4x", "8x", "16x"}, func(s string) {
		switch s {
		case "0.1x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 10}
		case "0.2x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 5}
		case "0.5x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 2}
		case "1x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 1}
		case "2x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.5}
		case "4x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.25}
		case "8x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.125}
		case "16x":
			lp.controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.0625}
		}
	})

	lp.speedSelect.Selected = "1x"

	lp.handler = keyHandler(w, lp.controlChan, lp.slider, lp.toggleBtn, lp.speedSelect)
	lp.slider.typedKey = lp.handler

	lp.setupMenu()

	w.SetContent(lp.render())
	w.Show()

	var err error
	start := time.Now()
	logz, err = logfile.NewFromTxLogfile(filename)
	if err != nil {
		log.Println(err)

	}
	log.Printf("Parsed %d records in %s", logz.Len(), time.Since(start))
	lp.slider.Max = float64(logz.Len())
	lp.posLabel.SetText("0.0%")
	lp.slider.Refresh()
	lp.PlayLog(lp.currLine, logz, lp.controlChan, w)

	return lp
}

func (lp *LogPlayer) setupMenu() {
	var menus []*fyne.Menu
	menus = append(menus, fyne.NewMenu("File"))
	for _, category := range symbol.T7SymbolsTuningOrder {
		var items []*fyne.MenuItem
		for _, mapName := range symbol.T7SymbolsTuning[category] {
			items = append(items, fyne.NewMenuItem(mapName, func() {
				lp.openMap(mapName)
			}))
		}
		menus = append(menus, fyne.NewMenu(category, items...))
	}
	menu := fyne.NewMainMenu(menus...)
	lp.Window.SetMainMenu(menu)
}

func (lp *LogPlayer) render() fyne.CanvasObject {

	/*
		if lp.logType == ".T8L" {
			bottom = container.NewGridWithColumns(2,
				lp.newMapBtn("Fuel", "IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "BFuelCal.TempEnrichFacMap"),
				lp.newMapBtn("Ignition", "IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "IgnAbsCal.fi_NormalMAP"),
			)
		}
	*/

	main := container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			container.NewGridWithColumns(4,
				lp.prevBtn,
				lp.toggleBtn,
				lp.restartBtn,
				lp.nextBtn,
			),
			layout.NewFixedWidth(75, lp.speedSelect),
			container.NewBorder(nil, nil, nil, lp.posLabel, lp.slider),
		),
		nil,
		nil,
		nil,
		lp.db,
	)
	return main
}

func (lp *LogPlayer) openMap(symbolName string) error {
	if symbolName == "" {
		return errors.New("symbolName is empty")
	}
	axis := symbol.GetInfo(symbol.ECU_T7, symbolName)
	mw, found := lp.openMaps[axis.Z]
	if !found {
		w := lp.app.NewWindow(fmt.Sprintf("Map Viewer - %s", axis.Z))

		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := lp.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			return err
		}

		mv, err := widgets.NewMapViewer(
			widgets.WithXData(xData),
			widgets.WithYData(yData),
			widgets.WithZData(zData),
			widgets.WithXCorrFac(xCorrFac),
			widgets.WithYCorrFac(yCorrFac),
			widgets.WithZCorrFac(zCorrFac),
			widgets.WithXFrom(axis.XFrom),
			widgets.WithYFrom(axis.YFrom),
			widgets.WithInterPolFunc(interpolate.Interpolate),
		)
		if err != nil {
			return fmt.Errorf("x: %s y: %s z: %s err: %w", axis.X, axis.Y, axis.Z, err)
		}

		w.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
			if ke.Name == fyne.KeySpace {
				lp.toggleBtn.Tapped(&fyne.PointEvent{})
				return
			}
			mv.TypedKey(ke)
		})

		w.SetCloseIntercept(func() {
			log.Println("closing", axis.Z)
			delete(lp.openMaps, axis.Z)
			lp.mvh.Unsubscribe(axis.XFrom, mv)
			lp.mvh.Unsubscribe(axis.YFrom, mv)
			mv.Close()
			w.Close()
		})

		mw = lp.newMapViewerWindow(w, mv, axis)

		w.SetContent(mv)
		w.Show()
	}
	mw.RequestFocus()
	return nil
}

func (lp *LogPlayer) newMapViewerWindow(w fyne.Window, mv *widgets.MapViewer, axis symbol.Axis) *MapViewerWindow {
	mw := &MapViewerWindow{Window: w, mv: mv}
	lp.openMaps[axis.Z] = mw

	if axis.XFrom == "" {
		axis.XFrom = "MAF.m_AirInlet"
	}

	if axis.YFrom == "" {
		axis.YFrom = "ActualIn.n_Engine"
	}

	lp.mvh.Subscribe(axis.XFrom, mv)
	lp.mvh.Subscribe(axis.YFrom, mv)

	return mw
}

func (lp *LogPlayer) PlayLog(currentLine binding.Float, logz logfile.Logfile, control <-chan *controlMsg, ww fyne.Window) {
	play := true
	var nextFrame, currentMillis int64
	var playonce bool
	speedMultiplier := 1.0

	go func() {
		for op := range control {
			currentMillis := time.Now().UnixMilli()
			switch op.Op {
			case OpPlaybackSpeed:
				speedMultiplier = op.Rate
			case OpTogglePlayback:
				play = !play
			case OpSeek:
				logz.Seek(op.Pos)
				playonce = true
			case OpPrev:
				pos := logz.Pos() - 2
				if pos < 0 {
					pos = 0
				}
				playonce = true
				logz.Seek(pos)
				nextFrame = currentMillis
			case OpNext:
				playonce = true
			case OpExit:
				return
			}
		}
	}()

	for !lp.closed {
		currentMillis = time.Now().UnixMilli()
		if logz.Pos() >= logz.Len()-1 || (!play && !playonce) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if nextFrame-currentMillis > 4 {
			time.Sleep(time.Duration(nextFrame-currentMillis-2) * time.Millisecond)
			continue
		}
		if currentMillis < nextFrame {
			continue
		}
		currentLine.Set(float64(logz.Pos()))
		if rec := logz.Next(); rec != nil {
			lp.db.SetTimeText(rec.Time.Format("15:04:05.99"))
			delayTilNext := int64(float64(rec.DelayTillNext) * speedMultiplier)
			if delayTilNext > 1000 {
				delayTilNext = 100
			}
			nextFrame = currentMillis + delayTilNext
			for k, v := range rec.Values {
				lp.db.SetValue(k, v)
				if len(lp.openMaps) > 0 {
					fac := 1.0
					if k == "ActualIn.p_AirInlet" {
						fac = 1000
					}
					lp.mvh.SetValue(k, v*fac)
				}
			}
		}
		if playonce {
			playonce = false
		}
	}
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
