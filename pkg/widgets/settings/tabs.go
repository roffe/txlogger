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

func (sw *Widget) generalTab() *container.TabItem {
	return container.NewTabItem("General", container.NewVBox(
		leftLabeled("WD", sw.workDir),
		leftIcon(theme.InfoIcon(), sw.autoLoad),
		leftIcon(theme.WarningIcon(), sw.autoSave),
		leftIcon(theme.MoveUpIcon(), sw.cursorFollowCrosshair),
		leftIcon(theme.SearchIcon(), container.NewVBox(sw.livePreview, sw.realtimeBars)),
		leftIcon(theme.ViewFullScreenIcon(), sw.meshView),
		leftLabeled("Color blind mode", sw.colorBlindMode),
	))
}

func (sw *Widget) graphicsTab() *container.TabItem {
	return container.NewTabItem("Graphics", container.NewVBox(
		leftLabeled("Plot renderer", sw.plotRendererSelect),
		leftLabeled("Mesh renderer", sw.meshRendererSelect),
	))
}

func (sw *Widget) canTab() *container.TabItem {
	fixedLabel := func(text string) fyne.CanvasObject {
		return xlayout.NewFixedWidth(70, widget.NewLabel(text))
	}
	return container.NewTabItem("CAN", container.NewVBox(
		container.NewBorder(nil, nil, fixedLabel("Adapter"), sw.debugCheckbox, sw.adapterSelector),
		container.NewBorder(nil, nil, fixedLabel("Port"), sw.refreshBtn, sw.portSelector),
		container.NewBorder(nil, nil, fixedLabel("Info"), nil, sw.portDescription),
		container.NewBorder(nil, nil, fixedLabel("Speed"), nil, sw.speedSelector),
	))
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

	return container.NewTabItem("Logging", container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Logging rate (Hz)"), sw.freqValue, sw.freqSlider),
		widget.NewSeparator(),
		leftLabeled("Log format", sw.logFormat),
		container.NewBorder(nil, logFolderButtons, widget.NewLabel("Log folder"), nil, sw.logPath),
	))
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

	body := container.NewVBox(
		sw.wblSelectContainer,
		sw.wblADscanner,
		container.NewBorder(nil, nil, sw.wblPortLabel, sw.wblPortRefreshButton, sw.wblPortSelect),
		sw.wblADScannerSymbol,
	)

	image := container.NewHBox(layout.NewSpacer(), sw.wblImage, layout.NewSpacer())

	return container.NewTabItem("WBL", container.NewBorder(image, nil, nil, nil, body))
}

func (sw *Widget) dashboardTab() *container.TabItem {
	return container.NewTabItem("Dashboard", container.NewVBox(
		widget.NewLabel("Dashboard settings"),
		leftIcon(theme.InfoIcon(), sw.swapRPMandSpeed),
		leftIcon(theme.InfoIcon(), sw.useMPH),
	))
}

func (sw *Widget) adScannerTab() *container.TabItem {
	sw.wbleditor = NewWBLEditor(sw.GetWBLSupportPoints(), sw.GetWBLLambdaValues())
	sw.wbleditor.Hide()
	return container.NewTabItem("AD Scanner", sw.wbleditor)
}
