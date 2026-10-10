package estimatedoutput

import (
	"image/color"
	"math"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/colors"
)

func mouseAt(x float32) *desktop.MouseEvent {
	return &desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, 10)}}
}

// renderedGraph returns a graph with a renderer laid out at 600x400.
func renderedGraph(t *testing.T, rpm []float64, series []graphSeries) (*dynoGraph, *dynoRenderer) {
	t.Helper()
	test.NewTempApp(t)
	g := newDynoGraph()
	r := test.TempWidgetRenderer(t, g).(*dynoRenderer)
	g.SetData(rpm, series)
	r.Layout(fyne.NewSize(600, 400))
	return g, r
}

func visibleTexts(objs []*canvas.Text) []string {
	var out []string
	for _, o := range objs {
		if o.Visible() {
			out = append(out, o.Text)
		}
	}
	return out
}

// TestNiceStep picks 1/2/5 x 10^n steps covering the range in at most
// maxTicks intervals.
func TestNiceStep(t *testing.T) {
	tests := []struct {
		rng      float64
		maxTicks int
		want     float64
	}{
		{100, 5, 20},
		{7, 8, 1},
		{1, 4, 0.5},
		{0.3, 2, 0.2},
		{95, 1, 100},
		{30, 0, 50}, // maxTicks clamps to 1
		{460, 8, 100},
	}
	for _, tt := range tests {
		if got := niceStep(tt.rng, tt.maxTicks); math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("niceStep(%v, %d) = %v, want %v", tt.rng, tt.maxTicks, got, tt.want)
		}
	}
}

// TestDynoGraphObjects: the renderer keeps one line per segment and one
// marker/value label per point for every series, plus axis labels.
func TestDynoGraphObjects(t *testing.T) {
	rpm := []float64{2000, 3000, 4000}
	series := []graphSeries{
		{name: "A", col: seriesColors[0], values: []float64{100, 200, 150}, visible: true},
		{name: "B", col: seriesColors[1], values: []float64{50, 60, 70}, visible: true},
	}
	_, r := renderedGraph(t, rpm, series)
	// bg + grid pool + cursor + 2*(2 lines + 3 markers) + 2*3 value labels + y labels + 3 x labels + readout
	if want := 1 + gridPool + 1 + 2*(2+3) + 2*3 + gridPool + 3 + 1; len(r.Objects()) != want {
		t.Fatalf("objects = %d, want %d", len(r.Objects()), want)
	}
	if r.MinSize() != fyne.NewSize(420, 300) {
		t.Fatalf("min size = %v", r.MinSize())
	}
	if r.lines[0][0].StrokeColor != seriesColors[0] || r.markers[1][2].FillColor != seriesColors[1] {
		t.Fatal("series colors not applied")
	}
	// y axis anchored at 0 up to the data max with a nice step
	if got := visibleTexts(r.yLabels); !slices.Equal(got, []string{"0", "50", "100", "150", "200"}) {
		t.Fatalf("y ticks = %v", got)
	}
	if got := visibleTexts(r.xLabels); !slices.Equal(got, []string{"2000", "3000", "4000"}) {
		t.Fatalf("x labels = %v", got)
	}
	// higher values sit higher on screen; markers center on their column
	m0, m1 := r.markers[0][0], r.markers[0][1]
	if !(m1.Position1.Y < m0.Position1.Y) {
		t.Fatalf("200 drawn below 100: %v vs %v", m1.Position1, m0.Position1)
	}
	if got := (m0.Position1.X + m0.Position2.X) / 2; math.Abs(float64(got-(r.plotLeft+r.xStep/2))) > 0.01 {
		t.Fatalf("marker x = %v, want column center %v", got, r.plotLeft+r.xStep/2)
	}
	if got := visibleTexts(r.valueLabels[0]); !slices.Equal(got, []string{"100", "200", "150"}) {
		t.Fatalf("value labels = %v", got)
	}
}

// TestDynoGraphNaNAndVisibility: NaN points hide their marker and both
// adjacent segments; hidden series draw nothing and drop out of the y range.
func TestDynoGraphNaNAndVisibility(t *testing.T) {
	nan := math.NaN()
	rpm := []float64{2000, 3000, 4000, 5000}
	series := []graphSeries{
		{name: "A", values: []float64{100, nan, 150, 160}, visible: true},
		{name: "Big", values: []float64{900, 900, 900, 900}, visible: false},
	}
	g, r := renderedGraph(t, rpm, series)
	if r.markers[0][1].Visible() || r.lines[0][0].Visible() || r.lines[0][1].Visible() || !r.lines[0][2].Visible() {
		t.Fatal("NaN point must hide its marker and both segments only")
	}
	for i := range rpm {
		if r.markers[1][i].Visible() || r.valueLabels[1][i].Visible() {
			t.Fatal("hidden series drew a marker or label")
		}
	}
	if got := visibleTexts(r.yLabels); got[len(got)-1] != "160" {
		t.Fatalf("y range includes the hidden series: %v", got)
	}

	g.SetSeriesVisible("Big", true)
	if !r.markers[1][0].Visible() {
		t.Fatal("SetSeriesVisible did not show the series")
	}
	if got := visibleTexts(r.yLabels); got[len(got)-1] != "1000" {
		t.Fatalf("y range after showing Big: %v", got)
	}
}

