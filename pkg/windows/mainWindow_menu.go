package windows

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecu/t8/t8file"
	"github.com/roffe/txlogger/pkg/update"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/boosttuner"
	"github.com/roffe/txlogger/pkg/widgets/canflasher"
	"github.com/roffe/txlogger/pkg/widgets/dtcreader"
	"github.com/roffe/txlogger/pkg/widgets/editparameters"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/matrixbuilder"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
	"github.com/roffe/txlogger/pkg/widgets/rescaler"
	"github.com/roffe/txlogger/pkg/widgets/symbolbrowser"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmmod"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmstatus"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/t5cli"
	"github.com/roffe/txlogger/pkg/widgets/trionic7/t7esp"
)

func (mw *MainWindow) setupMenu() {
	getFW := func() symbol.SymbolCollection {
		return mw.fw
	}

	openItem := fyne.NewMenuItemWithIcon("Open", theme.FolderIcon(), nil)
	openItem.ChildMenu = fyne.NewMenu("File",
		fyne.NewMenuItemWithIcon("Open binary", theme.DocumentIcon(), mw.loadBinary),
		fyne.NewMenuItemWithIcon("Open log", theme.DocumentIcon(), func() {
			cb := func(r fyne.URIReadCloser) {
				defer r.Close()
				filename := r.URI().Name()
				mw.Log("opening logfile " + filename)
				sz := mw.Window.Content().Size()
				p := fyne.NewPos(sz.Width/2, sz.Height/2)
				mw.LoadLogfile(filename, r, p)
			}
			widgets.SelectFile(cb, "Log file", "csv", "bpl", "t5l", "t7l", "t8l")
		}),
		fyne.NewMenuItemWithIcon("Open log in new window", theme.DocumentIcon(), func() {
			cb := func(r fyne.URIReadCloser) {
				defer r.Close()
				filename := r.URI().Path()
				mw.LoadLogfileCombined(filename, r, fyne.Position{}, true)
			}
			widgets.SelectFile(cb, "logfile", "t5l", "t7l", "t8l", "csv", "bpl")
		}),
		fyne.NewMenuItemWithIcon("Open log folder", theme.FolderIcon(), func() {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "windows":
				cmd = exec.Command("explorer", mw.settings.GetLogPath())
			case "darwin":
				cmd = exec.Command("open", mw.settings.GetLogPath())
			default:
				cmd = exec.Command("xdg-open", mw.settings.GetLogPath())
			}
			if err := cmd.Start(); err != nil {
				mw.Error(err)
			}
		}),
		fyne.NewMenuItemWithIcon("Open AS2 file", theme.DocumentIcon(), func() {
			cb := func(r fyne.URIReadCloser) {
				defer r.Close()
				filename := r.URI().Path()
				mw.Log("Opening AS2 file " + filename)
				if err := mw.LoadAS2File(filename); err != nil {
					mw.Error(err)
				}
			}
			widgets.SelectFile(cb, "AS2 file", "as2")
		}),
	)

	leading := []*fyne.Menu{
		fyne.NewMenu("File",
			fyne.NewMenuItemWithIcon("About", theme.HelpIcon(), func() {
				if w := mw.wm.HasWindow("About"); w != nil {
					mw.wm.Raise(w)
					return
				}
				inner := multiwindow.NewInnerWindow("About", mw.about())
				inner.Icon = theme.HelpIcon()
				mw.wm.Add(inner)
			}),
			openItem,
			fyne.NewMenuItemWithIcon("Settings", theme.SettingsIcon(), mw.openSettings),
			fyne.NewMenuItemWithIcon("What's new", theme.InfoIcon(), mw.showWhatsNew),
			fyne.NewMenuItemWithIcon("Check for updates", theme.ViewRefreshIcon(), func() {
				update.UpdateCheck(mw.app, mw.Window)
			}),
		),
		fyne.NewMenu("Tools",
			fyne.NewMenuItemWithIcon("Symbol Browser", theme.ListIcon(), func() {
				if w := mw.wm.HasWindow("Symbol Browser"); w != nil {
					mw.wm.Raise(w)
					return
				}
				getECU := func() symbol.ECUType {
					return symbol.ECUTypeFromString(mw.selects.ecuSelect.Selected)
				}
				openMap := func(typ symbol.ECUType, title, mapName string) {
					mw.openMap(typ, title, mapName, "")
				}
				browser := symbolbrowser.New(getFW, getECU, openMap, mw.Error)
				inner := multiwindow.NewInnerWindow("Symbol Browser", browser)
				inner.Icon = theme.ListIcon()
				mw.wm.Add(inner)
				inner.Resize(fyne.Size{Width: 760, Height: 520})
			}),
			fyne.NewMenuItemWithIcon("Compare symbols with other binary", theme.SearchReplaceIcon(), mw.openSymbolCompare),
			fyne.NewMenuItemWithIcon("Matrix Builder", theme.InfoIcon(), mw.openMatrixBuilder),
			fyne.NewMenuItemWithIcon("T5 CLI", theme.ComputerIcon(), mw.openT5CLI),
			//fyne.NewMenuItemWithIcon("Rescale AccPedalMap", theme.GridIcon(), func() {
			//	mw.openRescaler(symbol.ECU_T8, "TrqMastCal.X_AccPedalMAP")
			//}),
		),
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

	if mw.previewFeatures {
		leading[len(leading)-1].Items = append(
			leading[len(leading)-1].Items,
			fyne.NewMenuItemWithIcon("Canflasher", theme.UploadIcon(), func() {
				if w := mw.wm.HasWindow("Canflasher"); w != nil {
					mw.wm.Raise(w)
					return
				}
				inner := multiwindow.NewInnerWindow("Canflasher", canflasher.New(&canflasher.Config{
					CSW: mw.settings,
				}))
				inner.Icon = theme.UploadIcon()
				mw.wm.Add(inner)
				inner.Resize(fyne.NewSize(450, 250))
			}),
			fyne.NewMenuItemWithIcon("T7 Boost Auto-Tuner", theme.MediaFastForwardIcon(), mw.openBoostTuner),
		)
	}

	mw.leadingMenus = leading
	mw.trailingMenus = trailing
}

