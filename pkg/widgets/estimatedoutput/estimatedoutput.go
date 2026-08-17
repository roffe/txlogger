package estimatedoutput

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

type Config struct {
	ECU        string
	GetFW      func() symbol.FirmwareFile
	Colorblind colors.ColorBlindMode
}

type Widget struct {
	widget.BaseWidget

	cfg  *Config
	calc Calculator

	opts    map[string]float64
	visible map[string]bool

	graph       *dynoGraph
	table       *heatTable
	legend      *fyne.Container
	legendNames []string
	summary     *widget.Label

	pulls   []*logPull
	pullIdx int // -1 = average of all pulls
	pullSel *widget.Select
	logPrm  logParams
	logInfo *widget.Label

	content fyne.CanvasObject
}

func New(cfg *Config) *Widget {
	w := &Widget{
		cfg:     cfg,
		opts:    make(map[string]float64),
		visible: make(map[string]bool),
		logPrm:  defaultLogParams,
	}
	w.ExtendBaseWidget(w)
	w.calc = For(cfg.ECU)
	if w.calc == nil {
		w.content = container.NewCenter(widget.NewLabel("No output estimator for ECU " + cfg.ECU))
		return w
	}
	w.graph = newDynoGraph()
	w.table = newHeatTable(cfg.Colorblind)
	w.legend = container.NewHBox()
	w.summary = widget.NewLabel("")
	w.summary.Wrapping = fyne.TextWrapWord
	tabs := container.NewAppTabs(
		container.NewTabItem("Graph",
			container.NewBorder(nil, container.NewHScroll(w.legend), nil, nil, w.graph),
		),
		container.NewTabItem("Table", w.table),
	)
	w.content = container.NewBorder(nil, w.summary, w.buildOptions(), w.buildLogOptions(), tabs)
	w.recalc()
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

func (w *Widget) buildOptions() fyne.CanvasObject {
	items := []fyne.CanvasObject{}
	for _, opt := range w.calc.Options(w.cfg.GetFW()) {
		w.opts[opt.Key] = opt.Default
		key := opt.Key
		switch opt.Kind {
		case OptionBool:
			c := widget.NewCheck(opt.Label, func(v bool) {
				w.opts[key] = 0
				if v {
					w.opts[key] = 1
				}
				w.recalc()
			})
			c.Checked = opt.Default == 1
			if opt.Disabled {
				c.Disable()
			}
			items = append(items, c)
		case OptionChoice:
			sel := widget.NewSelect(opt.Choices, nil)
			sel.SetSelectedIndex(int(opt.Default))
			sel.OnChanged = func(string) {
				w.opts[key] = float64(sel.SelectedIndex())
				w.recalc()
			}
			if opt.Disabled {
				sel.Disable()
			}
			items = append(items, widget.NewLabel(opt.Label), sel)
		case OptionNumber:
			e := numericentry.New(strconv.FormatFloat(opt.Default, 'f', -1, 64))
			e.OnChanged = func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					w.opts[key] = v
					w.recalc()
				}
			}
			items = append(items, widget.NewLabel(opt.Label), e)
		}
	}
	// items = append(items, w.buildLogOptions()...)
	items = append(items, layout.NewSpacer(),
		widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), w.recalc))
	return container.NewVBox(items...)
}

// numField is a labeled numeric entry bound to a float, recalculating on
// every valid change. The value is remembered between sessions under the
// preference key, falling back to whatever the float already holds.
func (w *Widget) numField(key, label string, val *float64) []fyne.CanvasObject {
	prefs := fyne.CurrentApp().Preferences()
	*val = prefs.FloatWithFallback(key, *val)
	e := numericentry.New(strconv.FormatFloat(*val, 'f', -1, 64))
	e.OnChanged = func(s string) {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			*val = v
			prefs.SetFloat(key, v)
			w.recalc()
		}
	}
	return []fyne.CanvasObject{widget.NewLabel(label), e}
}

func (w *Widget) recalc() {
	res, err := w.calc.Calculate(w.cfg.GetFW(), w.opts)
	if err != nil {
		w.summary.SetText("Error: " + err.Error())
		return
	}
	if len(w.pulls) > 0 {
		e85, eff := w.opts["e85"] == 1, effFactor(w.opts)
		if w.pullIdx >= 0 && w.pullIdx < len(w.pulls) {
			res.Curves = append(res.Curves, w.pulls[w.pullIdx].curves(w.logPrm, res.RPM, e85, eff)...)
		} else {
			res.Curves = append(res.Curves, averagePullCurves(w.pulls, w.logPrm, res.RPM, e85, eff)...)
		}
	}
	series := make([]graphSeries, len(res.Curves))
	names := make([]string, len(res.Curves))
	for i, c := range res.Curves {
		vis, seen := w.visible[c.Name]
		if !seen {
			vis = !c.Hidden
			w.visible[c.Name] = vis
		}
		series[i] = graphSeries{
			name:    c.Name,
			col:     seriesColors[i%len(seriesColors)],
			values:  c.Values,
			visible: vis,
		}
		names[i] = c.Name
	}
	w.graph.SetData(res.RPM, series)
	w.table.SetData(res.Table)
	w.rebuildLegend(series, names)
	w.summary.SetText(summarize(res))
}

func (w *Widget) rebuildLegend(series []graphSeries, names []string) {
	if strings.Join(names, "|") == strings.Join(w.legendNames, "|") {
		return
	}
	w.legendNames = names
	objs := make([]fyne.CanvasObject, 0, len(series))
	for _, s := range series {
		swatch := canvas.NewRectangle(s.col)
		swatch.SetMinSize(fyne.NewSize(12, 12))
		swatch.CornerRadius = 2
		name := s.name
		chk := widget.NewCheck(name, func(v bool) {
			w.visible[name] = v
			w.graph.SetSeriesVisible(name, v)
		})
		chk.Checked = s.visible
		objs = append(objs, container.NewHBox(chk, container.NewCenter(swatch)))
	}
	w.legend.Objects = objs
	w.legend.Refresh()
}

func summarize(res *Result) string {
	var parts []string
	for _, c := range res.Curves {
		if !c.Peak || len(c.Values) == 0 {
			continue
		}
		best := -1
		for i, v := range c.Values {
			if math.IsNaN(v) {
				continue
			}
			if best < 0 || v > c.Values[best] {
				best = i
			}
		}
		if best < 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("peak %s %.0f @ %.0f rpm", c.Name, c.Values[best], res.RPM[best]))
	}
	if len(res.Limiters) == len(res.RPM) {
		type span struct {
			name     string
			from, to float64
			lastIdx  int
		}
		var spans []*span
		for i, l := range res.Limiters {
			if l == "" {
				continue
			}
			if len(spans) > 0 {
				if last := spans[len(spans)-1]; last.name == l && last.lastIdx == i-1 {
					last.to = res.RPM[i]
					last.lastIdx = i
					continue
				}
			}
			spans = append(spans, &span{name: l, from: res.RPM[i], to: res.RPM[i], lastIdx: i})
		}
		for _, s := range spans {
			if s.from == s.to {
				parts = append(parts, fmt.Sprintf("%s @ %.0f rpm", s.name, s.from))
			} else {
				parts = append(parts, fmt.Sprintf("%s %.0f-%.0f rpm", s.name, s.from, s.to))
			}
		}
	}
	return strings.Join(parts, " · ")
}
