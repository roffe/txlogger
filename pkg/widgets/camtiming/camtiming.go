package camtiming

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

const (
	degMin, degMax = -270.0, 270.0 // wide enough for the wildest cam in the list
	degStep        = 90.0
	lobeSamples    = 60
)

type Config struct {
	ECU string
}

// camFields is the editable spec of one cam, with its own template list.
// Intake and exhaust pick independently because mixing shafts across
// engines — a T5 intake with a T7 exhaust, B234i with B202 — is the whole
// point of half the cam swaps people do. Typing over the fields is how a
// cam that is in no list at all gets in.
type camFields struct {
	template, compare          *widget.Select
	lift, open, close, advance *numericentry.Widget
}

func newCamFields(exhaust bool, onChange func()) camFields {
	f := camFields{
		lift:    numericentry.New("0"),
		open:    numericentry.New("0"),
		close:   numericentry.New("0"),
		advance: numericentry.New("0"),
	}
	for _, e := range []*numericentry.Widget{f.lift, f.open, f.close, f.advance} {
		e.OnChanged = func(string) { onChange() }
	}
	f.template = widget.NewSelect(camLabels(exhaust), func(label string) {
		if c, ok := byLabel(label, exhaust); ok {
			f.set(c)
		}
		onChange()
	})
	f.compare = widget.NewSelect(append([]string{"(none)"}, camLabels(exhaust)...), func(string) {
		onChange()
	})
	return f
}

// byLabel resolves a template select back to its cam.
func byLabel(label string, exhaust bool) (Cam, bool) {
	for _, c := range Cams {
		if c.Exhaust == exhaust && c.Label() == label {
			return c, true
		}
	}
	return Cam{}, false
}

// compareCam is the greyed reference cam drawn behind this one, if any.
func (f camFields) compareCam(exhaust bool) (Cam, bool) {
	if f.compare.SelectedIndex() <= 0 {
		return Cam{}, false
	}
	return byLabel(f.compare.Selected, exhaust)
}

// camLabels lists the templates for one side of the head.
func camLabels(exhaust bool) []string {
	var out []string
	for _, c := range Cams {
		if c.Exhaust == exhaust {
			out = append(out, c.Label())
		}
	}
	return out
}

func (f camFields) cam(exhaust bool) Cam {
	c := Cam{
		Exhaust:  exhaust,
		PeakLift: parse(f.lift),
		Open:     parse(f.open),
		Close:    parse(f.close),
	}
	return c.Advance(parse(f.advance))
}

func (f camFields) set(c Cam) {
	setNum(f.lift, c.PeakLift)
	setNum(f.open, c.Open)
	setNum(f.close, c.Close)
	setNum(f.advance, 0)
}

type Widget struct {
	widget.BaseWidget
	cfg *Config

	intake, exhaust camFields
	diagram         *chart
	readout         *fyne.Container

	ve      *veTab
	tabs    *container.AppTabs
	content fyne.CanvasObject
}

