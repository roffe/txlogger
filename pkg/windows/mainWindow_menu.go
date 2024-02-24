package windows

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/mapviewerhandler"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/skratchdot/open-golang/open"
	sdialog "github.com/sqweek/dialog"
)

const chunkSize = 128

func (mw *MainWindow) setupMenu() {
	menus := []*fyne.Menu{
		fyne.NewMenu("File",
			fyne.NewMenuItem("Load binary", func() {
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
			}),
			fyne.NewMenuItem("Play log", func() {
				filename, err := sdialog.File().Filter("logfile", "t7l", "t8l", "csv").SetStartDir(mw.settings.GetLogPath()).Load()
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
			}),
			fyne.NewMenuItem("Open log folder", func() {
				if err := open.Run(mw.settings.GetLogPath()); err != nil {
					fyne.LogError("Failed to open logs folder", err)
				}
			}),
		),
	}
	mw.menu = mainmenu.New(mw, menus, mw.openMap, mw.openMapz)
}

func (mw *MainWindow) openMapz(typ symbol.ECUType, mapNames ...string) {
	joinedNames := strings.Join(mapNames, "|")
	mv, found := mw.openMaps[joinedNames]
	if !found {
		w := mw.app.NewWindow(strings.Join(mapNames, ", ") + " - Map Viewer")
		if mw.fw == nil {
			mw.Log("No binary loaded")
			return
		}
		view, err := widgets.NewMapViewerMulti(typ, mw.fw, mapNames...)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		mw.openMaps[joinedNames] = mw.newMapViewerWindow(w, view, symbol.Axis{})

		for _, mv := range view.Children() {
			mw.mvh.Subscribe(mv.Info().XFrom, mv)
			mw.mvh.Subscribe(mv.Info().YFrom, mv)
		}

		w.SetCloseIntercept(func() {
			delete(mw.openMaps, joinedNames)
			for _, mv := range view.Children() {
				mw.mvh.Unsubscribe(mv.Info().XFrom, mv)
				mw.mvh.Unsubscribe(mv.Info().YFrom, mv)
			}
			w.Close()
		})

		w.SetContent(view)
		w.Show()
		return
	}
	mv.RequestFocus()
}

func (mw *MainWindow) newMapViewerWindow(w fyne.Window, mv mapviewerhandler.MapViewerWindowWidget, axis symbol.Axis) mapviewerhandler.MapViewerWindowInterface {
	mww := mapviewerhandler.NewWindow(w, mv)

	mw.openMaps[axis.Z] = mww
	if axis.XFrom == "" {
		axis.XFrom = "MAF.m_AirInlet"
	}
	if axis.YFrom == "" {
		axis.YFrom = "ActualIn.n_Engine"
	}

	mw.mvh.Subscribe(mw.settings.GetLambdaSymbolName(), mv)
	mw.mvh.Subscribe(axis.XFrom, mv)
	mw.mvh.Subscribe(axis.YFrom, mv)
	return mww
}

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	axis := symbol.GetInfo(typ, mapName)
	mv, found := mw.openMaps[axis.Z]
	if !found {
		//w := fyne.CurrentApp().Driver().CreateWindow("Map Viewer - " + axis.Z)
		w := mw.app.NewWindow(axis.Z + " - Map Viewer")
		//w.SetFixedSize(true)
		if mw.fw == nil {
			mw.Log("No binary loaded")
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
					dialog.ShowError(err, w)
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

			log.Println("Save", mw.filename)
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
			widgets.WithAutload(mw.settings.GetAutoLoad()),
			widgets.WithLambdaSymbolName(mw.settings.GetLambdaSymbolName()),
		)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		w.Canvas().SetOnTypedKey(mv.TypedKey)

		mw.openMaps[axis.Z] = mw.newMapViewerWindow(w, mv, axis)

		w.SetCloseIntercept(func() {
			delete(mw.openMaps, axis.Z)
			mw.mvh.Unsubscribe(mw.settings.GetLambdaSymbolName(), mv)
			mw.mvh.Unsubscribe(axis.XFrom, mv)
			mw.mvh.Unsubscribe(axis.YFrom, mv)
			w.Close()
		})

		w.SetContent(mv)
		w.Show()
		return
	}
	mv.RequestFocus()
}
