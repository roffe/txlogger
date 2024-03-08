package windows

import (
	"errors"
	"fmt"
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

		w := lp.app.NewWindow(strings.Join(mapNames, ", ") + " - Map Viewer")
		lp.openMaps[joinedNames] = w

		var cancelFuncs []func()
		for _, mv := range view.Children() {
			xf := mv.Info().XFrom
			yf := mv.Info().YFrom
			if xf != "" {
				cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc(xf, func(value float64) {
					mv.SetValue(xf, value)
				}))
			}
			if yf != "" {
				cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc(yf, func(value float64) {
					mv.SetValue(yf, value)
				}))
			}
		}

		w.SetCloseIntercept(func() {
			delete(lp.openMaps, joinedNames)
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

func (lp *LogPlayer) openMap(typ symbol.ECUType, symbolName string) {
	if lp.symbols == nil {
		dialog.ShowError(errors.New("no symbols loaded"), lp.Window)
		return
	}
	if symbolName == "" {
		dialog.ShowError(errors.New("no symbol name"), lp.Window)
	}
	axis := symbol.GetInfo(typ, symbolName)
	w, found := lp.openMaps[axis.Z]
	if !found {
		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := lp.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			dialog.ShowError(err, lp.Window)
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
			widgets.WithEditable(false),
			widgets.WithLambdaSymbolName(lp.lambSymbolName),
		)
		if err != nil {
			dialog.ShowError(fmt.Errorf("x: %s y: %s z: %s err: %w", axis.X, axis.Y, axis.Z, err), lp.Window)
			return
		}

		var cancelFuncs []func()

		if axis.XFrom != "" {
			cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc(axis.XFrom, func(value float64) {
				mv.SetValue(axis.XFrom, value)
			}))
		}

		if axis.YFrom != "" {
			cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc(axis.YFrom, func(value float64) {
				mv.SetValue(axis.YFrom, value)
			}))
		}

		cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc("Lambda.External", func(value float64) {
			mv.SetValue("Lambda.External", value)
		}))

		w := lp.app.NewWindow(fmt.Sprintf("%s - Map Viewer", axis.Z))
		lp.openMaps[axis.Z] = w

		w.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
			if ke.Name == fyne.KeySpace {
				lp.toggleBtn.Tapped(&fyne.PointEvent{})
				return
			}
			mv.TypedKey(ke)
		})

		w.SetCloseIntercept(func() {
			delete(lp.openMaps, axis.Z)
			for _, f := range cancelFuncs {
				f()
			}
			w.Close()
		})

		w.SetContent(mv)
		w.Show()
		return
	}
	w.RequestFocus()
}
