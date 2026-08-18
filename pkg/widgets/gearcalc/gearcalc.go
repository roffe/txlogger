// Package gearcalc is a port of github.com/roffe/t7gearcal: computes the
// GearCal.Ratio / GearCal.Range values for the Trionic 7 manual gearbox
// calibration from gear ratios, final drive and tire diameter.
package gearcalc

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

var gearNames = [6]string{"1st", "2nd", "3rd", "4th", "5th", "6th"}

type template struct {
	finalDrive, rpm, tireDiameter float64
	ratios, tolerances            [6]float64
}

var templates = map[string]template{
	"FM55": {
		4.05, 3000, 0.626,
		[6]float64{3.38, 1.76, 1.18, 0.89, 0.66, 0},
		[6]float64{25.9, 19.8, 14.9, 11.5, 11, 0},
	},
	"FM57": {
		3.828, 3000, 0.626,
		[6]float64{3.38, 1.76, 1.18, 0.89, 0.66, 0},
		[6]float64{27.4, 21, 15.5, 11.7, 11.5, 0},
	},
	"Roffe's Quaife": {
		3.61, 3000, 0.626,
		[6]float64{3.00, 1.933, 1.368, 1.045, 0.833, 0.704},
		[6]float64{25.9, 19.8, 14.9, 11.5, 11, 10.5},
	},
}

var templateNames = []string{"FM55", "FM57", "Roffe's Quaife"}

// gearCalc returns the speed in km/h at the given engine rpm and the
// GearCal ratio (as calibrated in the T7 binary) for one gear.
func gearCalc(rpm, gr, fd, td float64) (speed, ratio float64) {
	wheelRPM := rpm / (gr * fd)
	speed = wheelRPM * math.Pi * td * 60 / 1000
	ratio = rpm * 10 / speed
	return speed, ratio
}

type Widget struct {
	widget.BaseWidget

	finalDrive, repRPM, tireDiameter *numericentry.Widget
	ratios, tolerances               [6]*numericentry.Widget

	table   *fyne.Container
	chart   *chart
	content fyne.CanvasObject
}

func New() *Widget {
	w := &Widget{
		finalDrive:   numericentry.New("4.05"),
		repRPM:       numericentry.New("3000"),
		tireDiameter: numericentry.New("0.626"),
		table:        container.NewGridWithColumns(4),
		chart:        newChart(),
	}
	w.ExtendBaseWidget(w)

	for i := range w.ratios {
		w.ratios[i] = numericentry.New("0")
		w.tolerances[i] = numericentry.New("0")
	}

	templateSelect := widget.NewSelect(templateNames, func(name string) {
		w.loadTemplate(templates[name])
	})

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Template"), templateSelect,
		widget.NewLabel("Final drive"), w.finalDrive,
		widget.NewLabel("Representative RPM"), w.repRPM,
		widget.NewLabel("Tire diameter (m)"), w.tireDiameter,
	)

	gearGrid := container.NewGridWithColumns(3,
		widget.NewLabelWithStyle("Gear", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Ratio", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Tolerance %", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	for i := range w.ratios {
		gearGrid.Add(widget.NewLabel(gearNames[i]))
		gearGrid.Add(w.ratios[i])
		gearGrid.Add(w.tolerances[i])
	}

	left := container.NewVBox(form, gearGrid, w.table)
	w.content = container.NewBorder(nil, nil, left, nil, w.chart)

	// attach after construction so the SetTexts above don't calc half-built
	for _, e := range []*numericentry.Widget{w.finalDrive, w.repRPM, w.tireDiameter} {
		e.OnChanged = func(string) { w.calc() }
	}
	for i := range w.ratios {
		w.ratios[i].OnChanged = func(string) { w.calc() }
		w.tolerances[i].OnChanged = func(string) { w.calc() }
	}

	templateSelect.SetSelected("FM55")
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

func (w *Widget) loadTemplate(t template) {
	setText := func(e *numericentry.Widget, v float64) {
		e.SetText(strconv.FormatFloat(v, 'f', -1, 64))
	}
	setText(w.finalDrive, t.finalDrive)
	setText(w.repRPM, t.rpm)
	setText(w.tireDiameter, t.tireDiameter)
	for i := range w.ratios {
		setText(w.ratios[i], t.ratios[i])
		setText(w.tolerances[i], t.tolerances[i])
	}
	w.calc()
}

func parse(e *numericentry.Widget) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(e.Text, ",", "."), 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func (w *Widget) calc() {
	fd := parse(w.finalDrive)
	rpm := parse(w.repRPM)
	td := parse(w.tireDiameter)

	bold := func(s string) *widget.Label {
		return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	objs := []fyne.CanvasObject{
		bold("Gear"), bold("GearCal.Ratio"), bold("Range (±)"), bold("km/h"),
	}
	var lines []gearLine
	for i := range w.ratios {
		gr := parse(w.ratios[i])
		tol := parse(w.tolerances[i])
		// gear ratio 0 (no 6th gear) or unparsable input skips the row
		if !(gr > 0 && fd > 0 && td > 0 && rpm > 0) {
			continue
		}
		speed, ratio := gearCalc(rpm, gr, fd, td)
		objs = append(objs,
			widget.NewLabel(gearNames[i]),
			widget.NewLabel(strconv.Itoa(int(math.Round(ratio)))),
			widget.NewLabel(strconv.Itoa(int(math.Round(ratio*tol/100)))),
			widget.NewLabel(fmt.Sprintf("%.1f", speed)),
		)
		lines = append(lines, gearLine{
			name: gearNames[i],
			k:    speed / rpm, // km/h per engine rpm, straight line through origin
			col:  seriesColors[i],
		})
	}
	w.table.Objects = objs
	w.table.Refresh()
	w.chart.SetLines(lines)
}
