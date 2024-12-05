package windows

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/dashboard"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/plotter"
	"github.com/roffe/txlogger/pkg/snd"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	mw.addSymbolBtn = mw.addSymbolBtnFunc()
	mw.loadSymbolsFileBtn = mw.loadSymbolsFileBtnFunc()
	mw.loadSymbolsEcuBtn = mw.loadSymbolsEcuBtnFunc()
	mw.helpBtn = widget.NewButtonWithIcon("Help", theme.HelpIcon(), func() {
		Help(mw.app)
	})
	mw.syncSymbolsBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), mw.SyncSymbols)
	mw.dashboardBtn = mw.newDashboardBtn()
	mw.plotterBtn = mw.newPlotterBtn()
	mw.logplayerBtn = mw.newLogplayerBtn()
	mw.logBtn = mw.newLogBtn()
	mw.settingsBtn = mw.newSettingsBtn()
}

func (mw *MainWindow) addSymbolBtnFunc() *widget.Button {
	return widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		sym := mw.fw.GetByName(mw.symbolLookup.Text)
		if sym == nil {
			dialog.ShowError(fmt.Errorf("%q not found", mw.symbolLookup.Text), mw)
			return
		}
		mw.symbolList.Add(sym)
	})
}

func (mw *MainWindow) newPlotterBtn() *widget.Button {
	return widget.NewButtonWithIcon("Plotter", theme.VisibilityIcon(), func() {
		quit := make(chan struct{})
		onClose := func() {
			close(quit)
			mw.plotter = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.tab)
			mw.SetCloseIntercept(mw.CloseIntercept)
		}

		values := make(map[string][]float64)
		for _, sym := range mw.symbolList.Symbols() {
			values[sym.Name] = append(values[sym.Name], sym.Float64())
		}

		mw.plotter = plotter.NewPlotter(values)

		mw.SetCloseIntercept(func() {
			onClose()
		})

		mw.SetContent(mw.plotter)
	})
}

func (mw *MainWindow) loadSymbolsFileBtnFunc() *widget.Button {
	return widget.NewButtonWithIcon("Load binary", theme.FileIcon(), func() {
		filename, err := sdialog.File().Filter("Binary file", "bin").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			mw.Log(err.Error())
			return
		}
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			mw.Log(err.Error())
			return
		}
		mw.app.Preferences().SetString(prefsLastBinFile, filename)
		mw.filename = filename
	})
}

func (mw *MainWindow) loadSymbolsEcuBtnFunc() *widget.Button {
	return widget.NewButtonWithIcon("Load from ECU", theme.DownloadIcon(), func() {
		mw.Disable()
		go func() {
			defer mw.Enable()
			if err := mw.LoadSymbolsFromECU(); err != nil {
				mw.Log(err.Error())
				return
			}
		}()
	})
}

func (mw *MainWindow) newLogplayerBtn() *widget.Button {
	return widget.NewButtonWithIcon("Log Player", theme.MediaFastForwardIcon(), func() {
		filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").SetStartDir(mw.settings.GetLogPath()).Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			mw.Log(err.Error())
			return
		}

		go NewLogPlayer(mw.app, filename, mw.fw)
	})
}

func (mw *MainWindow) newLogBtn() *widget.Button {
	return widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		if mw.loggingRunning {
			if mw.dlc != nil {
				mw.dlc.Close()
				time.Sleep(200 * time.Millisecond)
			}
			return
		}
		for _, v := range mw.symbolList.Symbols() {
			if v.Name == "AirMassMast.m_Request" && mw.ecuSelect.Selected == "T7" {
				dialog.ShowError(fmt.Errorf("AirMassMast.m_Request is not supported on T7, Did you forget to change preset?"), mw)
				return
			}
			if v.Name == "m_Request" && mw.ecuSelect.Selected == "T8" {
				dialog.ShowError(fmt.Errorf("m_Request is not supported on T8, Did you forget to change preset?"), mw)
				return
			}
			v.Skip = false
		}
		mw.startLogging()
		mw.symbolList.UpdateBars(mw.settings.GetRealtimeBars())
	})
}

