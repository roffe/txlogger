package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/t7logger/pkg/widgets"
)

func NewDashboard(a fyne.App) fyne.Window {
	var rpmValue float64
	w := a.NewWindow("Dashboard")
	w.Resize(fyne.NewSize(800, 600))
	rpm, setFunc := widgets.NewGauge()
	w.SetContent(container.NewBorder(
		widget.NewLabel("RPM"),
		container.NewGridWithColumns(2,
			widget.NewButton("-100", func() {
				rpmValue -= 0.1
				setFunc(rpmValue)
			}),
			widget.NewButton("+100", func() {
				rpmValue += 0.1
				setFunc(rpmValue)
			}),
		),
		nil,
		nil,
		rpm))
	return w
}
