package windows

import (
	"net/url"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func Help(app fyne.App) {
	u, err := url.Parse("https://txlogger.com")
	if err != nil {
		panic(err)
	}
	link := widget.NewHyperlink(u.Host, u)
	link.Alignment = fyne.TextAlignLeading
	link.TextStyle = fyne.TextStyle{Bold: true}

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
			widget.NewLabel("Return/Home: Go to start"),
			widget.NewLabel("Plus: Increase playback speed"),
			widget.NewLabel("Minus: Decrease playback speed"),
			widget.NewLabel("Num Enter Reset playback speed"),
		)),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), container.NewVBox(
			widget.NewLabel("TxLogger"),
			widget.NewLabel("Version: "+app.Metadata().Version+" Build: "+strconv.Itoa(app.Metadata().Build)),
			widget.NewLabel("Author: Roffe"),
			link,
			widget.NewSeparator(),
			widget.NewLabel("tHANKS tO:"),
			widget.NewLabel("SAAB for making the cars we love"),
			widget.NewLabel("The guys who made TrionicCANFlasher and TxSuite"),
			widget.NewLabel("Kalej"),
			widget.NewLabel("Artursson"),
			widget.NewLabel("Schottis"),
			widget.NewLabel("o2o Crew"),
			widget.NewLabel("All supporters"),
		)),
	)
	w.SetContent(tabs)
	w.Show()
}
