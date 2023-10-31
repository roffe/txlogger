package windows

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
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

		log.Println("symZ", symZ)
		updateFunc := func(idx, value int) {
			buff := symZ.EncodeInt(value)
			if mw.dlc != nil {
				start := time.Now()
				if err := mw.dlc.SetRAM(symZ.Address+uint32(idx*len(buff)), buff); err != nil {
					mw.Log(err.Error())
				}
				log.Printf("set %s idx: %d value: %d took %s", axis.Z, idx, value, time.Since(start))
			}
		}

		var mv *widgets.MapViewer

		signed := symZ.Type&symbol.SIGNED == symbol.SIGNED
		char := symZ.Type&symbol.CHAR == symbol.CHAR
		long := symZ.Type&symbol.LONG == symbol.LONG

		loadFunc := func() {
			start := time.Now()
			if mw.dlc != nil {
				data, err := mw.dlc.GetRAM(symZ.Address, uint32(symZ.Length))
				if err != nil {
					dialog.ShowError(err, w)
					return
				}
				var ints []int
				r := bytes.NewReader(data)

				switch {
				case signed && char:
					log.Println("int8")
					x := make([]int8, symZ.Length)
					if err := binary.Read(r, binary.BigEndian, &x); err != nil {
						log.Println(err)
					}
					for _, v := range x {
						ints = append(ints, int(v))
					}
				case !signed && char:
					log.Println("uint8")
					x := make([]uint8, symZ.Length)
					if err := binary.Read(r, binary.BigEndian, &x); err != nil {
						log.Println(err)
					}
					for _, v := range x {
						ints = append(ints, int(v))
					}
				case signed && !char && !long:
					log.Println("int16")
					x := make([]int16, symZ.Length/2)
					if err := binary.Read(r, binary.BigEndian, &x); err != nil {
						log.Println(err)
					}
					for _, v := range x {
						ints = append(ints, int(v))
					}
				case !signed && !char && !long:
					log.Println("uint16")
					x := make([]uint16, symZ.Length/2)
					if err := binary.Read(r, binary.BigEndian, &x); err != nil {
						log.Println(err)
					}
					for _, v := range x {
						ints = append(ints, int(v))
					}
				case signed && !char && long:
					log.Println("int32")
					x := make([]uint32, symZ.Length/4)
					if err := binary.Read(r, binary.BigEndian, &x); err != nil {
						log.Println(err)
					}
					for _, v := range x {
						ints = append(ints, int(v))
					}
				case !signed && !char && long:
					log.Println("uint32")
					x := make([]uint32, symZ.Length/4)
					if err := binary.Read(r, binary.BigEndian, &x); err != nil {
						log.Println(err)
					}
					for _, v := range x {
						ints = append(ints, int(v))
					}
				}
				mv.SetZ(ints)
			}
			log.Printf("get %s took %s", axis.Z, time.Since(start))
		}

		saveFunc := func() {
			log.Println("saveFunc")
		}

		mv, err = widgets.NewMapViewer(
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

		w.SetCloseIntercept(func() {
			delete(mw.openMaps, axis.Z)
			mw.mvh.Unsubscribe(axis.XFrom, mv)
			mw.mvh.Unsubscribe(axis.YFrom, mv)
			w.Close()
		})

		mw.openMaps[axis.Z] = mw.newMapViewerWindow(w, mv, axis)
		w.SetContent(mv)
		w.Show()

		// if we are online, try to load the map from ECU
		if mw.dlc != nil {
			go func() { loadFunc() }()
		}
		return
	}
	mv.RequestFocus()
}
