package settings

import (
	"errors"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
)

func newImageFromResource(name string) *canvas.Image {
	var img *canvas.Image
	switch name {
	case "mtx-l":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.MtxL))
		img.SetMinSize(fyne.NewSize(224, 224))
	case "lc-2":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.Lc2))
		img.SetMinSize(fyne.NewSize(400, 224))
	case "uego":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.Uego))
		img.SetMinSize(fyne.NewSize(315, 224))
	case "lambdatocan":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.LambdaToCan))
		img.SetMinSize(fyne.NewSize(481, 224))
	case "t7":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.T7))
		img.SetMinSize(fyne.NewSize(320, 224))
	case "plx":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.PLX))
		img.SetMinSize(fyne.NewSize(470, 224))
	case "combi":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.CombiV2))
		img.SetMinSize(fyne.NewSize(360, 245))
	case "zeitronix":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.ZeitronixZT2))
		img.SetMinSize(fyne.NewSize(252, 252))
	case "stagafr":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.STAGAfr))
		img.SetMinSize(fyne.NewSize(252, 252))
	}
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleFastest

	return img
}

func (sw *Widget) newLogFormat() *widget.Select {
	return widget.NewSelect([]string{"CSV", "TXL"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLogFormat, s)
	})
}

func (sw *Widget) newWBLSelector() *fyne.Container {
	sw.wblSource = widget.NewSelect([]string{
		"None",
		"ECU",
		aem.ProductString,
		"CombiAdapter",
		ecumaster.ProductString,
		innovate.ProductString,
		plx.ProductString,
		zeitronix.ProductString,
		stag.ProductString,
	}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsWblSource, s)
		fyne.CurrentApp().Preferences().SetString(prefsWidebandSymbolName, sw.GetWidebandSymbolName())
		var ecuSet bool
		var portSelect bool
		switch s {
		case "ECU":
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Show()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			ecuSet = true
			portSelect = false
		case ecumaster.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Show()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			portSelect = false
		case innovate.ProductString:
			sw.images.mtxl.Show()
			sw.images.lc2.Show()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			portSelect = true
		case aem.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Show()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			portSelect = true
		case plx.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Show()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			portSelect = true
		case "CombiAdapter":
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Show()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			portSelect = false
		case zeitronix.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Show()
			sw.images.stagafr.Hide()
			portSelect = true
		case stag.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Show()
			portSelect = true
		default:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			sw.images.stagafr.Hide()
			portSelect = false
		}

		if portSelect {
			sw.wblPortLabel.Show()
			sw.wblPortSelect.Show()
			sw.wblPortRefreshButton.Show()
		} else {
			sw.wblPortLabel.Hide()
			sw.wblPortSelect.Hide()
			sw.wblPortRefreshButton.Hide()
		}

		if ecuSet || s == "CombiAdapter" {
			sw.wblADscanner.Show()
			if sw.wblADscanner.Checked {
				sw.minimumVoltageWidebandLabel.Show()
				sw.maximumVoltageWidebandLabel.Show()
				sw.lowLabel.Show()
				sw.highLabel.Show()
				sw.minimumVoltageWidebandEntry.Show()
				sw.maximumVoltageWidebandEntry.Show()
				sw.lowEntry.Show()
				sw.highEntry.Show()
			}
		} else {
			sw.wblADscanner.Hide()
			sw.minimumVoltageWidebandLabel.Hide()
			sw.maximumVoltageWidebandLabel.Hide()
			sw.lowLabel.Hide()
			sw.highLabel.Hide()
			sw.minimumVoltageWidebandEntry.Hide()
			sw.maximumVoltageWidebandEntry.Hide()
			sw.lowEntry.Hide()
			sw.highEntry.Hide()
		}

		//sw.container.Refresh()
	})
	return container.NewBorder(
		nil,
		nil,
		widget.NewLabel("Source"),
		nil,
		sw.wblSource,
	)
}

func (sw *Widget) newFreqSlider() *widget.Slider {
	slider := widget.NewSlider(5, 300)
	slider.Step = 5
	slider.OnChanged = func(f float64) {
		sw.freqValue.SetText(strconv.FormatFloat(f, 'f', 0, 64))
	}
	slider.OnChangeEnded = func(f float64) {
		fyne.CurrentApp().Preferences().SetInt(prefsFreq, int(f))
	}
	return slider
}

