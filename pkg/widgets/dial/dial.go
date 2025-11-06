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

	needle                *canvas.Line
	highestObservedMarker *canvas.Line
	lastHighestObserved   time.Time

	pips      []*canvas.Line
	pipLabels []*canvas.Text

	face        *canvas.Arc
	center      *canvas.Circle
	displayText *canvas.Text
	titleText   *canvas.Text

	size    fyne.Size
	minsize fyne.Size

	diameter                   float32
	radius                     float32
	middle                     fyne.Position
	needleOffset, needleLength float32
	needleRotConst             float64 // = common.Pi15/(Max-Min)
	lineRotConst               float64 // = common.Pi15/steps

	// Precomputed trig for pips: angle_i = lineRotConst*float64(i) - common.Pi43
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

	c.face = canvas.NewArc(-135.73, 135.8, 0.985, color.RGBA{0x80, 0x80, 0x80, 0xFF})
	c.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
	c.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 3}
	c.highestObservedMarker = &canvas.Line{StrokeColor: color.RGBA{R: 216, G: 250, B: 8, A: 0xFF}, StrokeWidth: 6}

	c.titleText = &canvas.Text{Text: c.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	c.titleText.TextStyle.Monospace = true
	c.titleText.Alignment = fyne.TextAlignCenter

	c.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	c.displayText.Alignment = fyne.TextAlignCenter

	// Pip color gradient
	fac := float64(0xA5) / float64(c.cfg.Steps)

	// Build pips + labels once; also track the longest label length
	for i := 0; i < c.cfg.Steps+1; i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		c.pips = append(c.pips, pip)

		if i%2 == 0 {
			val := c.cfg.Min + (float64(i)/float64(c.cfg.Steps))*(c.cfg.Max-c.cfg.Min)*c.cfg.GaugeFactor
			txt := strconv.FormatFloat(val, 'f', c.gaugePrec, 64)

			lbl := &canvas.Text{
				Text:      txt,
				Color:     color.RGBA{0xE0, 0xE0, 0xE0, 0xFF},
				Alignment: fyne.TextAlignCenter,
			}
			//lbl.TextStyle.Monospace = true
			if n := len(txt); n > c.maxLabelChars {
				c.maxLabelChars = n
			}
			c.pipLabels = append(c.pipLabels, lbl)
		} else {
			c.pipLabels = append(c.pipLabels, nil)
		}
	}

	// Constants
	c.needleRotConst = common.Pi15 / totalRange
	c.lineRotConst = common.Pi15 / steps

	// Precompute pip sin/cos (size-independent)
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

// rotate angle (in radians) without refreshing per-object
func (c *Dial) rotateNoRefresh(hand *canvas.Line, rotation float64, offset, length float32) {
	s, co := math.Sincos(rotation)
	c.applySinCos(hand, float32(s), float32(co), offset, length)
}

func (c *Dial) applySinCos(hand *canvas.Line, sinRot, cosRot float32, offset, length float32) {
	x2 := length * sinRot
	y2 := -length * cosRot
	offX := offset * sinRot
	offY := -offset * cosRot
	midxOffX := c.middle.X + offX
	midY := c.middle.Y + offY
	hand.Position1 = fyne.Position{X: midxOffX, Y: midY}
	hand.Position2 = fyne.Position{X: midxOffX + x2, Y: midY + y2}
}

func (c *Dial) rotateNeedleNoRefresh(hand *canvas.Line, facePosition float64, offset, length float32) {
	normalized := facePosition - c.cfg.Min
	if normalized < 0 {
		normalized = 0
	}
	c.rotateNoRefresh(hand, c.needleRotConst*normalized-common.Pi43, offset, length)
}

