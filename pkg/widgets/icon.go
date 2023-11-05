package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Icon struct {
	widget.BaseWidget
	cfg       *IconConfig
	text      *canvas.Text
	container *fyne.Container
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

func (ic *Icon) render() {
	ic.text = canvas.NewText("----", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
	ic.text.TextSize = 25
	ic.text.TextStyle.Monospace = true
	ic.text.Alignment = fyne.TextAlignLeading
	ic.container = container.NewWithoutLayout(ic.cfg.Image, ic.text)
}

func (ic *Icon) SetText(text string) {
	ic.cfg.Text = text
	ic.text.Text = text
	ic.text.Refresh()
}

func (ic *Icon) CreateRenderer() fyne.WidgetRenderer {
	return &IconRenderer{
		ic: ic,
	}
}

type IconRenderer struct {
	ic *Icon
}

func (icr *IconRenderer) Layout(size fyne.Size) {
	icr.ic.cfg.Image.Move(fyne.NewPos(0, 0))
	icr.ic.cfg.Image.Resize(icr.ic.cfg.Minsize)
	icr.ic.text.Resize(fyne.NewSize(size.Width, 30))
	icr.ic.text.Move(fyne.NewPos(14, 87))
}

func (icr *IconRenderer) MinSize() fyne.Size {
	return icr.ic.cfg.Minsize
}

func (icr *IconRenderer) Refresh() {
}

func (icr *IconRenderer) Destroy() {
}

func (icr *IconRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{icr.ic.container}
}
