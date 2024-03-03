package windows

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	mw.addSymbolBtn = widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		sym := mw.fw.GetByName(mw.symbolLookup.Text)
		if sym == nil {
			dialog.ShowError(fmt.Errorf("%q not found", mw.symbolLookup.Text), mw)
			return
		}

		mw.symbolList.Add(sym)
		mw.SaveSymbolList()
		//log.Printf("Name: %s, Method: %d, Value: %d, Type: %X", s.Name, s.Method, s.Value, s.Type)
	})

	mw.loadSymbolsFileBtn = widget.NewButtonWithIcon("Load binary", theme.FileIcon(), func() {
		// d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		// 	if err != nil {
		// 		// dialog.ShowError(err, mw)
		// 		mw.Log(err.Error())
		// 		return
		// 	}
		// 	if reader == nil {
		// 		return
		// 	}
		// 	defer reader.Close()
		// 	log.Println("lol")
		// }, mw)
		// d.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
		// d.Resize(mw.Window.Canvas().Size())
		// d.Show()
		// return

		filename, err := sdialog.File().Filter("Binary file", "bin").SetStartDir(mw.settings.GetLogPath()).Load()
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
		mw.app.Preferences().SetString(prefsLastBinFile, filename)
		mw.filename = filename
	})

	mw.loadSymbolsEcuBtn = widget.NewButtonWithIcon("Load from ECU", theme.DownloadIcon(), func() {
		//		mw.progressBar.Start()
		mw.Disable()
		go func() {
			defer mw.Enable()
			//		defer mw.progressBar.Stop()
			if err := mw.LoadSymbolsFromECU(); err != nil {
				// dialog.ShowError(err, mw)
				mw.Log(err.Error())
				return
			}
		}()
	})

	mw.loadConfigBtn = widget.NewButtonWithIcon("Load symbols", theme.FileIcon(), func() {
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
		mw.SyncSymbols()
	})

	mw.saveConfigBtn = widget.NewButtonWithIcon("Save symbols", theme.DocumentSaveIcon(), func() {
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
		var unsubDB func()

		onClose := func() {
			if unsubDB != nil {
				unsubDB()
			}
			if mw.dashboard != nil {
				mw.dashboard.Close()
			}
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.render())
			mw.SetCloseIntercept(mw.closeIntercept)
		}

		dbcfg := &widgets.DashboardConfig{
			App:       mw.app,
			Mw:        mw,
			Logplayer: false,
			LogBtn:    mw.logBtn,
			OnClose:   onClose,
			UseMPH:    mw.settings.GetUseMPH(),
		}

		switch mw.ecuSelect.Selected {
		case "T7":
			dbcfg.AirDemToString = datalogger.AirDemToStringT7
		case "T8":
			dbcfg.AirDemToString = datalogger.AirDemToStringT8
		}

		mw.dashboard = widgets.NewDashboard(dbcfg)

		unsubDB = ebus.SubscribeAllFunc(mw.dashboard.SetValue)

		mw.SetCloseIntercept(func() {
			onClose()
		})

		mw.SetContent(mw.dashboard)
	})

	mw.logplayerBtn = widget.NewButtonWithIcon("Log Player", theme.MediaFastForwardIcon(), func() {
		filename, err := sdialog.File().Filter("logfile", "t7l", "t8l", "csv").SetStartDir("logs").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}

		go NewLogPlayer(mw.app, filename, mw.fw)
	})

	mw.logBtn = widget.NewButtonWithIcon("Start logging", theme.MediaPlayIcon(), func() {
		for _, v := range mw.symbolList.Symbols() {
			if v.Name == "AirMassMast.m_Request" && mw.ecuSelect.Selected == "T7" {
				dialog.ShowError(fmt.Errorf("AirMassMast.m_Request is not supported on T7, Did you forget to change preset?"), mw)
				return
			}
			if v.Name == "m_Request" && mw.ecuSelect.Selected == "T8" {
				dialog.ShowError(fmt.Errorf("m_Request is not supported on T8, Did you forget to change preset?"), mw)
				return
			}
		}
		if mw.loggingRunning {
			if mw.dlc != nil {
				mw.dlc.Close()
			}
			return
		}
		mw.startLogging()
		mw.symbolList.UpdateBars(mw.settings.GetRealtimeBars())
	})
	mw.settingsBtn = mw.newSettingsBtn()
}

func (mw *MainWindow) newSettingsBtn() *widget.Button {
	btn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		mw.SetContent(mw.settings)
		mw.SetCloseIntercept(func() {
			mw.SetCloseIntercept(mw.closeIntercept)
			mw.SetContent(mw.render())
		})
	})
	return btn
}

func (mw *MainWindow) startLogging() {
	device, err := mw.canSettings.GetAdapter(mw.ecuSelect.Selected, mw.Log)
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
	mw.logBtn.SetText("Stop logging")
	mw.Disable()

	var cancel func()

	if mw.settings.GetLivePreview() {
		cancel = ebus.SubscribeAllFunc(mw.symbolList.SetValue)
	}

	go func() {
		defer mw.Enable()
		if err := mw.dlc.Start(); err != nil {
			mw.Log(err.Error())
		}
		if cancel != nil {
			cancel()
		}
		mw.dlc = nil
		mw.loggingRunning = false
		mw.logBtn.SetIcon(theme.MediaPlayIcon())
		mw.logBtn.SetText("Start logging")
	}()
}

func (mw *MainWindow) newDataLogger(device gocan.Adapter) (datalogger.Logger, error) {
	return datalogger.New(datalogger.Config{
		ECU:            mw.ecuSelect.Selected,
		Lambda:         mw.settings.GetLambdaSource(),
		Device:         device,
		Symbols:        mw.symbolList.Symbols(),
		Rate:           mw.settings.GetFreq(),
		OnMessage:      mw.Log,
		CaptureCounter: mw.captureCounter,
		ErrorCounter:   mw.errorCounter,
		FpsCounter:     mw.fpsCounter,
		LogFormat:      mw.settings.GetLogFormat(),
		LogPath:        mw.settings.GetLogPath(),
	})
}
