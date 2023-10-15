package windows

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/goki/freetype/truetype"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	cellWidth  = 35
	cellHeight = 35
)

type MapViewer struct {
	widget.BaseWidget

	xName, yName, zName string

	numColums, numRows, numData int
	xData                       []uint16
	yData                       []uint16
	zData                       []int
	tracker                     *canvas.Rectangle
	innerView                   *fyne.Container
	xIdx, yIdx                  float64
	objects                     []fyne.CanvasObject
	ipf                         interpolate.InterPolFunc

	w fyne.Window
}

func NewMapViewer(w fyne.Window, xAxisName, yAxisName, mapName string, symbols symbol.SymbolCollection, interPolfunc interpolate.InterPolFunc) (*MapViewer, error) {
	xData, yData, zData, corrFac, err := getXYZ(xAxisName, yAxisName, mapName, symbols)
	if err != nil {
		return nil, err
	}

	if len(xData)*len(yData) != len(zData) {
		return nil, fmt.Errorf("len(xData) * len(yData) != len(zData)")
	}

	width := float32(len(xData) * cellWidth)
	height := float32(len(yData) * cellHeight)

	valueMap := canvas.NewImageFromImage(createImage(mapName, xData, yData, zData, corrFac))
	valueMap.ScaleMode = canvas.ImageScaleSmooth
	valueMap.SetMinSize(fyne.NewSize(width, height))
	valueMap.Resize(fyne.NewSize(width, height))
	//textOverlay := createAndConfigureImage(createText)

	yAxisButtons := container.New(&layout.Vertical{})
	for i := len(yData) - 1; i >= 0; i-- {
		yAxisButtons.Add(widget.NewButton(fmt.Sprintf("%d", yData[i]), func() {
		}))
	}
	xAxisButtons := container.New(&layout.Horizontal{})
	for i := 0; i < len(xData); i++ {
		xAxisButtons.Add(widget.NewButton(fmt.Sprintf("%d", xData[i]), func() {
		}))
	}

	tracker := widgets.NewTracker()

	//mouseTracker := &widgets.MouseTracker{}

	inner := container.NewStack(
		valueMap,
		widgets.NewGrid(len(xData), len(yData)),
		container.NewStack(tracker /* mouseTracker*/),
	)

	mv := &MapViewer{
		w: w,

		xName: xAxisName,
		yName: yAxisName,
		zName: mapName,

		ipf:       interPolfunc,
		numColums: len(xData),
		numRows:   len(yData),
		numData:   len(zData),
		xData:     xData,
		yData:     yData,
		zData:     zData,
		tracker:   tracker,
		innerView: inner,
		objects: []fyne.CanvasObject{
			container.NewBorder(xAxisButtons, nil, yAxisButtons, nil, inner),
		},
	}

	return mv, nil
}

func (mv *MapViewer) Close() {
	if mv.w != nil {
		mv.w.Close()
	}
}

func (mv *MapViewer) XName() string {
	return mv.xName
}

func (mv *MapViewer) YName() string {
	return mv.yName
}

func (mv *MapViewer) Name() string {
	return mv.zName
}

func (mv *MapViewer) SetXY(xValue, yValue uint16) error {
	//	log.Println("Set", xValue, yValue)
	xIdx, yIdx, _, err := mv.ipf(mv.xData, mv.yData, mv.zData, xValue, yValue)
	if err != nil {
		return err
	}
	if yIdx < 0 {
		yIdx = 0
	} else if yIdx > float64(mv.numRows-1) {
		yIdx = float64(mv.numRows - 1)
	}
	if xIdx < 0 {
		xIdx = 0
	} else if xIdx > float64(mv.numColums-1) {
		xIdx = float64(mv.numColums - 1)
	}
	mv.xIdx = xIdx
	mv.yIdx = yIdx
	sz := mv.innerView.Size()
	mv.tracker.Move(
		fyne.NewPos(
			float32(xIdx)*sz.Width/float32(mv.numColums),
			float32(float64(mv.numRows-1)-yIdx)*sz.Height/float32(mv.numRows),
		),
	)
	return nil
}

func (mv *MapViewer) MinSize() fyne.Size {
	var w, h float32
	for _, o := range mv.objects {
		childSize := o.MinSize()
		if childSize.Height > h {
			h = childSize.Height
		}
		w += childSize.Width
	}
	return fyne.NewSize(w, h)
}

func (mv *MapViewer) Resize(size fyne.Size) {
	mv.objects[0].(*fyne.Container).Resize(size)
	sz := mv.innerView.Size()
	mv.tracker.Move(
		fyne.NewPos(
			float32(mv.xIdx)*(sz.Width/float32(mv.numColums)),
			float32(float64(mv.numRows-1)-mv.yIdx)*(sz.Height/float32(mv.numRows)),
		),
	)
	w := sz.Width / float32(mv.numColums)
	h := sz.Height / float32(mv.numRows)
	mv.tracker.Resize(fyne.NewSize(w, h))

}

func (mv *MapViewer) CreateRenderer() fyne.WidgetRenderer {
	return &MapViewerRenderer{
		obj: mv,
	}
}

type MapViewerRenderer struct {
	obj *MapViewer
}

