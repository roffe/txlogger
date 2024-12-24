package windows

import (
	"errors"
	"strconv"

	"fyne.io/fyne/v2"
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

	mw      *MainWindow
	form    *widget.Form
	entries *gaugeEntries
}

type gaugeEntries struct {
	name                *widget.Entry
	unit                *widget.Entry
	displayString       *widget.Entry
	symbolName          *widget.Select
	symbolNameSecondary *widget.Select
	min                 *numericentry.Widget
	max                 *numericentry.Widget
	steps               *numericentry.Widget
	typ                 *widget.Select
}

func NewGaugeCreator(mw *MainWindow) *GaugeCreator {
	g := &GaugeCreator{
		mw:      mw,
		entries: &gaugeEntries{},
	}
	g.ExtendBaseWidget(g)

	g.entries.name = widget.NewEntry()
	g.entries.unit = widget.NewEntry()
	g.entries.displayString = widget.NewEntry()
	g.entries.displayString.SetText("%.0f")
	g.entries.symbolName = widget.NewSelect(mw.symbolList.Names(), func(s string) {
	})
	g.entries.symbolNameSecondary = widget.NewSelect(mw.symbolList.Names(), func(s string) {
	})
	g.entries.symbolNameSecondary.Disable()

	g.entries.min = numericentry.New()
	g.entries.min.SetText("0")
	g.entries.max = numericentry.New()
	g.entries.max.SetText("100")
	g.entries.steps = numericentry.New()
	g.entries.steps.SetText("10")

	g.entries.typ = widget.NewSelect([]string{"Dial", "DualDial", "VBar", "Hbar", "CBar"}, func(s string) {
		if s == "DualDial" {
			g.entries.symbolNameSecondary.Enable()
			g.form.Refresh()
		} else {
			g.entries.symbolNameSecondary.Disable()
			g.form.Refresh()
		}
	})

	g.form = widget.NewForm(
		widget.NewFormItem("Type", g.entries.typ),
		widget.NewFormItem("Name", g.entries.name),
		widget.NewFormItem("Unit", g.entries.unit),
		widget.NewFormItem("Display String", g.entries.displayString),
		widget.NewFormItem("Symbol", g.entries.symbolName),
		widget.NewFormItem("Secondary Symbol", g.entries.symbolNameSecondary),
		widget.NewFormItem("Min value", g.entries.min),
		widget.NewFormItem("Max value", g.entries.max),
		widget.NewFormItem("Steps", g.entries.steps),
	)

	g.form.SubmitText = "Create"

	g.form.OnSubmit = g.onSubmit

	return g
}

func (g *GaugeCreator) onSubmit() {
	min, err := strconv.ParseFloat(g.entries.min.Text, 64)
	if err != nil {
		g.mw.Error(err)
		return
	}
	max, err := strconv.ParseFloat(g.entries.max.Text, 64)
	if err != nil {
		g.mw.Error(err)
		return
	}
	steps, err := strconv.ParseInt(g.entries.steps.Text, 10, 64)
	if err != nil {
		g.mw.Error(err)
		return
	}

	switch g.entries.typ.Selected {
	case "Dial":
		dialCfg := widgets.GaugeConfig{
			Title:         g.entries.unit.Text,
			DisplayString: g.entries.displayString.Text,
			Min:           min,
			Max:           max,
			Steps:         int(steps),
			MinSize:       fyne.NewSize(100, 100),
		}
		d := dial.New(dialCfg)

		cancel := ebus.SubscribeFunc(g.entries.symbolName.Selected, d.SetValue)

		iw := newInnerWindow(g.entries.name.Text, d)
		iw.CloseIntercept = func() {
			cancel()
			g.mw.wm.Remove(iw)
		}
		if !g.mw.wm.Add(iw) {
			cancel()
		}
	case "DualDial":
		dualDialCfg := widgets.GaugeConfig{
			Title:         g.entries.unit.Text,
			DisplayString: g.entries.displayString.Text,
			Min:           min,
			Max:           max,
			Steps:         int(steps),
			MinSize:       fyne.NewSize(100, 100),
		}
		dd := dualdial.New(dualDialCfg)

		cancel := ebus.SubscribeFunc(g.entries.symbolName.Selected, dd.SetValue)
		cancel2 := ebus.SubscribeFunc(g.entries.symbolNameSecondary.Selected, dd.SetValue2)

		iw := newInnerWindow(g.entries.name.Text, dd)
		iw.CloseIntercept = func() {
			cancel()
			cancel2()
			g.mw.wm.Remove(iw)
		}
		if !g.mw.wm.Add(iw) {
			cancel()
			cancel2()
		}
	case "VBar":
		vbarConfig := widgets.GaugeConfig{
			Title:   g.entries.unit.Text,
			Min:     min,
			Max:     max,
			Steps:   int(steps),
			MinSize: fyne.NewSize(100, 100),
		}
		vb := vbar.New(vbarConfig)

		cancel := ebus.SubscribeFunc(g.entries.symbolName.Selected, vb.SetValue)

		iw := newInnerWindow(g.entries.name.Text, vb)
		iw.CloseIntercept = func() {
			cancel()
			g.mw.wm.Remove(iw)
		}
		if !g.mw.wm.Add(iw) {
			cancel()
		}
	case "HBar":
		hbarConfig := widgets.GaugeConfig{
			Title:   g.entries.unit.Text,
			Min:     min,
			Max:     max,
			Steps:   int(steps),
			MinSize: fyne.NewSize(100, 100),
		}
		hb := hbar.New(hbarConfig)

		cancel := ebus.SubscribeFunc(g.entries.symbolName.Selected, hb.SetValue)

		iw := newInnerWindow(g.entries.name.Text, hb)

		iw.CloseIntercept = func() {
			cancel()
			g.mw.wm.Remove(iw)
		}
		if !g.mw.wm.Add(iw) {
			cancel()
		}
	case "CBar":
		cbarConfig := widgets.GaugeConfig{
			Title:   g.entries.unit.Text,
			Min:     min,
			Max:     max,
			Steps:   int(steps),
			MinSize: fyne.NewSize(100, 100),
		}
		cb := cbar.New(cbarConfig)

		cancel := ebus.SubscribeFunc(g.entries.symbolName.Selected, cb.SetValue)

		iw := newInnerWindow(cbarConfig.Title, cb)

		iw.CloseIntercept = func() {
			cancel()
			g.mw.wm.Remove(iw)
		}
		if !g.mw.wm.Add(iw) {
			cancel()
		}
	default:
		g.mw.Error(errors.New("unknown gauge type"))
	}
}

func (g *GaugeCreator) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.form)
}
