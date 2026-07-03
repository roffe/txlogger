package dualdial

import (
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type DualDial struct {
	widget.BaseWidget

	cfg *widgets.GaugeConfig

	titleText     *canvas.Text
	displayString string

	value  float64
	value2 float64

	// All gauge geometry (face, pips, both needles, center) in one object
	shader *canvas.Shader

	pipLabels []*canvas.Text

	displayText  *canvas.Text
	displayText2 *canvas.Text

	steps  float64
	factor float64

	size    fyne.Size
	minsize fyne.Size

	diameter       float32
	radius         float32
	middle         fyne.Position
	needleRotConst float64
	lineRotConst   float64

	// cached sin/cos for pip labels (angle_i = lineRotConst*i - common.Pi43)
	pipSin []float32
	pipCos []float32

	// fast float formatting buffers
	fmtPrec   int
	gaugePrec int
	buf1      []byte
	buf2      []byte

	// Label sizing cache (avoid per-label MinSize on each layout)
	maxLabelChars int
	labelBoxW     float32
	labelBoxH     float32
}

func New(cfg *widgets.GaugeConfig) *DualDial {
	s := &DualDial{
		cfg:           cfg,
		steps:         10,
		displayString: "%.0f",
		minsize:       fyne.NewSize(100, 100),
		fmtPrec:       -1,
	}
	s.ExtendBaseWidget(s)

	if cfg.Steps > 0 {
		s.steps = float64(cfg.Steps)
	}
	if cfg.DisplayString != "" {
		s.displayString = cfg.DisplayString
		if n := common.ParseFixedPrec(s.displayString); n >= 0 {
			s.fmtPrec = n
		}
	}
	if cfg.GaugeTextString != "" {
		if n := common.ParseFixedPrec(cfg.GaugeTextString); n >= 0 {
			s.gaugePrec = n
		}
	}
	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		s.minsize = cfg.MinSize
	}

	s.factor = s.cfg.Max / s.steps

	totalRange := s.cfg.Max - s.cfg.Min
	if totalRange <= 0 {
		totalRange = 1
	}
	s.needleRotConst = common.Pi15 / totalRange
	s.lineRotConst = common.Pi15 / s.steps

	s.shader = canvas.NewShader(
		"txlogger-dualdial",
		[]byte(dualDialShaderPreludeGL+dualDialShaderBody),
		[]byte(dualDialShaderPreludeES+dualDialShaderBody),
	)
	s.shader.Uniforms = map[string]float32{
		"size_d":    100,
		"steps":     float32(s.steps),
		"needle_a":  s.needleAngle(0),
		"needle2_a": s.needleAngle(0),
	}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	s.displayText.Alignment = fyne.TextAlignCenter

	s.displayText2 = &canvas.Text{Text: "0", Color: color.RGBA{R: 0xff, G: 0x0, B: 0, A: 0xFF}, TextSize: 35}
	s.displayText2.Alignment = fyne.TextAlignCenter

	// Labels at every other pip; also track the longest label length
	for i := 0; i <= int(s.steps); i++ {
		if i%2 == 0 {
			val := s.cfg.Min + (float64(i)/float64(s.cfg.Steps))*(s.cfg.Max-s.cfg.Min)
			txt := strconv.FormatFloat(val, 'f', s.gaugePrec, 64)
			lbl := &canvas.Text{
				Text:      txt,
				Color:     color.RGBA{0xE0, 0xE0, 0xE0, 0xFF},
				Alignment: fyne.TextAlignCenter,
			}
			if n := len(txt); n > s.maxLabelChars {
				s.maxLabelChars = n
			}
			s.pipLabels = append(s.pipLabels, lbl)
		} else {
			s.pipLabels = append(s.pipLabels, nil)
		}
	}

	// precompute pip trig for label placement (size independent)
	s.pipSin = make([]float32, int(s.steps)+1)
	s.pipCos = make([]float32, int(s.steps)+1)
	for i := 0; i <= int(s.steps); i++ {
		ang := s.lineRotConst*float64(i) - common.Pi43
		sinA, cosA := math.Sincos(ang)
		s.pipSin[i] = float32(sinA)
		s.pipCos[i] = float32(cosA)
	}

	return s
}

func (c *DualDial) GetConfig() *widgets.GaugeConfig { return c.cfg }

