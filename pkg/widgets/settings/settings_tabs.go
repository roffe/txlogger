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

func (sw *Widget) generalTab() *container.TabItem {
	return container.NewTabItem("General", container.NewVBox(
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("WD"),
			nil,
			sw.workDir,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.InfoIcon()),
			nil,
			sw.autoLoad,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.WarningIcon()),
			nil,
			sw.autoSave,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.MoveUpIcon()),
			nil,
			sw.cursorFollowCrosshair,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.SearchIcon()),
			nil,
			container.NewVBox(
				sw.livePreview,
				sw.realtimeBars,
			),
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.ViewFullScreenIcon()),
			nil,
			sw.meshView,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("Color blind mode"),
			nil,
			sw.colorBlindMode,
		),
	))
}

func (sw *Widget) loggingTab() *container.TabItem {
	return container.NewTabItem("Logging", container.NewVBox(
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("Logging rate (Hz)"),
			sw.freqValue,
			sw.freqSlider,
		),
		widget.NewSeparator(),
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("Log format"),
			nil,
			sw.logFormat,
		),
		container.NewBorder(
			nil,
			container.NewGridWithColumns(2,
				widget.NewButtonWithIcon("Reset", theme.ContentClearIcon(), func() {
					logPath, err := common.GetLogPath()
					if err != nil {
						fyne.LogError("Could not get log path", err)
					}
					sw.logPath.SetText(logPath)
					fyne.CurrentApp().Preferences().SetString(prefsLogPath, logPath)
				}),
				widget.NewButtonWithIcon("Browse", theme.FileIcon(), func() {
					cb := func(dir string) {
						sw.logPath.SetText(dir)
						fyne.CurrentApp().Preferences().SetString(prefsLogPath, dir)
					}
					widgets.SelectFolder(cb)
				}),
			),
			widget.NewLabel("Log folder"),
			nil,
			sw.logPath,
		),
	))
}

func (sw *Widget) wblTab() *container.TabItem {
	sw.wblPortLabel = widget.NewLabel("WBL Port")
	sw.wblPortSelect = widget.NewSelect(append([]string{"txbridge", "CAN"}, sw.ListPorts()...), func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsWBLPort, s)
	})

	sw.wblPortRefreshButton = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.wblPortSelect.Options = append([]string{"txbridge", "CAN"}, sw.ListPorts()...)
		sw.wblPortSelect.Refresh()
	})

	sw.wblADscanner = sw.newADscannerCheck()

	return container.NewTabItem(
		"WBL",
		container.NewBorder(
			container.NewHBox(
				layout.NewSpacer(),
				sw.images.wblImage,
				layout.NewSpacer(),
			),
			nil,
			nil,
			nil,
			container.NewVBox(
				sw.wblSelectContainer,
				container.NewBorder(
					nil,
					nil,
					nil,
					nil,
					sw.wblADscanner,
				),
				container.NewBorder(
					nil,
					nil,
					sw.wblPortLabel,
					sw.wblPortRefreshButton,
					sw.wblPortSelect,
				),
			),
		),
	)
}

func (sw *Widget) dashboardTab() *container.TabItem {
	return container.NewTabItem("Dashboard", container.NewVBox(
		widget.NewLabel("Dashboard settings"),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.InfoIcon()),
			nil,
			sw.swapRPMandSpeed,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.InfoIcon()),
			nil,
			sw.useMPH,
		),
	))
}

func (sw *Widget) adScannerTab() *container.TabItem {
	sw.wbleditor = NewWBLEditor(sw.GetWBLSupportPoints(), sw.GetWBLLambdaValues())
	sw.wbleditor.Hide()
	return container.NewTabItem("AD Scanner", sw.wbleditor)
}

func (sw *Widget) canTab() *container.TabItem {
	return container.NewTabItem("CAN", container.NewVBox(
		container.NewBorder(
			nil,
			nil,
			xlayout.NewFixedWidth(70, widget.NewLabel("Adapter")),
			sw.debugCheckbox,
			sw.adapterSelector,
		),
		container.NewBorder(
			nil,
			nil,
			xlayout.NewFixedWidth(70, widget.NewLabel("Port")),
			sw.refreshBtn,
			sw.portSelector,
		),
		container.NewBorder(
			nil,
			nil,
			xlayout.NewFixedWidth(70, widget.NewLabel("Info")),
			nil,
			sw.portDescription,
		),
		container.NewBorder(
			nil,
			nil,
			xlayout.NewFixedWidth(70, widget.NewLabel("Speed")),
			nil,
			sw.speedSelector,
		),
	))
}