func (mw *MainWindow) openT5CLI() {
	if w := mw.wm.HasWindow("T5 CLI"); w != nil {
		mw.wm.Raise(w)
		return
	}
	cli := t5cli.New(func() (gocan.Adapter, error) {
		return mw.settings.GetAdapter("T5")
	})
	inner := multiwindow.NewInnerWindow("T5 CLI", cli)
	inner.Icon = theme.ComputerIcon()
	inner.OnClose = cli.Close
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(700, 480))
}

func (mw *MainWindow) getAdapter() (gocan.Adapter, error) {
	device, err := mw.settings.GetAdapter(mw.selects.ecuSelect.Selected)
	if err != nil {
		mw.Error(err)
		return nil, err
	}
	return device, nil
}

func (mw *MainWindow) openDTCReader() {
	if w := mw.wm.HasWindow("DTC Reader"); w != nil {
		mw.wm.Raise(w)
		return
	}
	getFW := func() symbol.SymbolCollection { return mw.fw }
	getECU := func() string { return mw.selects.ecuSelect.Selected }
	inner := multiwindow.NewInnerWindow("DTC Reader", dtcreader.New(getFW, getECU, mw.getAdapter, mw.Log, mw.Error))
	inner.Icon = theme.InfoIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.Size{Width: 600, Height: 400})
}

func (mw *MainWindow) openEditParameters() {
	if w := mw.wm.HasWindow("Edit Parameters"); w != nil {
		mw.wm.Raise(w)
		return
	}
	param := editparameters.NewEditParameters(mw.getAdapter, mw.Error, mw.Log)
	inner := multiwindow.NewInnerWindow("Edit Parameters", param)
	inner.Icon = theme.InfoIcon()
	mw.wm.Add(inner)
}

