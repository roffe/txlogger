package windows

import (
	"fmt"
	"log"
	"os"
	"strings"

	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/skratchdot/open-golang/open"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	mw.addSymbolBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		defer mw.symbolConfigList.Refresh()
		s, ok := mw.symbolMap[mw.symbolLookup.Text]
		if !ok {
			mw.vars.Add(&kwp2000.VarDefinition{
				Name: mw.symbolLookup.Text,
			})
			return
		}
		mw.vars.Add(s)
		mw.SaveSymbolList()
		//log.Printf("Name: %s, Method: %d, Value: %d, Type: %X", s.Name, s.Method, s.Value, s.Type)
	})

	mw.loadSymbolsFileBtn = widget.NewButtonWithIcon("Load from binary", theme.FileIcon(), func() {
		filename, err := sdialog.File().Filter("Binary file", "bin").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		mw.SyncSymbols()
	})

	mw.loadSymbolsEcuBtn = widget.NewButtonWithIcon("Load from ECU", theme.DownloadIcon(), func() {
		//		mw.progressBar.Start()
		mw.disableBtns()
		go func() {
			defer mw.enableBtns()
			//		defer mw.progressBar.Stop()
			if err := mw.loadSymbolsFromECU(); err != nil {
				// dialog.ShowError(err, mw)
				mw.Log(err.Error())
				return
			}
			mw.SyncSymbols()
		}()
	})

	mw.loadConfigBtn = widget.NewButtonWithIcon("Load config", theme.FileIcon(), func() {
		filename, err := sdialog.File().Filter("*.json", "json").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		if err := mw.LoadConfig(filename); err != nil {
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		mw.symbolConfigList.Refresh()
		mw.SyncSymbols()
	})

	mw.saveConfigBtn = widget.NewButtonWithIcon("Save config", theme.DocumentSaveIcon(), func() {
		filename, err := sdialog.File().Filter("json", "json").Save()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		if !strings.HasSuffix(filename, ".json") {
			filename += ".json"
		}
		if err := mw.SaveConfig(filename); err != nil {
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return

		}
	})

	mw.helpBtn = widget.NewButtonWithIcon("Help", theme.HelpIcon(), func() {
		Help(mw.app)
	})

	mw.syncSymbolsBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), mw.SyncSymbols)

	mw.dashboardBtn = widget.NewButtonWithIcon("Dashboard", theme.InfoIcon(), func() {
		onClose := func() {
			if mw.dlc != nil {
				mw.dlc.DetachDashboard(mw.dashboard)
			}
			if mw.dashboard != nil {
				mw.dashboard.Close()
			}
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.Content())
		}

		mw.dashboard = widgets.NewDashboard(mw.app, mw, false, mw.logBtn, onClose)
		if mw.dlc != nil {
			mw.dlc.AttachDashboard(mw.dashboard)
		}

		mw.SetContent(mw.dashboard)
	})

	mw.logplayerBtn = widget.NewButtonWithIcon("Log Player", theme.MediaFastForwardIcon(), func() {
		filename, err := sdialog.File().Filter("trionic logfile", "t7l", "t8l").SetStartDir("logs").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}

		onClose := func() {
			if mw.dlc != nil {
				mw.dlc.DetachDashboard(mw.dashboard)
			}
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.Content())
		}
		go NewLogPlayer(mw.app, filename, mw.symbols, onClose)
	})

	mw.logfolderBtn = widget.NewButtonWithIcon("Logs Folder", theme.FolderOpenIcon(), func() {
		if _, err := os.Stat("logs"); os.IsNotExist(err) {
			if err := os.Mkdir("logs", 0755); err != nil {
				if err != os.ErrExist {
					mw.Log(fmt.Sprintf("failed to create logs dir: %s", err))
					return
				}
			}
		}

		path, err := os.Getwd()
		if err != nil {
			log.Println(err)
			return
		}

		if err := open.Run(path + "\\logs"); err != nil {
			log.Println(err)
		}
	})

	mw.logBtn = widget.NewButtonWithIcon("Start logging", theme.MediaPlayIcon(), func() {
		if mw.loggingRunning {
			if mw.dlc != nil {
				mw.dlc.Close()
			}
			return
		}
		if !mw.loggingRunning {
			device, err := mw.canSettings.GetAdapter(mw.ecuSelect.Selected, mw.Log)
			if err != nil {
				mw.Log(err.Error())
				return
			}
			mw.dlc, err = datalogger.New(datalogger.Config{
				ECU:                   mw.ecuSelect.Selected,
				Dev:                   device,
				Variables:             mw.vars.Get(),
				Freq:                  int(mw.freqSlider.Value),
				OnMessage:             mw.Log,
				CaptureCounter:        mw.captureCounter,
				ErrorCounter:          mw.errorCounter,
				ErrorPerSecondCounter: mw.errorPerSecondCounter,
			})
			if err != nil {
				mw.Log(err.Error())
				return
			}
			mw.loggingRunning = true
			mw.logBtn.SetIcon(theme.MediaStopIcon())
			mw.logBtn.SetText("Stop logging")
			mw.disableBtns()

			if mw.dashboard != nil {
				mw.dlc.AttachDashboard(mw.dashboard)
			}

			fac := func(mv *widgets.MapViewer, name string) *func(v float64) {
				fun := func(v float64) {
					mv.SetValue(name, v)
				}
				return &fun
			}
			for _, mv := range mw.openMaps {
				setRpm := fac(mv.mv, "ActualIn.n_Engine")
				setAir := fac(mv.mv, "MAF.m_AirInlet")
				mw.dlc.Subscribe("ActualIn.n_Engine", setRpm)
				mw.dlc.Subscribe("MAF.m_AirInlet", setAir)
			}

			go func() {
				defer mw.enableBtns()
				if err := mw.dlc.Start(); err != nil {
					mw.Log(err.Error())
				}
				if mw.dashboard != nil {
					mw.dlc.DetachDashboard(mw.dashboard)
				}
				mw.loggingRunning = false
				mw.dlc = nil
				mw.logBtn.SetIcon(theme.MediaPlayIcon())
				mw.logBtn.SetText("Start logging")
			}()
		}
	})

	//mw.fuelBtn = mw.newMapBtn("Fuel", "BFuelCal.AirXSP", "BFuelCal.RpmYSP", "BFuelCal.Map")
	//mw.ignitionBtn = mw.newMapBtn("Ignition", "BFuelCal.AirXSP", "BFuelCal.RpmYSP", "IgnNormCal.Map")
	//
	//mw.fuelBtn = mw.newMapBtn("Fuel", "IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "BFuelCal.TempEnrichFacMap")
	//mw.ignitionBtn = mw.newMapBtn("Ignition", "IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "IgnAbsCal.fi_NormalMAP")

	/*
		mw.fuelBtn = widget.NewButtonWithIcon("Fuel", theme.GridIcon(), func() {
			mw.openShort("Fuel")
		})
		mw.ignitionBtn = widget.NewButtonWithIcon("Ignition", theme.GridIcon(), func() {
			mw.openShort("Ignition")
		})
	*/

}

