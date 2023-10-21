package windows

import (
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
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/mapviewer"
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
	fyne.Window

	app fyne.App

	prevBtn    *widget.Button
	toggleBtn  *widget.Button
	restartBtn *widget.Button
	nextBtn    *widget.Button

	currLine binding.Float

	posLabel *widget.Label

	selec *widget.Select

	slider *slider

	db          *Dashboard
	controlChan chan *controlMsg

	symbols symbol.SymbolCollection

	logType string

	openMaps map[string]*mapviewer.MapViewer

	symbolSubs map[string][]*mapviewer.MapViewer

	handler func(ev *fyne.KeyEvent)

	closed bool
}

func NewLogPlayer(a fyne.App, filename string, symbols symbol.SymbolCollection, onClose func()) fyne.Window {
	w := a.NewWindow("LogPlayer " + filename)
	w.Resize(fyne.NewSize(1024, 530))

	lp := &LogPlayer{
		app:         a,
		Window:      w,
		db:          NewDashboard(a, w, true, nil, onClose),
		openMaps:    make(map[string]*mapviewer.MapViewer),
		controlChan: make(chan *controlMsg, 10),
		symbols:     symbols,
		logType:     strings.ToUpper(filepath.Ext(filename)),
		closed:      false,
	}

	w.SetCloseIntercept(func() {
		lp.controlChan <- &controlMsg{Op: OpExit}
		if lp.db != nil {
			close(lp.db.metricsChan)
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

	lp.selec = widget.NewSelect([]string{"0.1x", "0.2x", "0.5x", "1x", "2x", "4x", "8x", "16x"}, func(s string) {
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

	lp.selec.Selected = "1x"

	lp.handler = keyHandler(w, lp.controlChan, lp.slider, lp.toggleBtn, lp.selec)
	lp.slider.typedKey = lp.handler
	w.Canvas().SetOnTypedKey(lp.handler)
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

func (lp *LogPlayer) render() fyne.CanvasObject {

	/*
		var objects []fyne.CanvasObject
		var bottom *fyne.Container
		if lp.logType == ".T7L" {
			for _, name := range []struct {
				label string
				name  string
			}{
				{"Fuel", "BFuelCal.Map"},
				{"Fuel/E85", "BFuelCal.StartMap"},
				{"Ignition", "IgnNormCal.Map"},
				{"Ignition/E85", "IgnE85Cal.fi_AbsMap"},
				{"Ignition/Idle", "IgnIdleCal.fi_IdleMap"},
			} {

				x, y, z := symbol.GetInfo(symbol.ECU_T7, name.name)
				if x == "" || y == "" || z == "" {
					continue
				}
				objects = append(objects, lp.newMapBtn(name.label, x, y, z))

			}

			bottom = container.NewGridWithColumns(len(objects), objects...)
		}

		if lp.logType == ".T8L" {
			bottom = container.NewGridWithColumns(2,
				lp.newMapBtn("Fuel", "IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "BFuelCal.TempEnrichFacMap"),
				lp.newMapBtn("Ignition", "IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "IgnAbsCal.fi_NormalMAP"),
			)
		}

		if bottom == nil {
			bottom = container.NewStack()
		}

		if lp.symbols == nil {
			bottom.Hide()
		}
	*/

	tree := widget.NewTreeWithStrings(mapToTreeMap(symbol.T7SymbolsTuning))
	tree.OnSelected = func(uid string) {
		//		log.Printf("%q", uid)
		if uid == "" || !strings.Contains(uid, ".") {
			if !tree.IsBranchOpen(uid) {
				tree.OpenBranch(uid)
			} else {
				tree.CloseBranch(uid)
			}
			tree.UnselectAll()
			return
		}
		if lp.symbols == nil {
			return
		}
		axis := symbol.GetInfo(symbol.ECU_T7, uid)
		log.Println(axis)
		mv, found := lp.openMaps[axis.Z]
		if !found {
			w := lp.app.NewWindow("")
			//w.Canvas().SetOnTypedKey(lp.handler)
			mv, err := mapviewer.NewMapViewer(w, axis, lp.symbols, interpolate.Interpolate)
			if err != nil {
				log.Printf("X: %s Y: %s Z: %s err: %v", axis.X, axis.Y, axis.Z, err)
				return
			}
			w.SetCloseIntercept(func() {
				log.Println("closing", axis.Z)
				delete(lp.openMaps, axis.Z)
				subsx, found := lp.symbolSubs[axis.XFrom]
				if found {
					for i, sub := range subsx {
						if sub == mv {
							lp.symbolSubs[axis.XFrom] = append(subsx[:i], subsx[i+1:]...)
							break
						}
					}
				}
				subsy, found := lp.symbolSubs[axis.YFrom]
				if found {
					for i, sub2 := range subsy {
						if sub2 == mv {
							lp.symbolSubs[axis.YFrom] = append(subsy[:i], subsy[i+1:]...)
							break
						}
					}
				}
				if axis.Z == "MAFCal.m_RedundantAirMap" {
					subst, found := lp.symbolSubs["MAF.m_AirInlet"]
					if found {
						for i, sub3 := range subst {
							if sub3 == mv {
								lp.symbolSubs["MAF.m_AirInlet"] = append(subst[:i], subst[i+1:]...)
								break
							}
						}
					}
				}
				mv.Close()
				w.Close()
			})
			lp.openMaps[axis.Z] = mv

			if lp.symbolSubs == nil {
				lp.symbolSubs = make(map[string][]*mapviewer.MapViewer)
			}

			if axis.XFrom == "" {
				axis.XFrom = "MAF.m_AirInlet"
			}

			if axis.YFrom == "" {
				axis.YFrom = "ActualIn.n_Engine"
			}

			lp.symbolSubs[axis.XFrom] = append(lp.symbolSubs[axis.XFrom], mv)
			lp.symbolSubs[axis.YFrom] = append(lp.symbolSubs[axis.YFrom], mv)

			if axis.Z == "MAFCal.m_RedundantAirMap" {
				lp.symbolSubs["MAF.m_AirInlet"] = append(lp.symbolSubs["MAF.m_AirInlet"], mv)
			}

			w.SetContent(mv)
			w.Show()
			return
		}

		mv.W.RequestFocus()

	}

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
			widgets.FixedWidth(75, lp.selec),
			container.NewBorder(nil, nil, nil, lp.posLabel, lp.slider),
		),
		nil,
		nil,
		nil,
		lp.db.Content(),
	)

	split := container.NewHSplit(
		main,
		container.NewVScroll(tree),
	)
	split.Offset = 0.7
	return split
}

/*
func (lp *LogPlayer) newMapBtn(btnTitle, supXName, supYName, mapName string) *widget.Button {
	return widget.NewButtonWithIcon(btnTitle, theme.GridIcon(), func() {
		if lp.symbols == nil {
			return
		}
		mv, found := lp.openMaps[mapName]
		if !found {
			w := lp.app.NewWindow("Map Viewer - " + mapName)
			mv, err := NewMapViewer(w, supXName, supYName, mapName, lp.symbols, interpolate.Interpolate)
			if err != nil {
				log.Printf("X: %s Y: %s Z: %s err: %v", supXName, supYName, mapName, err)
				return
			}

			w.SetCloseIntercept(func() {
				delete(lp.openMaps, mapName)
				w.Close()
			})
			lp.openMaps[mapName] = mv
			w.SetContent(mv)
			w.Show()
			return
		}
		mv.w.RequestFocus()
	})
}
*/

func (lp *LogPlayer) PlayLog(currentLine binding.Float, logz logfile.Logfile, control <-chan *controlMsg, ww fyne.Window) {
	play := true
	var nextFrame, currentMillis int64
	var playonce bool
	speedMultiplier := 1.0
	var airUpdate uint8

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

	var tmpAir, tmpmReq float64
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
			nextFrame = currentMillis + int64(float64(rec.DelayTillNext)*speedMultiplier)
			for k, v := range rec.Values {
				// Set values on dashboard
				lp.db.SetValue(k, v)

				if k == "MAF.m_AirInlet" {
					tmpAir = v
					airUpdate++
				}
				if k == "m_Request" {
					tmpmReq = v
					airUpdate++
				}
				if airUpdate == 2 {
					if s, found := lp.symbolSubs["AirDIFF"]; found {
						for _, sub := range s {
							sub.SetValue("AirDIFF", tmpmReq-tmpAir)
						}
					}
					airUpdate = 0
				}
				fac := 1.0
				if k == "ActualIn.p_AirInlet" {
					fac = 1000
				}
				subs, found := lp.symbolSubs[k]
				if found {
					for _, sub := range subs {
						sub.SetValue(k, v*fac)
					}
				}
			}
			lp.db.SetTimeText(rec.Time.Format("15:04:05.99"))
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
