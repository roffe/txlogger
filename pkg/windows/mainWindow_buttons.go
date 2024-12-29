package windows

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/internal/snd"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	mw.buttons.addSymbolBtn = mw.addSymbolBtnFunc()
	mw.buttons.loadSymbolsEcuBtn = mw.loadSymbolsEcuBtnFunc()
	mw.buttons.syncSymbolsBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), mw.SyncSymbols)
	mw.buttons.dashboardBtn = mw.newDashboardBtn()
	mw.buttons.logplayerBtn = mw.newLogplayerBtn()
	mw.buttons.logBtn = mw.newLogBtn()
}

func (mw *MainWindow) addSymbolBtnFunc() *widget.Button {
	return widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		sym := mw.fw.GetByName(mw.symbolLookup.Text)
		if sym == nil {
			mw.Error(fmt.Errorf("%q not found", mw.symbolLookup.Text))
			return
		}
		mw.symbolList.Add(sym)
	})
}

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

func (mw *MainWindow) newLogplayerBtn() *widget.Button {
	return widget.NewButtonWithIcon("Play log", theme.MediaFastForwardIcon(), func() {
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
		mw.symbolList.UpdateBars(mw.settings.GetRealtimeBars())
	})
}

func (mw *MainWindow) newDashboardBtn() *widget.Button {
	return widget.NewButtonWithIcon("Dashboard", theme.InfoIcon(), func() {
		if mw.wm.HasWindow("Dashboard") {
			return
		}

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
			mw.SetContent(mw.content)
			mw.SetCloseIntercept(mw.CloseIntercept)
		}

		dbcfg := &dashboard.Config{
			App:       mw.app,
			Mw:        mw,
			Logplayer: false,
			//LogBtn:          mw.logBtn,
			OnClose:         onClose,
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
							if mw.oCtx != nil {
								if err := snd.Pedro(mw.oCtx); err != nil {
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

		//mw.SetContent(mw.dashboard)
		db := newInnerWindow("Dashboard", mw.dashboard)
		db.Icon = theme.InfoIcon()
		db.CloseIntercept = func() {
			onClose()
		}
		mw.wm.Add(db)
	})
}

func (mw *MainWindow) startLogging() {
	device, err := mw.settings.CanSettings.GetAdapter(mw.selects.ecuSelect.Selected, mw.Log)
	if err != nil {
		d := dialog.NewError(err, mw)
		d.Show()
		return
	}

	mw.dlc, err = mw.newDataLogger(device)
	if err != nil {
		mw.Error(err)
		return
	}

	mw.loggingRunning = true

	mw.buttons.logBtn.SetIcon(theme.MediaStopIcon())
	mw.buttons.logBtn.SetText("Stop")
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
			mw.Error(err)
			return
		}
		for _, f := range cancelFuncs {
			f()
		}
		mw.dlc = nil
		mw.loggingRunning = false
		mw.buttons.logBtn.SetIcon(theme.MediaPlayIcon())
		mw.buttons.logBtn.SetText("Start")
	}()
}

func (mw *MainWindow) newDataLogger(device gocan.Adapter) (datalogger.IClient, error) {
	return datalogger.New(datalogger.Config{
		ECU:            mw.selects.ecuSelect.Selected,
		Device:         device,
		Symbols:        mw.symbolList.Symbols(),
		Rate:           mw.settings.GetFreq(),
		OnMessage:      mw.Log,
		CaptureCounter: mw.counters.captureCounter,
		ErrorCounter:   mw.counters.errorCounter,
		FpsCounter:     mw.counters.fpsCounter,
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
