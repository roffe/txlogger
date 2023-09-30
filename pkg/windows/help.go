package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func Help(app fyne.App) {
	w := app.NewWindow("Help")
	w.Resize(fyne.NewSize(400, 600))
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Keyboard Shortcuts", theme.VisibilityIcon(), container.NewVBox(
			widget.NewLabel("Keyboard Shortcuts"),
			widget.NewLabel("F12: Capture screenshot"),
			widget.NewSeparator(),
			widget.NewLabel("Logplayer supports the following keyboard shortcuts"),
			widget.NewLabel("Space: Play/Pause"),
			widget.NewLabel("Left: Previous frame"),
			widget.NewLabel("Right: Next frame"),
			widget.NewLabel("Up: Skip 10 frames forward"),
			widget.NewLabel("Down: Skip 10 frames backward"),
			widget.NewLabel("PGUP: Skip 100 frames forward"),
			widget.NewLabel("PGDN: Skip 100 frames backward"),
			widget.NewLabel("Home/Return: Go to start"),
			widget.NewLabel("Plus: Increase playback speed"),
			widget.NewLabel("Minus: Decrease playback speed"),
		)),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), container.NewVBox(
			widget.NewLabel("TxLogger"),
			widget.NewLabel("Version: "+app.Metadata().Version),
			widget.NewLabel("Author: Roffe"),
			widget.NewSeparator(),
			widget.NewLabel("tHANKS tO:"),
			widget.NewLabel("SAAB for making the cars we love"),
			widget.NewLabel("The guys who made TrionicCANFlasher and TxSuite"),
			widget.NewLabel("Artursson"),
			widget.NewLabel("Schottis"),
			widget.NewLabel("o2o Crew"),
			widget.NewLabel("All supporters"),
		)),
	)
	w.SetContent(tabs)
	w.Show()
}
