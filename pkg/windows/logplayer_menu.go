package windows

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
)

func (lp *LogPlayer) setupMenu() {
	var menus []*fyne.Menu
	menus = append(menus, fyne.NewMenu("File"))
	for _, category := range symbol.T7SymbolsTuningOrder {
		var items []*fyne.MenuItem

		for _, mapName := range symbol.T7SymbolsTuning[category] {
			if strings.Contains(mapName, "|") {
				parts := strings.Split(mapName, "|")
				names := parts[1:]
				itm := fyne.NewMenuItem(parts[0], func() {
					lp.openMapz(names...)
				})
				items = append(items, itm)
				continue
			}
			items = append(items, fyne.NewMenuItem(mapName, func() {
				lp.openMap(mapName)
			}))
		}
		menus = append(menus, fyne.NewMenu(category, items...))
	}
	menu := fyne.NewMainMenu(menus...)
	lp.Window.SetMainMenu(menu)
}

func (lp *LogPlayer) openMapz(mapNames ...string) {
	joinedNames := strings.Join(mapNames, "|")
	mv, found := lp.openMaps[joinedNames]
	if !found {
		w := lp.app.NewWindow("Map Viewer - " + strings.Join(mapNames, ", "))
		//w.SetFixedSize(true)
		if lp.symbols == nil {
			dialog.ShowError(errors.New("no symbols loaded"), lp.Window)
			return
		}
		view, err := widgets.NewMapViewerMulti(lp.symbols, mapNames...)
		if err != nil {
			dialog.ShowError(err, lp.Window)
			return
		}
		lp.openMaps[joinedNames] = lp.newMapViewerWindow(w, view, symbol.Axis{})

		for _, mv := range view.Children() {
			lp.mvh.Subscribe(mv.Info().XFrom, mv)
			lp.mvh.Subscribe(mv.Info().YFrom, mv)
		}

		w.SetCloseIntercept(func() {
			log.Println("closing", joinedNames)
			delete(lp.openMaps, joinedNames)
			for _, mv := range view.Children() {
				lp.mvh.Unsubscribe(mv.Info().XFrom, mv)
				lp.mvh.Unsubscribe(mv.Info().YFrom, mv)
			}

			w.Close()
		})

		w.SetContent(view)
		w.Show()

		return
	}
	mv.RequestFocus()
}

func (lp *LogPlayer) openMap(symbolName string) error {
	if symbolName == "" {
		return errors.New("symbolName is empty")
	}
	axis := symbol.GetInfo(symbol.ECU_T7, symbolName)
	mw, found := lp.openMaps[axis.Z]
	if !found {
		w := lp.app.NewWindow(fmt.Sprintf("Map Viewer - %s", axis.Z))

		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := lp.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			return err
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
			return fmt.Errorf("x: %s y: %s z: %s err: %w", axis.X, axis.Y, axis.Z, err)
		}

		w.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
			if ke.Name == fyne.KeySpace {
				lp.toggleBtn.Tapped(&fyne.PointEvent{})
				return
			}
			mv.TypedKey(ke)
		})

		w.SetCloseIntercept(func() {
			log.Println("closing", axis.Z)
			delete(lp.openMaps, axis.Z)
			lp.mvh.Unsubscribe(axis.XFrom, mv)
			lp.mvh.Unsubscribe(axis.YFrom, mv)
			mv.Close()
			w.Close()
		})

		mw = lp.newMapViewerWindow(w, mv, axis)

		w.SetContent(mv)
		w.Show()
	}
	mw.RequestFocus()
	return nil
}