func (sw *Widget) newADscannerCheck() *widget.Check {
	return widget.NewCheck("use AD Scanner (don't forget to add symbol)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsUseADScanner, b)
		if b {
			sw.minimumVoltageWidebandLabel.Show()
			sw.maximumVoltageWidebandLabel.Show()
			sw.lowLabel.Show()
			sw.highLabel.Show()
			sw.minimumVoltageWidebandEntry.Show()
			sw.maximumVoltageWidebandEntry.Show()
			sw.lowEntry.Show()
			sw.highEntry.Show()
		} else {
			sw.minimumVoltageWidebandLabel.Hide()
			sw.maximumVoltageWidebandLabel.Hide()
			sw.lowLabel.Hide()
			sw.highLabel.Hide()
			sw.minimumVoltageWidebandEntry.Hide()
			sw.maximumVoltageWidebandEntry.Hide()
			sw.lowEntry.Hide()
			sw.highEntry.Hide()
		}
	})
}

func (sw *Widget) newMeshView() *widget.Check {
	return widget.NewCheck("3D Mesh on map viewing", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsMeshView, b)
	})
}

func (sw *Widget) newAutoUpdateLoad() *widget.Check {
	return widget.NewCheck("Load maps from ECU when connected", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateLoadEcu, b)
	})
}

func (sw *Widget) newAutoUpdateSave() *widget.Check {
	return widget.NewCheck("Save changes automaticly if connected to ECU (requires open bin)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateSaveEcu, b)
	})
}

func (sw *Widget) newCursorFollowCrosshair() *widget.Check {
	return widget.NewCheck("Cursor follows crosshair in MapViewer (one hand mapping)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsCursorFollowCrosshair, b)
	})
}

func (sw *Widget) newLivePreview() *widget.Check {
	return widget.NewCheck("Live preview values in symbollist (uncheck if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsLivePreview, b)
	})
}

func (sw *Widget) newRealtimeBars() *widget.Check {
	return widget.NewCheck("Bars on live preview values (uncheck if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsRealtimeBars, b)
	})
}

func (sw *Widget) newUserMPH() *widget.Check {
	return widget.NewCheck("Use mph instead of km/h", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsUseMPH, b)
	})
}

func (sw *Widget) newSwapRPMandSpeed() *widget.Check {
	return widget.NewCheck("Swap RPM and speed gauge position", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsSwapRPMandSpeed, b)
	})
}

func (sw *Widget) newColorBlindMode() *widget.Select {
	return widget.NewSelect(colors.SupportedColorBlindModes[:], func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsColorBlindMode, s)
		ebus.Publish(ebus.TOPIC_COLORBLINDMODE, float64(sw.colorBlindMode.SelectedIndex()))
	})
}

func (sw *Widget) newAdapterSelector() *widget.Select {
	return widget.NewSelect(gocan.ListAdapterNames(), func(s string) {
		if info, found := sw.adapters[s]; found {
			fyne.CurrentApp().Preferences().SetString(prefsAdapter, s)
			if info.RequiresSerialPort {
				sw.portSelector.Enable()
				sw.speedSelector.Enable()
				return
			} else {
				sw.portDescription.SetText("")
			}
			sw.portSelector.Disable()
			sw.speedSelector.Disable()
		}
	})
}

func (sw *Widget) newPortSelector() *widget.Select {
	return widget.NewSelect(sw.ListPorts(), func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsPort, s)
		itm, ok := portCache[s]
		if ok {
			var desc string
			if itm.Manufacturer != "" {
				desc += itm.Manufacturer
			}
			if itm.Product != "" {
				if desc != "" {
					desc += " "
				}
				desc += itm.Product
			}
			if itm.SerialNumber != "" {
				if desc != "" {
					desc += " "
				}
				desc += itm.SerialNumber
			}
			sw.portDescription.SetText(desc)
		} else {
			sw.portDescription.SetText("")
		}

	})
}

