package windows

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
)

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

		mv, err := mapviewer.New(
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
			mapviewer.WithEditable(false),
			mapviewer.WithWidebandSymbolName(lp.lambSymbolName),
			mapviewer.WithWBL(true),
			//mapviewer.WithFollowCrosshair(lp.app.Preferences().BoolWithFallback("cursorFollowCrosshair", false)),
			mapviewer.WithAxisLabels(axis.XDescription, axis.YDescription, axis.ZDescription),
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

		/*
			cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc("Lambda.External", func(value float64) {
				mv.SetValue("Lambda.External", value)
			}))

			cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc("DisplProt.LambdaScanner", func(value float64) {
				mv.SetValue("DisplProt.LambdaScanner", value)
			}))
		*/

		cancelFuncs = append(cancelFuncs, lp.ebus.SubscribeFunc(lp.lambSymbolName, func(value float64) {
			mv.SetValue(lp.lambSymbolName, value)
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
