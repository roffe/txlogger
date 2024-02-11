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
	w := app.NewWindow("Help")
	w.Resize(fyne.NewSize(400, 600))

	tx, _ := url.Parse("https://txlogger.com")
	kv, _ := url.Parse("https://www.kvaser.com")
	mt, _ := url.Parse("https://www.maptun.com/en/")
	tt, _ := url.Parse("https://www.trionictuning.com")

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("About", theme.InfoIcon(), container.NewVBox(
			widget.NewHyperlink("txlogger.com", tx),
			widget.NewLabel("Version: "+app.Metadata().Version+" Build: "+strconv.Itoa(app.Metadata().Build)),
			widget.NewLabel("Author: Roffe"),
			widget.NewSeparator(),
			widget.NewLabel("Special thanks to:"),
			widget.NewLabel("SAAB for making the cars we ❤️!"),
			widget.NewLabel("MattiasC, Dilemma, J.K Nilsson, Manick"),
			widget.NewLabel("Artursson, Schottis, Chriva, Myrtilos"),
			widget.NewLabel("Kalej, Bojer"),
			widget.NewHyperlink("TrionicTuning", tt),
			widget.NewHyperlink("Kvaser AB", kv),
			widget.NewHyperlink("Maptun Performance AB", mt),
			widget.NewLabel("o2o Crew"),
		)),
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
	)
	w.SetContent(tabs)
	w.Show()
}
