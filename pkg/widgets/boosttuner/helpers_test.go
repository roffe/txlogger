package boosttuner

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
)

// mkSym builds a 16-bit symbol holding vals, encoded with correction factor cf.
// signed selects int16 storage (needed for negative axis values).
func mkSym(t *testing.T, name string, signed bool, cf float64, vals ...float64) *symbol.Symbol {
	t.Helper()
	s := &symbol.Symbol{Name: name, Correctionfactor: cf}
	if signed {
		s.Type = symbol.SIGNED
	}
	data := s.EncodeFloat64s(vals)
	s.Length = uint16(len(data))
	if err := s.SetData(data); err != nil {
		t.Fatal(err)
	}
	return s
}

// Geometry of the fake binary: RegMap is 2 rpm rows x 3 load columns, the PID
// maps are 2 rpm rows x 3 load-error columns.
var (
	fakeLoadXSP = []float64{400, 800, 1200}
	fakeEngSP   = []float64{2000, 4000}
	fakeRegMap  = []float64{30, 40, 50, 35, 45, 55} // %
	fakePIDXSP  = []float64{-100, 0, 100}
	fakePIDYSP  = []float64{2000, 4000}
	fakePMap    = []float64{10, 20, 30, 40, 50, 60}
	fakeIMap    = []float64{1, 2, 3, 4, 5, 6}
	fakeDMap    = []float64{100, 200, 300, 400, 500, 600}
)

// fakeBinary returns an in-memory binary with every BoostCal symbol the tuner
// reads. edit may delete or replace entries before the collection is built.
func fakeBinary(t *testing.T, edit func(map[string]*symbol.Symbol)) symbol.FirmwareFile {
	t.Helper()
	syms := map[string]*symbol.Symbol{
		symSetLoadXSP:           mkSym(t, symSetLoadXSP, false, 1, fakeLoadXSP...),
		symNEngSP:               mkSym(t, symNEngSP, false, 1, fakeEngSP...),
		symRegMap:               mkSym(t, symRegMap, false, 0.1, fakeRegMap...),
		symPIDXSP:               mkSym(t, symPIDXSP, true, 1, fakePIDXSP...),
		symPIDYSP:               mkSym(t, symPIDYSP, false, 1, fakePIDYSP...),
		symPMap:                 mkSym(t, symPMap, false, 1, fakePMap...),
		symIMap:                 mkSym(t, symIMap, false, 1, fakeIMap...),
		symDMap:                 mkSym(t, symDMap, false, 1, fakeDMap...),
		"BoostCal.IFacMax":      mkSym(t, "BoostCal.IFacMax", false, 1, 200),
		"BoostCal.FilterFactor": mkSym(t, "BoostCal.FilterFactor", false, 1, 10),
	}
	if edit != nil {
		edit(syms)
	}
	list := make([]*symbol.Symbol, 0, len(syms))
	n := 0
	for _, s := range syms {
		n++
		s.Number = n
		list = append(list, s)
	}
	return symbol.NewCollection(list...)
}

// saveRecorder captures what the tuner asks the main window to write.
type saveRecorder struct {
	calls []saveCall
	err   error
}

type saveCall struct {
	name string
	data []float64
}

func (r *saveRecorder) save(name string, data []float64) error {
	r.calls = append(r.calls, saveCall{name, append([]float64(nil), data...)})
	return r.err
}