// TestDynoGraphDecimalTicks: small ranges get decimal tick labels.
func TestDynoGraphDecimalTicks(t *testing.T) {
	_, r := renderedGraph(t, []float64{1000, 2000}, []graphSeries{{name: "L", values: []float64{0.3, 0.8}, visible: true}})
	if got := visibleTexts(r.yLabels); !slices.Equal(got, []string{"0.0", "0.1", "0.2", "0.3", "0.4", "0.5", "0.6", "0.7", "0.8"}) {
		t.Fatalf("y ticks = %v", got)
	}
}

// TestDynoGraphHover: moving over the plot selects the nearest rpm column,
// shows the cursor and a readout of visible values; leaving clears it.
func TestDynoGraphHover(t *testing.T) {
	nan := math.NaN()
	rpm := []float64{2000, 3000, 4000}
	series := []graphSeries{
		{name: "Power (hp)", values: []float64{100, 200, 150}, visible: true},
		{name: "Torque (Nm)", values: []float64{300, nan, 280}, visible: true},
		{name: "Hidden", values: []float64{1, 2, 3}, visible: false},
	}
	g, r := renderedGraph(t, rpm, series)

	g.MouseIn(mouseAt(r.plotLeft + r.xStep*1.5))
	if g.hoverIdx != 1 || !r.cursor.Visible() {
		t.Fatalf("hoverIdx = %d, cursor visible = %v", g.hoverIdx, r.cursor.Visible())
	}
	if r.readout.Text != "3000 rpm   Power (hp) 200" {
		t.Fatalf("readout = %q", r.readout.Text)
	}
	if want := r.plotLeft + 1.5*r.xStep; r.cursor.Position1.X != want || r.cursor.Position1.Y != r.plotTop || r.cursor.Position2.Y != r.plotBottom {
		t.Fatalf("cursor at %v-%v, want x=%v", r.cursor.Position1, r.cursor.Position2, want)
	}

	// left of the plot clamps to the first column, far right to the last
	g.MouseMoved(mouseAt(0))
	if g.hoverIdx != 0 || r.readout.Text != "2000 rpm   Power (hp) 100   Torque (Nm) 300" {
		t.Fatalf("left: idx %d readout %q", g.hoverIdx, r.readout.Text)
	}
	g.MouseMoved(mouseAt(5000))
	if g.hoverIdx != 2 {
		t.Fatalf("right: idx %d", g.hoverIdx)
	}

	g.MouseOut()
	if g.hoverIdx != -1 || r.cursor.Visible() || r.readout.Text != "" {
		t.Fatalf("after MouseOut: idx %d cursor %v readout %q", g.hoverIdx, r.cursor.Visible(), r.readout.Text)
	}
}

// TestDynoGraphHoverWithoutData: hovering before a renderer exists or with
// no data is ignored.
func TestDynoGraphHoverWithoutData(t *testing.T) {
	test.NewTempApp(t)
	g := newDynoGraph()
	g.MouseMoved(mouseAt(100))
	g.MouseOut()
	if g.hoverIdx != -1 {
		t.Fatalf("hoverIdx = %d", g.hoverIdx)
	}
	r := test.TempWidgetRenderer(t, g).(*dynoRenderer)
	g.MouseMoved(mouseAt(100))
	if g.hoverIdx != -1 || r.indexAt(100) != -1 {
		t.Fatalf("no data: hoverIdx %d indexAt %d", g.hoverIdx, r.indexAt(100))
	}
	// zero size: layout is skipped, nothing positioned
	r.Layout(fyne.NewSize(0, 0))
	if r.xStep != 0 {
		t.Fatalf("xStep = %v after zero-size layout", r.xStep)
	}
}

