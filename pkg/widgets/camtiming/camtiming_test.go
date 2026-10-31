package camtiming

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// Every published stock cam quotes events and a duration that agree; if a
// spec is ever typed in wrong this is what says so.
func TestPublishedDurations(t *testing.T) {
	want := map[string]float64{
		"7509201": 246, "7509219": 252, "7560808": 252, "7560964": 254,
		"7561467": 240, "9116690": 246, "9116708": 246, "9145632": 240,
		"9145640": 240, "9145657": 246, "9145665": 246, "9170887": 231,
		"9188855": 228, "9170895": 231,
	}
	for _, c := range Cams {
		d, ok := want[c.Name]
		if !ok {
			continue
		}
		if got := c.Duration(); got != d {
			t.Errorf("%s duration = %v, published %v", c.Name, got, d)
		}
	}
}

func TestTimingMath(t *testing.T) {
	// the B235R pair: 9170887 intake, 9170895 exhaust
	in := Cam{PeakLift: 8.31, Open: 12, Close: 39}
	ex := Cam{Exhaust: true, PeakLift: 8.31, Open: 37, Close: 14}

	if got := in.Centerline(); got != 103.5 { // 231/2 - 12
		t.Errorf("ICL = %v, want 103.5", got)
	}
	if got := ex.Centerline(); got != 101.5 { // 231/2 - 14
		t.Errorf("ECL = %v, want 101.5", got)
	}
	if got := LSA(in, ex); got != 102.5 {
		t.Errorf("LSA = %v, want 102.5", got)
	}
	if got := Overlap(in, ex); got != 26 { // IVO + EVC
		t.Errorf("overlap = %v, want 26", got)
	}

	// advancing the intake 4° opens and closes it 4° earlier, keeps the
	// duration and moves the centerline with it
	adv := in.Advance(4)
	if adv.Open != 16 || adv.Close != 35 || adv.Duration() != in.Duration() {
		t.Errorf("advanced = %+v, want IVO 16 / IVC 35 / 231°", adv)
	}
	if got := adv.Centerline(); got != 99.5 {
		t.Errorf("advanced ICL = %v, want 99.5", got)
	}
	if got := Overlap(adv, ex); got != 30 {
		t.Errorf("advanced overlap = %v, want 30", got)
	}

	// lift is zero at the quoted events and peaks at the centerline
	if got := in.Lift(-12); got != 0 {
		t.Errorf("lift at IVO = %v, want 0", got)
	}
	if got := in.Lift(219); got != 0 {
		t.Errorf("lift at IVC = %v, want 0", got)
	}
	if got := in.Lift(in.Centerline()); math.Abs(got-8.31) > 1e-9 {
		t.Errorf("peak lift = %v, want 8.31", got)
	}
}

func TestVEFrom(t *testing.T) {
	// 2290cc four at 1.000 bar / 20 °C: one cylinder holds
	// 1e5/(287.05*293.15) * 572.5e-6 m³ = 680.6 mg
	if got := veFrom(680.6, 1, 20, 2290); math.Abs(got-100) > 0.1 {
		t.Errorf("VE = %v%%, want 100%%", got)
	}
	// twice the airmass at the same conditions is twice the VE
	if got := veFrom(1361.2, 1, 20, 2290); math.Abs(got-200) > 0.2 {
		t.Errorf("VE = %v%%, want 200%%", got)
	}
	// colder air is denser, so the same airmass is less VE
	if cold, warm := veFrom(700, 1.8, 20, 2290), veFrom(700, 1.8, 60, 2290); cold >= warm {
		t.Errorf("VE at 20 °C (%v) should be below 60 °C (%v)", cold, warm)
	}
	for _, bad := range [][4]float64{{700, 0, 20, 2290}, {700, 1, -300, 2290}, {700, 1, 20, 0}} {
		if got := veFrom(bad[0], bad[1], bad[2], bad[3]); got != 0 {
			t.Errorf("veFrom%v = %v, want 0", bad, got)
		}
	}
}

