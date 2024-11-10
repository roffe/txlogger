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

	value float64

	canvas fyne.CanvasObject

	middle        float32
	heightFactor  float32
	diameterEight float32
	twoEight      float32
	size          fyne.Size
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
	s.face = &canvas.Rectangle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF}, FillColor: color.RGBA{0x00, 0x00, 0x00, 0x00}, StrokeWidth: 3}

	// Initialize bar with appropriate starting color
	fillColor, strokeColor := s.getColorForValue(0)
	s.bar = &canvas.Rectangle{StrokeColor: strokeColor, FillColor: fillColor}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	bar := container.NewWithoutLayout()
	for i := int(s.cfg.Steps + 1); i > 0; i-- {
		_, strokeColor := s.getColorForValue(float64(i) / float64(s.cfg.Steps+1) * s.cfg.Max)
		line := &canvas.Line{StrokeColor: strokeColor, StrokeWidth: 2}
		s.bars = append(s.bars, line)
		bar.Add(line)
	}
	bar.Objects = append(bar.Objects, s.bar, s.titleText, s.face, s.displayText)
	return bar
}

func (s *VBar) Size() fyne.Size {
	return s.canvas.Size()
}

func (s *VBar) SetValue(value float64) {
	if value == s.value {
		return
	}
	s.value = value

	fillColor, _ := s.getColorForValue(value)
	s.bar.FillColor = fillColor

	valueHeightfactor := float32(value) * s.heightFactor
	s.bar.Resize(fyne.NewSize(s.size.Width-s.twoEight, valueHeightfactor))
	s.bar.Move(fyne.NewPos(s.diameterEight, s.size.Height-valueHeightfactor))
	s.displayText.Text = strconv.FormatFloat(value, 'f', 0, 64)
	s.displayText.Refresh()
}

func (s *VBar) Value() float64 {
	return s.value
}

func (s *VBar) CreateRenderer() fyne.WidgetRenderer {
	return &vbarRenderer{vbar: s}
}

func (s *VBar) getColorForValue(value float64) (fillColor, strokeColor color.RGBA) {
	ratio := value / s.cfg.Max

	switch s.cfg.ColorScale {
	case BlueYellowScale:
		// Blue to Yellow to Red scale
		if ratio < 0.5 {
			// Blue to Yellow (0.0 - 0.5)
			blueRatio := 1 - (ratio * 2) // 1.0 to 0.0
			return color.RGBA{
					R: uint8(0xDD * ratio * 2),          // 0x00 to 0xDD
					G: uint8(0x77 + (0x33 * ratio * 2)), // 0x77 to 0xAA
					B: uint8(0xBB * blueRatio),          // 0xBB to 0x33
					A: 0x80,
				}, color.RGBA{
					R: uint8(0xDD * ratio * 2),
					G: uint8(0x77 + (0x33 * ratio * 2)),
					B: uint8(0xBB * blueRatio),
					A: 0xFF,
				}
		}
		// Yellow to Red (0.5 - 1.0)
		redRatio := (ratio - 0.5) * 2 // 0.0 to 1.0
		return color.RGBA{
				R: uint8(0xDD - (0x11 * redRatio)), // 0xDD to 0xCC
				G: uint8(0xAA - (0x77 * redRatio)), // 0xAA to 0x33
				B: uint8(0x33),                     // 0x33
				A: 0x80,
			}, color.RGBA{
				R: uint8(0xDD - (0x11 * redRatio)),
				G: uint8(0xAA - (0x77 * redRatio)),
				B: uint8(0x33),
				A: 0xFF,
			}

	default: // TraditionalScale
		// Traditional Green to Red scale
		return color.RGBA{
				R: uint8(0xA5 * ratio),
				G: uint8(0xA5 * (1 - ratio)),
				B: 0x00,
				A: 0x80,
			}, color.RGBA{
				R: uint8(0xA5 * ratio),
				G: uint8(0xA5 * (1 - ratio)),
				B: 0x00,
				A: 0xFF,
			}
	}
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
	// log.Println("vbar.Layout", vr.vbar.cfg.Title, space.Width, space.Height)
	vr.vbar.size = space
	vr.vbar.canvas.Resize(space)
	vr.vbar.middle = space.Width * .5
	vr.vbar.diameterEight = space.Width * common.OneEight
	vr.vbar.twoEight = vr.vbar.diameterEight * 2
	stepFactor := float32(space.Height) / float32(vr.vbar.cfg.Steps)
	vr.vbar.heightFactor = float32(space.Height) / float32(vr.vbar.cfg.Max)
	vr.vbar.face.Move(fyne.NewPos(0, -2))
	vr.vbar.face.Resize(space.AddWidthHeight(0, 3))

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
			line.Position1 = fyne.NewPos(s.middle-space.Width*common.OneThird, stepFactor)
			line.Position2 = fyne.NewPos(s.middle+space.Width*common.OneThird, stepFactor)
			continue
		}
		line.Position1 = fyne.NewPos(s.middle-space.Width*common.OneSeventh, stepFactor)
		line.Position2 = fyne.NewPos(s.middle+space.Width*common.OneSeventh, stepFactor)
	}

	valueHeightfactor := float32(vr.vbar.value) * vr.vbar.heightFactor
	vr.vbar.bar.Resize(fyne.NewSize(s.size.Width-s.twoEight, valueHeightfactor))
	vr.vbar.bar.Move(fyne.NewPos(s.diameterEight, s.size.Height-valueHeightfactor))
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
