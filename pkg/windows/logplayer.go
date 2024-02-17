package windows

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/mapviewerhandler"
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

	menu *mainmenu.MainMenu

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

	openMaps map[string]mapviewerhandler.MapViewerWindowInterface

	mvh *mapviewerhandler.MapViewerHandler

	handler func(ev *fyne.KeyEvent)

	closed bool

	lambSymbolName string

	speedLock sync.RWMutex

	fyne.Window
}

func NewLogPlayer(a fyne.App, filename string, symbols symbol.SymbolCollection, onClose func()) *LogPlayer {
	w := a.NewWindow("LogPlayer " + filename)
	w.Resize(fyne.NewSize(1024, 530))

	dbCfg := &widgets.DashboardConfig{
		App:       a,
		Mw:        w,
		Logplayer: true,
		LogBtn:    nil,
		OnClose:   onClose,
		UseMPH:    a.Preferences().BoolWithFallback("useMPH", false),
	}

	lp := &LogPlayer{
		app: a,
		db:  widgets.NewDashboard(dbCfg),

		controlChan: make(chan *controlMsg, 10),

		openMaps: make(map[string]mapviewerhandler.MapViewerWindowInterface),
		symbols:  symbols,
		mvh:      mapviewerhandler.New(),

		closed: false,

		lambSymbolName: "DisplProt.LambdaScanner",

		Window: w,
	}

	l, err := readFirstLine(filename)
	if err != nil {
		log.Println(err)
	}

	if strings.Contains(l, datalogger.EXTERNALWBLSYM) {
		lp.lambSymbolName = datalogger.EXTERNALWBLSYM
	}

	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		if strings.Contains(l, "AirMassMast.m_Request") {
			lp.logType = "T8"
			dbCfg.AirDemToString = datalogger.AirDemToStringT8
		} else {
			lp.logType = "T7"
			dbCfg.AirDemToString = datalogger.AirDemToStringT7
		}

	case ".t7l":
		lp.logType = "T7"
		dbCfg.AirDemToString = datalogger.AirDemToStringT7
	case ".t8l":
		lp.logType = "T8"
		dbCfg.AirDemToString = datalogger.AirDemToStringT8
	}

	lp.menu = mainmenu.New(lp, []*fyne.Menu{
		fyne.NewMenu("File"),
	}, lp.openMap, lp.openMapz)

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
			lp.toggleBtn.SetIcon(theme.MediaPauseIcon())
		} else {
			lp.toggleBtn.SetIcon(theme.MediaPlayIcon())
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

	lp.Canvas().SetOnTypedKey(lp.handler)

	w.SetMainMenu(lp.menu.GetMenu(lp.logType))
	w.SetContent(lp.render())
	w.Show()

	start := time.Now()
	logz, err = logfile.Open(filename)
	if err != nil {
		log.Println(err)
		return lp
	}
	log.Printf("Parsed %d records in %s", logz.Len(), time.Since(start))

	lp.slider.Max = float64(logz.Len())
	lp.posLabel.SetText("0.0%")
	lp.slider.Refresh()
	go lp.PlayLog(logz)

	return lp
}

func (lp *LogPlayer) render() fyne.CanvasObject {
	main := container.NewBorder(
		nil,
		container.NewBorder(
			nil,
			nil,
			container.NewGridWithColumns(3,
				lp.prevBtn,
				lp.toggleBtn,
				lp.nextBtn,
			),
			container.NewGridWithColumns(2,
				lp.restartBtn,
				layout.NewFixedWidth(75, lp.speedSelect),
			),
			container.NewBorder(nil, nil, nil, lp.posLabel, lp.slider),
		),
		nil,
		nil,
		lp.db,
	)
	return main
}

func (lp *LogPlayer) newMapViewerWindow(w fyne.Window, mv mapviewerhandler.MapViewerWindowWidget, axis symbol.Axis) *mapviewerhandler.MapViewerWindow {
	mw := mapviewerhandler.NewWindow(w, mv)
	lp.openMaps[axis.Z] = mw

	if axis.XFrom == "" {
		axis.XFrom = "MAF.m_AirInlet"
	}

	if axis.YFrom == "" {
		axis.YFrom = "ActualIn.n_Engine"
	}

	lp.mvh.Subscribe(datalogger.EXTERNALWBLSYM, mv)
	lp.mvh.Subscribe(axis.XFrom, mv)
	lp.mvh.Subscribe(axis.YFrom, mv)

	return mw
}

func (lp *LogPlayer) PlayLog(logz logfile.Logfile) {
	play := true
	if play {
		lp.toggleBtn.SetIcon(theme.MediaPauseIcon())
	}
	var nextFrame, currentMillis int64
	var playonce bool
	speedMultiplier := 1.0

	go func() {
		for op := range lp.controlChan {
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
		if nextFrame-currentMillis > 15 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if currentMillis < nextFrame {
			continue
		}
		lp.currLine.Set(float64(logz.Pos()))
		if rec := logz.Next(); rec != nil {
			lp.db.SetTimeText(currentTimeFormatted(rec.Time))
			delayTilNext := int64(float64(rec.DelayTillNext) * speedMultiplier)
			if delayTilNext > 1000 {
				delayTilNext = 100
			}
			nextFrame = (currentMillis + delayTilNext) - 3
			for k, v := range rec.Values {
				lp.db.SetValue(k, v)
				if len(lp.openMaps) > 0 {
					lp.mvh.SetValue(k, v)
				}
			}
		}
		if playonce {
			playonce = false
		}
	}
}

func currentTimeFormatted(t time.Time) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1e6)
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

func readFirstLine(filename string) (string, error) {
	// Open the file for reading.
	file, err := os.Open(filename)
	if err != nil {
		return "", err // Return an empty string and the error.
	}
	defer file.Close() // Ensure the file is closed after finishing reading.

	// Create a new scanner to read the file.
	scanner := bufio.NewScanner(file)
	if scanner.Scan() { // Read the first line.
		return scanner.Text(), nil // Return the first line and no error.
	}

	// If there is an error scanning the file, return it.
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// If the file is empty, return an empty string with no error.
	return "", nil
}
