package windows

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmmod"
	"github.com/roffe/txlogger/pkg/widgets/trionic5/pgmstatus"
	"github.com/roffe/txlogger/pkg/widgets/txupdater"
	"github.com/skratchdot/open-golang/open"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) setupMenu() {
	otherFunc := func(str string) {
		switch str {
		case "Register EU0D":
			if w := mw.wm.HasWindow("Register EU0D"); w != nil {
				mw.wm.Raise(w)
				return
			}
			inner := multiwindow.NewInnerWindow("Register EU0D", NewMyrtilosRegistration(mw))
			inner.Icon = theme.InfoIcon()
			mw.wm.Add(inner)
		}
	}

	leading := []*fyne.Menu{
		fyne.NewMenu("File",
			fyne.NewMenuItem("About", func() {
				if w := mw.wm.HasWindow("About"); w != nil {
					mw.wm.Raise(w)
					return
				}
				inner := multiwindow.NewInnerWindow("About", About())
				inner.Icon = theme.HelpIcon()
				mw.wm.Add(inner)
			}),
			fyne.NewMenuItem("Open binary", mw.loadBinary),
			fyne.NewMenuItem("Open log", func() {
				filename, err := sdialog.File().Filter("Log file", "csv", "t5l", "t7l", "t8l").Load()
				if err != nil {
					if err.Error() == "Cancelled" {
						return
					}
					mw.Error(err)
					return
				}
				mw.LoadLogfile(filename, fyne.NewPos(10, 10))
			}),
			fyne.NewMenuItem("Open log folder", func() {
				if err := open.Run(mw.settings.GetLogPath()); err != nil {
					mw.Error(fmt.Errorf("failed to open logs folder: %w", err))
				}
			}),
			fyne.NewMenuItem("Settings", func() {
				mw.openSettings()
			}),
			fyne.NewMenuItem("Update txbridge firmware", func() {
				updater := multiwindow.NewInnerWindow("txbridge firmware updater", txupdater.New(
					mw.settings.CanSettings.GetSerialPort(),
				))
				updater.Icon = theme.DownloadIcon()
				updater.Resize(fyne.NewSize(400, 300))
				mw.wm.Add(updater)
			}),
			fyne.NewMenuItem("What's new", func() {
				mw.showWhatsNew()
			}),
		),
		//fyne.NewMenu("Preset",
		//	fyne.NewMenuItem("Save", mw.savePreset),
		//	fyne.NewMenuItem("New", mw.newPreset),
		//	fyne.NewMenuItem("Import", mw.importPreset),
		//	fyne.NewMenuItem("Export", mw.exportPreset),
		//	fyne.NewMenuItem("Delete", mw.deletePreset),
		//),

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

	mw.menu = mainmenu.New(mw, leading, trailing, mw.openMap, otherFunc)
}

func (mw *MainWindow) loadBinary() {
	if mw.dlc != nil {
		mw.Error(errors.New("stop logging before loading a new binary"))
		return
	}
	go func() {
		filename, err := sdialog.File().Filter("Binary file", "bin").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			mw.Error(err)
			return
		}
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			mw.Error(err)
			return
		}
	}()
}

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}

	axis := symbol.GetInfo(typ, mapName)

	if w := mw.wm.HasWindow(axis.Z + " - " + axis.ZDescription); w != nil {
		mw.wm.Raise(w)
		return
	}

	xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := mw.fw.GetXYZ(axis.X, axis.Y, axis.Z)
	if err != nil {
		mw.Error(err)
		return
	}

	symZ := mw.fw.GetByName(axis.Z)

	if axis.X == "Pwm_ind_trot!" {
		xData = xData[:8]
	}

	// Special cases
	switch mapName {
	case "Pgm_mod!":
		pgm := pgmmod.New()
		pgm.LoadFunc = func() ([]byte, error) {
			return symZ.Bytes(), nil
		}
		pgm.Set(symZ.Bytes())
		mapWindow := multiwindow.NewInnerWindow("Pgm_mod!", pgm)
		mapWindow.Icon = theme.GridIcon()
		mw.wm.Add(mapWindow)
		return
	case "Pgm_status":
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
		return
	}

	var mv *mapviewer.MapViewer

	updateFunc := func(idx int, value []int) {
		if mw.dlc != nil && mw.settings.GetAutoSave() {
			buff := bytes.NewBuffer([]byte{})
			var dataLen int
			for i, val := range value {
				buff.Write(symZ.EncodeInt(val))
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
			mw.Log(fmt.Sprintf("set %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
		}
	}

	loadFunc := func() {
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
			ints := symZ.BytesToInts(data)
			mv.SetZ(ints)
			mw.Log(fmt.Sprintf("load %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
		}
	}

	saveFunc := func(data []int) {
		if mw.dlc == nil {
			return
		}
		start := time.Now()
		buff := bytes.NewBuffer(symZ.EncodeInts(data))
		var startPos uint32
		switch mw.selects.ecuSelect.Selected {
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

		mw.Log(fmt.Sprintf("save %s %s", axis.Z, time.Since(start).Truncate(10*time.Millisecond)))
	}

	saveFileFunc := func(data []int) {
		ss := mw.fw.GetByName(axis.Z)
		if ss == nil {
			mw.Log(fmt.Sprintf("failed to find symbol %s", axis.Z))
			return
		}
		if err := ss.SetData(ss.EncodeInts(data)); err != nil {
			mw.Error(err)
			return
		}
		if err := mw.fw.Save(mw.filename); err != nil {
			mw.Error(err)
			return
		}
		mw.Log(fmt.Sprintf("Saved %s", axis.Z))
	}

	mv, err = mapviewer.New(
		mapviewer.WithSymbol(symZ),
		mapviewer.WithXData(xData),
		mapviewer.WithYData(yData),
		mapviewer.WithZData(zData),
		mapviewer.WithXCorrFac(xCorrFac),
		mapviewer.WithYCorrFac(yCorrFac),
		mapviewer.WithZCorrFac(zCorrFac),
		mapviewer.WithXOffset(symbol.T5Offsets[axis.X]),
		mapviewer.WithYOffset(symbol.T5Offsets[axis.Y]),
		mapviewer.WithZOffset(symbol.T5Offsets[axis.Z]),
		mapviewer.WithXFrom(axis.XFrom),
		mapviewer.WithYFrom(axis.YFrom),
		mapviewer.WithInterPolFunc(interpolate.Interpolate),
		mapviewer.WithUpdateECUFunc(updateFunc),
		mapviewer.WithLoadECUFunc(loadFunc),
		mapviewer.WithSaveECUFunc(saveFunc),
		mapviewer.WithSaveFileFunc(saveFileFunc),
		mapviewer.WithMeshView(mw.settings.GetMeshView()),
		mapviewer.WithEditable(true),
		mapviewer.WithButtons(true),
		mapviewer.WithFollowCrosshair(mw.settings.GetCursorFollowCrosshair()),
		mapviewer.WithAxisLabels(axis.XDescription, axis.YDescription, axis.ZDescription),
	)
	if err != nil {
		mw.Error(err)
		return
	}

	if mw.settings.GetAutoLoad() && mw.dlc != nil {
		p := progressmodal.New(mw.Window.Content(), "Loading "+axis.Z)
		p.Show()
		loadFunc()
		p.Hide()
	}

	mapWindow := multiwindow.NewInnerWindow(axis.Z+" - "+axis.ZDescription, mv)
	mapWindow.Icon = theme.GridIcon()

	mv.OnMouseDown = func() {
		mw.wm.Raise(mapWindow)
	}

	var cancelFuncs []func()
	if axis.XFrom != "" {
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(axis.XFrom, func(value float64) {
			mv.SetValue(axis.XFrom, value)
		}))
	}
	if axis.YFrom != "" {
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(axis.YFrom, func(value float64) {
			mv.SetValue(axis.YFrom, value)
		}))
	}
	mapWindow.OnClose = func() {
		for _, f := range cancelFuncs {
			f()
		}
	}

	mw.wm.Add(mapWindow)

}
