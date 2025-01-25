package windows

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/msglist"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	// mw.buttons.logplayerBtn = mw.newLogplayerBtn()
	//mw.buttons.loadSymbolsEcuBtn = mw.loadSymbolsEcuBtnFunc()
	mw.buttons.addSymbolBtn = mw.addSymbolBtnFunc()
	mw.buttons.syncSymbolsBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), mw.SyncSymbols)
	mw.buttons.dashboardBtn = mw.newDashboardBtn()
	mw.buttons.logBtn = mw.newLogBtn()
	mw.buttons.openLogBtn = mw.newOpenLogBtn()
	mw.buttons.layoutRefreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		mw.selects.layoutSelect.SetOptions(listLayouts())
	})
	mw.buttons.debugBtn = mw.newDebugBtn()
	mw.buttons.symbolListBtn = mw.newSymbolListBtn()
	mw.buttons.addGaugeBtn = mw.newaddGaugeBtn()
}

func (mw *MainWindow) newaddGaugeBtn() *widget.Button {
	return widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		if w := mw.wm.HasWindow("Create gauge"); w != nil {
			mw.wm.Raise(w)
			return
		}
		gs := NewGaugeCreator(mw)
		iw := multiwindow.NewSystemWindow("Create gauge", gs)
		iw.Icon = theme.ContentAddIcon()
		mw.wm.Add(iw)
	})
}

func (mw *MainWindow) newSymbolListBtn() *widget.Button {
	return widget.NewButtonWithIcon("Symbol list", theme.ListIcon(), func() {
		if w := mw.wm.HasWindow("Symbol list"); w != nil {
			mw.wm.Raise(w)
			return
		}

		mw.symbolList.UpdateBars(mw.settings.GetRealtimeBars())
		symbolListWindow := multiwindow.NewSystemWindow("Symbol list", container.NewBorder(
			nil,
			container.NewBorder(
				nil,
				nil,
				nil,
				container.NewGridWithColumns(5,
					widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), mw.savePreset),
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), mw.newPreset),
					widget.NewButtonWithIcon("", theme.UploadIcon(), mw.exportPreset),
					widget.NewButtonWithIcon("", theme.DownloadIcon(), mw.importPreset),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), mw.deletePreset),
				),
				mw.selects.presetSelect,
			),
			//layout.NewSpacer(),
			//mw.buttons.loadSymbolsEcuBtn,
			nil,
			nil,
			container.NewBorder(
				container.NewBorder(
					nil,
					nil,
					widget.NewIcon(theme.SearchIcon()),
					container.NewHBox(
						mw.buttons.addSymbolBtn,
						mw.buttons.syncSymbolsBtn,
					),
					mw.selects.symbolLookup,
				),
				nil,
				nil,
				nil,
				mw.symbolList,
			),
		))
		symbolListWindow.Icon = theme.ListIcon()

		// Fixa så livepreview values kan togglas på/av. med nya ui't så funkar det inte som det ska

		//var cancelFuncs []func()
		//if mw.settings.GetLivePreview() {
		//	for _, sym := range mw.symbolList.Symbols() {
		//		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(sym.Name, func(f float64) {
		//			mw.symbolList.SetValue(sym.Name, f)
		//		}))
		//	}
		//} else {
		//	mw.symbolList.Clear()
		//}
		//symbolListWindow.OnClose = func() {
		//	for _, f := range cancelFuncs {
		//		f()
		//	}
		//}

		mw.wm.Add(symbolListWindow)
		if mw.startup {
			symbolListWindow.Move(fyne.NewPos(0, 0))
			symbolListWindow.Resize(fyne.NewSize(500, 550))
		}
	})
}

