package windows

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/widgets"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	mw.addSymbolBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		sym := mw.fw.GetByName(mw.symbolLookup.Text)
		if sym == nil {
			dialog.ShowError(fmt.Errorf("symbol not found"), mw)
			return
		}

		mw.symbolList.Add(sym)
		mw.SaveSymbolList()
		//log.Printf("Name: %s, Method: %d, Value: %d, Type: %X", s.Name, s.Method, s.Value, s.Type)
	})

	mw.loadSymbolsFileBtn = widget.NewButtonWithIcon("Load from binary", theme.FileIcon(), func() {
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
				mw.dlc.Attach(mw.symbolList)
				mw.dlc.Detach(mw.dashboard)
			}
			if mw.dashboard != nil {
				mw.dashboard.Close()
			}
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.render())
			mw.SetCloseIntercept(mw.closeIntercept)
		}

		mw.dashboard = widgets.NewDashboard(mw.app, mw, false, mw.logBtn, onClose)
		if mw.dlc != nil {
			mw.dlc.Attach(mw.dashboard)
		}

		if mw.dlc != nil {
			mw.dlc.Detach(mw.symbolList)
		}

		mw.SetCloseIntercept(func() {
			onClose()
		})

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
				mw.dlc.Detach(mw.dashboard)
			}
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.Content())
		}
		go NewLogPlayer(mw.app, filename, mw.fw, onClose)
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

	if mw.settings.GetLivePreview() {
		// Attach our symbol list message handler
		mw.dlc.Attach(mw.symbolList)
	}

	if mw.dashboard != nil {
		// Attach our dashboard message handler
		mw.dlc.Attach(mw.dashboard)
	}

	// Attach our mapview message handler
	mw.dlc.Attach(mw.mvh)

	go mw.logDaemon()
}

func (mw *MainWindow) newDataLogger(device gocan.Adapter) (datalogger.Logger, error) {
	return datalogger.New(datalogger.Config{
		ECU:            mw.ecuSelect.Selected,
		Device:         device,
		Symbols:        mw.symbolList.Symbols(),
		Rate:           mw.settings.GetFreq(),
		OnMessage:      mw.Log,
		CaptureCounter: mw.captureCounter,
		ErrorCounter:   mw.errorCounter,
	})
}

func (mw *MainWindow) logDaemon() {
	defer mw.Enable()
	if err := mw.dlc.Start(); err != nil {
		mw.Log(err.Error())
	}
	if mw.dashboard != nil {
		mw.dlc.Detach(mw.dashboard)
	}
	mw.dlc.Detach(mw.mvh)
	mw.dlc.Detach(mw.symbolList)
	mw.dlc = nil
	mw.loggingRunning = false
	mw.logBtn.SetIcon(theme.MediaPlayIcon())
	mw.logBtn.SetText("Start logging")
}
