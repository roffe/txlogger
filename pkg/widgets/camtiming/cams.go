// Package camtiming draws the valve timing diagram of the SAAB 16v
// camshafts: valve lift against crank angle for a chosen intake/exhaust
// pair, with the duration, centerlines, lobe separation and overlap that
// follow from it.
//
// The specs are the published SAAB and aftermarket figures collected on
// 900aero.com/main/tech_main_cams.htm. Their events are quoted at the
// manufacturer's own reference lift, so what is drawn is advertised
// duration, not duration at 0.050": every stock entry below satisfies
// duration = open + 180 + close, which is what makes them comparable to
// each other and nothing more. Nothing here is measured off a real cam.
//
// B207 (Trionic 8) is absent on purpose: it runs intake CVVT, so its
// timing is a live value in the log rather than a fixed spec.
package camtiming

import "math"

// Cam is one camshaft. Open and Close are the published events in crank
// degrees: intake IVO °BTDC / IVC °ABDC, exhaust EVO °BBDC / EVC °ATDC.
// Applies names the engines it was fitted to, without saying which side
// of the head it is — the Exhaust flag and the list it appears in do
// that, and the template selects have little enough room as it is.
type Cam struct {
	Name     string
	Applies  string
	Exhaust  bool
	PeakLift float64 // mm at the valve
	Open     float64
	Close    float64
}

func (c Cam) Label() string { return c.Name + " — " + c.Applies }

// Duration is the advertised duration in crank degrees.
func (c Cam) Duration() float64 { return c.Open + 180 + c.Close }

// window returns the crank angles the valve is off its seat, in degrees
// relative to the overlap TDC (0) — the top between exhaust and intake
// stroke, which is the only point both cams are referenced to.
func (c Cam) window() (open, close float64) {
	if c.Exhaust {
		return -(180 + c.Open), c.Close
	}
	return -c.Open, 180 + c.Close
}

// Centerline is the crank angle of peak lift measured away from the
// overlap TDC: intake after it, exhaust before it.
func (c Cam) Centerline() float64 {
	open, close := c.window()
	mid := (open + close) / 2
	if c.Exhaust {
		return -mid
	}
	return mid
}

// Advance returns the cam turned deg degrees earlier in the cycle, as an
// adjustable cam gear does. Negative retards it. Duration is unchanged.
func (c Cam) Advance(deg float64) Cam {
	c.Open += deg
	c.Close -= deg
	return c
}

// Lift is the valve lift in mm at crank angle deg (0 = overlap TDC).
//
// ponytail: no lobe profile is published for any of these cams, so the
// lobe is a sin² hump over the advertised duration — symmetric about the
// centerline, zero lift and zero velocity at the quoted events, and an
// area-to-peak ratio of 0.5, about what a real automotive lobe gives.
// Swap in a table of measured lift if one ever turns up.
func (c Cam) Lift(deg float64) float64 {
	open, close := c.window()
	if deg <= open || deg >= close {
		return 0
	}
	s := math.Sin(math.Pi * (deg - open) / (close - open))
	return c.PeakLift * s * s
}

// Overlap is the crank degrees both valves are off their seats around the
// exhaust/intake TDC. Zero when the cams do not overlap at all.
func Overlap(intake, exhaust Cam) float64 {
	_, exClose := exhaust.window()
	inOpen, _ := intake.window()
	return math.Max(0, exClose-inOpen)
}

// LSA is the lobe separation angle, the cam degrees between the two
// centerlines.
func LSA(intake, exhaust Cam) float64 {
	return (intake.Centerline() + exhaust.Centerline()) / 2
}

// Cams is every cam the source publishes timing for, stock first. The
// three it lists without any figures (7518913, 9148305, 9148313) are left
// out — a name with no events draws nothing.
var Cams = []Cam{
	{"7509201", "B202 turbo 1984-85", false, 8.65, 10, 56},
	{"7509219", "B202 turbo 1984-85", true, 8.65, 56, 16},
	{"7560808", "B202 T16 1986-93", false, 8.65, 16, 56},
	{"7560964", "B202/B212 1986-93", true, 8.65, 61, 13},
	{"7561467", "B202i/B212 1986-93", false, 8.65, 16, 44},
	{"9116690", "B234 1990-92", false, 8.65, 13, 53},
	{"9116708", "B234 1990-92", true, 8.65, 50, 16},
	{"9145632", "B204/B206 1994-2000", false, 8.65, 14, 46},
	{"9145640", "B204/B206 1994-2000", true, 8.65, 44, 16},
	{"9145657", "B234i 1994-2000", false, 8.65, 13, 53},
	{"9145665", "B234i 1994-2000", true, 8.65, 48, 18},
	{"9170887", "B205/B235R 1999-2001", false, 8.31, 12, 39},
	{"9188855", "B205 1999-2001", true, 8.07, 34, 14},
	{"9170895", "B235R 1999-2001", true, 8.31, 37, 14},

	// Aftermarket. The source's own Swedish Dynamics row disagrees with
	// itself — it prints 254° and 35° overlap where its events give 252°
	// and 38° — so the events are used and the printed figures ignored.
	{"SD Red", "Swedish Dynamics Red-series", false, 9.37, 22, 61},
	{"SD Red", "Swedish Dynamics Red-series", true, 8.65, 56, 16},
	{"Catcams Sport-1", "hydraulic tappet", false, 9.55, 12, 56},
	{"Catcams Sport-1", "hydraulic tappet", true, 9.55, 56, 12},
	{"Catcams Sport-2", "hydraulic tappet", false, 9.75, 23, 67},
	{"Catcams Sport-2", "hydraulic tappet", true, 9.75, 67, 23},
	{"Catcams Rally", "hydraulic tappet", false, 10.95, 28, 64},
	{"Catcams Rally", "hydraulic tappet", true, 10.9, 58, 22},
	{"Catcams Turbo", "mechanical tappet", false, 11.3, 18, 58},
	{"Catcams Turbo", "mechanical tappet", true, 11.3, 58, 18},
	{"Catcams Race", "mechanical tappet", false, 12.5, 39, 69},
	{"Catcams Race", "mechanical tappet", true, 11.95, 65, 35},
}

// find returns the named cam for one side of the head. The callers name
// constants from the table above, so a miss is a programming error and
// the zero Cam draws nothing.
func find(name string, exhaust bool) Cam {
	for _, c := range Cams {
		if c.Name == name && c.Exhaust == exhaust {
			return c
		}
	}
	return Cam{}
}