// The chart draws from a renderer, so a widget that is laid out and then
// fed new data must not panic or come up empty.
// CAMTIMING_DUMP=<path> writes the capture there (used for the webpage docs)
func TestRender(t *testing.T) {
	test.NewApp()
	w := New(&Config{ECU: "T7"})
	win := test.NewWindow(w)
	defer win.Close()
	win.Resize(fyne.NewSize(1000, 620))
	w.recalc()
	if n := len(w.diagram.renderer.objects); n < 10 {
		t.Errorf("diagram has %d canvas objects, expected the lobes to be drawn", n)
	}
	// intake and exhaust pick templates independently: a B202 T16 intake
	// on a B235R exhaust is a swap people actually run
	w.intake.template.SetSelected("7560808 — B202 T16 1986-93")
	if in, ex := w.cams(); in.Duration() != 252 || ex.Duration() != 231 {
		t.Errorf("mixed pair = %v°/%v°, want 252°/231°", in.Duration(), ex.Duration())
	}
	w.intake.compare.SetSelected("Catcams Race — mechanical tappet")
	w.exhaust.compare.SetSelected("Catcams Race — mechanical tappet")
	w.intake.advance.SetText("-6")
	if got := len(w.diagram.series); got != 4 {
		t.Errorf("compare set: %d series, want 4", got)
	}

	// a synthetic pull: airmass rising to a VE peak mid range
	for rpm := 2000.0; rpm <= 6000; rpm += 20 {
		ve := 95 + 15*math.Sin(math.Pi*(rpm-2000)/4000)
		air := ve / 100 * 1.8e5 / (287.05 * 313.15) * (2290 / 4 * 1e-6) * 1e6
		w.ve.samples = append(w.ve.samples, veSample{rpm: rpm, air: air, bar: 1.8, iat: 40, pedal: 100})
	}
	w.ve.recalc()
	if len(w.ve.chart.series) != 1 || len(w.ve.chart.series[0].pts) < 10 {
		t.Fatalf("VE curve: %+v", w.ve.chart.series)
	}
	var peak float64
	for _, p := range w.ve.chart.series[0].pts {
		peak = math.Max(peak, p.y)
	}
	if math.Abs(peak-110) > 2 {
		t.Errorf("peak VE = %v%%, want ~110%%", peak)
	}

	// the hover readout: mid intake lobe the exhaust valve is shut
	w.diagram.hoverPos = fyne.NewPos(640, 300)
	w.diagram.hoverX = 100
	w.diagram.renderer.updateHover()
	if got := w.diagram.renderer.tipTexts[0].Text; got != "100°" {
		t.Errorf("hover header = %q, want \"100°\"", got)
	}
	if got := w.diagram.renderer.tipTexts[2].Text; !strings.HasSuffix(got, "—") {
		t.Errorf("hover exhaust = %q, want it closed", got)
	}
	v, ok := valueAt(w.diagram.series[0].pts, w.intake.cam(false).Centerline())
	if !ok || math.Abs(v-8.65) > 0.01 {
		t.Errorf("lift at ICL = %v (%v), want 8.65", v, ok)
	}

	// the VE chart only gets a layout once its tab is on screen
	w.tabs.SelectIndex(1)
	win.Canvas().Capture()
	w.ve.chart.hoverPos = fyne.NewPos(660, 200)
	w.ve.chart.hoverX = 4000
	w.ve.chart.renderer.updateHover()
	if got := w.ve.chart.renderer.tipTexts[0].Text; got != "4000 rpm" {
		t.Errorf("VE hover header = %q, want \"4000 rpm\"", got)
	}
	at4k, _ := valueAt(w.ve.chart.series[0].pts, 4000)
	if got, want := w.ve.chart.renderer.tipTexts[1].Text, fmt.Sprintf("VE  %.1f %%", at4k); got != want {
		t.Errorf("VE hover value = %q, want %q", got, want)
	}

	if path := os.Getenv("CAMTIMING_DUMP"); path != "" {
		if os.Getenv("CAMTIMING_TAB") == "" {
			w.tabs.SelectIndex(0)
		}
		img := win.Canvas().Capture()
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

// The reported bug: the readout stayed dead until a template was picked
// again, so hover has to work on a freshly opened widget with nothing
// touched in between.
func TestHoverAfterOpen(t *testing.T) {
	test.NewApp()
	w := New(&Config{ECU: "T7"})
	win := test.NewWindow(w)
	defer win.Close()
	win.Resize(fyne.NewSize(1000, 620))
	win.Canvas().Capture() // first paint, as opening the window does

	test.MoveMouse(win.Canvas(), fyne.NewPos(640, 300))
	tip := w.diagram.renderer.tipTexts[0]
	if !tip.Visible() || !strings.HasSuffix(tip.Text, "°") {
		t.Errorf("tooltip after open: %q visible=%v", tip.Text, tip.Visible())
	}
	if !w.diagram.renderer.cursor.Visible() {
		t.Error("cursor line not shown")
	}

	test.MoveMouse(win.Canvas(), fyne.NewPos(5, 5)) // off the plot
	if w.diagram.renderer.cursor.Visible() {
		t.Error("cursor still shown with the pointer off the plot")
	}
}