func (mw *MainWindow) openRegisterEU0D() {
	if w := mw.wm.HasWindow("Register EU0D"); w != nil {
		mw.wm.Raise(w)
		return
	}
	inner := multiwindow.NewInnerWindow("Register EU0D", NewMyrtilosRegistration(mw))
	inner.Icon = theme.InfoIcon()
	mw.wm.Add(inner)
}

func (mw *MainWindow) openESPCalibration() {
	if w := mw.wm.HasWindow("ESP Calibration selection"); w != nil {
		mw.wm.Raise(w)
		return
	}
	t, ok := mw.fw.(*symbol.T7File)
	if !ok {
		mw.Error(errors.New("not a T7 file"))
		return
	}
	esp := t7esp.New(mw.filename, t)
	inner := multiwindow.NewInnerWindow("ESP Calibration selection", esp)
	inner.Icon = theme.InfoIcon()
	inner.DisableResize = true
	mw.wm.Add(inner)
}

func (mw *MainWindow) openFirmwareInfoEdit() {
	if w := mw.wm.HasWindow("Firmware info edit"); w != nil {
		mw.wm.Raise(w)
		return
	}
	tf := new(t8file.T8File)
	filename := fyne.CurrentApp().Preferences().String("lastBinFile")
	tf.GetInfo(filename)
	tf.ShowEditT8Dialog(mw)
}

