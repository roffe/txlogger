package settings

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ollama"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
)

var (
	logFormats  = []string{"CSV", "BPL" /*"TXL"*/}
	portSpeeds  = []string{"9600", "19200", "38400", "57600", "115200", "230400", "460800", "500000", "921600", "1mbit", "2mbit", "3mbit"}
	gaugeStyles = []string{"Modern", "Classic"}
)

// --- generic control helpers -----------------------------------------------

// checkBox builds a checkbox bound to a boolean preference.
func checkBox(label string, p boolPref) *widget.Check {
	return widget.NewCheck(label, p.set)
}

// checkBoxTopic is checkBox that also announces the new value on the bus, for
// settings that open widgets react to live.
func checkBoxTopic(label string, p boolPref, topic string) *widget.Check {
	return widget.NewCheck(label, func(b bool) {
		p.set(b)
		var v float64
		if b {
			v = 1
		}
		ebus.Publish(topic, v)
	})
}

// indexSelect builds a select whose chosen option index is stored in an
// integer preference (the option order defines the stored value).
func indexSelect(options []string, p intPref) *widget.Select {
	sel := widget.NewSelect(options, nil)
	sel.OnChanged = func(string) {
		p.set(sel.SelectedIndex())
	}
	return sel
}

// setVisible shows or hides a group of canvas objects.
func setVisible(show bool, objs ...fyne.CanvasObject) {
	for _, o := range objs {
		if show {
			o.Show()
		} else {
			o.Hide()
		}
	}
}

// --- wideband source table -------------------------------------------------

// wblSource describes a selectable wideband source and the UI it enables.
type wblSource struct {
	name       string
	image      string // image resource key, "" for none
	portSelect bool   // show the WBL port selector
	adScanner  bool   // show the AD scanner controls
}

var wblSources = []wblSource{
	{name: "None"},
	{name: "ECU", image: "t7", adScanner: true}, // T7 image used as a placeholder
	{name: aem.ProductString, image: "uego", portSelect: true},
	{name: ecumaster.ProductString, image: "lambdatocan"},
	{name: innovate.ProductString, image: "mtx-l", portSelect: true},
	{name: plx.ProductString, image: "plx", portSelect: true},
	{name: stag.ProductString, image: "stagafr", portSelect: true},
	{name: zeitronix.ProductString, image: "zeitronix", portSelect: true},
}

func (sw *Widget) newWBLSelector() *fyne.Container {
	names := make([]string, len(wblSources))
	byName := make(map[string]wblSource, len(wblSources))
	for i, s := range wblSources {
		names[i] = s.name
		byName[s.name] = s
	}

	sw.wblSource = widget.NewSelect(names, func(s string) {
		prefWblSource.set(s)
		sw.wblSymbolChanged()

		src := byName[s]
		if src.image != "" {
			if img := newImageFromResource(src.image); img != nil {
				sw.wblImage.Resource = img.Resource
				sw.wblImage.SetMinSize(img.MinSize())
				sw.wblImage.Refresh()
			}
		}
		setVisible(src.portSelect, sw.wblPortLabel, sw.wblPortSelect, sw.wblPortRefreshButton)
		setVisible(src.adScanner, sw.wblADscanner, sw.wblADScannerSymbol)
	})

	return container.NewBorder(nil, nil, widget.NewLabel("Source"), nil, sw.wblSource)
}

// --- individual controls ---------------------------------------------------

func (sw *Widget) newFreqSlider() *widget.Slider {
	slider := widget.NewSlider(5, 300)
	slider.Step = 5
	slider.OnChanged = func(f float64) {
		sw.freqValue.SetText(strconv.FormatFloat(f, 'f', 0, 64))
	}
	slider.OnChangeEnded = func(f float64) {
		prefFreq.set(int(f))
	}
	return slider
}

func (sw *Widget) newLogFormat() *widget.Select {
	return widget.NewSelect(logFormats, prefLogFormat.set)
}

func (sw *Widget) newADscannerCheck() *widget.Check {
	return widget.NewCheck("use AD Scanner (don't forget to add symbol)", func(b bool) {
		setVisible(b, sw.wblADScannerSymbol)
		prefUseADScanner.set(b)
		sw.wblSymbolChanged()
	})
}

// wblSymbolChanged stores the symbol the wideband settings now resolve to and
// announces the move, so an open dashboard re-points at it instead of having to
// be closed and reopened.
func (sw *Widget) wblSymbolChanged() {
	prefWidebandSymbolName.set(sw.GetWidebandSymbolName())
	ebus.Publish(ebus.TOPIC_WBLSYMBOL, 1)
}

func (sw *Widget) newColorBlindMode() *widget.Select {
	return widget.NewSelect(colors.SupportedColorBlindModes[:], func(s string) {
		prefColorBlindMode.set(s)
		ebus.Publish(ebus.TOPIC_COLORBLINDMODE, float64(sw.colorBlindMode.SelectedIndex()))
	})
}

func (sw *Widget) newAdapterSelector() *widget.Select {
	return widget.NewSelect(nil, func(s string) {
		info, found := sw.adapters[s]
		if !found {
			return
		}
		prefAdapter.set(s)
		if info.RequiresSerialPort {
			sw.portSelector.Enable()
			sw.speedSelector.Enable()
			return
		}
		sw.portDescription.SetText("")
		sw.portSelector.Disable()
		sw.speedSelector.Disable()
	})
}

