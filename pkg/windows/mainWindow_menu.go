package windows

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
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
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecu/t8/t8file"
	"github.com/roffe/txlogger/pkg/update"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/dtcreader"
	"github.com/roffe/txlogger/pkg/widgets/editparameters"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
	"github.com/roffe/txlogger/pkg/widgets/symbolbrowser"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmmod"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmstatus"
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
				inner := multiwindow.NewInnerWindow("About", About())
				inner.Icon = theme.HelpIcon()
				mw.wm.Add(inner)
			}),
			openItem,
			fyne.NewMenuItemWithIcon("Symbol Browser", theme.ListIcon(), func() {
				if w := mw.wm.HasWindow("Symbol Browser"); w != nil {
					mw.wm.Raise(w)
					return
				}
				getECU := func() symbol.ECUType {
					return symbol.ECUTypeFromString(mw.selects.ecuSelect.Selected)
				}
				browser := symbolbrowser.New(getFW, getECU, mw.openMap, mw.Error)
				inner := multiwindow.NewInnerWindow("Symbol Browser", browser)
				inner.Icon = theme.ListIcon()
				mw.wm.Add(inner)
				inner.Resize(fyne.Size{Width: 760, Height: 520})
			}),
			fyne.NewMenuItemWithIcon("Settings", theme.SettingsIcon(), func() {
				mw.openSettings()
			}),
			fyne.NewMenuItemWithIcon("What's new", theme.InfoIcon(), func() {
				mw.showWhatsNew()
			}),
			fyne.NewMenuItemWithIcon("Check for updates", theme.ViewRefreshIcon(), func() {
				update.UpdateCheck(mw.app, mw.Window)
			}),
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

	mw.leadingMenus = leading
	mw.trailingMenus = trailing
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
func (mw *MainWindow) newMapViewer(typ symbol.ECUType, mapName string) (*mapviewer.MapViewer, *mapviewer.Config, symbol.Axis, []func(), error) {
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
				realTemp := LookupCoolantTemperature(val, kyltempSteg.Ints(), kyltempTab.Ints())
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

	cfg := &mapviewer.Config{
		Name: symZ.Name,

		XData: xData,
		YData: yData,
		ZData: zData,

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

func (mw *MainWindow) openMap(typ symbol.ECUType, title string, mapName string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}

	mv, cfg, axis, cancelFuncs, err := mw.newMapViewer(typ, mapName)
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
		mv, cfg, axis, cancels, err := mw.newMapViewer(typ, strings.TrimSpace(name))
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

func LookupCoolantTemperature(axisvalue int, kyltempSteg, kyltempTab []int) int {
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
