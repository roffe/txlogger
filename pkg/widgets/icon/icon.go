package icon

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

type Icon struct {
	widget.BaseWidget
	cfg  *Config
	text *canvas.Text
}

type Config struct {
	Image   *canvas.Image
	Text    string
	Minsize fyne.Size
}

func New(cfg *Config) *Icon {
	ic := &Icon{
		cfg: cfg,
	}
	cfg.Image.FillMode = canvas.ImageFillContain
	cfg.Image.ScaleMode = canvas.ImageScaleFastest

	cfg.Image.SetMinSize(cfg.Minsize)

	ic.text = canvas.NewText("----", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
	ic.text.TextSize = 25
	ic.text.TextStyle.Monospace = true
	ic.text.Alignment = fyne.TextAlignLeading
	return ic
}

func (ic *Icon) SetText(text string) {
	if text == ic.cfg.Text {
		return
	}
	ic.cfg.Text = text
	ic.text.Text = text
	ic.text.Refresh()
}

func (ic *Icon) CreateRenderer() fyne.WidgetRenderer {
	return &IconRenderer{ic}
}

type IconRenderer struct {
	*Icon
}

func (ic *IconRenderer) Layout(size fyne.Size) {
	ic.cfg.Image.Move(fyne.NewPos(0, 0))
	ic.cfg.Image.Resize(ic.cfg.Minsize)
	ic.text.Resize(fyne.NewSize(size.Width, 30))
	ic.text.Move(fyne.NewPos(14, 87))
}

func (ic *IconRenderer) MinSize() fyne.Size {
	return ic.cfg.Minsize
}

func (ic *IconRenderer) Refresh() {
}

func (ic *IconRenderer) Destroy() {
}

func (ic *IconRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{ic.cfg.Image, ic.text}
}