func (mw *MainWindow) openPgmMod() {
	if w := mw.wm.HasWindow("Pgm_mod!"); w != nil {
		mw.wm.Raise(w)
		return
	}
	symZ := mw.fw.GetByName("Pgm_mod!")
	if symZ == nil {
		mw.Error(errors.New("Pgm_mod! symbol not found in loaded binary"))
		return
	}
	pgm := pgmmod.New()
	pgm.LoadFunc = func() ([]byte, error) {
		if mw.dlc != nil {
			log.Printf("Loading Pgm_mod! from ECU $%X", symZ.SramOffset)
			data, err := mw.dlc.GetRAM(symZ.SramOffset, uint32(symZ.Length))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
		log.Printf("Loading Pgm_mod! from Binary $%X", symZ.Address)
		return symZ.Bytes(), nil
	}

	pgm.SaveFunc = func(data []byte) error {
		if len(data) != int(symZ.Length) {
			return fmt.Errorf("data length mismatch: got %d, want %d", len(data), symZ.Length)
		}
		if mw.dlc != nil {
			log.Printf("Saving Pgm_mod! to ECU $%X", symZ.SramOffset)
			if err := mw.dlc.SetRAM(symZ.SramOffset, data); err != nil {
				return err
			}
			return nil
		}
		log.Printf("Saving Pgm_mod! to Binary $%X", symZ.Address)
		return symZ.SetData(data)
	}

	pgm.Set(symZ.Bytes())
	mapWindow := multiwindow.NewInnerWindow("Pgm_mod!", pgm)
	mapWindow.Icon = theme.GridIcon()
	mw.wm.Add(mapWindow)
}

func (mw *MainWindow) openPgmStatus() {
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

// newMapViewer builds a fully wired MapViewer for a single symbol (file/ECU
// load+save funcs, live X/Y crosshair subscriptions) but does not create a
// window. openMap wraps one; openMultiMap arranges several in a grid. The
// returned cancelFuncs must be called when the containing window closes.
func (mw *MainWindow) newMapViewer(typ symbol.ECUType, mapName string, regionMap string) (*mapviewer.MapViewer, *mapviewer.Config, symbol.Axis, []func(), error) {
	var axis symbol.Axis
	if mw.as2 != nil {
		axis.Z = mapName
		axes := mw.as2.Axes(mapName)
		if len(axes) == 0 {
			return nil, nil, axis, nil, fmt.Errorf("map %q not found in as2 file", mapName)
		}
		if len(axes) == 1 {
			axis.Y = axes[0].SupportPoints
			axis.YFrom = axes[0].Signal
		} else {
			for i, a := range axes {
				log.Printf("Axis %d: %+v", i, a)
				if i == 0 {
					axis.X = a.SupportPoints
					axis.XFrom = a.Signal
					continue
				}
				if i == 1 {
					axis.Y = a.SupportPoints
					axis.YFrom = a.Signal
					continue
				}
			}
		}
	} else {
		axis = symbol.GetInfo(typ, mapName)
	}

	symX := mw.fw.GetByName(axis.X)
	if symX == nil {
		switch axis.X {
		case "BstKnkCal.fi_offsetXSP":
			symX = mw.fw.GetByName("BstKnkCal.OffsetXSP")
			axis.X = "BstKnkCal.OffsetXSP"
		case "IgnStartCal.X_EthActSP":
			symX = mw.fw.GetByName("IgnStartCal.n_EngXSP")
			axis.X = "IgnStartCal.n_EngXSP"
			axis.XDescription = "Engine speed (rpm)"
		}
	}

	symY := mw.fw.GetByName(axis.Y)
	symZ := mw.fw.GetByName(axis.Z)

	if mw.as2 != nil {
		if symZ != nil {
			symZ.Correctionfactor = mw.as2.GetCorrectionfactor(mapName)
		}
	}

	if symZ == nil {
		return nil, nil, axis, nil, fmt.Errorf("failed to find symbol %s", axis.Z)
	}

	var xData, yData, zData []float64
	zData = symZ.Float64s()

	if symX != nil {
		xData = symX.Float64s()
	} else {
		xData = []float64{0}
	}

	if symY != nil {
		if strings.HasPrefix(symZ.Name, "Eftersta_fak") {
			valz := symZ.Ints()
			yData = make([]float64, len(valz))
			for idx, val := range symY.Ints() {
				kyltempSteg := mw.fw.GetByName("Kyltemp_steg!")
				kyltempTab := mw.fw.GetByName("Kyltemp_tab!")
				if kyltempSteg == nil || kyltempTab == nil {
					return nil, nil, axis, nil, fmt.Errorf("missing coolant temperature symbols")
				}
				realTemp := lookupCoolantTemperature(val, kyltempSteg.Ints(), kyltempTab.Ints())
				yData[idx] = float64(realTemp)
			}
		} else {
			yData = symY.Float64s()
		}
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
			// mw.Log(fmt.Sprintf("set $%d %s %s", addr, axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
			mw.Log(fmt.Sprintf("set %s $%X %dms", axis.Z, addr+uint32(idx*dataLen), time.Since(start).Truncate(10*time.Millisecond).Milliseconds()))
		}
	}

	loadRamFunc := func() {
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

	saveRamFunc := func(data []float64) {
		if mw.dlc == nil {
			return
		}
		start := time.Now()
		buff := bytes.NewBuffer(symZ.EncodeFloat64s(data))
		var startPos uint32
		switch mw.selects.ecuSelect.Selected {
		case "T5":
			startPos = symZ.SramOffset
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

		// mw.Log(fmt.Sprintf("save %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
		mw.Log(fmt.Sprintf("save %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
	}

	loadFileFunc := func() {
		if symZ != nil {
			// log.Println("load", symZ.Name)
			if err := mv.SetZData(symZ.Float64s()); err != nil {
				mw.Error(err)
				return
			}
			mv.Refresh()
		}
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

	if mw.as2 != nil {
		if symX != nil {
			xPrecision = mw.as2.Precision(axis.X)
			log.Printf("Precision for %s: %d", axis.X, xPrecision)
			//if xPrecision == 0 {
			//	log.Printf("Warning: precision for %s is 0, defaulting to 2", axis.X)
			//	xPrecision = 2
			//}
		}
		if symY != nil {
			yPrecision = mw.as2.Precision(axis.Y)
			log.Printf("Precision for %s: %d", axis.Y, yPrecision)
			//if yPrecision == 0 {
			//	log.Printf("Warning: precision for %s is 0, defaulting to 2", axis.Y)
			//	yPrecision = 2
			//}
		}
		zPrecision = mw.as2.Precision(axis.Z)
		log.Printf("Precision for %s: %d", axis.Z, zPrecision)
		//if zPrecision == 0 {
		//	log.Printf("Warning: precision for %s is 0, defaulting to 2", axis.Z)
		//	zPrecision = 2
		//}
	} else {
		if symX != nil {
			xPrecision = symbol.GetPrecision(symX.Correctionfactor)
		}
		if symY != nil {
			yPrecision = symbol.GetPrecision(symY.Correctionfactor)
		}
		zPrecision = symbol.GetPrecision(symZ.Correctionfactor)
	}

	switch mapName {
	case "TransCal.m_TriggMaxTab":
		yData = []float64{0, 500, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 4500, 5000, 5500, 6000, 6500}
	case "TransCal.FilterConstAir":
		yData = []float64{899, 2499, 3499, 3500}
	}

	var regionBorder []bool
	if regionMap != "" {
		regionBorder = mw.closedLoopRegion(typ, regionMap, xData, yData)
	}

	cfg := &mapviewer.Config{
		Name: symZ.Name,

		XData: xData,
		YData: yData,
		ZData: zData,

		RegionBorder: regionBorder,

		XPrecision: xPrecision,
		YPrecision: yPrecision,
		ZPrecision: zPrecision,

		XLabel: axis.XDescription,
		YLabel: axis.YDescription,
		ZLabel: axis.ZDescription,

		LoadFileFunc: loadFileFunc,
		SaveFileFunc: saveFileFunc,
		LoadECUFunc:  loadRamFunc,
		SaveECUFunc:  saveRamFunc,
		OnUpdateCell: updateFunc,

		MeshView:              mw.settings.GetMeshView(),
		MeshRenderer:          mw.settings.GetMeshRenderer(),
		Editable:              true,
		CursorFollowCrosshair: mw.settings.GetCursorFollowCrosshair(),
		ColorblindMode:        mw.settings.GetColorBlindMode(),
	}

	cfg.Buttons = []*mapviewer.MapViewerButton{
		{
			Label: "Load File",
			Icon:  theme.DocumentIcon(),
			OnTapped: func() {
				loadFileFunc()
			},
		},
		{
			Label: "Save File",
			Icon:  theme.DocumentSaveIcon(),
			OnTapped: func() {
				saveFileFunc(cfg.ZData)
			},
		},
		{
			Label: "Load ECU",
			Icon:  theme.DownloadIcon(),
			OnTapped: func() {
				p := progressmodal.New(fyne.CurrentApp().Driver().AllWindows()[0].Canvas(), "Loading map from ECU")
				p.Show()
				go func() {
					loadRamFunc()
					fyne.Do(func() {
						p.Hide()
					})
				}()
			},
		},
		{
			Label: "Save ECU",
			Icon:  theme.UploadIcon(),
			OnTapped: func() {
				p := progressmodal.New(fyne.CurrentApp().Driver().AllWindows()[0].Canvas(), "Saving map to ECU")
				p.Show()
				go func() {
					saveRamFunc(cfg.ZData)
					fyne.Do(func() {
						p.Hide()
					})
				}()
			},
		},
	}

	var err error
	mv, err = mapviewer.New(cfg)
	if err != nil {
		return nil, nil, axis, nil, err
	}

	var cancelFuncs []func()
	if axis.XFrom != "" {
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(axis.XFrom, mv.SetX))
	}
	if axis.YFrom != "" {
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(axis.YFrom, mv.SetY))
	}
	cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(ebus.TOPIC_COLORBLINDMODE, func(value float64) {
		mv.SetColorBlindMode(colors.ColorBlindMode(int(value)))
	}))

	return mv, cfg, axis, cancelFuncs, nil
}

func (mw *MainWindow) openMap(typ symbol.ECUType, title string, mapName string, regionMap string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}

	mv, cfg, axis, cancelFuncs, err := mw.newMapViewer(typ, mapName, regionMap)
	if err != nil {
		mw.Error(err)
		return
	}

	windowName := axis.Z
	if title != "" {
		windowName += " - " + title
	}
	if w := mw.wm.HasWindow(windowName); w != nil {
		mw.wm.Raise(w)
		for _, fn := range cancelFuncs {
			fn()
		}
		return
	}

	mapWindow := multiwindow.NewInnerWindow(axis.Z+" - "+axis.ZDescription, mv)
	mapWindow.Icon = theme.GridIcon()

	cfg.OnMouseDown = func() {
		mw.wm.Raise(mapWindow)
	}

	mapWindow.OnClose = func() {
		for _, fn := range cancelFuncs {
			fn()
		}
	}

	if mw.settings.GetAutoLoad() && mw.dlc != nil {
		go func() {
			openMapLock.Lock()
			defer openMapLock.Unlock()
			p := progressmodal.New(mw.Window.Canvas(), "Loading "+axis.Z)
			fyne.DoAndWait(p.Show)
			cfg.LoadECUFunc()
			fyne.Do(p.Hide)
		}()
	}

	mw.wm.Add(mapWindow)
}

// openMultiMap opens several maps (data = symbol names joined by "|") tightly
// arranged in one window for an at-a-glance overview, e.g. boost RegMap plus
// P/I/D factors. ponytail: plain 2-column grid of MapViewers, no dedicated
// widget — add one only if these views ever need shared crosshair/selection.
func (mw *MainWindow) openMultiMap(typ symbol.ECUType, title string, data string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}
	if w := mw.wm.HasWindow(title); w != nil {
		mw.wm.Raise(w)
		return
	}

	grid := container.NewGridWithColumns(2)
	var cfgs []*mapviewer.Config
	var loadFuncs []func()
	var cancelFuncs []func()

	for _, name := range strings.Split(data, "|") {
		mv, cfg, axis, cancels, err := mw.newMapViewer(typ, strings.TrimSpace(name), "")
		if err != nil {
			mw.Error(err)
			continue
		}
		cfgs = append(cfgs, cfg)
		cancelFuncs = append(cancelFuncs, cancels...)
		if cfg.LoadECUFunc != nil {
			loadFuncs = append(loadFuncs, cfg.LoadECUFunc)
		}
		label := axis.Z
		if axis.ZDescription != "" {
			label += " - " + axis.ZDescription
		}
		grid.Add(container.NewBorder(widget.NewLabel(label), nil, nil, nil, mv))
	}

	if len(grid.Objects) == 0 {
		return
	}

	mapWindow := multiwindow.NewInnerWindow(title, grid)
	mapWindow.Icon = theme.GridIcon()

	for _, cfg := range cfgs {
		cfg.OnMouseDown = func() {
			mw.wm.Raise(mapWindow)
		}
	}

	mapWindow.OnClose = func() {
		for _, fn := range cancelFuncs {
			fn()
		}
	}

	if mw.settings.GetAutoLoad() && mw.dlc != nil {
		go func() {
			openMapLock.Lock()
			defer openMapLock.Unlock()
			p := progressmodal.New(mw.Window.Canvas(), "Loading "+title)
			fyne.DoAndWait(p.Show)
			for _, fn := range loadFuncs {
				fn()
			}
			fyne.Do(p.Hide)
		}()
	}

	mw.wm.Add(mapWindow)
	mapWindow.Resize(fyne.NewSize(1000, 750))
}

// closedLoopRegion flags the cells of a fuel map (X = airmass, Y = rpm) that lie
// in the closed-loop area: airmass <= the per-rpm max load read from a LambdaCal
// MaxLoad table (regionMap). The table is indexed by its own rpm axis, so the
// limit is linearly interpolated onto each map rpm row. Result is row-major
// (rpmIdx*len(xData)+airIdx), matching ZData order. Returns nil if anything is
// missing so the caller simply skips the outline.
func (mw *MainWindow) closedLoopRegion(typ symbol.ECUType, regionMap string, xData, yData []float64) []bool {
	sym := mw.fw.GetByName(regionMap)
	if sym == nil {
		return nil
	}
	rpmSym := mw.fw.GetByName(symbol.GetInfo(typ, regionMap).Y)
	if rpmSym == nil {
		return nil
	}
	limitRpm := rpmSym.Float64s()
	limit := sym.Float64s()
	if len(limitRpm) == 0 || len(limit) != len(limitRpm) {
		return nil
	}
	region := make([]bool, len(xData)*len(yData))
	for r, rpm := range yData {
		maxAir := lookup1D(limitRpm, limit, rpm)
		for c, air := range xData {
			region[r*len(xData)+c] = air <= maxAir
		}
	}
	return region
}

// lookup1D does a clamped linear interpolation of ys over the (ascending) xs
// breakpoints. xs and ys must be the same non-zero length.
func lookup1D(xs, ys []float64, x float64) float64 {
	n := len(xs)
	if x <= xs[0] {
		return ys[0]
	}
	if x >= xs[n-1] {
		return ys[n-1]
	}
	for i := 1; i < n; i++ {
		if x <= xs[i] {
			t := (x - xs[i-1]) / (xs[i] - xs[i-1])
			return ys[i-1] + t*(ys[i]-ys[i-1])
		}
	}
	return ys[n-1]
}

func lookupCoolantTemperature(axisvalue int, kyltempSteg, kyltempTab []int) int {
	index := -1
	retval := -1
	smallestDiff := 256
	secondvalue := -1

	// find index in kyltempSteg
	for idx, v := range kyltempSteg {
		diff := int(math.Abs(float64(v - axisvalue)))
		if diff < smallestDiff {
			// need a neighbor to interpolate with
			if v < axisvalue {
				if idx+1 >= len(kyltempSteg) {
					continue
				}
				secondvalue = kyltempSteg[idx+1]
			} else {
				if idx-1 < 0 {
					continue
				}
				secondvalue = kyltempSteg[idx-1]
			}
			index = idx
			smallestDiff = diff
		}
	}

	if index >= 0 && index < len(kyltempTab) && secondvalue != -1 {
		// get value from kyltempTab
		retval = kyltempTab[index]
		firstvalue := kyltempSteg[index]

		// sval := -1000
		diff := int(math.Abs(float64(secondvalue - firstvalue)))
		if diff == 0 {
			// avoid div by zero; just return base value minus 40 like original end
			return retval - 40
		}

		diff2 := axisvalue - firstvalue
		percentage := float64(diff2) / float64(diff)

		var sval int
		if secondvalue > firstvalue {
			// need the next value from kyltempTab as well
			if index+1 >= len(kyltempTab) {
				return retval - 40
			}
			sval = kyltempTab[index+1]
		} else {
			if index-1 < 0 {
				return retval - 40
			}
			sval = kyltempTab[index-1]
			percentage = -percentage
		}

		// interpolate
		retval += int(percentage * float64(sval-retval))
		retval -= 40
	}

	return retval
}

// openBoostTuner opens (or raises) the T7 boost auto-tuner. It reads the current
// BoostCal maps from the loaded binary and writes tuned maps back through a save
// closure that takes a one-time .bak of the file before the first write.
func (mw *MainWindow) openBoostTuner() {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}
	if w := mw.wm.HasWindow("Boost Auto-Tuner"); w != nil {
		mw.wm.Raise(w)
		return
	}
	save := func(symbolName string, data []float64) error {
		sym := mw.fw.GetByName(symbolName)
		if sym == nil {
			return fmt.Errorf("symbol %s not found", symbolName)
		}
		if err := sym.SetData(sym.EncodeFloat64s(data)); err != nil {
			return err
		}
		if mw.filename != "" {
			if bak := mw.filename + ".bak"; !fileExists(bak) {
				if orig, err := os.ReadFile(mw.filename); err == nil {
					_ = os.WriteFile(bak, orig, 0o644)
				}
			}
		}
		return mw.fw.Save(mw.filename)
	}
	bt := boosttuner.New(boosttuner.Config{
		Symbols:      mw.fw,
		Save:         save,
		MeshRenderer: mw.settings.GetMeshRenderer(),
		Colorblind:   mw.settings.GetColorBlindMode(),
	})
	inner := multiwindow.NewInnerWindow("Boost Auto-Tuner", bt)
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(1100, 760))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// openMatrixBuilder opens (or raises) the matrix builder window. The builder
// loads its own log files, so it is independent of any open log player.
func (mw *MainWindow) openMatrixBuilder() {
	if w := mw.wm.HasWindow("Matrix builder"); w != nil {
		mw.wm.Raise(w)
		return
	}
	inner := multiwindow.NewInnerWindow("Matrix builder", matrixbuilder.New(mw.settings.GetMeshRenderer()))
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(1000, 720))
}

