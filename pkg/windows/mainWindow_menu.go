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
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/skratchdot/open-golang/open"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) setupMenu() {
	otherFunc := func(str string) {
		switch str {
		case "Register EU0D":
			mr := NewMyrtilosRegistration(mw.app, mw)
			mr.Show()
		}
	}

	menus := []*fyne.Menu{
		fyne.NewMenu("File",
			fyne.NewMenuItem("Load binary", mw.loadBinary),
			fyne.NewMenuItem("Play log", mw.playLog),
			fyne.NewMenuItem("Open log folder", func() {
				if err := open.Run(mw.settings.GetLogPath()); err != nil {
					mw.Error(fmt.Errorf("failed to open logs folder: %w", err))
				}
			}),
			fyne.NewMenuItem("Settings", func() {
				if mw.wm.HasWindow("Settings") {
					return
				}
				inner := newInnerWindow("Settings", mw.settings)
				inner.Icon = theme.SettingsIcon()
				inner.CloseIntercept = func() {
					mw.wm.Remove(inner)
				}
				mw.wm.Add(inner)
			}),
			fyne.NewMenuItem("Help", func() {
				if mw.wm.HasWindow("Help") {
					return
				}
				inner := newInnerWindow("Help", Help())
				inner.Icon = theme.HelpIcon()
				inner.CloseIntercept = func() {
					mw.wm.Remove(inner)
				}
				mw.wm.Add(inner)
			}),
		),
		fyne.NewMenu("Preset",
			fyne.NewMenuItem("Save", mw.savePreset),
			fyne.NewMenuItem("New", mw.newPreset),
			fyne.NewMenuItem("Import", mw.importPreset),
			fyne.NewMenuItem("Export", mw.exportPreset),
			fyne.NewMenuItem("Delete", mw.deletePreset),
		),
		fyne.NewMenu("Other",
			fyne.NewMenuItem("Update txbridge firmware", func() {
				updater := newInnerWindow("txbridge firmware updater", widgets.NewTxUpdater(
					mw.settings.CanSettings.GetSerialPort(),
				))
				updater.Icon = theme.DownloadIcon()
				updater.CloseIntercept = func() {
					mw.wm.Remove(updater)
				}
				mw.wm.Add(updater)
			}),
		),
	}
	mw.menu = mainmenu.New(mw, menus, mw.openMap, otherFunc)
}

func (mw *MainWindow) loadBinary() {
	if mw.dlc != nil {
		mw.Error(errors.New("stop logging before loading a new binary"))
		return
	}
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
}

func (mw *MainWindow) playLog() {
	filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").SetStartDir(mw.settings.GetLogPath()).Load()
	if err != nil {
		if err.Error() == "Cancelled" {
			return
		}
		mw.Error(err)
		return
	}
	go NewLogPlayer(mw.app, filename, mw.fw)
}

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}

	axis := symbol.GetInfo(typ, mapName)

	if mw.wm.HasWindow(axis.Z + " - " + axis.ZDescription) {
		return
	}

	xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := mw.fw.GetXYZ(axis.X, axis.Y, axis.Z)
	if err != nil {
		mw.Error(err)
		return
	}

	symZ := mw.fw.GetByName(axis.Z)

	var mv *widgets.MapViewer

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
			switch mw.ecuSelect.Selected {
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

			switch mw.ecuSelect.Selected {
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
		switch mw.ecuSelect.Selected {
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

	if axis.X == "Pwm_ind_trot!" {
		xData = xData[:8]

	}

	mv, err = widgets.NewMapViewer(
		widgets.WithSymbol(symZ),
		widgets.WithXData(xData),
		widgets.WithYData(yData),
		widgets.WithZData(zData),
		widgets.WithXCorrFac(xCorrFac),
		widgets.WithYCorrFac(yCorrFac),
		widgets.WithZCorrFac(zCorrFac),
		widgets.WithXOffset(symbol.T5Offsets[axis.X]),
		widgets.WithYOffset(symbol.T5Offsets[axis.Y]),
		widgets.WithZOffset(symbol.T5Offsets[axis.Z]),
		widgets.WithXFrom(axis.XFrom),
		widgets.WithYFrom(axis.YFrom),
		widgets.WithInterPolFunc(interpolate.Interpolate),
		widgets.WithUpdateECUFunc(updateFunc),
		widgets.WithLoadECUFunc(loadFunc),
		widgets.WithSaveECUFunc(saveFunc),
		widgets.WithSaveFileFunc(saveFileFunc),
		widgets.WithMeshView(mw.settings.GetMeshView()),
		widgets.WithWidebandSymbolName(mw.settings.GetWidebandSymbolName()),
		widgets.WithEditable(true),
		widgets.WithButtons(true),
		widgets.WithWBL(mw.settings.GetWidebandType() != "None"),
		widgets.WithFollowCrosshair(mw.settings.GetCursorFollowCrosshair()),
		widgets.WithAxisLabels(axis.XDescription, axis.YDescription, axis.ZDescription),
	)
	if err != nil {
		mw.Error(err)
		return
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

	//if mw.settings.GetWidebandType() != "ECU" {
	cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(mw.settings.GetWidebandSymbolName(), func(value float64) {
		mv.SetValue(mw.settings.GetWidebandSymbolName(), value)
	}))
	//}

	if mw.settings.GetAutoLoad() && mw.dlc != nil {
		p := widgets.NewProgressModal(mw.Window.Content(), "Loading "+axis.Z)
		p.Show()
		loadFunc()
		p.Hide()
	}

	mapWindow := newInnerWindow(axis.Z+" - "+axis.ZDescription, mv)
	mapWindow.Icon = theme.GridIcon()
	mapWindow.CloseIntercept = func() {
		for _, f := range cancelFuncs {
			f()
		}
		mw.wm.Remove(mapWindow)
	}
	mw.wm.Add(mapWindow)
	/*
		w := mw.app.NewWindow(axis.Z + " - " + axis.ZDescription)

		mw.openMaps[axis.Z] = w
		w.SetCloseIntercept(func() {
			delete(mw.openMaps, axis.Z)
			for _, f := range cancelFuncs {
				f()
			}
			w.Close()
		})
		w.SetContent(mv)
		w.Show()
	*/
}