// TestDynoGraphLabelThinning: when columns get narrower than the labels,
// x labels and value callouts are thinned out instead of overlapping.
func TestDynoGraphLabelThinning(t *testing.T) {
	test.NewTempApp(t)
	g := newDynoGraph()
	r := test.TempWidgetRenderer(t, g).(*dynoRenderer)
	n := 40
	rpm := make([]float64, n)
	vals := make([]float64, n)
	for i := range rpm {
		rpm[i] = float64(1000 + 150*i)
		vals[i] = float64(100000 + i)
	}
	g.SetData(rpm, []graphSeries{{name: "S", values: vals, visible: true}})
	r.Layout(fyne.NewSize(300, 200))
	xVis := len(visibleTexts(r.xLabels))
	if xVis == 0 || xVis >= n {
		t.Fatalf("visible x labels = %d of %d", xVis, n)
	}
	if got := len(visibleTexts(r.valueLabels[0])); got >= n {
		t.Fatalf("visible value labels = %d of %d", got, n)
	}
	// a refresh re-lays out at the same size and keeps the thinning
	r.Refresh()
	if got := len(visibleTexts(r.xLabels)); got != xVis {
		t.Fatalf("visible x labels after refresh = %d, want %d", got, xVis)
	}
	// widening the widget brings more labels back
	r.Layout(fyne.NewSize(3000, 200))
	if got := len(visibleTexts(r.xLabels)); got <= xVis {
		t.Fatalf("visible x labels at 3000 px = %d, want more than %d", got, xVis)
	}
}

// TestDynoGraphValueLabelCollision: a second series' callout that would
// overlap the first dodges below its marker, or hides when there is no room;
// a callout that would stick out above the plot goes below its marker.
func TestDynoGraphValueLabelCollision(t *testing.T) {
	series := []graphSeries{
		{name: "A", values: []float64{100, 100}, visible: true},
		{name: "B", values: []float64{100, 100}, visible: true},
		{name: "C", values: []float64{100, 100}, visible: true},
		{name: "Top", values: []float64{400, 400}, visible: true},
	}
	_, r := renderedGraph(t, []float64{1000, 2000}, series)
	a, b, c := r.valueLabels[0][0], r.valueLabels[1][0], r.valueLabels[2][0]
	if !a.Visible() || !b.Visible() {
		t.Fatal("first two callouts must be placed")
	}
	if !(a.Position().Y < b.Position().Y) {
		t.Fatalf("second callout did not dodge below: %v vs %v", a.Position(), b.Position())
	}
	if c.Visible() {
		t.Fatal("third overlapping callout must hide")
	}
	// 400 is the top of the y axis: its marker sits on the plot top edge
	top, m := r.valueLabels[3][0], r.markers[3][0]
	if !top.Visible() || top.Position().Y < r.plotTop || top.Position().Y < m.Position2.Y {
		t.Fatalf("top callout at y=%v, want below its marker (bottom %v) inside the plot (top %v)", top.Position().Y, m.Position2.Y, r.plotTop)
	}
}

func heatData() *Table {
	return &Table{
		Title:     "T",
		Cols:      []float64{1000, 2000, 3000},
		Rows:      []float64{100, 50},
		Cells:     [][]float64{{10, 20, 30}, {0, 5, 10}},
		Limiters:  [][]string{{limFuelCut, "", limAirmass}, {"", limAirmass, ""}},
		Precision: 1,
	}
}

func renderedHeat(t *testing.T, d *Table, size fyne.Size) (*heatTable, *heatRenderer) {
	t.Helper()
	test.NewTempApp(t)
	h := newHeatTable(colors.ColorBlindMode(0))
	r := test.TempWidgetRenderer(t, h).(*heatRenderer)
	h.SetData(d)
	r.Layout(size)
	return h, r
}

// TestHeatTableEmpty: no data renders only the background.
func TestHeatTableEmpty(t *testing.T) {
	h, r := renderedHeat(t, nil, fyne.NewSize(480, 300))
	if len(r.Objects()) != 1 {
		t.Fatalf("objects = %d, want background only", len(r.Objects()))
	}
	h.SetData(&Table{})
	if len(r.Objects()) != 1 || r.MinSize() != fyne.NewSize(480, 300) {
		t.Fatalf("empty table objects = %d", len(r.Objects()))
	}
	r.Destroy()
}