func (mw *MainWindow) newDebugBtn() *widget.Button {
	return widget.NewButtonWithIcon("Debug log", theme.InfoIcon(), func() {
		if w := mw.wm.HasWindow("Debug log"); w != nil {
			mw.wm.Raise(w)
			return
		}
		dbl := msglist.New(mw.outputData)
		debugWindow := multiwindow.NewSystemWindow("Debug log", dbl)
		debugWindow.Icon = theme.ContentCopyIcon()
		debugWindow.OnTappedIcon = func() {
			str, err := mw.outputData.Get()
			if err != nil {
				mw.Error(err)
				return
			}
			fyne.CurrentApp().Clipboard().SetContent(strings.Join(str, "\n"))
			// fyne.CurrentApp().Driver().AllWindows()[0].Clipboard().SetContent(strings.Join(str, "\n"))
			dialog.ShowInformation("Debug log", "Content copied to clipboard", mw)
		}
		xy := mw.wm.Size().Subtract(dbl.MinSize().AddWidthHeight(20, 60))
		mw.wm.Add(debugWindow, fyne.NewPos(xy.Width, xy.Height))
	})
}

func (mw *MainWindow) newOpenLogBtn() *widget.Button {
	return widget.NewButtonWithIcon("Open log in new Window", theme.MediaFastForwardIcon(), func() {
		go func() {
			filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").Load()
			if err != nil {
				if err.Error() == "Cancelled" {
					return
				}
				fyne.LogError("Error loading log file", err)
				return
			}

			mw.LoadLogfileCombined(filename, fyne.Position{}, true)

		}()
	})
}

func (mw *MainWindow) addSymbolBtnFunc() *widget.Button {
	return widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		sym := mw.fw.GetByName(mw.selects.symbolLookup.Text)
		if sym == nil {
			mw.Error(fmt.Errorf("%q not found", mw.selects.symbolLookup.Text))
			return
		}
		mw.symbolList.Add(sym)
	})
}

/*
func (mw *MainWindow) loadSymbolsEcuBtnFunc() *widget.Button {
	return widget.NewButtonWithIcon("Load from ECU", theme.DownloadIcon(), func() {
		mw.Disable()
		go func() {
			defer mw.Enable()
			if err := mw.LoadSymbolsFromECU(); err != nil {
				mw.Error(err)
				return
			}
		}()
	})
}
*/

/*
func (mw *MainWindow) newLogplayerBtn() *widget.Button {
	return widget.NewButtonWithIcon("Play log", theme.MediaFastForwardIcon(), func() {
		//n := fyne.NewNotification("Not implemented", "test")
		//mw.app.SendNotification(n)
		filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").SetStartDir(mw.settings.GetLogPath()).Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			mw.Error(err)
			return
		}

		lp := NewLogPlayer(mw.app, filename, mw.fw)
		lp.Show()
	})

}
*/

func (mw *MainWindow) newLogBtn() *widget.Button {
	return widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		if mw.loggingRunning {
			if mw.dlc != nil {
				mw.dlc.Close()
			}
			return
		}
		for _, v := range mw.symbolList.Symbols() {
			if v.Name == "AirMassMast.m_Request" && mw.selects.ecuSelect.Selected == "T7" {
				mw.Error(fmt.Errorf("AirMassMast.m_Request is not supported on T7, Did you forget to change preset?"))
				return
			}
			if v.Name == "m_Request" && mw.selects.ecuSelect.Selected == "T8" {
				mw.Error(fmt.Errorf("m_Request is not supported on T8, Did you forget to change preset?"))
				return
			}
			v.Skip = false
		}
		mw.startLogging()
	})
}

