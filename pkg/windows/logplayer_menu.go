package windows

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/widgets"
)

func (lp *LogPlayer) openMapz(typ symbol.ECUType, mapNames ...string) {
	joinedNames := strings.Join(mapNames, "|")
	mv, found := lp.openMaps[joinedNames]
	if !found {
		w := lp.app.NewWindow(strings.Join(mapNames, ", ") + " - Map Viewer")
		//w.SetFixedSize(true)
		if lp.symbols == nil {
			dialog.ShowError(errors.New("no symbols loaded"), lp.Window)
			return
		}
		view, err := widgets.NewMapViewerMulti(typ, lp.symbols, mapNames...)
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

func (lp *LogPlayer) openMap(typ symbol.ECUType, symbolName string) {
	if lp.symbols == nil {
		dialog.ShowError(errors.New("no symbols loaded"), lp.Window)
		return
	}
	if symbolName == "" {
		dialog.ShowError(errors.New("no symbol name"), lp.Window)
	}
	axis := symbol.GetInfo(typ, symbolName)
	mw, found := lp.openMaps[axis.Z]
	if !found {
		w := lp.app.NewWindow(fmt.Sprintf("%s - Map Viewer", axis.Z))

		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := lp.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			dialog.ShowError(err, lp.Window)
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
			widgets.WithEditable(false),
		)
		if err != nil {

			dialog.ShowError(fmt.Errorf("x: %s y: %s z: %s err: %w", axis.X, axis.Y, axis.Z, err), lp.Window)
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
}
