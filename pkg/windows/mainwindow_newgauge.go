package windows

import (
	"errors"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/dial"
	"github.com/roffe/txlogger/pkg/widgets/dualdial"
	"github.com/roffe/txlogger/pkg/widgets/hbar"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
	"github.com/roffe/txlogger/pkg/widgets/vbar"
)

var _ fyne.Widget = (*GaugeCreator)(nil)

type GaugeCreator struct {
	widget.BaseWidget

	mw        *MainWindow
	form      *widget.Form
	entries   *gaugeEntries
	container *fyne.Container
}

type gaugeEntries struct {
	name                    *widget.Entry
	unit                    *widget.SelectEntry
	displayString           *widget.SelectEntry
	symbolName              *widget.Select
	symbolNameSecondary     *widget.Select
	min, max, center, steps *numericentry.Widget
	typ                     *widget.Select
}

func NewGaugeCreator(mw *MainWindow) *GaugeCreator {
	g := &GaugeCreator{
		mw:      mw,
		entries: &gaugeEntries{},
	}
	g.ExtendBaseWidget(g)

	g.entries.name = widget.NewEntry()
	g.entries.unit = widget.NewSelectEntry([]string{"rpm", "mg/c", "km/h", "mph", "°C", "λ", "%"})

	g.entries.displayString = widget.NewSelectEntry([]string{"%.0f", "%.1f", "%.2f", "%.3f", "%.4f", "%.5f"})

	g.entries.symbolName = widget.NewSelect(mw.symbolList.Names(), func(s string) {
	})

	g.entries.symbolNameSecondary = widget.NewSelect(mw.symbolList.Names(), func(s string) {
	})
	g.entries.symbolNameSecondary.Disable()

	g.entries.min = numericentry.New()
	g.entries.min.SetText("0")
	g.entries.max = numericentry.New()
	g.entries.max.SetText("100")

	g.entries.center = numericentry.New()
	g.entries.center.SetText("50")
	g.entries.center.Disable()

	g.entries.steps = numericentry.New()
	g.entries.steps.SetText("10")

	g.entries.typ = widget.NewSelect([]string{"Dial", "DualDial", "VBar", "HBar", "CBar"}, func(s string) {
		switch s {
		case "Dial":
			g.entries.symbolNameSecondary.Disable()
			g.entries.center.Disable()
			g.form.Refresh()
		case "DualDial":
			g.entries.symbolNameSecondary.Enable()
			g.entries.center.Disable()
			g.form.Refresh()
		case "VBar":
			g.entries.symbolNameSecondary.Disable()
			g.entries.center.Disable()
			g.form.Refresh()
		case "HBar":
			g.entries.symbolNameSecondary.Disable()
			g.entries.center.Disable()
			g.form.Refresh()
		case "CBar":
			g.entries.symbolNameSecondary.Disable()
			g.entries.center.Enable()
			g.form.Refresh()
		}
	})

	typ := widget.NewFormItem("Type", g.entries.typ)
	typ.HintText = "Select the type of gauge to create"

	name := widget.NewFormItem("Name", g.entries.name)
	name.HintText = "Name will be title of the gauge window"

	unit := widget.NewFormItem("Unit", g.entries.unit)
	unit.HintText = "Unit will be displayed at the bottom of the gauge"

	dispstr := widget.NewFormItem("Display format", g.entries.displayString)
	dispstr.HintText = "Select the number of decimals to display"

	sym := widget.NewFormItem("Symbol", g.entries.symbolName)
	sym2 := widget.NewFormItem("Secondary Symbol", g.entries.symbolNameSecondary)

	min := widget.NewFormItem("Min value", g.entries.min)
	max := widget.NewFormItem("Max value", g.entries.max)
	center := widget.NewFormItem("Center value", g.entries.center)

	steps := widget.NewFormItem("Steps", g.entries.steps)
	steps.HintText = "Number of steps in the gauge"

	g.form = widget.NewForm(
		typ,
		name,
		unit,
		dispstr,
		sym,
		sym2,
		min,
		max,
		center,
		steps,
	)

	g.container = container.NewBorder(
		nil,
		widget.NewButtonWithIcon("Create", theme.ContentAddIcon(), func() {
			g.onSubmit()
		}),
		nil,
		nil,
		g.form,
	)

	return g
}

func float64or0(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
func intor10(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 10
	}
	return i
}

func (g *GaugeCreator) onSubmit() {
	min := float64or0(g.entries.min.Text)
	max := float64or0(g.entries.max.Text)
	center := float64or0(g.entries.center.Text)
	steps := intor10(g.entries.steps.Text)

	var cancelFuncs []func()
	var gauge fyne.CanvasObject

	gaugeConfig := widgets.GaugeConfig{
		Title:               g.entries.unit.Text,
		DisplayString:       g.entries.displayString.Text,
		Min:                 min,
		Max:                 max,
		Center:              center,
		Steps:               int(steps),
		MinSize:             fyne.NewSize(100, 100),
		SymbolName:          g.entries.symbolName.Selected,
		SymbolNameSecondary: g.entries.symbolNameSecondary.Selected,
	}
	switch g.entries.typ.Selected {
	case "Dial":
		gaugeConfig.Type = "Dial"
		dial := dial.New(gaugeConfig)
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(g.entries.symbolName.Selected, dial.SetValue))
		gauge = dial
	case "DualDial":
		gaugeConfig.Type = "DualDial"
		dualDial := dualdial.New(gaugeConfig)
		cancelFuncs = append(cancelFuncs,
			ebus.SubscribeFunc(g.entries.symbolName.Selected, dualDial.SetValue),
			ebus.SubscribeFunc(g.entries.symbolNameSecondary.Selected, dualDial.SetValue2),
		)
		gauge = dualDial
	case "VBar":
		gaugeConfig.Type = "VBar"
		gaugeConfig.MinSize = fyne.NewSize(50, 100)
		vbar := vbar.New(gaugeConfig)
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(g.entries.symbolName.Selected, vbar.SetValue))
		gauge = vbar
	case "HBar":
		gaugeConfig.Type = "HBar"
		gaugeConfig.MinSize = fyne.NewSize(100, 50)
		hbar := hbar.New(gaugeConfig)
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(g.entries.symbolName.Selected, hbar.SetValue))
		gauge = hbar
	case "CBar":
		gaugeConfig.Type = "CBar"
		gaugeConfig.MinSize = fyne.NewSize(100, 50)
		cbar := cbar.New(gaugeConfig)
		cancelFuncs = append(cancelFuncs, ebus.SubscribeFunc(g.entries.symbolName.Selected, cbar.SetValue))
		gauge = cbar
	default:
		g.mw.Error(errors.New("unknown gauge type"))
		return
	}
	iw := newInnerWindow(g.entries.name.Text, gauge)
	iw.CloseIntercept = func() {
		for _, cancel := range cancelFuncs {
			cancel()
		}
		g.mw.wm.Remove(iw)
	}
	if !g.mw.wm.Add(iw) {
		for _, cancel := range cancelFuncs {
			cancel()
		}
	}
}

func (g *GaugeCreator) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.container)
}
