package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	xlayout "github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets"
)

// leftLabeled places a text label to the left of content.
func leftLabeled(label string, content fyne.CanvasObject) *fyne.Container {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, content)
}

// leftIcon places an icon to the left of content.
func leftIcon(res fyne.Resource, content fyne.CanvasObject) *fyne.Container {
	return container.NewBorder(nil, nil, widget.NewIcon(res), nil, content)
}

// section groups related controls under a titled card, the main visual
// building block of the settings panel.
func section(title string, rows ...fyne.CanvasObject) *widget.Card {
	return widget.NewCard(title, "", container.NewVBox(rows...))
}

// tab wraps a stack of cards in a padded page.
func tab(title string, icon fyne.Resource, cards ...fyne.CanvasObject) *container.TabItem {
	page := container.NewPadded(container.NewVBox(cards...))
	return container.NewTabItemWithIcon(title, icon, page)
}

func (sw *Widget) generalTab() *container.TabItem {
	return tab("General", theme.SettingsIcon(),
		section("ECU connection",
			leftIcon(theme.InfoIcon(), sw.autoLoad),
			leftIcon(theme.WarningIcon(), sw.autoSave),
		),
		section("Map editor & preview",
			leftIcon(theme.MoveUpIcon(), sw.cursorFollowCrosshair),
			leftIcon(theme.ViewFullScreenIcon(), sw.meshView),
			leftIcon(theme.SearchIcon(), container.NewVBox(sw.livePreview, sw.realtimeBars)),
		),
		section("Appearance",
			leftLabeled("Color blind mode", sw.colorBlindMode),
		),
		section("Working directory",
			sw.workDir,
		),
	)
}

func (sw *Widget) graphicsTab() *container.TabItem {
	return tab("Graphics", theme.ColorPaletteIcon(),
		section("Renderers",
			leftLabeled("Plot renderer", sw.plotRendererSelect),
			leftLabeled("Mesh renderer", sw.meshRendererSelect),
		),
		section("Dashboard",
			leftIcon(theme.MoveUpIcon(), sw.swapRPMandSpeed),
			leftIcon(theme.InfoIcon(), sw.useMPH),
		),
	)
}

func (sw *Widget) canTab() *container.TabItem {
	fixedLabel := func(text string) fyne.CanvasObject {
		return xlayout.NewFixedWidth(70, widget.NewLabel(text))
	}
	return tab("CAN", theme.ComputerIcon(),
		section("Adapter",
			container.NewBorder(nil, nil, fixedLabel("Adapter"), sw.debugCheckbox, sw.adapterSelector),
			container.NewBorder(nil, nil, fixedLabel("Port"), sw.refreshBtn, sw.portSelector),
			container.NewBorder(nil, nil, fixedLabel("Info"), nil, sw.portDescription),
			container.NewBorder(nil, nil, fixedLabel("Speed"), nil, sw.speedSelector),
		),
	)
}

func (sw *Widget) loggingTab() *container.TabItem {
	logFolderButtons := container.NewGridWithColumns(2,
		widget.NewButtonWithIcon("Reset", theme.ContentClearIcon(), func() {
			logPath, err := common.GetLogPath()
			if err != nil {
				fyne.LogError("Could not get log path", err)
			}
			sw.logPath.SetText(logPath)
			prefLogPath.set(logPath)
		}),
		widget.NewButtonWithIcon("Browse", theme.FileIcon(), func() {
			widgets.SelectFolder(func(dir string) {
				sw.logPath.SetText(dir)
				prefLogPath.set(dir)
			})
		}),
	)

	return tab("Logging", theme.DocumentSaveIcon(),
		section("Capture",
			container.NewBorder(nil, nil, widget.NewLabel("Logging rate (Hz)"), sw.freqValue, sw.freqSlider),
			leftLabeled("Log format", sw.logFormat),
		),
		section("Storage",
			container.NewBorder(nil, logFolderButtons, widget.NewLabel("Log folder"), nil, sw.logPath),
		),
		section("Experimental",
			container.NewBorder(nil, nil, widget.NewIcon(theme.WarningIcon()), widget.NewLabel("MIGHT CORRUPT RAM!!"), sw.experimentalT5FastLogger),
		),
	)
}

func (sw *Widget) wblTab() *container.TabItem {
	wblPorts := func() []string {
		return append([]string{"txbridge", "CAN"}, sw.ListPorts()...)
	}

	sw.wblPortLabel = widget.NewLabel("WBL Port")
	sw.wblPortSelect = widget.NewSelect(wblPorts(), prefWBLPort.set)
	sw.wblPortRefreshButton = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.wblPortSelect.Options = wblPorts()
		sw.wblPortSelect.Refresh()
	})

	sw.wblADscanner = sw.newADscannerCheck()

	adSymbols := []string{
		"AD_EGR",
		"DisplProt.AD_Scanner",
		"LambdaScan.AD_Scanner",
		"LambdaScan.AD_Scanner2",
	}
	sw.wblADScannerSymbol = widget.NewSelect(adSymbols, prefWBLADScannerSymbol.set)
	sw.wblADScannerSymbol.SetSelected(sw.GetADScannerSymbolName())

	image := container.NewHBox(layout.NewSpacer(), sw.wblImage, layout.NewSpacer())

	settings := section("Wideband source",
		sw.wblSelectContainer,
		sw.wblADscanner,
		container.NewBorder(nil, nil, sw.wblPortLabel, sw.wblPortRefreshButton, sw.wblPortSelect),
		sw.wblADScannerSymbol,
	)

	page := container.NewPadded(container.NewVBox(image, settings))
	return container.NewTabItemWithIcon("WBL", theme.MediaRecordIcon(), page)
}

func (sw *Widget) adScannerTab() *container.TabItem {
	sw.wbleditor = NewWBLEditor(sw.GetWBLSupportPoints(), sw.GetWBLLambdaValues())
	sw.wbleditor.Hide()
	return container.NewTabItemWithIcon("AD Scanner", theme.SearchIcon(), sw.wbleditor)
}
