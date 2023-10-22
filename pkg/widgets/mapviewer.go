package widgets

import (
	"fmt"
	"image/color"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
)

type MapViewerOption func(*MapViewer) error

func WithXFrom(xFrom string) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.xFrom = xFrom
		return nil
	}
}

func WithYFrom(yFrom string) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.yFrom = yFrom
		return nil
	}
}

func WithXData(xData []int) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.numColumns = len(xData)
		mv.xData = xData
		return nil
	}
}

func WithYData(yData []int) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.numRows = len(yData)
		mv.yData = yData
		return nil
	}
}

func WithZData(zData []int) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.numData = len(zData)
		mv.zData = zData
		return nil
	}
}

func WithXCorrFac(xCorrFac float64) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.xCorrFac = xCorrFac
		return nil
	}
}

func WithYCorrFac(yCorrFac float64) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.yCorrFac = yCorrFac
		return nil
	}
}

func WithZCorrFac(zCorrFac float64) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.zCorrFac = zCorrFac
		return nil
	}
}

func WithInterPolFunc(ipf interpolate.InterPolFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.ipf = ipf
		return nil
	}
}

func NewMapViewer(options ...MapViewerOption) (*MapViewer, error) {
	mv := &MapViewer{
		yAxis:   make([]*canvas.Text, 0),
		xAxis:   make([]*canvas.Text, 0),
		setChan: make(chan xyUpdate, 100),
	}
	mv.ExtendBaseWidget(mv)
	for _, option := range options {
		if err := option(mv); err != nil {
			return nil, err
		}
	}

	log.Printf("NewMapViewer: mv.numColumns: %d mv.numRows: %d mv.numData: %d xf: %s  yf: %s", mv.numColumns, mv.numRows, mv.numData, mv.xFrom, mv.yFrom)
	mv.render()

	if mv.numColumns*mv.numRows != mv.numData && mv.numColumns > 1 && mv.numRows > 1 {
		return nil, fmt.Errorf("NewMapViewer mv.numColumns * mv.numRows != mv.numData")
	}

	go func() {
		for xy := range mv.setChan {
			mv.setXY(xy.x, xy.y)
		}
	}()

	return mv, nil
}

func (mv *MapViewer) render() {
	// y must be created before x as it's width is used to calculate x's offset
	mv.createYAxis()
	mv.createXAxis()

	mv.crosshair = NewRectangle(color.RGBA{0xfc, 0x4a, 0xFA, 255}, 4)
	mv.cursor = NewRectangle(color.RGBA{0x00, 0x0a, 0xFF, 255}, 4)

	mv.textValues, mv.valueTexts = createTextValues(mv.zData, mv.zCorrFac)

	width := float32(mv.numColumns * cellWidth)
	height := float32(mv.numRows * cellHeight)

	mv.valueMap = canvas.NewImageFromImage(createImage(mv.xData, mv.yData, mv.zData, mv.zCorrFac))
	mv.valueMap.ScaleMode = canvas.ImageScalePixels
	mv.valueMap.SetMinSize(fyne.NewSize(width, height))
	mv.valueMap.Resize(fyne.NewSize(width, height))

	mv.grid = NewGrid(mv.numColumns, mv.numRows)

	mv.innerView = container.NewStack(
		mv.valueMap,
		mv.grid,
		mv.valueTexts,
		container.NewWithoutLayout(
			mv.crosshair,
			mv.cursor,
		),
	)

	mv.content = container.NewBorder(
		mv.xAxisButtons,
		container.NewGridWithColumns(2,
			widget.NewButtonWithIcon("Load", theme.DocumentIcon(), func() {
				log.Println("Load")
			}),
			widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
				log.Println("Save")
			}),
		),
		mv.yAxisButtons,
		nil,
		mv.innerView,
	)
}

func (mv *MapViewer) createXAxis() {
	mv.xAxisButtons = container.New(&layout.Horizontal{Offset: mv.yAxisButtons})
	if mv.numColumns > 1 {
		prec := 0
		if mv.xCorrFac < 1 {
			prec = 2
		}
		for i := 0; i < mv.numColumns; i++ {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.xData[i])*mv.xCorrFac, 'f', prec, 64), TextSize: 13}
			mv.xAxis = append(mv.xAxis, text)
			mv.xAxisButtons.Add(text)
		}
	}
}

func (mv *MapViewer) createYAxis() {
	mv.yAxisButtons = container.New(&layout.Vertical{})
	if mv.numRows > 1 {
		prec := 0
		if mv.yCorrFac < 1 {
			prec = 2
		}
		for i := mv.numRows - 1; i >= 0; i-- {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.yData[i])*mv.yCorrFac, 'f', prec, 64), TextSize: 13}
			mv.yAxis = append(mv.yAxis, text)
			mv.yAxisButtons.Add(text)
		}
	}
}
