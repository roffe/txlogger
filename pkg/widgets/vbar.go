package widgets

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
)

type VBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	bars        []*canvas.Line

	cfg *VBarConfig

	value        float64
	canvas       fyne.CanvasObject
	size         fyne.Size
	layoutValues struct {
		middle        float32
		heightFactor  float32
		diameterEight float32
		twoEight      float32
		titleX        float32
		displayTextX  float32
	}
}

type VBarConfig struct {
	Title      string
	Min, Max   float64
	Steps      int
	Minsize    fyne.Size
	ColorScale ColorScheme
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
	s.face = &canvas.Rectangle{
		StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},
		FillColor:   color.RGBA{0x00, 0x00, 0x00, 0x00},
		StrokeWidth: 3,
	}

	fillColor, strokeColor := s.getColorForValue(0)
	s.bar = &canvas.Rectangle{
		StrokeColor: strokeColor,
		FillColor:   fillColor,
	}

	s.titleText = &canvas.Text{
		Text:     s.cfg.Title,
		Color:    color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF},
		TextSize: 25,
	}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{
		Text:     "0",
		Color:    color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF},
		TextSize: 25,
	}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	// Pre-calculate text positions
	s.layoutValues.titleX = -s.titleText.Size().Width * 0.5
	s.layoutValues.displayTextX = -s.displayText.Size().Width * 0.5

	bar := container.NewWithoutLayout()
	maxSteps := s.cfg.Steps + 1
	s.bars = make([]*canvas.Line, maxSteps)
	for i := maxSteps; i > 0; i-- {
		_, strokeColor := s.getColorForValue(float64(i) / float64(maxSteps) * s.cfg.Max)
		line := &canvas.Line{StrokeColor: strokeColor, StrokeWidth: 2}
		s.bars[maxSteps-i] = line
		bar.Add(line)
	}
	bar.Objects = append(bar.Objects, s.bar, s.titleText, s.face, s.displayText)
	return bar
}

func (s *VBar) Size() fyne.Size {
	return s.canvas.Size()
}

func (s *VBar) Resize(space fyne.Size) {
	if s.size == space {
		return
	}

	s.size = space
	s.canvas.Resize(space)

	// Cache layout calculations
	s.layoutValues.middle = space.Width * 0.5
	s.layoutValues.diameterEight = space.Width * common.OneEight
	s.layoutValues.twoEight = s.layoutValues.diameterEight * 2
	s.layoutValues.heightFactor = float32(space.Height) / float32(s.cfg.Max)

	stepFactor := float32(space.Height) / float32(s.cfg.Steps)

	// Face layout
	s.face.Move(fyne.NewPos(0, -2))
	s.face.Resize(space.AddWidthHeight(0, 3))

	// Text layout
	s.titleText.Move(fyne.NewPos(
		s.layoutValues.middle+s.layoutValues.titleX,
		space.Height+2,
	))
	s.displayText.Move(fyne.NewPos(
		s.layoutValues.middle+s.layoutValues.displayTextX,
		space.Height-s.displayText.MinSize().Height,
	))

	// Bar lines layout
	oneThird := space.Width * common.OneThird
	oneSeventh := space.Width * common.OneSeventh
	middle := s.layoutValues.middle

	for i, line := range s.bars {
		y := float32(i) * stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(middle-oneThird, y)
			line.Position2 = fyne.NewPos(middle+oneThird, y)
		} else {
			line.Position1 = fyne.NewPos(middle-oneSeventh, y)
			line.Position2 = fyne.NewPos(middle+oneSeventh, y)
		}
	}

	// Update bar position
	valueHeightFactor := float32(s.value) * s.layoutValues.heightFactor
	s.bar.Resize(fyne.NewSize(s.size.Width-s.layoutValues.twoEight, valueHeightFactor))
	s.bar.Move(fyne.NewPos(s.layoutValues.diameterEight, s.size.Height-valueHeightFactor))
}

func (s *VBar) SetValue(value float64) {
	if value == s.value {
		return
	}
	s.value = value

	fillColor, _ := s.getColorForValue(value)
	s.bar.FillColor = fillColor

	valueHeightFactor := float32(value) * s.layoutValues.heightFactor
	s.bar.Resize(fyne.NewSize(s.size.Width-s.layoutValues.twoEight, valueHeightFactor))
	s.bar.Move(fyne.NewPos(s.layoutValues.diameterEight, s.size.Height-valueHeightFactor))

	// Format number directly without conversion
	s.displayText.Text = strconv.FormatFloat(value, 'f', 0, 64)
	s.displayText.Refresh()
}

func (s *VBar) Value() float64 {
	return s.value
}

func (s *VBar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.canvas)
}

func (s *VBar) getColorForValue(value float64) (fillColor, strokeColor color.RGBA) {
	ratio := value / s.cfg.Max

	if s.cfg.ColorScale == BlueYellowScale {
		if ratio < 0.5 {
			blueRatio := 1 - (ratio * 2)
			r := uint8(0xDD * ratio * 2)
			g := uint8(0x77 + (0x33 * ratio * 2))
			b := uint8(0xBB * blueRatio)
			return color.RGBA{R: r, G: g, B: b, A: 0x80},
				color.RGBA{R: r, G: g, B: b, A: 0xFF}
		}
		redRatio := (ratio - 0.5) * 2
		r := uint8(0xDD - (0x11 * redRatio))
		g := uint8(0xAA - (0x77 * redRatio))
		return color.RGBA{R: r, G: g, B: 0x33, A: 0x80},
			color.RGBA{R: r, G: g, B: 0x33, A: 0xFF}
	}

	// Traditional scale
	r := uint8(0xA5 * ratio)
	g := uint8(0xA5 * (1 - ratio))
	return color.RGBA{R: r, G: g, B: 0, A: 0x80},
		color.RGBA{R: r, G: g, B: 0, A: 0xFF}
}