// needle angle for a face value; clamped below Min like the CPU renderer,
// free to overshoot above Max
func (c *DualDial) needleAngle(value float64) float32 {
	normalized := value - c.cfg.Min
	if normalized < 0 {
		normalized = 0
	}
	return float32(c.needleRotConst*normalized - common.Pi43)
}

func (c *DualDial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value

	c.shader.Uniforms["needle_a"] = c.needleAngle(value)

	c.buf1 = c.buf1[:0]
	if c.fmtPrec >= 0 {
		c.buf1 = strconv.AppendFloat(c.buf1, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf1 = common.AppendFormatFloat(c.buf1, c.displayString, value)
	}
	if !common.SameTextBytes(c.displayText.Text, c.buf1) {
		c.displayText.Text = string(c.buf1)
		canvas.Refresh(c.displayText)
	}

	canvas.Refresh(c.shader)
}

func (c *DualDial) SetValue2(value float64) {
	if value == c.value2 {
		return
	}
	c.value2 = value

	c.shader.Uniforms["needle2_a"] = c.needleAngle(value)

	c.buf2 = c.buf2[:0]
	if c.fmtPrec >= 0 {
		c.buf2 = strconv.AppendFloat(c.buf2, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf2 = common.AppendFormatFloat(c.buf2, c.displayString, value)
	}
	if !common.SameTextBytes(c.displayText2.Text, c.buf2) {
		c.displayText2.Text = string(c.buf2)
		canvas.Refresh(c.displayText2)
	}

	canvas.Refresh(c.shader)
}

func (c *DualDial) CreateRenderer() fyne.WidgetRenderer { return &DualDialRenderer{DualDial: c} }

type DualDialRenderer struct {
	*DualDial
	objects []fyne.CanvasObject
}

func (c *DualDialRenderer) Layout(space fyne.Size) {
	if c.size == space {
		return
	}
	c.size = space

	c.diameter = min(space.Width, space.Height)
	c.radius = c.diameter * common.OneHalf
	c.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)

	size := fyne.Size{Width: c.diameter, Height: c.diameter}
	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	c.shader.Move(topleft)
	c.shader.Resize(size)
	c.shader.Uniforms["size_d"] = c.diameter

	// Title & display text sizes (no math.Round needed)
	c.titleText.TextSize = c.radius * common.OneFourth
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))

	sixthDiameter := c.diameter * common.OneSixth

	c.displayText.TextSize = c.radius * common.OneThird
	c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneFifth))
	c.displayText.Resize(size)

	c.displayText2.TextSize = c.radius * common.OneThird
	c.displayText2.Move(topleft.AddXY(0, -sixthDiameter))
	c.displayText2.Resize(size)

	// Labels: reuse precomputed trig scaled by current radius
	radius43 := c.radius * common.OneFourth * 3

	// Label padding and cached box dims (avoid lbl.MinSize per label)
	labelPad := max(float32(6.0), c.radius*0.14)
	const charWidthFactor = 0.62
	const heightFactor = 1.15
	labelTextSize := c.radius * 0.10
	c.labelBoxW = float32(c.maxLabelChars) * float32(charWidthFactor) * labelTextSize
	c.labelBoxH = float32(heightFactor) * labelTextSize

	for i, lbl := range c.pipLabels {
		if lbl == nil {
			continue
		}
		lbl.TextSize = labelTextSize

		// place inside the gauge slightly inward from long pip inner end
		labelRadius := radius43 - labelPad
		cx := c.middle.X + c.pipSin[i]*labelRadius
		cy := c.middle.Y - c.pipCos[i]*labelRadius

		lbl.Resize(fyne.NewSize(c.labelBoxW, c.labelBoxH))
		lbl.Move(fyne.NewPos(cx-c.labelBoxW/2, cy-c.labelBoxH/2))
	}
}

func (c *DualDialRenderer) MinSize() fyne.Size { return c.minsize }
func (c *DualDialRenderer) Refresh()           {}
func (c *DualDialRenderer) Destroy()           {}

func (c *DualDialRenderer) Objects() []fyne.CanvasObject {
	if c.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(c.pipLabels)+4)
		objs = append(objs, c.shader)
		for _, v := range c.pipLabels {
			if v != nil {
				objs = append(objs, v)
			}
		}
		objs = append(objs, c.titleText, c.displayText, c.displayText2)
		c.objects = objs
	}
	return c.objects
}