/*
func (mw *MainWindow) openShort(name string) {
	switch name {
	case "Fuel":
		switch mw.ecuSelect.Selected {
		case "T7":
			mw.openMap(symbol.GetInfo(symbol.ECU_T7, "BFuelCal.Map"))
			mw.openMap(symbol.GetInfo(symbol.ECU_T7, "BFuelCal.StartMap"))

		case "T8":
			//mw.openMap("IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "BFuelCal.TempEnrichFacMap")
			symbol.GetInfo(symbol.ECU_T8, "BFuelCal.TempEnrichFacMap")
		}

	case "Ignition":
		switch mw.ecuSelect.Selected {
		case "T7":
			mw.openMap(symbol.GetInfo(symbol.ECU_T7, "IgnNormCal.Map"))
			mw.openMap(symbol.GetInfo(symbol.ECU_T7, "IgnE85Cal.fi_AbsMap"))
		case "T8":
			mw.openMap(symbol.GetInfo(symbol.ECU_T8, "IgnAbsCal.fi_NormalMAP"))
			//mw.openMap("IgnAbsCal.m_AirNormXSP", "IgnAbsCal.n_EngNormYSP", "IgnAbsCal.fi_NormalMAP")
		}
	}
}

func (mw *MainWindow) newMap(supXName, supYName, mapName string) {
	mv, found := mw.openMaps[mapName]
	if !found {

		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		mv, err := NewMapViewer(nil, supXName, supYName, mapName, mw.symbols, interpolate.Interpolate)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		var tmpRpm float64
		setRpm := func(v float64) {
			tmpRpm = v
		}
		setAir := func(v float64) {
			mv.SetXY(int(v), int(tmpRpm))
		}

		// w.SetCloseIntercept(func() {
		// 	delete(mw.openMaps, mapName)
		// 	if mw.dlc != nil {
		// 		mw.dlc.Unsubscribe("ActualIn.n_Engine", &setRpm)
		// 		mw.dlc.Unsubscribe("MAF.m_AirInlet", &setAir)
		// 	}
		// 	w.Close()
		// })

		if mw.dlc != nil {
			mw.dlc.Subscribe("ActualIn.n_Engine", &setRpm)
			mw.dlc.Subscribe("MAF.m_AirInlet", &setAir)
		}
		mw.openMaps[mapName] = mv
	}
	mw.leading.RemoveAll()
	mw.leading.Add(mv)
}
*/