// csvLog renders a CSV log: a quoted ISONICO timestamp per row followed by the
// named columns. NaN cells are left empty, which the CSV reader keeps as NaN.
func csvLog(cols []string, rows ...[]float64) string {
	var b strings.Builder
	b.WriteString("Time," + strings.Join(cols, ",") + "\n")
	t0 := time.Date(2026, 1, 2, 15, 4, 5, 0, time.Local)
	for i, row := range rows {
		fmt.Fprintf(&b, "%q", t0.Add(time.Duration(i)*50*time.Millisecond).Format(logfile.ISONICO))
		for _, v := range row {
			b.WriteByte(',')
			if !math.IsNaN(v) {
				b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// boostCols are the log columns a complete boost log carries, in the order
// sample fields are written.
var boostCols = []string{
	"ActualIn.n_Engine", "Boost.SetValue", "BoostProt.RegConValue",
	"BoostProt.PFac", "BoostProt.IFac", "BoostProt.DFac", "BoostAdap.Adaption",
	"BoostProt.LoadDiff", "BoostProt.PWMCalc", "BoostProt.ST_PWM2Perc",
	"MAF.m_AirInletBoost",
}

// sample is one logged row of every boost channel (raw 0.1% duty units).
type sample struct {
	rpm, setv, regCon, p, i, d, adap, loadDiff, pwm, twopct float64
}

// boostLog renders samples as a CSV log with all required channels; the airmass
// column is derived from SetValue - LoadDiff.
func boostLog(samples ...sample) string {
	rows := make([][]float64, len(samples))
	for i, s := range samples {
		rows[i] = []float64{s.rpm, s.setv, s.regCon, s.p, s.i, s.d, s.adap, s.loadDiff, s.pwm, s.twopct, s.setv - s.loadDiff}
	}
	return csvLog(boostCols, rows...)
}

// repeat returns n copies of s.
func repeat(s sample, n int) []sample {
	out := make([]sample, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// loadLog parses a log and merges it into the tuner the same way the "Add log
// files" callback does once parsing has finished.
func loadLog(t *testing.T, bt *BoostTuner, name, data string) {
	t.Helper()
	local, n, err := parseLog(name, strings.NewReader(data))
	if err != nil {
		t.Fatalf("parseLog(%s): %v", name, err)
	}
	bt.mergeLog(name, local, n)
	bt.rebuildOrder()
	bt.resolveChannels()
	bt.refreshChannelList()
	bt.refreshLogList()
}

// children returns the child objects of the container types the tuner builds
// its tabs from, without creating any renderers.
func children(o fyne.CanvasObject) []fyne.CanvasObject {
	switch v := o.(type) {
	case *fyne.Container:
		return v.Objects
	case *container.Split:
		return []fyne.CanvasObject{v.Leading, v.Trailing}
	case *container.Scroll:
		return []fyne.CanvasObject{v.Content}
	case *container.AppTabs:
		out := make([]fyne.CanvasObject, 0, len(v.Items))
		for _, it := range v.Items {
			out = append(out, it.Content)
		}
		return out
	}
	return nil
}

// findAll returns, depth first, every object of type T under root that match
// accepts.
func findAll[T fyne.CanvasObject](root fyne.CanvasObject, match func(T) bool) []T {
	var out []T
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if o == nil {
			return
		}
		if v, ok := o.(T); ok && (match == nil || match(v)) {
			out = append(out, v)
		}
		for _, c := range children(o) {
			walk(c)
		}
	}
	walk(root)
	return out
}

// findButton returns the button labelled text under root.
func findButton(t *testing.T, root fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	b := findAll(root, func(b *widget.Button) bool { return b.Text == text })
	if len(b) == 0 {
		t.Fatalf("button %q not found", text)
	}
	return b[0]
}

// labelTexts returns the text of every label under root.
func labelTexts(root fyne.CanvasObject) []string {
	var out []string
	for _, l := range findAll[*widget.Label](root, nil) {
		out = append(out, l.Text)
	}
	return out
}

// siblingLabel returns the first label in the container that directly holds obj.
func siblingLabel(t *testing.T, root, obj fyne.CanvasObject) *widget.Label {
	t.Helper()
	for _, c := range findAll[*fyne.Container](root, nil) {
		holds := false
		for _, o := range c.Objects {
			if o == obj {
				holds = true
			}
		}
		if !holds {
			continue
		}
		for _, o := range c.Objects {
			if l, ok := o.(*widget.Label); ok {
				return l
			}
		}
	}
	t.Fatal("no sibling label found")
	return nil
}

// findSlider returns the slider built by BoostTuner.slider with the given title
// and its value readout label.
func findSlider(t *testing.T, root fyne.CanvasObject, title string) (*widget.Slider, *widget.Label) {
	t.Helper()
	for _, c := range findAll[*fyne.Container](root, nil) {
		if len(c.Objects) != 2 {
			continue
		}
		if l, ok := c.Objects[0].(*widget.Label); !ok || l.Text != title {
			continue
		}
		s := findAll[*widget.Slider](c.Objects[1], nil)
		v := findAll[*widget.Label](c.Objects[1], nil)
		if len(s) == 1 && len(v) == 1 {
			return s[0], v[0]
		}
	}
	t.Fatalf("slider %q not found", title)
	return nil, nil
}

// tab returns the content of the named top-level tab.
func tab(t *testing.T, bt *BoostTuner, name string) fyne.CanvasObject {
	t.Helper()
	for _, it := range bt.tabs.Items {
		if it.Text == name {
			return it.Content
		}
	}
	t.Fatalf("tab %q not found", name)
	return nil
}

// mapTexts renders mv and returns its cell value texts in ZData order (row 0,
// the lowest rpm, first). Value texts are the black canvas.Text cells; axis
// labels use the theme colour.
func mapTexts(t *testing.T, mv *mapviewer.MapViewer, cols int) []string {
	t.Helper()
	var grid []string
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *fyne.Container:
			for _, c := range v.Objects {
				walk(c)
			}
		case *canvas.Text:
			if v.Color == color.Black {
				grid = append(grid, v.Text)
			}
		case fyne.Widget:
			for _, c := range test.WidgetRenderer(v).Objects() {
				walk(c)
			}
		}
	}
	walk(mv)
	if cols == 0 || len(grid)%cols != 0 {
		t.Fatalf("got %d cell texts, not a multiple of %d columns", len(grid), cols)
	}
	// The grid lays out the top (highest) row first.
	rows := len(grid) / cols
	out := make([]string, 0, len(grid))
	for r := 0; r < rows; r++ {
		start := (rows - 1 - r) * cols
		out = append(out, grid[start:start+cols]...)
	}
	return out
}

// displayed returns the map viewer currently shown in host.
func displayed(t *testing.T, host *fyne.Container) *mapviewer.MapViewer {
	t.Helper()
	if len(host.Objects) != 1 {
		t.Fatalf("display holds %d objects, want 1", len(host.Objects))
	}
	mv, ok := host.Objects[0].(*mapviewer.MapViewer)
	if !ok {
		t.Fatalf("display holds %T, want *mapviewer.MapViewer", host.Objects[0])
	}
	return mv
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func nearSlice(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !near(a[i], b[i]) {
			return false
		}
	}
	return true
}
