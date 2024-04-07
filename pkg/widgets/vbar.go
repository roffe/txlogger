package widgets

import (
	"image/color"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type VBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	bars        []*canvas.Line

	cfg *VBarConfig

	value float64

	canvas fyne.CanvasObject

	middle        float32
	heightFactor  float32
	diameterEight float32
	twoEight      float32
	size          fyne.Size
}

type VBarConfig struct {
	Title    string
	Min, Max float64
	Steps    int
	Minsize  fyne.Size
}

func NewVBar(cfg *VBarConfig) *VBar {
	s := &VBar{
		cfg: cfg,
	}
	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}
	s.canvas = s.render()
	s.ExtendBaseWidget(s)
	return s
}

func (s *VBar) render() *fyne.Container {
	s.face = &canvas.Rectangle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, FillColor: color.RGBA{0x00, 0x00, 0x00, 0x00}, StrokeWidth: 3}
	s.bar = &canvas.Rectangle{StrokeColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80}, FillColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80}}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	bar := container.NewWithoutLayout(s.face)
	for i := int(s.cfg.Steps + 1); i > 0; i-- {
		line := &canvas.Line{StrokeColor: color.RGBA{byte(i * 10), 0xE5 - byte(i*10), 0x00, 0xFF}, StrokeWidth: 2}
		s.bars = append(s.bars, line)
		bar.Add(line)
	}
	bar.Objects = append(bar.Objects, s.bar, s.titleText, s.displayText)
	return bar
}

func (s *VBar) Size() fyne.Size {
	return s.canvas.Size()
}
func (s *VBar) SetValue(value float64) {
	s.value = value
	br := uint8(0xA5 * (value / s.cfg.Max))
	bg := 0xA5 - br
	s.bar.FillColor = color.RGBA{br, bg, 0x00, 0x80}
	valueHeightfactor := float32(value) * s.heightFactor
	s.bar.Resize(fyne.NewSize(s.size.Width-s.twoEight, valueHeightfactor))
	s.bar.Move(fyne.NewPos(s.diameterEight, s.size.Height-valueHeightfactor))
	s.displayText.Text = strconv.FormatFloat(value, 'f', 0, 64)
	//s.displayText.Move(fyne.NewPos(s.middle-s.displayText.Size().Width*.5, s.size.Height-valueHeightfactor-12.5))
	s.displayText.Refresh()
}

func (s *VBar) Value() float64 {
	return s.value
}

func (s *VBar) CreateRenderer() fyne.WidgetRenderer {
	return &vbarRenderer{vbar: s}
}

type vbarRenderer struct {
	vbar *VBar
}

func (vr *vbarRenderer) Destroy() {
}

func (vr *vbarRenderer) Layout(space fyne.Size) {
	if vr.vbar.size.Width == space.Width && vr.vbar.size.Height == space.Height {
		return
	}
	log.Println("vbar.Layout", vr.vbar.cfg.Title, space.Width, space.Height)
	vr.vbar.size = space
	vr.vbar.canvas.Resize(space)
	vr.vbar.middle = space.Width * .5
	vr.vbar.diameterEight = space.Width * oneEight
	vr.vbar.twoEight = vr.vbar.diameterEight * 2
	stepFactor := float32(space.Height) / float32(vr.vbar.cfg.Steps)
	vr.vbar.heightFactor = float32(space.Height) / float32(vr.vbar.cfg.Max)
	vr.vbar.face.Resize(space)

	titleX := vr.vbar.middle - vr.vbar.titleText.Size().Width*.5
	displayTextX := vr.vbar.middle - vr.vbar.displayText.Size().Width*.5
	//displayTextY := space.Height - (float32(vr.vbar.value) * stepFactor) - 12.5
	displayTextY := space.Height - vr.vbar.displayText.MinSize().Height
	vr.vbar.titleText.Move(fyne.NewPos(titleX, space.Height+2))
	vr.vbar.displayText.Move(fyne.NewPos(displayTextX, displayTextY))

	s := vr.vbar
	for i, line := range s.bars {
		stepFactor := float32(i) * stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(s.middle-space.Width*oneThird, stepFactor)
			line.Position2 = fyne.NewPos(s.middle+space.Width*oneThird, stepFactor)
			continue
		}
		line.Position1 = fyne.NewPos(s.middle-space.Width*oneSeventh, stepFactor)
		line.Position2 = fyne.NewPos(s.middle+space.Width*oneSeventh, stepFactor)
	}

	vr.vbar.SetValue(vr.vbar.value)
}

func (vr *vbarRenderer) MinSize() fyne.Size {
	return vr.vbar.cfg.Minsize
}

func (vr *vbarRenderer) Refresh() {
	vr.vbar.canvas.Refresh()
}

func (vr *vbarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.vbar.canvas}
}