func (c *Dial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value

	// Update needle position (no immediate refresh)
	c.rotateNeedleNoRefresh(c.needle, value, c.needleOffset, c.needleLength)

	// Highest observed marker with lazy reset
	if value > c.highestObserved {
		c.highestObserved = value
		c.lastHighestObserved = time.Now()
		c.rotateNeedleNoRefresh(c.highestObservedMarker, value, c.radius-2, 6)
	} else if time.Since(c.lastHighestObserved) > 10*time.Second {
		c.highestObserved = value
		c.lastHighestObserved = time.Now()
		c.rotateNeedleNoRefresh(c.highestObservedMarker, value, c.radius-2, 6)
	}

	// Update text with minimal allocs
	c.buf = c.buf[:0]
	if c.fmtPrec >= 0 {
		c.buf = strconv.AppendFloat(c.buf, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf = common.AppendFormatFloat(c.buf, c.displayString, value)
	}
	c.displayText.Text = string(c.buf)

	// Single refresh for the three updated objects
	canvas.Refresh(c.needle)
	canvas.Refresh(c.highestObservedMarker)
	canvas.Refresh(c.displayText)
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
	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14

	// Stroke sizes
	stroke := c.diameter * common.OneSixthieth
	midStroke := c.diameter * common.OneEighthieth
	smallStroke := c.diameter * common.OneTwohundredth

	size := fyne.Size{Width: c.diameter, Height: c.diameter}
	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	// Title (no rounding needed)
	c.titleText.TextSize = float32(int(c.radius * common.OneFourth))
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))

	// Center element
	center := c.radius * common.OneFourth
	c.center.Move(c.middle.SubtractXY(center*common.OneHalf, center*common.OneHalf))
	c.center.Resize(fyne.Size{Width: center, Height: center})

	// Display text
	c.displayText.TextSize = float32(int(c.radius * common.OneThird))
	c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneFifth))
	c.displayText.Resize(size)

	// Face + needle
	c.needle.StrokeWidth = stroke
	c.rotateNeedleNoRefresh(c.needle, c.value, c.needleOffset, c.needleLength)

	c.face.Move(c.middle.SubtractXY(c.radius, c.radius))
	c.face.Resize(fyne.Size{Width: c.diameter, Height: c.diameter})

	// Pips: reuse precomputed sin/cos, scale with current radii
	fourthRadius := c.radius * common.OneFourth
	eightRadius := c.radius * common.OneEight
	radius43 := c.radius * common.OneFourth * 3
	radius87 := c.radius * common.OneEight * 7

	// Label padding and cached box dims (avoid lbl.MinSize per label)
	labelPad := max(float32(6.0), c.radius*0.04)

	// Assume monospace, digits only: width ≈ chars * 0.62 * TextSize; height ≈ 1.15 * TextSize
	// This keeps alignment stable and removes per-label measuring.
	const charWidthFactor = 0.62
	const heightFactor = 1.15

	labelTextSize := c.radius * 0.10
	c.labelBoxW = float32(c.maxLabelChars) * float32(charWidthFactor) * labelTextSize
	c.labelBoxH = float32(heightFactor) * labelTextSize

	for i, p := range c.pips {
		if i%2 == 0 {
			p.StrokeWidth = max(2.0, midStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], radius43, fourthRadius-1)

			// Label for long pip
			lbl := c.pipLabels[i]
			if lbl != nil {
				lbl.TextSize = labelTextSize

				// Place label on the INSIDE of the gauge
				labelRadius := radius43 - labelPad
				cx := c.middle.X + c.pipSin[i]*labelRadius
				cy := c.middle.Y - c.pipCos[i]*labelRadius

				boxW := c.labelBoxW
				boxH := c.labelBoxH
				lbl.Resize(fyne.NewSize(boxW, boxH))
				lbl.Move(fyne.NewPos(cx-boxW/2, cy-boxH/2))
			}
		} else {
			p.StrokeWidth = max(2.0, smallStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], radius87, eightRadius-1)
		}
	}

	c.highestObservedMarker.StrokeWidth = max(2.0, midStroke)
	c.rotateNeedleNoRefresh(c.highestObservedMarker, c.highestObserved, c.radius-2, 6)

}

func (c *DialRenderer) MinSize() fyne.Size { return c.minsize }
func (c *DialRenderer) Refresh()           {}
func (c *DialRenderer) Destroy()           {}

func (c *DialRenderer) Objects() []fyne.CanvasObject {
	if c.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(c.pips)+len(c.pipLabels)+7)
		for _, v := range c.pips {
			objs = append(objs, v)
		}
		for _, t := range c.pipLabels {
			if t != nil {
				objs = append(objs, t)
			}
		}
		objs = append(objs, c.face, c.titleText, c.center,
			c.highestObservedMarker, c.needle, c.displayText)
		c.objects = objs
	}
	return c.objects
}

// --- helpers ---

// max helper that matches your float32 usage
func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