// TestHeatTableCells: cells take the mapviewer color ramp over the table's
// range, show their value with the table precision, and mark the active
// limiter in the corner; the legend lists each limiter once, sorted.
func TestHeatTableCells(t *testing.T) {
	d := heatData()
	_, r := renderedHeat(t, d, fyne.NewSize(480, 300))
	// bg + 6 * (cell, text, corner) + 3 col headers + 2 row headers + 2 legend pairs
	if want := 1 + 6*3 + 3 + 2 + 2*2; len(r.Objects()) != want {
		t.Fatalf("objects = %d, want %d", len(r.Objects()), want)
	}
	for ri, row := range d.Cells {
		for ci, v := range row {
			if got, want := r.cells[ri][ci].FillColor, colors.GetColorInterpolation(0, 30, v, 0); got != want {
				t.Errorf("cell %d,%d color = %v, want %v", ri, ci, got, want)
			}
			txt := r.cellTexts[ri][ci]
			if want := map[float64]string{0: "0.0", 5: "5.0", 10: "10.0", 20: "20.0", 30: "30.0"}[v]; !txt.Visible() || txt.Text != want {
				t.Errorf("cell %d,%d text = %q (visible %v), want %q", ri, ci, txt.Text, txt.Visible(), want)
			}
			corner := r.corners[ri][ci]
			lim := d.Limiters[ri][ci]
			if corner.Visible() != (lim != "") {
				t.Errorf("cell %d,%d corner visible = %v for limiter %q", ri, ci, corner.Visible(), lim)
			}
			if lim != "" && corner.FillColor != limiterColors[lim] {
				t.Errorf("cell %d,%d corner color = %v", ri, ci, corner.FillColor)
			}
		}
	}
	if r.legend[1].(*canvas.Text).Text != limAirmass || r.legend[3].(*canvas.Text).Text != limFuelCut {
		t.Fatalf("legend = %q, %q", r.legend[1].(*canvas.Text).Text, r.legend[3].(*canvas.Text).Text)
	}
	if got := []string{r.rowHdrs[0].Text, r.rowHdrs[1].Text, r.colHdrs[0].Text, r.colHdrs[2].Text}; !slices.Equal(got, []string{"100", "50", "1000", "3000"}) {
		t.Fatalf("headers = %v", got)
	}
	// top row sits above the bottom row
	if !(r.cells[0][0].Position().Y < r.cells[1][0].Position().Y) {
		t.Fatal("row 0 is not on top")
	}
}

// TestHeatTableCompact: small cells drop the value text, narrow columns
// thin out the rpm headers, and a too-small widget lays out nothing.
func TestHeatTableCompact(t *testing.T) {
	d := &Table{Cols: make([]float64, 30), Rows: []float64{1, 2}, Cells: [][]float64{make([]float64, 30), make([]float64, 30)}}
	for i := range d.Cols {
		d.Cols[i] = float64(1000 + 100*i)
		d.Cells[0][i] = float64(i)
	}
	_, r := renderedHeat(t, d, fyne.NewSize(300, 120))
	if r.cellTexts[0][0].Visible() {
		t.Fatal("cell text shown in a narrow cell")
	}
	hidden := 0
	for _, h := range r.colHdrs {
		if !h.Visible() {
			hidden++
		}
	}
	if hidden == 0 {
		t.Fatal("no rpm headers thinned out")
	}
	if len(r.legend) != 0 {
		t.Fatal("legend without limiters")
	}

	// uniform values still get a valid range; tiny size skips layout
	_, r = renderedHeat(t, &Table{Cols: []float64{1}, Rows: []float64{1}, Cells: [][]float64{{5}}}, fyne.NewSize(10, 10))
	if r.cells[0][0].Size() != (fyne.Size{}) {
		t.Fatalf("cell laid out in a 10x10 widget: %v", r.cells[0][0].Size())
	}
	r.Layout(fyne.NewSize(200, 100))
	if r.cells[0][0].Size().Width == 0 {
		t.Fatal("cell not laid out at 200x100")
	}
	r.Refresh()
}

// TestLimiterColorFor: known limiters get their palette color, unknown ones
// the neutral gray.
func TestLimiterColorFor(t *testing.T) {
	if limiterColorFor(limAirmass) != limiterColors[limAirmass] {
		t.Fatal("known limiter color")
	}
	if got := limiterColorFor("something else"); got != (color.RGBA{0x89, 0x87, 0x81, 0xff}) {
		t.Fatalf("unknown limiter color = %v", got)
	}
	// every limiter the estimators emit has its own color
	for _, l := range []string{limTorqueEngine, limTorqueE85, limTorqueE85Auto, limTorqueGear, limOverboost, limAirmass, limTurboSpeed, limFuelCut} {
		if _, ok := limiterColors[l]; !ok {
			t.Errorf("no color for %q", l)
		}
	}
}

// TestDynoGraphDegenerate: flat zero data still gets a 0..1 axis and a
// cramped widget keeps a minimum plot area.
func TestDynoGraphDegenerate(t *testing.T) {
	test.NewTempApp(t)
	g := newDynoGraph()
	r := test.TempWidgetRenderer(t, g).(*dynoRenderer)
	g.SetData([]float64{1000, 2000}, []graphSeries{{name: "Z", values: []float64{0, 0}, visible: true}})
	r.Layout(fyne.NewSize(30, 20))
	if r.plotBottom-r.plotTop != 24 {
		t.Fatalf("plot height = %v, want the 24px minimum", r.plotBottom-r.plotTop)
	}
	if r.xStep != 5 {
		t.Fatalf("xStep = %v, want 10px minimum plot width / 2", r.xStep)
	}
	if got := visibleTexts(r.yLabels); !slices.Equal(got, []string{"0.0", "0.5", "1.0"}) {
		t.Fatalf("y ticks = %v", got)
	}
}