// openRescaler opens (or raises) the map rescaler for a single map. It reads the
// map and its X/Y axes from the loaded binary, lets the user edit the axis
// support points, resamples the surface onto them, and writes the result back
// through a save closure that takes a one-time .bak before the first write.
func (mw *MainWindow) openRescaler(typ symbol.ECUType, mapName string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}
	winName := "Rescale " + mapName
	if w := mw.wm.HasWindow(winName); w != nil {
		mw.wm.Raise(w)
		return
	}

	axis := symbol.GetInfo(typ, mapName)
	symX := mw.fw.GetByName(axis.X)
	symY := mw.fw.GetByName(axis.Y)
	symZ := mw.fw.GetByName(axis.Z)
	if symZ == nil || symY == nil {
		mw.Error(fmt.Errorf("rescaler: missing symbol(s) for %s", mapName))
		return
	}

	xData := []float64{0}
	xPrecision := 0
	if symX != nil {
		xData = symX.Float64s()
		xPrecision = symbol.GetPrecision(symX.Correctionfactor)
	}

	cfg := &rescaler.Config{
		Name:       axis.Z,
		XLabel:     axis.X,
		YLabel:     axis.Y,
		ZLabel:     axis.Z,
		XData:      xData,
		YData:      symY.Float64s(),
		ZData:      symZ.Float64s(),
		XPrecision: xPrecision,
		YPrecision: symbol.GetPrecision(symY.Correctionfactor),
		ZPrecision: symbol.GetPrecision(symZ.Correctionfactor),
		Apply: func(newX, newY, newZ []float64) error {
			if symX != nil {
				if err := symX.SetData(symX.EncodeFloat64s(newX)); err != nil {
					return err
				}
			}
			if err := symY.SetData(symY.EncodeFloat64s(newY)); err != nil {
				return err
			}
			if err := symZ.SetData(symZ.EncodeFloat64s(newZ)); err != nil {
				return err
			}
			if mw.filename != "" {
				if bak := mw.filename + ".bak"; !fileExists(bak) {
					if orig, err := os.ReadFile(mw.filename); err == nil {
						_ = os.WriteFile(bak, orig, 0o644)
					}
				}
			}
			if err := mw.fw.Save(mw.filename); err != nil {
				return err
			}
			mw.Log("Rescaled and saved " + axis.Z)
			return nil
		},
	}

	inner := multiwindow.NewInnerWindow(winName, rescaler.New(cfg))
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(900, 720))
}
