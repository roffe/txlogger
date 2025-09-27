package windows

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmmod"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmstatus"
	"github.com/roffe/txlogger/pkg/widgets/trionic7/t7esp"
	"github.com/roffe/txlogger/pkg/widgets/trionic7/t7fwinfo"
	// "github.com/skratchdot/open-golang/open"
)

func (mw *MainWindow) setupMenu() {
	funcMap := map[string]func(string){
		"Register EU0D": func(str string) {
			if w := mw.wm.HasWindow("Register EU0D"); w != nil {
				mw.wm.Raise(w)
				return
			}
			inner := multiwindow.NewInnerWindow("Register EU0D", NewMyrtilosRegistration(mw))
			inner.Icon = theme.InfoIcon()
			mw.wm.Add(inner)
		},
		"ESP Calibration": func(str string) {
			if w := mw.wm.HasWindow("ESP Calibration selection"); w != nil {
				mw.wm.Raise(w)
				return
			}
			if t, ok := mw.fw.(*symbol.T7File); ok {
				esp := t7esp.New(mw.filename, t)
				inner := multiwindow.NewInnerWindow("ESP Calibration selection", esp)
				inner.Icon = theme.InfoIcon()
				inner.DisableResize = true
				mw.wm.Add(inner)
			} else {
				mw.Error(errors.New("not a T7 file"))
			}
		},
		"Firmware information": func(str string) {
			if w := mw.wm.HasWindow("Firmware info"); w != nil {
				mw.wm.Raise(w)
				return
			}
			if t, ok := mw.fw.(*symbol.T7File); ok {
				fwinfo := t7fwinfo.New(t)
				inner := multiwindow.NewInnerWindow("Firmware info", fwinfo)
				inner.Icon = theme.InfoIcon()
				mw.wm.Add(inner)
			}
		},
	}

	leading := []*fyne.Menu{
		fyne.NewMenu("File",
			fyne.NewMenuItem("About", func() {
				if w := mw.wm.HasWindow("About"); w != nil {
					mw.wm.Raise(w)
					return
				}
				inner := multiwindow.NewInnerWindow("About", About())
				inner.Icon = theme.HelpIcon()
				mw.wm.Add(inner)
			}),
			fyne.NewMenuItem("Open binary", mw.loadBinary),
			fyne.NewMenuItem("Open log", func() {
				cb := func(r fyne.URIReadCloser) {
					defer r.Close()
					filename := r.URI().Name()
					mw.Log("opening logfile " + filename)
					mw.LoadLogfile(filename, r, fyne.NewPos(10, 10))
				}
				widgets.SelectFile(cb, "Log file", "csv", "t5l", "t7l", "t8l")
			}),
			fyne.NewMenuItem("Open log folder", func() {
				cmd := exec.Command("explorer.exe", mw.settings.GetLogPath())
				if err := cmd.Start(); err != nil {
					mw.Error(err)
				}
			}),
			fyne.NewMenuItem("Settings", func() {
				mw.openSettings()
			}),
			/*
				fyne.NewMenuItem("Update txbridge", func() {
					port := mw.settings.CANSettings.GetSerialPort()
					if mw.settings.CANSettings.GetAdapterName() == "txbridge wifi" {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						addr, err := mdns.Query(ctx, "txbridge.local")
						if err != nil {
							mw.Error(fmt.Errorf("mDNS lookup for txbridge.local failed: %w", err))
							port = "tcp://192.168.4.1:1337"
						} else {
							port = "tcp://" + addr.String() + ":1337"
						}
					}
					updater := multiwindow.NewInnerWindow("txbridge firmware updater", txupdater.New(port))
					updater.Icon = theme.DownloadIcon()
					updater.Resize(fyne.NewSize(400, 300))
					mw.wm.Add(updater)
				}),
			*/
			fyne.NewMenuItem("What's new", func() {
				mw.showWhatsNew()
			}),
		),
		//fyne.NewMenu("Preset",
		//	fyne.NewMenuItem("Save", mw.savePreset),
		//	fyne.NewMenuItem("New", mw.newPreset),
		//	fyne.NewMenuItem("Import", mw.importPreset),
		//	fyne.NewMenuItem("Export", mw.exportPreset),
		//	fyne.NewMenuItem("Delete", mw.deletePreset),
		//),

	}

	trailing := []*fyne.Menu{
		fyne.NewMenu("Arrange",
			fyne.NewMenuItem("Grid", func() {
				mw.wm.Arrange(&multiwindow.GridArranger{})
			}),
			fyne.NewMenuItem("Floating", func() {
				mw.wm.Arrange(&multiwindow.FloatingArranger{})
			}),
			fyne.NewMenuItem("Pack", func() {
				mw.wm.Arrange(&multiwindow.PackArranger{})
			}),
			fyne.NewMenuItem("Preserve", func() {
				mw.wm.Arrange(&multiwindow.PreservingArranger{})
			}),
		),
	}

	mw.menu = NewMenu(mw, leading, trailing, mw.openMap, funcMap)
}

