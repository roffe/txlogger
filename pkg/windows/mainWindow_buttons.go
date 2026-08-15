package windows

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/liveplotter"
	"github.com/roffe/txlogger/pkg/widgets/msglist"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

func (mw *MainWindow) createButtons() {
	mw.buttons.dashboardBtn = mw.newDashboardBtn()
	mw.buttons.logBtn = mw.newLogBtn()
	mw.buttons.layoutRefreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		mw.selects.layoutSelect.SetOptions(listLayouts())
	})
	mw.buttons.debugBtn = mw.newDebugBtn()
	mw.buttons.symbolListBtn = mw.newSymbolListBtn()
	mw.buttons.addGaugeBtn = mw.newaddGaugeBtn()
	mw.buttons.livePlotBtn = mw.newLivePlotBtn()
}

func (mw *MainWindow) newLivePlotBtn() *widget.Button {
	return widget.NewButtonWithIcon("Live plot", theme.MediaSkipNextIcon(), func() {
		if w := mw.wm.HasWindow("Live plot"); w != nil {
			mw.wm.Raise(w)
			return
		}

		names := mw.symbolList.Names()
		if len(names) == 0 {
			mw.Error(fmt.Errorf("no symbols selected to plot"))
			return
		}

		lp := liveplotter.New(&liveplotter.Config{
			Order:  names,
			Window: 120 * time.Second,
		})

		lpw := multiwindow.NewInnerWindow("Live plot", lp)
		lpw.Icon = theme.MediaSkipNextIcon()
		lpw.OnClose = lp.Close
		mw.wm.Add(lpw)
		lpw.Resize(fyne.NewSize(900, 500))
	})
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

		symbolListWindow := multiwindow.NewSystemWindow("Symbol list", mw.symbolList)
		symbolListWindow.Icon = theme.ListIcon()
		symbolListWindow.IgnoreSave = false

		// Fixa så livepreview values kan togglas på/av. med nya ui't så funkar det inte som det ska

		mw.wm.Add(symbolListWindow)
		if mw.startup {
			symbolListWindow.Move(fyne.NewPos(0, 0))
			symbolListWindow.Resize(fyne.NewSize(300, 550))
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
			if mw.dlcCancel != nil {
				mw.dlcCancel()
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
			Logplayer:      false,
			UseMPH:         mw.settings.GetUseMPH(),
			ClassicGauges:  mw.settings.GetClassicGauges(),
			High:           1.5,
			Low:            0.5,
			WidebandSymbol: mw.settings.GetWidebandSymbolName(),
		}

		switch mw.selects.ecuSelect.Selected {
		case "T7":
			dbcfg.AirDemToString = datalogger.AirDemToStringT7
		case "T8":
			dbcfg.AirDemToString = datalogger.AirDemToStringT8
		}

		db := dashboard.NewDashboard(dbcfg)

		// The dashboard routes the wideband gauge under the symbol name it was
		// built with. Feeding that route separately lets it follow the wideband
		// source as it changes in settings, instead of needing a reopen.
		wblRoute := dbcfg.WidebandSymbol

		var cancelFuncs []func()
		for _, m := range db.GetMetricNames() {
			if m == wblRoute {
				continue
			}
			cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(m, func(f float64) {
				db.SetValue(m, f)
			}))
		}

		var wblCancel func()
		followWBL := func() {
			if wblCancel != nil {
				wblCancel()
			}
			wblCancel = ebus.SubscribeFunc(mw.settings.GetWidebandSymbolName(), func(f float64) {
				db.SetValue(wblRoute, f)
			})
		}
		followWBL()

		// the ECU type is part of what the wideband setting resolves to
		cancelFuncs = append(cancelFuncs,
			ebus.SubscribeFunc(ebus.TOPIC_WBLSYMBOL, func(float64) { followWBL() }),
			ebus.SubscribeFunc(ebus.TOPIC_ECU, func(float64) { followWBL() }),
		)

		dbw := multiwindow.NewInnerWindow("Dashboard", db)
		dbw.Icon = theme.InfoIcon()

		dbcfg.FullscreenFunc = func(b bool) {
			if b {
				mw.SetMainMenu(nil)
				mw.Window.SetContent(db)
				mw.SetFullScreen(true)
			} else {
				mw.SetMainMenu(mw.GetMenu(mw.selects.ecuSelect.Selected))
				mw.Window.SetContent(mw.content)
				dbw.Close()
				mw.buttons.dashboardBtn.OnTapped()
				mw.SetFullScreen(false)
			}
		}

		dbw.OnClose = func() {
			wblCancel()
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
	if mw.symbolList.Count() == 0 {
		mw.Error(fmt.Errorf("no symbols selected for logging"))
		return
	}
	var device gocan.Adapter
	var err error
	deviceName := mw.selects.remoteSelect.Selected

	if mw.selects.remoteSelect.SelectedIndex() < 2 {
		device, err = mw.settings.GetAdapter(mw.selects.ecuSelect.Selected)
		if err != nil {
			mw.Error(err)
			return
		}
		deviceName = gocan.AdapterName(device)
	}

	if mw.selects.ecuSelect.Selected == "T5" {
		if strings.Contains(deviceName, "J2534") || strings.Contains(deviceName, "ELM327") {
			mw.Error(fmt.Errorf("%s is not supported for T5", deviceName))
			return
		}
	}

	mw.dlc, _, err = newDataLogger(mw, device)
	if err != nil {
		mw.Error(err)
		return
	}

	mw.loggingRunning = true

	mw.buttons.logBtn.Icon = theme.MediaStopIcon()
	mw.buttons.logBtn.SetText("Stop")
	mw.Disable()
	mw.canLED.On()
	ctx, cancel := context.WithCancel(context.Background())
	mw.dlcCancel = cancel
	go func() {
		defer cancel()
		mw.Log("Connecting to " + deviceName)
		if err := mw.dlc.Start(ctx); err != nil {
			mw.Error(err)
		}
		mw.Log(deviceName + " disconnected")
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

func newDataLogger(mw *MainWindow, device gocan.Adapter) (datalogger.IClient, string, error) {
	return datalogger.New(datalogger.Config{
		FilenamePrefix: strings.TrimSuffix(filepath.Base(mw.filename), filepath.Ext(mw.filename)),
		ECU:            mw.selects.ecuSelect.Selected,
		Device:         device,
		Symbols:        mw.symbolList.Symbols(),
		SeedKey:        mw.seedKey,
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
			Name:            mw.settings.GetWidebandName(),
			Port:            mw.settings.GetWidebandPort(),
			ADScanner:       mw.settings.GetUseADScanner(),
			ADScannerSymbol: mw.settings.GetADScannerSymbolName(),
			SupportPoints:   mw.settings.GetWBLSupportPoints(),
			LambdaValues:    mw.settings.GetWBLLambdaValues(),
		},
		// Remote: mw.selects.remoteSelect.Selected == "Remote",
		RemoteMode:                mw.selects.remoteSelect.SelectedIndex(),
		ExperimentalT5FastLogging: mw.settings.GetExperimentalT5FastLogger(),
	})
}
