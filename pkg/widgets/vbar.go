package widgets

import (
	"image/color"
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
	s.face = &canvas.Rectangle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, FillColor: color.RGBA{0x00, 0x00, 0x00, 0x00}, StrokeWidth: 2}
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
	// if value == s.value {
	// return
	// }
	if value > s.cfg.Max {
		value = s.cfg.Max
	}
	if value < s.cfg.Min {
		value = s.cfg.Min
	}

	s.value = value
	size := s.canvas.Size()
	heightFactor := float32(size.Height) / float32(s.cfg.Max)
	diameter := size.Width

	br := 0xA5 * (value / s.cfg.Max)
	bg := 0xA5 - br
	if bg < 0 {
		bg = 0
	}

	s.bar.FillColor = color.RGBA{byte(br), byte(bg), 0x00, 0x80}

	s.bar.Move(fyne.NewPos(diameter/8, size.Height-(float32(value)*heightFactor)))
	s.bar.Resize(fyne.NewSize(size.Width-(diameter/8*2), (float32(value) * heightFactor)))

	s.displayText.Text = strconv.FormatFloat(value, 'f', 0, 64)
	s.displayText.Move(fyne.NewPos(size.Width/2-s.displayText.Size().Width/2, size.Height-(float32(value)*heightFactor)-12.5))
	s.displayText.Refresh()
}

func (s *VBar) Value() float64 {
	return s.value
}

func (s *VBar) CreateRenderer() fyne.WidgetRenderer {
	return &vbarRenderer{s}
}

type vbarRenderer struct {
	vbar *VBar
}

func (vr *vbarRenderer) Destroy() {
}

func (vr *vbarRenderer) Layout(space fyne.Size) {
	vr.vbar.canvas.Resize(space)
	middle := space.Width / 2
	heightFactor := float32(space.Height) / float32(vr.vbar.cfg.Steps)

	vr.vbar.face.Resize(space)

	// Calculate positions once to avoid redundant calculations
	titleX := middle - vr.vbar.titleText.Size().Width/2
	displayTextX := middle - vr.vbar.displayText.Size().Width/2
	displayTextY := space.Height - (float32(vr.vbar.value) * heightFactor) - 12.5
	barY := space.Height - float32(vr.vbar.value)

	vr.vbar.titleText.Move(fyne.NewPos(titleX, space.Height+2))
	vr.vbar.displayText.Move(fyne.NewPos(displayTextX, displayTextY))
	vr.vbar.bar.Move(fyne.NewPos(0, barY))

	for i, line := range vr.vbar.bars {
		divisor := 3
		if i%2 != 0 {
			divisor = 7
		}

		offsetX := space.Width / float32(divisor)
		lineY := float32(i) * heightFactor

		line.Position1 = fyne.NewPos(middle-offsetX, lineY)
		line.Position2 = fyne.NewPos(middle+offsetX, lineY)
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