func New(cfg *Config) *Widget {
	if cfg == nil {
		cfg = &Config{}
	}
	w := &Widget{
		cfg:     cfg,
		diagram: newChart("crank °  (0 = TDC overlap)", "valve lift mm", degMin, degMax, degStep),
		readout: container.NewGridWithColumns(2),
	}
	w.ExtendBaseWidget(w)
	w.diagram.xUnit, w.diagram.yUnit, w.diagram.yDec = "°", "mm", 2
	w.intake = newCamFields(false, w.recalc)
	w.exhaust = newCamFields(true, w.recalc)

	form := container.New(layout.NewFormLayout(),
		bold("Intake"), widget.NewLabel(""),
		widget.NewLabel("Template"), w.intake.template,
		widget.NewLabel("Compare with"), w.intake.compare,
		widget.NewLabel("Lift (mm)"), w.intake.lift,
		widget.NewLabel("IVO (° BTDC)"), w.intake.open,
		widget.NewLabel("IVC (° ABDC)"), w.intake.close,
		widget.NewLabel("Advance (°)"), w.intake.advance,

		bold("Exhaust"), widget.NewLabel(""),
		widget.NewLabel("Template"), w.exhaust.template,
		widget.NewLabel("Compare with"), w.exhaust.compare,
		widget.NewLabel("Lift (mm)"), w.exhaust.lift,
		widget.NewLabel("EVO (° BBDC)"), w.exhaust.open,
		widget.NewLabel("EVC (° ATDC)"), w.exhaust.close,
		widget.NewLabel("Advance (°)"), w.exhaust.advance,
	)

	note := widget.NewLabel("Events are the manufacturer's advertised figures, not duration " +
		"at 0.050\". The lobe shape is modelled, not measured.")
	note.Wrapping = fyne.TextWrapWord
	left := container.NewVScroll(container.NewVBox(form, widget.NewSeparator(), w.readout, note))
	left.SetMinSize(fyne.NewSize(290, 100))
	timing := container.NewBorder(nil, nil, left, nil, w.diagram)

	w.ve = newVETab(w)
	w.tabs = container.NewAppTabs(
		container.NewTabItem("Timing diagram", timing),
		container.NewTabItem("Measured VE", w.ve.content),
	)
	w.content = w.tabs

	w.intake.compare.SetSelectedIndex(0)
	w.exhaust.compare.SetSelectedIndex(0)
	w.intake.template.SetSelected(find("9170887", false).Label()) // B235R
	w.exhaust.template.SetSelected(find("9170895", true).Label())
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

// cams returns the pair currently in the entry fields.
func (w *Widget) cams() (intake, exhaust Cam) {
	return w.intake.cam(false), w.exhaust.cam(true)
}

func (w *Widget) recalc() {
	in, ex := w.cams()

	ss := []series{
		lobeSeries(in, "Intake", intakeColor, false),
		lobeSeries(ex, "Exhaust", exhaustColor, false),
	}
	if c, ok := w.intake.compareCam(false); ok {
		ss = append(ss, lobeSeries(c, c.Name+" int", refColor, true))
	}
	if c, ok := w.exhaust.compareCam(true); ok {
		ss = append(ss, lobeSeries(c, c.Name+" exh", refColor, true))
	}

	markers := []marker{
		{x: in.Centerline(), col: intakeColor, label: fmt.Sprintf("ICL %.0f°", in.Centerline())},
		{x: -ex.Centerline(), col: exhaustColor, label: fmt.Sprintf("ECL %.0f°", ex.Centerline())},
	}
	var bands []band
	if ov := Overlap(in, ex); ov > 0 {
		inOpen, _ := in.window()
		bands = append(bands, band{x0: inOpen, x1: inOpen + ov, label: fmt.Sprintf("overlap %.0f°", ov)})
	}
	w.diagram.set(ss, markers, bands)

	w.readout.Objects = []fyne.CanvasObject{
		bold(""), bold("int / exh"),
		widget.NewLabel("Duration"), value("%.0f° / %.0f°", in.Duration(), ex.Duration()),
		widget.NewLabel("Peak lift"), value("%.2f / %.2f mm", in.PeakLift, ex.PeakLift),
		widget.NewLabel("Centerline"), value("%.1f° / %.1f°", in.Centerline(), ex.Centerline()),
		widget.NewLabel("Overlap"), value("%.0f°", Overlap(in, ex)),
		widget.NewLabel("LSA"), value("%.1f°", LSA(in, ex)),
	}
	w.readout.Refresh()
	w.ve.setCamInfo(fmt.Sprintf("cams: %.0f°/%.0f° duration, %.0f° overlap, %.1f° LSA",
		in.Duration(), ex.Duration(), Overlap(in, ex), LSA(in, ex)))
}

// lobeSeries samples one lobe over the crank angles its valve is open.
func lobeSeries(c Cam, name string, col color.RGBA, thin bool) series {
	open, close := c.window()
	if !(close > open) || c.PeakLift <= 0 {
		return series{name: name, col: col}
	}
	pts := make([]xy, lobeSamples+1)
	for i := range pts {
		d := open + (close-open)*float64(i)/lobeSamples
		pts[i] = xy{d, c.Lift(d)}
	}
	return series{name: name, col: col, pts: pts, thin: thin}
}

func bold(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

func value(format string, a ...any) *widget.Label {
	return widget.NewLabel(fmt.Sprintf(format, a...))
}

func setNum(e *numericentry.Widget, v float64) {
	e.SetText(strconv.FormatFloat(v, 'f', -1, 64))
}

// parse reads an entry, treating anything unparsable as 0 so a half typed
// number does not blank the whole diagram.
func parse(e *numericentry.Widget) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(e.Text, ",", "."), 64)
	if err != nil || math.IsNaN(v) {
		return 0
	}
	return v
}
