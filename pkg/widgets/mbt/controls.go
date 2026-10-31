package mbt

import (
	_ "embed"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

// num builds a labelled entry bound to p. The stored value is
// shown*scale + offset, so a field can be edited in MJ/kg or °C while the
// model keeps J/kg and K.
func (w *Widget) num(label string, p *float64, scale, offset float64) (*fyne.Container, *numericentry.Widget) {
	e := numericentry.New(strconv.FormatFloat((*p-offset)/scale, 'f', -1, 64))
	e.OnChanged = func(s string) {
		v, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
		if err != nil {
			return
		}
		*p = v*scale + offset
		w.recalc()
	}
	return container.NewGridWithColumns(2, widget.NewLabel(label), e), e
}

// row is num for the common unscaled case.
func (w *Widget) row(label string, p *float64) *fyne.Container {
	c, _ := w.num(label, p, 1, 0)
	return c
}

// helpMD is the widget's documentation, shown by the Help button. Embedded
// so it ships with the binary and cannot drift out of the package.
//
//go:embed MBT.md
var helpMD string

func (w *Widget) controls() fyne.CanvasObject {
	geometry, engineSel := w.geometryGroup()
	charge, fuelSel := w.chargeGroup()

	acc := widget.NewAccordion(
		widget.NewAccordionItem("Engine", container.NewVBox(engineSel, geometry)),
		widget.NewAccordionItem("Charge & fuel", container.NewVBox(fuelSel, charge)),
		widget.NewAccordionItem("Cycle", w.cycleGroup()),
		widget.NewAccordionItem("Burn angle", w.burnGroup()),
	)
	acc.Open(3) // the knobs that actually decide the answer

	help := widget.NewButtonWithIcon("Help", theme.HelpIcon(), w.openHelp)
	return container.NewBorder(help, nil, nil, nil,
		container.NewVScroll(container.NewVBox(acc)))
}

const helpTitle = "MBT ignition analyser help"

// openHelp shows MBT.md in a resizable window, through the host's window
// manager when it supplied one so the help sits alongside the other tools.
func (w *Widget) openHelp() {
	md := widget.NewRichTextFromMarkdown(helpMD)
	md.Wrapping = fyne.TextWrapWord
	content := container.NewVScroll(md)

	if w.cfg.OpenWindow != nil {
		w.cfg.OpenWindow(helpTitle, content)
		return
	}
	win := fyne.CurrentApp().NewWindow(helpTitle)
	win.SetContent(content)
	win.Resize(fyne.NewSize(820, 620))
	win.Show()
}

func (w *Widget) geometryGroup() (fyne.CanvasObject, fyne.CanvasObject) {
	bore, eBore := w.num("Bore (mm)", &w.model.Engine.Bore, 1, 0)
	stroke, eStroke := w.num("Stroke (mm)", &w.model.Engine.Stroke, 1, 0)
	rod, eRod := w.num("Con rod (mm)", &w.model.Engine.Rod, 1, 0)
	cr, eCR := w.num("Compression ratio", &w.model.Engine.CR, 1, 0)
	w.eBore, w.eStroke, w.eRod, w.eCR = eBore, eStroke, eRod, eCR

	names := make([]string, len(Engines))
	for i, e := range Engines {
		names[i] = e.Name
	}
	sel := widget.NewSelect(names, func(name string) {
		for _, e := range Engines {
			if e.Name != name {
				continue
			}
			w.model.Engine = e
			// SetText fires OnChanged, which recalcs; CR last so the
			// final recalc sees the whole geometry
			w.eBore.SetText(strconv.FormatFloat(e.Bore, 'f', -1, 64))
			w.eStroke.SetText(strconv.FormatFloat(e.Stroke, 'f', -1, 64))
			w.eRod.SetText(strconv.FormatFloat(e.Rod, 'f', -1, 64))
			w.eCR.SetText(strconv.FormatFloat(e.CR, 'f', -1, 64))
		}
	})
	sel.SetSelected(w.model.Engine.Name)
	return container.NewVBox(bore, stroke, rod, cr), sel
}

func (w *Widget) chargeGroup() (fyne.CanvasObject, fyne.CanvasObject) {
	qhv, eQHV := w.num("Heating value (MJ/kg)", &w.model.Params.QHV, 1e6, 0)
	afs, eAFs := w.num("Stoich A/F", &w.model.Params.AFs, 1, 0)
	w.eQHV, w.eAFs = eQHV, eAFs

	fuel := widget.NewSelect([]string{"Petrol", "E85"}, func(name string) {
		q, a := 44.0, 14.67
		if name == "E85" {
			q, a = 29.2, 9.8
		}
		w.eQHV.SetText(strconv.FormatFloat(q, 'f', -1, 64))
		w.eAFs.SetText(strconv.FormatFloat(a, 'f', -1, 64))
	})
	fuel.SetSelected("Petrol")

	tim, _ := w.num("Charge temp at IVC (°C)", &w.model.Params.Tim, 1, 273.15)
	tr, _ := w.num("Residual gas temp (°C)", &w.model.Params.Tr, 1, 273.15)

	note := widget.NewLabel(
		"λ moves MBT (through the burn angles). The temperatures and\n" +
			"residual fraction only move the pressure trace and peak\n" +
			"pressure — they are not in the MBT calculation.")
	note.TextStyle.Italic = true

	return container.NewVBox(
		qhv, afs,
		w.row("λ closed loop", &w.lamCL),
		w.row("λ open loop", &w.lamOL),
		tim, tr,
		w.row("Residual fraction", &w.model.Params.Xr),
		note,
	), fuel
}

func (w *Widget) cycleGroup() fyne.CanvasObject {
	return container.NewVBox(
		w.row("kc, compression", &w.model.Params.Kc),
		w.row("ke, expansion", &w.model.Params.Ke),
		w.row("cv (J/kg·K)", &w.model.Params.Cv),
		w.row("IVC (° ATDC)", &w.model.Params.ThetaIVC),
		w.row("EVO (° ATDC)", &w.model.Params.ThetaEVO),
	)
}

func (w *Widget) burnGroup() fyne.CanvasObject {
	note := widget.NewLabel(
		"MBT is defined as the angle a chosen burn fraction should land on.\n" +
			"45% at 9° ATDC is the robust choice: with Δθd and Δθb scaling\n" +
			"together, its optimum moves 1° over wide burn-angle variation\n" +
			"against 3° for 50% and 15° for peak pressure position.\n\n" +
			"Δθd and Δθb are at the reference point, scaled by rpm, load and\n" +
			"λ. None of it is measurable from a T7 log — Fit sets the overall\n" +
			"scale from the part-load cells of the loaded map, where the\n" +
			"calibration should already be at MBT.")
	note.TextStyle.Italic = true

	fitRow := container.NewGridWithColumns(2,
		widget.NewButtonWithIcon("Fit to map", theme.SearchReplaceIcon(), w.fit),
		container.NewGridWithColumns(2,
			widget.NewLabel("≤ mg/c"),
			func() *numericentry.Widget {
				e := numericentry.New(strconv.FormatFloat(w.fitTo, 'f', -1, 64))
				e.OnChanged = func(s string) {
					if v, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err == nil {
						w.fitTo = v
					}
				}
				return e
			}(),
		),
	)

	scale, eScale := w.num("Overall scale", &w.model.Burn.Scale, 1, 0)
	w.eScale = eScale

	return container.NewVBox(
		note,
		w.row("Burn fraction at MBT", &w.crit.Fraction),
		w.row("its angle at MBT (° ATDC)", &w.crit.Target),
		w.row("Δθd, 0-10% burn (CAD)", &w.model.Burn.Dev),
		w.row("Δθb, 10-85% burn (CAD)", &w.model.Burn.Fast),
		w.row("Reference rpm", &w.model.Burn.RefRPM),
		w.row("Reference load (mg/c)", &w.model.Burn.RefAir),
		w.row("Reference λ", &w.model.Burn.RefLambda),
		w.row("rpm exponent", &w.model.Burn.RPMExp),
		w.row("load exponent", &w.model.Burn.LoadExp),
		w.row("λ slope (per unit λ)", &w.model.Burn.LambdaK),
		scale,
		fitRow,
	)
}
