package windows

import (
	"net/url"
	"strconv"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

func Help() *container.AppTabs {
	kvaserLogo := canvas.NewImageFromResource(fyne.NewStaticResource("kvaser_logo.png", assets.KvaserLogoBytes))
	kvaserLogo.SetMinSize(fyne.NewSize(190, 117))
	kvaserLogo.FillMode = canvas.ImageFillContain
	kvaserLogo.ScaleMode = canvas.ImageScaleSmooth

	mackanizedLogo := canvas.NewImageFromResource(fyne.NewStaticResource("mackanized.png", assets.MackanizedBytes))
	mackanizedLogo.SetMinSize(fyne.NewSize(302, 80))
	mackanizedLogo.FillMode = canvas.ImageFillContain
	mackanizedLogo.ScaleMode = canvas.ImageScaleSmooth

	tx, _ := url.Parse("https://txlogger.com")
	tt, _ := url.Parse("https://www.trionictuning.com")
	kv, _ := url.Parse("https://www.kvaser.com")
	mk, _ := url.Parse("https://www.mackanized.eu/")

	mkLink := widget.NewHyperlink("Mackanized", mk)
	mkLink.Alignment = fyne.TextAlignCenter

	kvLink := widget.NewHyperlink("Kvaser AB", kv)
	kvLink.Alignment = fyne.TextAlignCenter

	lb2 := widget.NewLabel("Made with support from")
	lb2.Alignment = fyne.TextAlignCenter

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("About", theme.InfoIcon(),
			container.NewBorder(
				nil,
				nil,
				container.NewVBox(
					widget.NewHyperlink("txlogger.com", tx),
					widget.NewLabel("Version: "+fyne.CurrentApp().Metadata().Version+" Build: "+strconv.Itoa(fyne.CurrentApp().Metadata().Build)),
					widget.NewLabel("Author: Joakim \"Roffe\" Karlsson"),
					widget.NewLabel("Special thanks to:"),
					widget.NewLabel("SAAB for making the cars we ❤️❤️❤️"),
					widget.NewLabel("MattiasC, Dilemma, J.K Nilsson, Manick"),
					widget.NewLabel("Artursson, Schottis, Chriva, Myrtilos"),
					widget.NewLabel("Mackan, Kalej, Bojer, Saab74"),
					widget.NewLabel("catavares, Richardc9052, rk3"),
					widget.NewHyperlink("TrionicTuning", tt),
					widget.NewLabel("o2o Crew"),
				),
				container.NewVBox(
					lb2,
					container.NewBorder(
						nil,
						kvLink,
						nil,
						nil,
						kvaserLogo,
					),
					container.NewBorder(
						nil,
						mkLink,
						nil,
						nil,
						mackanizedLogo,
					),
					layout.NewSpacer(),
				),
			),
		),
		container.NewTabItemWithIcon("Keyboard Shortcuts", theme.VisibilityIcon(), container.NewGridWithColumns(2,
			container.NewVBox(
				widget.NewLabel("F12: Capture screenshot"),
				widget.NewSeparator(),
				widget.NewLabel("Logplayer supports the following keyboard shortcuts"),
				widget.NewLabel("Space: Play/Pause"),
				widget.NewLabel("Left: Previous frame"),
				widget.NewLabel("Right: Next frame"),
				widget.NewLabel("Up: Skip 10 frames forward"),
				widget.NewLabel("Down: Skip 10 frames backward"),
			),
			container.NewVBox(
				widget.NewLabel("PGUP: Skip 100 frames forward"),
				widget.NewLabel("PGDN: Skip 100 frames backward"),
				widget.NewLabel("Return/Home: Go to start"),
				widget.NewLabel("Plus: Increase playback speed"),
				widget.NewLabel("Minus: Decrease playback speed"),
				widget.NewLabel("Num Enter Reset playback speed"),
			)),
		),
	)
	return tabs
}
