package windows

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/skratchdot/open-golang/open"
	sdialog "github.com/sqweek/dialog"
)

const chunkSize = 128

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
					mw.Log("failed to open logs folder: " + err.Error())
				}
			}),
		),
		fyne.NewMenu("Preset",
			fyne.NewMenuItem("Save", mw.savePreset),
			fyne.NewMenuItem("New", mw.newPreset),
			fyne.NewMenuItem("Import", mw.importPreset),
			fyne.NewMenuItem("Export", mw.exportPreset),
			fyne.NewMenuItem("Delete", mw.deletePreset),
		),
	}
	mw.menu = mainmenu.New(mw, menus, mw.openMap, mw.openMapz, otherFunc)
}

func (mw *MainWindow) loadBinary() {
	if mw.dlc != nil {
		dialog.ShowError(errors.New("stop logging before loading a new binary"), mw.Window)
		return
	}
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
}

func (mw *MainWindow) playLog() {
	filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").SetStartDir(mw.settings.GetLogPath()).Load()
	if err != nil {
		if err.Error() == "Cancelled" {
			return
		}
		// dialog.ShowError(err, mw)
		mw.Log(err.Error())
		return
	}
	go NewLogPlayer(mw.app, filename, mw.fw)
}

func (mw *MainWindow) openMapz(typ symbol.ECUType, mapNames ...string) {
	if mw.fw == nil {
		mw.Log("No binary loaded")
		return
	}
	joinedNames := strings.Join(mapNames, "|")
	mv, found := mw.openMaps[joinedNames]
	if !found {
		w := mw.app.NewWindow(strings.Join(mapNames, ", ") + " - Map Viewer")
		view, err := widgets.NewMapViewerMulti(typ, mw.fw, mapNames...)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		mw.openMaps[joinedNames] = w

		var cancelFuncs []func()
		for _, mv := range view.Children() {
			xf := mv.Info().XFrom
			yf := mv.Info().YFrom
			if xf != "" {
				cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(xf, func(value float64) {
					mv.SetValue(xf, value)
				}))
			}
			if yf != "" {
				cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(yf, func(value float64) {
					mv.SetValue(yf, value)
				}))
			}
		}

		w.SetCloseIntercept(func() {
			delete(mw.openMaps, joinedNames)
			for _, f := range cancelFuncs {
				f()
			}
			w.Close()
		})

		w.SetContent(view)
		w.Show()
		return
	}
	mv.RequestFocus()
}

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	if mw.fw == nil {
		mw.Log("No binary loaded")
		return
	}
	axis := symbol.GetInfo(typ, mapName)
	mww, found := mw.openMaps[axis.Z]
	if found {
		mww.RequestFocus()
		return
	}
	xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := mw.fw.GetXYZ(axis.X, axis.Y, axis.Z)
	if err != nil {
		mw.Log(err.Error())
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
				mw.Log(err.Error())
			}
			mw.Log(fmt.Sprintf("set %s idx: %d took %s", axis.Z, idx, time.Since(start)))
		}
	}

	loadFunc := func() {
		if mw.dlc != nil {
			start := time.Now()
			var addr uint32

			switch mw.ecuSelect.Selected {
			case "T7":
				addr = symZ.Address
			case "T8":
				addr = symZ.Address + symZ.SramOffset
			}

			data, err := mw.dlc.GetRAM(addr, uint32(symZ.Length))
			if err != nil {
				mw.Log(err.Error())
				return
			}
			ints := symZ.BytesToInts(data)
			mv.SetZ(ints)
			mw.Log(fmt.Sprintf("load %s took %s", axis.Z, time.Since(start)))
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

		for buff.Len() > 0 {
			if buff.Len() > chunkSize {
				if err := mw.dlc.SetRAM(startPos, buff.Next(chunkSize)); err != nil {
					mw.Log(err.Error())
					return
				}
			} else {
				if err := mw.dlc.SetRAM(startPos, buff.Bytes()); err != nil {
					mw.Log(err.Error())
					return
				}
				buff.Reset()
			}
			startPos += chunkSize
		}
		mw.Log(fmt.Sprintf("save %s took %s", axis.Z, time.Since(start)))
	}

	saveFileFunc := func(data []int) {
		ss := mw.fw.GetByName(axis.Z)
		if ss == nil {
			mw.Log(fmt.Sprintf("failed to find symbol %s", axis.Z))
			return
		}
		if err := ss.SetData(ss.EncodeInts(data)); err != nil {
			mw.Log(err.Error())
			return
		}
		if err := mw.fw.Save(mw.filename); err != nil {
			mw.Log(err.Error())
			return
		}
		mw.Log(fmt.Sprintf("Saved %s", axis.Z))
	}

	mv, err = widgets.NewMapViewer(
		widgets.WithSymbol(symZ),
		widgets.WithXData(xData),
		widgets.WithYData(yData),
		widgets.WithZData(zData),
		widgets.WithXCorrFac(xCorrFac),
		widgets.WithYCorrFac(yCorrFac),
		widgets.WithZCorrFac(zCorrFac),
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
		mw.Log(err.Error())
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
	// mw.tab.Append(container.NewTabItem(axis.Z, mv))
	w := mw.app.NewWindow(axis.Z + " - " + axis.ZDescription)
	w.Canvas().SetOnTypedKey(mv.TypedKey)
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
}