func (mw *MainWindow) openMap(axis symbol.Axis) {
	//log.Printf("openMap: %s %s %s", axis.X, axis.Y, axis.Z)
	mv, found := mw.openMaps[axis.Z]
	if !found {
		w := mw.app.NewWindow("Map Viewer - " + axis.Z)
		//w.SetFixedSize(true)
		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		xData, yData, zData, _, _, corrFac, err := mw.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		mv, err := widgets.NewMapViewer(xData, yData, zData, corrFac, interpolate.Interpolate)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		fac := func(mv *widgets.MapViewer, name string) *func(v float64) {
			fun := func(v float64) {
				mv.SetValue(name, v)
			}
			return &fun
		}

		setRpm := fac(mv, "ActualIn.n_Engine")
		setAir := fac(mv, "MAF.m_AirInlet")

		w.SetCloseIntercept(func() {
			delete(mw.openMaps, axis.Z)
			if mw.dlc != nil {
				mw.dlc.Unsubscribe("ActualIn.n_Engine", setRpm)
				mw.dlc.Unsubscribe("MAF.m_AirInlet", setAir)
			}
			w.Close()
		})
		if mw.dlc != nil {
			mw.dlc.Subscribe("ActualIn.n_Engine", setRpm)
			mw.dlc.Subscribe("MAF.m_AirInlet", setAir)
		}
		mw.openMaps[axis.Z] = &MapViewerWindow{w, mv}
		w.SetContent(mv)
		w.Show()

		return
	}
	mv.RequestFocus()
}

/*
func (mw *MainWindow) newMapBtn(btnTitle, supXName, supYName, mapName string) *widget.Button {
	return widget.NewButtonWithIcon(btnTitle, theme.GridIcon(), func() {
		mv, found := mw.openMaps[mapName]
		if !found {
			w := mw.app.NewWindow("Map Viewer - " + mapName)
			if mw.symbols == nil {
				mw.Log("No binary loaded")
				return
			}
			mv, err := NewMapViewer(w, supXName, supYName, mapName, mw.symbols, interpolate.Interpolate)
			if err != nil {
				mw.Log(err.Error())
				return
			}

			var tmpRpm float64
			setRpm := func(v float64) {
				tmpRpm = v
			}
			setAir := func(v float64) {
				mv.SetXY(int(v), int(tmpRpm))
			}
			w.SetCloseIntercept(func() {
				delete(mw.openMaps, mapName)
				if mw.dlc != nil {
					mw.dlc.Unsubscribe("ActualIn.n_Engine", &setRpm)
					mw.dlc.Unsubscribe("MAF.m_AirInlet", &setAir)
				}
				w.Close()
			})
			if mw.dlc != nil {
				mw.dlc.Subscribe("ActualIn.n_Engine", &setRpm)
				mw.dlc.Subscribe("MAF.m_AirInlet", &setAir)
			}
			mw.openMaps[mapName] = mv
			w.SetContent(mv)
			w.Show()

			return
		}
		mv.w.RequestFocus()
	})
}
*/
