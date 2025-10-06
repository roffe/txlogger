package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

func (mw *MainWindow) newToolbar() *fyne.Container {
	return container.NewHBox(
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("ECU"),
			nil,
			mw.selects.ecuSelect,
		),
		widget.NewSeparator(),
		mw.buttons.symbolListBtn,
		mw.buttons.logBtn,

		//mw.buttons.logplayerBtn,
		mw.buttons.openLogBtn,
		mw.buttons.dashboardBtn,
		widget.NewButtonWithIcon("", theme.GridIcon(), func() {
			mw.wm.Arrange(&multiwindow.GridArranger{})
		}),
		mw.buttons.addGaugeBtn,
		widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
			mw.wm.CloseAll()
		}),
	)
}