func (mw *MainWindow) loadBinary() {
	if mw.dlc != nil {
		mw.Error(errors.New("stop logging before loading a new binary"))
		return
	}
	cb := func(r fyne.URIReadCloser) {
		defer r.Close()
		filename := r.URI().Path()
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			mw.Error(err)
			return
		}
	}
	widgets.SelectFile(cb, "Binary file", "bin")
}

var openMapLock sync.Mutex

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}

	axis := symbol.GetInfo(typ, mapName)

	if w := mw.wm.HasWindow(axis.Z + " - " + axis.ZDescription); w != nil {
		mw.wm.Raise(w)
		return
	}

	//log.Println(axis)

	symX := mw.fw.GetByName(axis.X)
	if symX == nil && axis.X == "BstKnkCal.fi_offsetXSP" {
		symX = mw.fw.GetByName("BstKnkCal.OffsetXSP")
	}

	symY := mw.fw.GetByName(axis.Y)
	symZ := mw.fw.GetByName(axis.Z)

	if symZ == nil {
		mw.Error(fmt.Errorf("failed to find symbol %s", axis.Z))
		return
	}

	var xData, yData, zData []float64
	zData = symZ.Float64s()

	if symX != nil {
		xData = symX.Float64s()
	} else {
		xData = []float64{0}
	}

	if symY != nil {
		yData = symY.Float64s()
	} else {
		yData = []float64{0}
		if symZ.Name == "Batt_korr_tab!" {
			yData = []float64{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5}
		} else if len(xData) <= 1 && len(yData) <= 1 && len(zData) > 1 {
			yData = make([]float64, len(zData))
			for i := range yData {
				yData[i] = float64(i)
			}
		} else {
			yData = []float64{0}
		}
	}

	if axis.X == "Pwm_ind_trot!" {
		xData = xData[:8]
	}

	// Special cases
	switch mapName {
	case "Pgm_mod!":
		pgm := pgmmod.New()
		pgm.LoadFunc = func() ([]byte, error) {
			return symZ.Bytes(), nil
		}
		pgm.Set(symZ.Bytes())
		mapWindow := multiwindow.NewInnerWindow("Pgm_mod!", pgm)
		mapWindow.Icon = theme.GridIcon()
		mw.wm.Add(mapWindow)
		return
	case "Pgm_status":
		if w := mw.wm.HasWindow("Pgm_status"); w != nil {
			return
		}
		pgs := pgmstatus.New()
		cancel := ebus.SubscribeFunc("Pgm_status", pgs.Set)
		iw := multiwindow.NewInnerWindow("Pgm_status", pgs)
		iw.Icon = theme.InfoIcon()
		iw.OnClose = func() {
			if cancel != nil {
				cancel()
			}
		}
		mw.wm.Add(iw)
		return
	}

	var mv *mapviewer.MapViewer

	updateFunc := func(idx int, value []float64) {
		if mw.dlc != nil && mw.settings.GetAutoSave() {
			buff := bytes.NewBuffer([]byte{})
			var dataLen int
			for i, val := range value {
				buff.Write(symZ.EncodeFloat64(val))
				if i == 0 {
					dataLen = buff.Len()
				}
			}

			var addr uint32
			switch mw.selects.ecuSelect.Selected {
			case "T5":
				addr = symZ.SramOffset
			case "T7":
				addr = symZ.Address
			case "T8":
				addr = symZ.Address + symZ.SramOffset
			}

			start := time.Now()
			if err := mw.dlc.SetRAM(addr+uint32(idx*dataLen), buff.Bytes()); err != nil {
				mw.Error(err)
				return
			}
			//mw.Log(fmt.Sprintf("set $%d %s %s", addr, axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
			mw.Log(fmt.Sprintf("set %s %dms", axis.Z, time.Since(start).Truncate(10*time.Millisecond).Milliseconds()))
		}
	}

	loadFunc := func() {
		if mw.dlc != nil {
			start := time.Now()
			var addr uint32

			switch mw.selects.ecuSelect.Selected {
			case "T5":
				addr = symZ.SramOffset
			case "T7":
				addr = symZ.Address
			case "T8":
				addr = symZ.Address + symZ.SramOffset
			}

			data, err := mw.dlc.GetRAM(addr, uint32(symZ.Length))
			if err != nil {
				mw.Error(err)
				return
			}

			if err := mv.SetZData(symZ.BytesToFloat64s(data)); err != nil {
				mw.Error(err)
				return
			}
			mw.Log(fmt.Sprintf("load %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
		}
	}

	saveFunc := func(data []float64) {
		if mw.dlc == nil {
			return
		}
		start := time.Now()
		buff := bytes.NewBuffer(symZ.EncodeFloat64s(data))
		var startPos uint32
		switch mw.selects.ecuSelect.Selected {
		case "T7":
			startPos = symZ.Address
		case "T8":
			startPos = symZ.Address + symZ.SramOffset
		}

		if err := mw.dlc.SetRAM(startPos, buff.Bytes()); err != nil {
			mw.Error(err)
			return
		}
		buff.Reset()

		//mw.Log(fmt.Sprintf("save %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
		mw.Log(fmt.Sprintf("save %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
	}

	saveFileFunc := func(data []float64) {
		ss := mw.fw.GetByName(axis.Z)
		if ss == nil {
			mw.Log(fmt.Sprintf("failed to find symbol %s", axis.Z))
			return
		}
		if err := ss.SetData(ss.EncodeFloat64s(data)); err != nil {
			mw.Error(err)
			return
		}
		if err := mw.fw.Save(mw.filename); err != nil {
			mw.Error(err)
			return
		}
		mw.Log(fmt.Sprintf("Saved %s", axis.Z))
	}

	var xPrecision, yPrecision, zPrecision int
	if symX != nil {
		xPrecision = symbol.GetPrecision(symX.Correctionfactor)
	}

	if symY != nil {
		yPrecision = symbol.GetPrecision(symY.Correctionfactor)
	}

	zPrecision = symbol.GetPrecision(symZ.Correctionfactor)

	mv, err := mapviewer.New(
		mapviewer.WithSymbol(symZ),
		mapviewer.WithXData(xData),
		mapviewer.WithYData(yData),
		mapviewer.WithZData(zData),
		mapviewer.WithXPrecision(xPrecision),
		mapviewer.WithYPrecision(yPrecision),
		mapviewer.WithZPrecision(zPrecision),
		mapviewer.WithXFrom(axis.XFrom),
		mapviewer.WithYFrom(axis.YFrom),
		mapviewer.WithUpdateECUFunc(updateFunc),
		mapviewer.WithLoadECUFunc(loadFunc),
		mapviewer.WithSaveECUFunc(saveFunc),
		mapviewer.WithSaveFileFunc(saveFileFunc),
		mapviewer.WithMeshView(mw.settings.GetMeshView()),
		mapviewer.WithEditable(true),
		mapviewer.WithButtons(true),
		mapviewer.WithFollowCrosshair(mw.settings.GetCursorFollowCrosshair()),
		mapviewer.WithAxisLabels(axis.XDescription, axis.YDescription, axis.ZDescription),
		mapviewer.WithColorBlindMode(mw.settings.GetColorBlindMode()),
	)
	if err != nil {
		mw.Error(err)
		return
	}

	if mw.settings.GetAutoLoad() && mw.dlc != nil {
		go func() {
			openMapLock.Lock()
			defer openMapLock.Unlock()
			p := progressmodal.New(mw.Window.Canvas(), "Loading "+axis.Z)
			fyne.DoAndWait(p.Show)
			loadFunc()
			fyne.Do(p.Hide)
		}()
	}

	mapWindow := multiwindow.NewInnerWindow(axis.Z+" - "+axis.ZDescription, mv)
	mapWindow.Icon = theme.GridIcon()

	mv.OnMouseDown = func() {
		mw.wm.Raise(mapWindow)
	}

	var cancelFuncs []func()
	if axis.XFrom != "" {
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(axis.XFrom, func(value float64) {
			// log.Printf("set %s %f", axis.XFrom, value)
			mv.SetX(value)
		}))
	}
	if axis.YFrom != "" {
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(axis.YFrom, func(value float64) {
			// log.Printf("set %s %f", axis.YFrom, value)
			mv.SetY(value)
		}))
	}

	cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(ebus.TOPIC_COLORBLINDMODE, func(value float64) {
		mv.SetColorBlindMode(widgets.ColorBlindMode(int(value)))
	}))

	mapWindow.OnClose = func() {
		for _, f := range cancelFuncs {
			f()
		}
	}

	mw.wm.Add(mapWindow)

}
