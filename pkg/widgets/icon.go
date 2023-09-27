package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

type Icon struct {
	cfg    *IconConfig
	text   *canvas.Text
	canvas fyne.CanvasObject
}

type IconConfig struct {
	Image   *canvas.Image
	Text    string
	Minsize fyne.Size
}

func NewIcon(cfg *IconConfig) *Icon {
	ic := &Icon{
		cfg: cfg,
	}

	cfg.Image.FillMode = canvas.ImageFillContain
	cfg.Image.SetMinSize(cfg.Minsize)

	ic.render()
	return ic
}

func (ic *Icon) render() *fyne.Container {
	ic.text = canvas.NewText("----", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
	ic.text.TextSize = 25
	ic.text.TextStyle.Monospace = true
	ic.text.Alignment = fyne.TextAlignLeading

	c := container.NewWithoutLayout(ic.cfg.Image, ic.text)
	c.Layout = ic
	ic.canvas = c
	return c
}

func (ic *Icon) Layout(_ []fyne.CanvasObject, space fyne.Size) {
	ic.cfg.Image.Move(fyne.NewPos(0, 0))
	ic.cfg.Image.Resize(ic.cfg.Minsize)
	ic.text.Resize(fyne.NewSize(space.Width, 30))
	ic.text.Move(fyne.NewPos(14, 87))
}

func (ic *Icon) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return ic.cfg.Minsize
}

func (ic *Icon) Content() fyne.CanvasObject {
	return ic.canvas
}

func (ic *Icon) Hide() {
	ic.cfg.Image.Hide()
	ic.text.Hide()
}

func (ic *Icon) Show() {
	ic.cfg.Image.Show()
	ic.text.Show()
}

func (ic *Icon) SetText(text string) {
	ic.cfg.Text = text
	ic.text.Text = text
	ic.text.Refresh()
}
