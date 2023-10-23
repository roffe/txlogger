package windows

import (
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) setupMenu() {
	var menus []*fyne.Menu
	menus = append(menus, fyne.NewMenu("File",
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
		fyne.NewMenuItem("Load log", func() {
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
	))

	for _, category := range symbol.T7SymbolsTuningOrder {
		var items []*fyne.MenuItem
		for _, mapName := range symbol.T7SymbolsTuning[category] {
			if strings.Contains(mapName, "|") {
				parts := strings.Split(mapName, "|")
				names := parts[1:]
				itm := fyne.NewMenuItem(parts[0], func() {
					mw.openMapz(names...)
				})
				items = append(items, itm)
				continue
			}
			itm := fyne.NewMenuItem(mapName, func() {
				mw.openMap(symbol.GetInfo(symbol.ECU_T7, mapName))
			})
			items = append(items, itm)
		}
		menus = append(menus, fyne.NewMenu(category, items...))
	}
	menu := fyne.NewMainMenu(menus...)
	mw.Window.SetMainMenu(menu)
}

func (mw *MainWindow) openMapz(mapNames ...string) {
	joinedNames := strings.Join(mapNames, "|")
	mv, found := mw.openMaps[joinedNames]
	if !found {
		w := mw.app.NewWindow("Map Viewer - " + strings.Join(mapNames, ", "))
		//w.SetFixedSize(true)
		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		view := widgets.NewMapViewerMulti(mw.symbols, mapNames...)
		mw.openMaps[joinedNames] = mw.newMapViewerWindow(w, view, symbol.Axis{})

		for _, mv := range view.Children() {
			mw.mvh.Subscribe(mv.Info().XFrom, mv)
			mw.mvh.Subscribe(mv.Info().YFrom, mv)
		}

		w.SetCloseIntercept(func() {
			log.Println("closing", joinedNames)
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

func (mw *MainWindow) openMap(axis symbol.Axis) {
	//log.Printf("openMap: %s %s %s", axis.X, axis.Y, axis.Z)
	mv, found := mw.openMaps[axis.Z]
	if !found {
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

		mv, err := widgets.NewMapViewer(
			widgets.WithXData(xData),
			widgets.WithYData(yData),
			widgets.WithZData(zData),
			widgets.WithXCorrFac(xCorrFac),
			widgets.WithYCorrFac(yCorrFac),
			widgets.WithZCorrFac(zCorrFac),
			widgets.WithXFrom(axis.XFrom),
			widgets.WithYFrom(axis.YFrom),
			widgets.WithInterPolFunc(interpolate.Interpolate),
		)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		w.Canvas().SetOnTypedKey(mv.TypedKey)

		w.SetCloseIntercept(func() {
			log.Println("closing", axis.Z)
			delete(mw.openMaps, axis.Z)
			mw.mvh.Unsubscribe(axis.XFrom, mv)
			mw.mvh.Unsubscribe(axis.YFrom, mv)

			w.Close()
		})

		mw.openMaps[axis.Z] = mw.newMapViewerWindow(w, mv, axis)
		w.SetContent(mv)
		w.Show()

		return
	}
	mv.RequestFocus()
}