func (sw *Widget) newSpeedSelector() *widget.Select {
	return widget.NewSelect(portSpeeds, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsSpeed, s)
	})
}

func (sw *Widget) newDebugCheckbox() *widget.Check {
	return widget.NewCheck("Debug", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsDebug, b)
	})
}

func (sw *Widget) newPortRefreshButton() *widget.Button {
	return widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.portSelector.Options = sw.ListPorts()
		sw.portSelector.Refresh()
	})
}

func (sw *Widget) loadPreferences() {
	freq := fyne.CurrentApp().Preferences().IntWithFallback(prefsFreq, 25)
	sw.freqSlider.SetValue(float64(freq))
	loadPrefsCheck(sw.autoLoad, prefsAutoUpdateLoadEcu, true)
	loadPrefsCheck(sw.autoSave, prefsAutoUpdateSaveEcu, false)
	loadPrefsCheck(sw.cursorFollowCrosshair, prefsCursorFollowCrosshair, false)
	loadPrefsCheck(sw.livePreview, prefsLivePreview, true)
	loadPrefsCheck(sw.meshView, prefsMeshView, true)
	loadPrefsCheck(sw.realtimeBars, prefsRealtimeBars, true)
	loadPrefsSelect(sw.logFormat, prefsLogFormat, "TXL")
	logPath, err := common.GetLogPath()
	if err != nil {
		fyne.LogError("Could not get log path", err)
	}
	loadPrefsText(sw.logPath, prefsLogPath, logPath)
	loadPrefsText(sw.logPath, prefsLogPath, logPath)
	loadPrefsSelect(sw.wblSource, prefsWblSource, "None")
	loadPrefsCheck(sw.wblADscanner, prefsUseADScanner, false)
	loadPrefsCheck(sw.useMPH, prefsUseMPH, false)
	loadPrefsCheck(sw.swapRPMandSpeed, prefsSwapRPMandSpeed, false)
	loadPrefsSelect(sw.wblPortSelect, prefsWBLPort, "")
	loadPrefsText(sw.minimumVoltageWidebandEntry, prefsminimumVoltageWideband, "0.0")
	loadPrefsText(sw.maximumVoltageWidebandEntry, prefsmaximumVoltageWideband, "5.0")
	loadPrefsText(sw.lowEntry, prefslowValue, "0.5")
	loadPrefsText(sw.highEntry, prefshighValue, "1.5")
	loadPrefsSelect(sw.colorBlindMode, prefsColorBlindMode, "Normal")

	if sw.wblADscanner.Checked {
		sw.minimumVoltageWidebandLabel.Show()
		sw.maximumVoltageWidebandLabel.Show()
		sw.lowLabel.Show()
		sw.highLabel.Show()
		sw.minimumVoltageWidebandEntry.Show()
		sw.maximumVoltageWidebandEntry.Show()
		sw.lowEntry.Show()
		sw.highEntry.Show()
	} else {
		sw.minimumVoltageWidebandLabel.Hide()
		sw.maximumVoltageWidebandLabel.Hide()
		sw.lowLabel.Hide()
		sw.highLabel.Hide()
		sw.minimumVoltageWidebandEntry.Hide()
		sw.maximumVoltageWidebandEntry.Hide()
		sw.lowEntry.Hide()
		sw.highEntry.Hide()
	}

	loadPrefsSelect(sw.adapterSelector, prefsAdapter, "")
	loadPrefsSelect(sw.portSelector, prefsPort, "")
	loadPrefsSelect(sw.speedSelector, prefsSpeed, "115200")
	loadPrefsCheck(sw.debugCheckbox, prefsDebug, false)
}

func loadPrefsSelect(s *widget.Select, prefKey string, fallback string) {
	s.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefKey, fallback))
}

func loadPrefsCheck(box *widget.Check, prefKey string, fallback bool) {
	box.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefKey, fallback))
}

func loadPrefsText(obj SetText, prefKey string, fallback string) {
	obj.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefKey, fallback))
}

func positiveFloatValidator(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSuffix(s, ".")

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.New("invalid number")
	}
	if val < 0 {
		return 0, errors.New("must be positive")
	}
	return val, nil
}