func (vr *MapViewerRenderer) Layout(size fyne.Size) {
}

func (vr *MapViewerRenderer) MinSize() fyne.Size {
	return vr.obj.MinSize()
}

func (vr *MapViewerRenderer) Refresh() {
}

func (vr *MapViewerRenderer) Destroy() {
}

func (vr *MapViewerRenderer) Objects() []fyne.CanvasObject {
	return vr.obj.objects
}

func getXYZ(xAxis, yAxis, zAxis string, symbols symbol.SymbolCollection) ([]uint16, []uint16, []int, float64, error) {
	x, y, z := symbols.GetByName(xAxis), symbols.GetByName(yAxis), symbols.GetByName(zAxis)

	asd := map[string]*symbol.Symbol{
		xAxis: x,
		yAxis: y,
		zAxis: z,
	}

	for k, v := range asd {
		if v == nil {
			return nil, nil, nil, 0, fmt.Errorf("failed to find %s", k)
		}
	}
	var out []int
	switch z.Name {
	case "BFuelCal.Map", "BFuelCal.StartMap", "BFuelCal.TempEnrichFacMap":
		for _, v := range z.Bytes() {
			out = append(out, int(v))
		}
	case "IgnNormCal.Map", "IgnE85Cal.fi_AbsMap", "IgnAbsCal.fi_NormalMAP":
		data := make([]int16, z.Length/2)

		reader := bytes.NewReader(z.Bytes())
		if err := binary.Read(reader, binary.BigEndian, &data); err != nil {
			log.Fatalf("Failed to convert zData to int16 slice: %v", err)
		}
		for _, v := range data {
			out = append(out, int(v))
		}
	}

	return x.DataToUint16(), y.DataToUint16(), out, z.Correctionfactor, nil
}

//go:embed luximr.ttf
var fontBytes []byte

func createImage(mapName string, xData, yData []uint16, zData []int, correctionFactor float64) *image.RGBA {
	width := len(xData) * cellWidth
	height := len(yData) * cellHeight

	// Load a font (you'll need to provide a valid font file path)
	/*
		fontFile := "luximr.ttf"
		fontBytes, err := os.ReadFile(fontFile)
		if err != nil {
			log.Fatalf("Failed to load font: %v", err)
		}
	*/

	fon, err := loadFontFace(fontBytes, 10)
	if err != nil {
		log.Println(err)
		return nil
	}

	// Create a new RGBA image
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Black),
		Face: fon,
	}

	// Calculate the colors for each cell based on data
	for y := 0; y < len(yData); y++ {
		for x := 0; x < len(xData); x++ {
			cellX := x * cellWidth
			cellY := (len(yData) - 1 - y) * cellHeight
			if cellY >= 0 && cellY+cellHeight <= height && cellX >= 0 && cellX+cellWidth <= width {
				index := y*len(xData) + x
				value := float64(zData[index]) * correctionFactor
				color := colorTheme(mapName, float64(value))
				draw.Draw(img, image.Rect(cellX, cellY, cellX+cellWidth, cellY+cellHeight), &image.Uniform{color}, image.Point{cellX, cellY}, draw.Src)
			}
		}
	}

	numColumns := len(xData)
	for i := len(yData); i > 0; i-- {
		for j := 0; j < numColumns; j++ {
			d.Dot = fixed.Point26_6{
				X: fixed.I(j*cellWidth + 3),
				Y: fixed.I(height - (i * cellHeight) + cellHeight - 10),
			}
			d.DrawString(strconv.FormatFloat(float64(zData[(i*numColumns)-(numColumns-j)])*correctionFactor, 'f', 2, 64))
		}
	}

	return img
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// getColorInterpolation returns a color interpolated on the color spectrum green to yellow to red.
// value should be between min and max.
func getColorInterpolation(min, max, value float64) color.RGBA {
	// Normalize the value to a 0-1 range
	t := (value - min) / (max - min)

	var r, g, b float64
	if t < 0.6 { // Green to Yellow interpolation
		r = lerp(0, 1, t/0.6)
		g = 1
	} else { // Yellow to Red interpolation
		r = 1
		g = lerp(1, 0, (t-0.6)/0.4)
	}
	b = 0

	// Convert from 0-1 range to 0-255 for color.RGBA
	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}

func colorTheme(mapName string, value float64) color.Color {
	var r, g, b, a uint8
	switch mapName {
	case "BFuelCal.Map", "BFuelCal.E85Map", "BFuelCal.LambdaOneFacMap", "BFuelCal.E85TempEnrichFacMap", "BFuelCal.TempEnrichFacMap":
		return getColorInterpolation(0.75, 1.55, value)
	case "IgnNormCal.Map", "IgnE85Cal.fi_AbsMap", "IgnAbsCal.fi_NormalMAP":
		return getColorInterpolation(-10, 40, value)
	default:
		r = 200
		g = 255
		b = 10
		a = 255
	}
	return color.RGBA{r, g, b, a}
}

func loadFontFace(fontBytes []byte, size float64) (font.Face, error) {
	f, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}

	return truetype.NewFace(f, &truetype.Options{Size: size}), nil
}