func (mw *MainWindow) newDashboardBtn() *widget.Button {
	return widget.NewButtonWithIcon("Dashboard", theme.InfoIcon(), func() {
		var cancelFuncs []func()

		onClose := func() {
			for _, f := range cancelFuncs {
				f()
			}
			if mw.dashboard != nil {
				mw.dashboard.Close()
			}
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.tab)
			mw.SetCloseIntercept(mw.CloseIntercept)
		}

		dbcfg := &dashboard.Config{
			App:             mw.app,
			Mw:              mw,
			Logplayer:       false,
			LogBtn:          mw.logBtn,
			OnClose:         onClose,
			UseMPH:          mw.settings.GetUseMPH(),
			SwapRPMandSpeed: mw.settings.GetSwapRPMandSpeed(),
			HighAFR:         mw.settings.GetHighAFR(),
			LowAFR:          mw.settings.GetLowAFR(),
			WidebandSymbol:  mw.settings.GetWidebandSymbolName(),
		}

		switch mw.ecuSelect.Selected {
		case "T7":
			dbcfg.AirDemToString = datalogger.AirDemToStringT7
		case "T8":
			dbcfg.AirDemToString = datalogger.AirDemToStringT8
		}

		mw.dashboard = dashboard.NewDashboard(dbcfg)

		//for _, s := range mw.symbolList.Symbols() {
		//	mw.dashboard.SetValue(s.Name, s.Float64())
		//}

		mw.SetCloseIntercept(func() {
			onClose()
		})

		for _, m := range mw.dashboard.GetMetricNames() {
			if m == "MAF.m_AirInlet" {
				var once sync.Once
				cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(m, func(f float64) {
					if f >= 1800 {
						once.Do(func() {
							if mw.otoCtx != nil {
								if err := snd.Pedro(mw.otoCtx); err != nil {
									mw.Log("Pedro failed: " + err.Error())
								}
							}
						})
					}
					mw.dashboard.SetValue(m, f)
				}))
				continue
			}

			cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(m, func(f float64) {
				mw.dashboard.SetValue(m, f)
			}))
		}

		mw.SetContent(mw.dashboard)
	})
}

func (mw *MainWindow) newSettingsBtn() *widget.Button {
	btn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		mw.SetContent(mw.settings)
		mw.SetCloseIntercept(func() {
			mw.SetCloseIntercept(mw.CloseIntercept)
			mw.SetContent(mw.tab)
		})
	})
	return btn
}

func (mw *MainWindow) startLogging() {
	device, err := mw.settings.CanSettings.GetAdapter(mw.ecuSelect.Selected, mw.Log)
	if err != nil {
		mw.Log(err.Error())
		return
	}

	mw.dlc, err = mw.newDataLogger(device)
	if err != nil {
		mw.Log(err.Error())
		return
	}

	mw.loggingRunning = true

	mw.logBtn.SetIcon(theme.MediaStopIcon())
	mw.logBtn.SetText("Stop")
	mw.Disable()

	//var cancel func()
	var cancelFuncs []func()

	if mw.settings.GetLivePreview() {
		for _, sym := range mw.symbolList.Symbols() {
			cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(sym.Name, func(f float64) {
				mw.symbolList.SetValue(sym.Name, f)
			}))
		}

		//cancel = ebus.SubscribeAllFunc(mw.symbolList.SetValue)
	} else {
		mw.symbolList.Clear()
	}

	go func() {
		defer mw.Enable()
		if err := mw.dlc.Start(); err != nil {
			mw.Log(err.Error())
		}
		for _, f := range cancelFuncs {
			f()
		}
		mw.dlc = nil
		mw.loggingRunning = false
		mw.logBtn.SetIcon(theme.MediaPlayIcon())
		mw.logBtn.SetText("Start")
	}()
}

func (mw *MainWindow) newDataLogger(device gocan.Adapter) (datalogger.IClient, error) {
	return datalogger.New(datalogger.Config{
		ECU:            mw.ecuSelect.Selected,
		Device:         device,
		Symbols:        mw.symbolList.Symbols(),
		Rate:           mw.settings.GetFreq(),
		OnMessage:      mw.Log,
		CaptureCounter: mw.captureCounter,
		ErrorCounter:   mw.errorCounter,
		FpsCounter:     mw.fpsCounter,
		LogFormat:      mw.settings.GetLogFormat(),
		LogPath:        mw.settings.GetLogPath(),
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