func (mw *MainWindow) newDashboardBtn() *widget.Button {
	return widget.NewButtonWithIcon("Dashboard", theme.InfoIcon(), func() {
		if w := mw.wm.HasWindow("Dashboard"); w != nil {
			mw.wm.Raise(w)
			return
		}

		dbcfg := &dashboard.Config{
			Logplayer:       false,
			UseMPH:          mw.settings.GetUseMPH(),
			SwapRPMandSpeed: mw.settings.GetSwapRPMandSpeed(),
			HighAFR:         mw.settings.GetHighAFR(),
			LowAFR:          mw.settings.GetLowAFR(),
			WidebandSymbol:  mw.settings.GetWidebandSymbolName(),
		}

		switch mw.selects.ecuSelect.Selected {
		case "T7":
			dbcfg.AirDemToString = datalogger.AirDemToStringT7
		case "T8":
			dbcfg.AirDemToString = datalogger.AirDemToStringT8
		}

		db := dashboard.NewDashboard(dbcfg)

		var cancelFuncs []func()
		for _, m := range db.GetMetricNames() {
			//if m == "MAF.m_AirInlet" {
			//	var once sync.Once
			//	cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(m, func(f float64) {
			//		if f >= 1800 {
			//			once.Do(func() {
			//				if mw.oCtx != nil {
			//					if err := snd.Pedro(mw.oCtx); err != nil {
			//						mw.Log("Pedro failed: " + err.Error())
			//					}
			//				}
			//			})
			//		}
			//		db.SetValue(m, f)
			//	}))
			//	continue
			//}

			cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(m, func(f float64) {
				db.SetValue(m, f)
			}))
		}

		dbw := multiwindow.NewInnerWindow("Dashboard", db)
		dbw.Icon = theme.InfoIcon()

		dbcfg.FullscreenFunc = func(b bool) {
			if b {
				mw.SetMainMenu(nil)
				mw.Window.SetContent(db)
				mw.SetFullScreen(true)
			} else {
				mw.SetMainMenu(mw.menu.GetMenu(mw.selects.ecuSelect.Selected))
				mw.Window.SetContent(mw.content)
				dbw.Close()
				mw.buttons.dashboardBtn.OnTapped()
				mw.SetFullScreen(false)
			}
		}

		dbw.OnClose = func() {
			for _, f := range cancelFuncs {
				f()
			}
			db.Close()
		}
		mw.wm.Add(dbw)
		if mw.startup {
			dbw.Move(fyne.NewPos(500, 0))
		}
	})
}
func (mw *MainWindow) startLogging() {
	device, err := mw.settings.CanSettings.GetAdapter(mw.selects.ecuSelect.Selected, mw.Log)
	if err != nil {
		d := dialog.NewError(err, mw)
		d.Show()
		return
	}

	mw.dlc, err = newDataLogger(mw, device)
	if err != nil {
		mw.Error(err)
		return
	}

	mw.loggingRunning = true

	mw.buttons.logBtn.Icon = theme.MediaStopIcon()
	mw.buttons.logBtn.SetText("Stop")
	mw.Disable()

	mw.canLED.On()
	go func() {
		if err := mw.dlc.Start(); err != nil {
			mw.Error(err)
		}
		mw.loggingRunning = false
		mw.dlc = nil
		fyne.Do(func() {
			mw.Enable()
			mw.buttons.logBtn.Icon = theme.MediaPlayIcon()
			mw.buttons.logBtn.SetText("Start")
			mw.canLED.Off()
			mw.counters.fpsCounterLabel.SetText("Fps: 0")
		})
	}()
}

func newDataLogger(mw *MainWindow, device gocan.Adapter) (datalogger.IClient, error) {
	return datalogger.New(datalogger.Config{
		FilenamePrefix: strings.TrimSuffix(filepath.Base(mw.filename), filepath.Ext(mw.filename)),
		ECU:            mw.selects.ecuSelect.Selected,
		Device:         device,
		Symbols:        mw.symbolList.Symbols(),
		Rate:           mw.settings.GetFreq(),
		OnMessage:      mw.Log,
		CaptureCounter: func(i int) {
			fyne.Do(func() {
				mw.counters.capturedCounterLabel.SetText("Cap: " + strconv.Itoa(i))
			})
		},
		ErrorCounter: func(i int) {
			fyne.Do(func() {
				mw.counters.errorCounterLabel.SetText("Err: " + strconv.Itoa(i))
			})
		},
		FpsCounter: func(i int) {
			fyne.Do(func() {
				mw.counters.fpsCounterLabel.SetText("Fps: " + strconv.Itoa(i))
			})
		},
		LogFormat: mw.settings.GetLogFormat(),
		LogPath:   mw.settings.GetLogPath(),
		WidebandConfig: datalogger.WidebandConfig{
			Type:                   mw.settings.GetWidebandType(),
			Port:                   mw.settings.GetWidebandPort(),
			MinimumVoltageWideband: mw.settings.GetMinimumVoltageWideband(),
			MaximumVoltageWideband: mw.settings.GetMaximumVoltageWideband(),
			LowAFR:                 mw.settings.GetLowAFR(),
			HighAFR:                mw.settings.GetHighAFR(),
		},
	})
}
