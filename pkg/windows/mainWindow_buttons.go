package windows

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/skratchdot/open-golang/open"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) createButtons() {
	mw.addSymbolBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		sym := mw.symbols.GetByName(mw.symbolLookup.Text)
		if sym == nil {
			dialog.ShowError(fmt.Errorf("symbol not found"), mw)
			return
		}

		mw.symbolList.Add(sym)
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
	})

	mw.loadSymbolsEcuBtn = widget.NewButtonWithIcon("Load from ECU", theme.DownloadIcon(), func() {
		//		mw.progressBar.Start()
		mw.DisableBtns()
		go func() {
			defer mw.EnableBtns()
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
				mw.dlc.Detach(mw.dashboard)
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
			mw.dlc.Attach(mw.dashboard)
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
				mw.dlc.Detach(mw.dashboard)
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

		if err := open.Run(datalogger.LOGPATH); err != nil {
			fyne.LogError("Failed to open logs folder", err)
		}
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
		if !mw.loggingRunning {
			device, err := mw.canSettings.GetAdapter(mw.ecuSelect.Selected, mw.Log)
			if err != nil {
				mw.Log(err.Error())
				return
			}
			mw.dlc, err = datalogger.New(datalogger.Config{
				ECU:                   mw.ecuSelect.Selected,
				Device:                device,
				Symbols:               mw.symbolList.Symbols(),
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
			mw.DisableBtns()

			if mw.dashboard != nil {
				mw.dlc.Attach(mw.dashboard)
			}

			mw.dlc.Attach(mw.mvh)
			mw.dlc.Attach(mw.symbolList)

			go func() {
				defer mw.EnableBtns()
				if err := mw.dlc.Start(); err != nil {
					mw.Log(err.Error())
				}
				if mw.dashboard != nil {
					mw.dlc.Detach(mw.dashboard)
				}
				mw.dlc.Detach(mw.mvh)
				mw.loggingRunning = false
				mw.dlc = nil
				mw.logBtn.SetIcon(theme.MediaPlayIcon())
				mw.logBtn.SetText("Start logging")
			}()
		}
	})
}

func (mw *MainWindow) newMapViewerWindow(w fyne.Window, mv MapViewerWindowWidget, axis symbol.Axis) MapViewerWindowInterface {
	mww := &MapViewerWindow{Window: w, mv: mv}
	mw.openMaps[axis.Z] = mww

	if axis.XFrom == "" {
		axis.XFrom = "MAF.m_AirInlet"
	}

	if axis.YFrom == "" {
		axis.YFrom = "ActualIn.n_Engine"
	}

	mw.mvh.Subscribe(axis.XFrom, mv)
	mw.mvh.Subscribe(axis.YFrom, mv)
	return mww
}