func (sw *Widget) newPortSelector() *widget.SelectEntry {
	sel := widget.NewSelectEntry(sw.ListPorts())
	sel.OnChanged = func(s string) {
		prefPort.set(s)
		if itm, ok := portCache[s]; ok {
			sw.portDescription.SetText(itm.SerialNumber)
		} else {
			sw.portDescription.SetText("")
		}
	}
	return sel
}

func (sw *Widget) newSpeedSelector() *widget.Select {
	return widget.NewSelect(portSpeeds, prefSpeed.set)
}

func (sw *Widget) newPortRefreshButton() *widget.Button {
	return widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.portSelector.SetOptions(sw.ListPorts())
		sw.portSelector.Refresh()
		sw.RefreshAdapters()
	})
}

// newAIControls builds the local-Ollama controls. The model list is fetched on
// demand rather than at startup: the server is usually not running, and a
// blocking probe every time settings open would be a needless stall.
func (sw *Widget) newAIControls() {
	sw.aiURL = widget.NewEntry()
	sw.aiURL.SetPlaceHolder(ollama.DefaultURL)
	sw.aiURL.OnChanged = prefAIURL.set

	sw.aiModel = widget.NewSelectEntry(nil)
	sw.aiModel.SetPlaceHolder("qwen3.6:27b")
	sw.aiModel.OnChanged = prefAIModel.set

	sw.aiThink = checkBox("Let the model reason before answering (slower)", prefAIThink)

	sw.aiCtxEntry = widget.NewEntry()
	sw.aiCtxEntry.OnChanged = func(s string) {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 2048 {
			prefAICtx.set(n)
		}
	}

	sw.aiStatus = widget.NewLabel("")
	sw.aiStatus.Importance = widget.LowImportance
	sw.aiStatus.Truncation = fyne.TextTruncateEllipsis

	sw.aiRefresh = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.aiStatus.SetText("querying " + prefAIURL.get() + " …")
		go func() {
			models, err := ollama.New(prefAIURL.get(), "").Models(context.Background())
			fyne.Do(func() {
				if err != nil {
					sw.aiStatus.SetText(err.Error())
					return
				}
				if len(models) == 0 {
					sw.aiStatus.SetText("no models installed — run: ollama pull qwen3.6:27b")
					return
				}
				sw.aiModel.SetOptions(models)
				sw.aiStatus.SetText(fmt.Sprintf("%d models available", len(models)))
			})
		}()
	})
}

// --- preference hydration --------------------------------------------------

// loadPreferences pushes persisted values into the controls once every widget
// has been constructed.
func (sw *Widget) loadPreferences() {
	sw.freqSlider.SetValue(float64(prefFreq.get()))
	sw.autoLoad.SetChecked(prefAutoLoad.get())
	sw.autoSave.SetChecked(prefAutoSave.get())
	sw.cursorFollowCrosshair.SetChecked(prefCursorFollowCrosshair.get())
	sw.meshView.SetChecked(prefMeshView.get())
	sw.realtimeBars.SetChecked(prefRealtimeBars.get())

	logFormat := prefLogFormat.get()
	if logFormat == "TXL" {
		prefLogFormat.set("CSV") // TXL is no longer supported, reset to CSV
		logFormat = "CSV"
	}
	sw.logFormat.SetSelected(logFormat)

	logPath, err := common.GetLogPath()
	if err != nil {
		fyne.LogError("Could not get log path", err)
	}
	sw.logPath.SetText(prefLogPath.getOr(logPath))

	if prefWblSource.get() == "CombiAdapter" { // removed source, would abort logging with "unknown WBL type"
		prefWblSource.set("None")
	}
	sw.wblSource.SetSelected(prefWblSource.get())
	sw.wblADscanner.SetChecked(prefUseADScanner.get())
	setVisible(sw.wblADscanner.Checked && sw.wblSource.Selected == "ECU", sw.wblADScannerSymbol)

	sw.useMPH.SetChecked(prefUseMPH.get())
	sw.wblPortSelect.SetSelected(prefWBLPort.get())
	sw.colorBlindMode.SetSelected(prefColorBlindMode.get())

	sw.adapterSelector.SetSelected(prefAdapter.get())
	sw.portSelector.SetText(prefPort.get())
	sw.speedSelector.SetSelected(prefSpeed.get())
	sw.debugCheckbox.SetChecked(prefDebug.get())

	// Graphics
	sw.plotRendererSelect.SetSelectedIndex(prefPlotterRenderer.get())
	sw.meshRendererSelect.SetSelectedIndex(prefMeshRenderer.get())
	sw.gaugeStyleSelect.SetSelected(prefGaugeStyle.get())

	// Logging
	sw.experimentalT5FastLogger.SetChecked(prefExperimentalT5FastLogger.get())

	// AI chat
	sw.aiURL.SetText(prefAIURL.get())
	sw.aiModel.SetText(prefAIModel.get())
	sw.aiThink.SetChecked(prefAIThink.get())
	sw.aiCtxEntry.SetText(strconv.Itoa(prefAICtx.get()))
}
