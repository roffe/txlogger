package dial

import (
	"image/color"
	"math"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type Dial struct {
	widget.BaseWidget
	displayString string

	cfg *widgets.GaugeConfig

	factor          float64
	value           float64
	highestObserved float64

	lastHighestObserved time.Time

	// All dial geometry (face, pips, needle, marker, center) in one object
	shader *canvas.Shader

	pipLabels []*canvas.Text

	displayText *canvas.Text
	titleText   *canvas.Text

	size    fyne.Size
	minsize fyne.Size

	diameter       float32
	radius         float32
	middle         fyne.Position
	needleRotConst float64 // = common.Pi15/(Max-Min)
	lineRotConst   float64 // = common.Pi15/steps

	// Precomputed trig for pip labels: angle_i = lineRotConst*float64(i) - common.Pi43
	pipSin []float32
	pipCos []float32

	// Fast float formatting
	fmtPrec   int // precision extracted from displayString like "%.0f", "%.1f", defaults to -1
	gaugePrec int // precision extracted from GaugeTextString like "%.0f", "%.1f", defaults to -1
	buf       []byte

	// Label sizing cache (avoids MinSize calls every layout)
	maxLabelChars int     // longest label length at construction
	labelBoxW     float32 // computed each layout from TextSize and maxLabelChars
	labelBoxH     float32 // computed each layout from TextSize
}

func New(cfg *widgets.GaugeConfig) *Dial {
	c := &Dial{
		cfg:           cfg,
		displayString: "%.0f",
		minsize:       fyne.NewSize(100, 100),
		fmtPrec:       -1,
	}
	c.ExtendBaseWidget(c)

	if cfg.DisplayString != "" {
		c.displayString = cfg.DisplayString
		if n := common.ParseFixedPrec(c.displayString); n >= 0 {
			c.fmtPrec = n
		}
	}
	if cfg.GaugeTextString != "" {
		if n := common.ParseFixedPrec(cfg.GaugeTextString); n >= 0 {
			c.gaugePrec = n
		}
	}
	if cfg.GaugeFactor == 0 {
		cfg.GaugeFactor = 1.0
	}
	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		c.minsize = cfg.MinSize
	}

	steps := float64(cfg.Steps)
	totalRange := c.cfg.Max - c.cfg.Min
	if totalRange <= 0 {
		totalRange = 1
	}

	c.factor = c.cfg.Max / steps

	// Constants
	c.needleRotConst = common.Pi15 / totalRange
	c.lineRotConst = common.Pi15 / steps

	c.shader = canvas.NewShader(
		"txlogger-dial",
		[]byte(dialShaderPreludeGL+dialShaderBody),
		[]byte(dialShaderPreludeES+dialShaderBody),
	)
	c.shader.Uniforms = map[string]float32{
		"size_d":   100,
		"steps":    float32(cfg.Steps),
		"needle_a": c.needleAngle(0),
		"marker_a": c.needleAngle(0),
	}

	c.titleText = &canvas.Text{Text: c.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	c.titleText.TextStyle.Monospace = true
	c.titleText.Alignment = fyne.TextAlignCenter

	c.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	c.displayText.Alignment = fyne.TextAlignCenter

	// Labels at every other pip; also track the longest label length
	for i := 0; i < c.cfg.Steps+1; i++ {
		if i%2 == 0 {
			val := c.cfg.Min + (float64(i)/float64(c.cfg.Steps))*(c.cfg.Max-c.cfg.Min)*c.cfg.GaugeFactor
			txt := strconv.FormatFloat(val, 'f', c.gaugePrec, 64)

			lbl := &canvas.Text{
				Text:      txt,
				Color:     color.RGBA{0xE0, 0xE0, 0xE0, 0xFF},
				Alignment: fyne.TextAlignCenter,
			}
			if n := len(txt); n > c.maxLabelChars {
				c.maxLabelChars = n
			}
			c.pipLabels = append(c.pipLabels, lbl)
		} else {
			c.pipLabels = append(c.pipLabels, nil)
		}
	}

	// Precompute pip sin/cos for label placement (size-independent)
	c.pipSin = make([]float32, c.cfg.Steps+1)
	c.pipCos = make([]float32, c.cfg.Steps+1)
	for i := 0; i <= c.cfg.Steps; i++ {
		ang := c.lineRotConst*float64(i) - common.Pi43
		s, co := math.Sincos(ang)
		c.pipSin[i] = float32(s)
		c.pipCos[i] = float32(co)
	}

	return c
}

func (c *Dial) GetConfig() *widgets.GaugeConfig { return c.cfg }

// needle angle for a face value; clamped below Min like the CPU renderer,
// free to overshoot above Max
func (c *Dial) needleAngle(value float64) float32 {
	normalized := value - c.cfg.Min
	if normalized < 0 {
		normalized = 0
	}
	return float32(c.needleRotConst*normalized - common.Pi43)
}

func (c *Dial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value

	c.shader.Uniforms["needle_a"] = c.needleAngle(value)

	// Highest observed marker with lazy reset
	if value > c.highestObserved || time.Since(c.lastHighestObserved) > 10*time.Second {
		c.highestObserved = value
		c.lastHighestObserved = time.Now()
		c.shader.Uniforms["marker_a"] = c.needleAngle(value)
	}

	// Update text with minimal allocs; skip refresh if formatted output is unchanged
	c.buf = c.buf[:0]
	if c.fmtPrec >= 0 {
		c.buf = strconv.AppendFloat(c.buf, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf = common.AppendFormatFloat(c.buf, c.displayString, value)
	}
	if !common.SameTextBytes(c.displayText.Text, c.buf) {
		c.displayText.Text = string(c.buf)
		canvas.Refresh(c.displayText)
	}

	canvas.Refresh(c.shader)
}

func (c *Dial) SetValue2(value float64) { c.SetValue(value) }

func (c *Dial) CreateRenderer() fyne.WidgetRenderer { return &DialRenderer{Dial: c} }

type DialRenderer struct {
	*Dial
	objects []fyne.CanvasObject
}

func (c *DialRenderer) Layout(space fyne.Size) {
	if c.size == space {
		return
	}
	c.size = space

	c.diameter = fyne.Min(space.Width, space.Height)
	c.radius = c.diameter * common.OneHalf
	c.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)

	size := fyne.Size{Width: c.diameter, Height: c.diameter}
	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	// Shader quad: the dial square padded so the marker overhang isn't clipped
	c.shader.Move(topleft.SubtractXY(dialPad, dialPad))
	c.shader.Resize(fyne.Size{Width: c.diameter + 2*dialPad, Height: c.diameter + 2*dialPad})
	c.shader.Uniforms["size_d"] = c.diameter

	// Title (no rounding needed)
	c.titleText.TextSize = float32(int(c.radius * common.OneFourth))
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))

	// Display text
	c.displayText.TextSize = float32(int(c.radius * common.OneThird))
	c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneFifth))
	c.displayText.Resize(size)

	// Labels: reuse precomputed sin/cos, scale with current radius
	radius43 := c.radius * common.OneFourth * 3
	labelPad := max(float32(6.0), c.radius*0.14)

	// Assume monospace, digits only: width ≈ chars * 0.62 * TextSize; height ≈ 1.15 * TextSize
	// This keeps alignment stable and removes per-label measuring.
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

		// Place label on the INSIDE of the gauge
		labelRadius := radius43 - labelPad
		cx := c.middle.X + c.pipSin[i]*labelRadius
		cy := c.middle.Y - c.pipCos[i]*labelRadius

		lbl.Resize(fyne.NewSize(c.labelBoxW, c.labelBoxH))
		lbl.Move(fyne.Position{X: cx - c.labelBoxW/2, Y: cy - c.labelBoxH/2})
	}
}

func (c *DialRenderer) MinSize() fyne.Size { return c.minsize }
func (c *DialRenderer) Refresh()           {}
func (c *DialRenderer) Destroy()           {}

func (c *DialRenderer) Objects() []fyne.CanvasObject {
	if c.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(c.pipLabels)+3)
		objs = append(objs, c.shader)
		for _, t := range c.pipLabels {
			if t != nil {
				objs = append(objs, t)
			}
		}
		objs = append(objs, c.titleText, c.displayText)
		c.objects = objs
	}
	return c.objects
}
