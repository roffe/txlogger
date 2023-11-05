package windows

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/mapviewerhandler"
	"github.com/roffe/txlogger/pkg/symbol"
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
				mw.SyncSymbols()
			}),
			fyne.NewMenuItem("Play log", func() {
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
			}),
			fyne.NewMenuItem("Open log folder", func() {
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
			}),
			fyne.NewMenuItem("Settings", func() {
				mw.Window.SetContent(mw.settings)
			}),
		),
	}
	mw.menu = mainmenu.New(mw, menus, mw.openMap, mw.openMapz)
}

func (mw *MainWindow) openMapz(typ symbol.ECUType, mapNames ...string) {
	joinedNames := strings.Join(mapNames, "|")
	mv, found := mw.openMaps[joinedNames]
	if !found {
		w := mw.app.NewWindow("Map Viewer - " + strings.Join(mapNames, ", "))
		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		view, err := widgets.NewMapViewerMulti(typ, mw.symbols, mapNames...)
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

	mw.mvh.Subscribe(axis.XFrom, mv)
	mw.mvh.Subscribe(axis.YFrom, mv)
	return mww
}

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	axis := symbol.GetInfo(typ, mapName)
	mv, found := mw.openMaps[axis.Z]
	if !found {
		//w := fyne.CurrentApp().Driver().CreateWindow("Map Viewer - " + axis.Z)
		w := mw.app.NewWindow("Map Viewer - " + axis.Z)
		//w.SetFixedSize(true)
		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := mw.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		symZ := mw.symbols.GetByName(axis.Z)

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
				start := time.Now()
				if err := mw.dlc.SetRAM(symZ.Address+uint32(idx*dataLen), buff.Bytes()); err != nil {
					mw.Log(err.Error())
				}
				mw.Log(fmt.Sprintf("set %s idx: %d took %s", axis.Z, idx, time.Since(start)))
			}
		}

		loadFunc := func() {
			if mw.dlc != nil {
				start := time.Now()
				data, err := mw.dlc.GetRAM(symZ.Address, uint32(symZ.Length))
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
			startPos := symZ.Address
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
			widgets.WithUpdateFunc(updateFunc),
			widgets.WithLoadFunc(loadFunc),
			widgets.WithSaveFunc(saveFunc),
		)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		w.Canvas().SetOnTypedKey(mv.TypedKey)

		mw.openMaps[axis.Z] = mw.newMapViewerWindow(w, mv, axis)

		w.SetCloseIntercept(func() {
			delete(mw.openMaps, axis.Z)
			mw.mvh.Unsubscribe(axis.XFrom, mv)
			mw.mvh.Unsubscribe(axis.YFrom, mv)
			w.Close()
		})

		// if we are online, try to load the map from ECU
		if mw.dlc != nil && mw.settings.GetAutoLoad() {
			go func() { loadFunc() }()
		}

		w.SetContent(mv)
		w.Show()
		return
	}
	mv.RequestFocus()
}
