package symbollist

import (
	"image/color"
	"math"
	"slices"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	xlayout "github.com/roffe/txlogger/pkg/layout"
)

const (
	barAlpha uint8 = 80
)

type Widget struct {
	widget.BaseWidget
	cfg        *Config
	entryMap   map[string]*SymbolWidgetEntry
	entries    []*SymbolWidgetEntry
	container  *fyne.Container
	scroll     *container.Scroll
	updateBars bool
	subs       map[string]func()
	mu         sync.Mutex
}

type Config struct {
	// EBus           *eventbus.Controller
	Symbols        []*symbol.Symbol
	ColorBlindMode colors.ColorBlindMode
}

func New(cfg *Config) *Widget {
	sl := &Widget{
		cfg:      cfg,
		entryMap: make(map[string]*SymbolWidgetEntry),
		subs:     make(map[string]func()),
	}
	sl.ExtendBaseWidget(sl)
	sl.render()
	sl.LoadSymbols(cfg.Symbols...)
	return sl
}

func (s *Widget) render() {
	s.container = container.NewVBox()
	s.scroll = container.NewVScroll(s.container)
}

func (s *Widget) SetColorBlindMode(mode colors.ColorBlindMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.ColorBlindMode = mode
}

func (s *Widget) UpdateBars(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateBars = enabled
}

func (s *Widget) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.cfg.Symbols)+1)
	hasWBL := false
	for _, sym := range s.cfg.Symbols {
		if sym.Name == datalogger.EXTERNALWBLSYM {
			hasWBL = true
		}
		names = append(names, sym.Name)
	}
	if !hasWBL {
		names = append(names, datalogger.EXTERNALWBLSYM)
	}
	slices.SortFunc(names, compareFold)
	return names
}

// compareFold orders ASCII strings case-insensitively without the per-comparison
// allocations of strings.ToLower.
func compareFold(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return int(ca) - int(cb)
		}
	}
	return len(a) - len(b)
}

func (s *Widget) SetValue(name string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, found := s.entryMap[name]
	if found {
		if value == val.value {
			return
		}
		val.value = value
		if value < val.min {
			val.min = value
		} else if value > val.max {
			val.max = value
		}
		if s.updateBars {
			if span := val.max - val.min; span > 0 {
				val.valueBarFactor = float32((value - val.min) / span)
			} else {
				val.valueBarFactor = 0
			}
			col := colors.GetColorInterpolation(val.min, val.max, value, s.cfg.ColorBlindMode)
			col.A = barAlpha
			val.valueBar.FillColor = col
			totalWidth := val.symbolName.Size().Width
			val.valueBar.Resize(fyne.Size{Width: val.valueBarFactor * totalWidth, Height: 26})
		}
		val.buf = strconv.AppendFloat(val.buf[:0], value, 'f', val.prec, 64)
		if string(val.buf) != val.lastText {
			val.lastText = string(val.buf)
			val.symbolValue.SetText(val.lastText)
		}
	}
}

func (s *Widget) Disable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		// e.symbolCorrectionfactor.Disable()
		e.deleteBTN.Disable()
	}
}

func (s *Widget) Enable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		// e.symbolCorrectionfactor.Enable()
		e.deleteBTN.Enable()
	}
}

func (s *Widget) Add(symbols ...*symbol.Symbol) {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := false
	for _, sym := range symbols {
		if _, found := s.entryMap[sym.Name]; found {
			continue
		}

		cancel := ebus.SubscribeFunc(sym.Name, func(value float64) {
			s.SetValue(sym.Name, value)
		})
		s.subs[sym.Name] = cancel

		deleteFunc := func(sw *SymbolWidgetEntry) {
			s.mu.Lock()
			defer s.mu.Unlock()
			for i, e := range s.entries {
				if e == sw {
					s.cfg.Symbols = append(s.cfg.Symbols[:i], s.cfg.Symbols[i+1:]...)
					s.entries = append(s.entries[:i], s.entries[i+1:]...)
					delete(s.entryMap, sw.symbol.Name)
					if cancel, found := s.subs[sw.symbol.Name]; found {
						cancel()
						delete(s.subs, sw.symbol.Name)
					}
					s.container.Remove(sw)
					break
				}
			}
		}
		entry := s.newSymbolWidgetEntry(sym, deleteFunc)
		s.cfg.Symbols = append(s.cfg.Symbols, sym)
		s.entries = append(s.entries, entry)
		// append directly instead of container.Add to avoid a layout+refresh per entry
		s.container.Objects = append(s.container.Objects, entry)
		s.entryMap[sym.Name] = entry
		added = true
	}
	if added {
		s.container.Refresh()
	}
}

func (s *Widget) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		// NaN sentinel so the next sample always renders, even if it equals
		// the last value seen before the clear
		e.value = math.NaN()
		e.min = 0
		e.max = 0
		e.valueBarFactor = 0
		e.lastText = "---"
		e.symbolValue.SetText("---")
	}
}

func (s *Widget) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.container.RemoveAll()
	s.cfg.Symbols = s.cfg.Symbols[:0]
	s.entries = s.entries[:0]
	for _, cancel := range s.subs {
		cancel()
	}
	clear(s.entryMap)
	clear(s.subs)
}

func (s *Widget) LoadSymbols(symbols ...*symbol.Symbol) {
	s.clear()
	s.Add(symbols...)
}

func (s *Widget) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cfg.Symbols)
}

func (s *Widget) Symbols() []*symbol.Symbol {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*symbol.Symbol, len(s.cfg.Symbols))
	copy(out, s.cfg.Symbols)
	return out
}

func (s *Widget) MinSize() fyne.Size {
	return fyne.Size{Width: 280, Height: 221}
}

var headerSizes = []float64{.70, .20, .10}

func (s *Widget) CreateRenderer() fyne.WidgetRenderer {
	name := widget.NewLabel("Name")
	name.TextStyle = fyne.TextStyle{Bold: true}

	value := widget.NewLabel("Value")
	value.TextStyle = fyne.TextStyle{Bold: true}

	// num := widget.NewLabel("#")
	// num.TextStyle = fyne.TextStyle{Bold: true}

	// typ := widget.NewLabel("Type")
	// typ.TextStyle = fyne.TextStyle{Bold: true}

	// factor := widget.NewLabel("Factor")
	// factor.TextStyle = fyne.TextStyle{Bold: true}

	customLayout := xlayout.NewHPortion(headerSizes)
	// header := container.New(ll, name, value, num /* typ,*/, factor, widget.NewLabel(""))
	header := container.New(customLayout, name, value, widget.NewLabel(""))

	return widget.NewSimpleRenderer(container.NewBorder(
		header,
		nil,
		nil,
		nil,
		s.scroll,
	))
}

func (s *Widget) newSymbolWidgetEntry(sym *symbol.Symbol, deleteFunc func(*SymbolWidgetEntry)) *SymbolWidgetEntry {
	sw := &SymbolWidgetEntry{
		w:          s,
		symbol:     sym,
		prec:       symbol.GetPrecision(sym.Correctionfactor),
		deleteFunc: deleteFunc,
		// NaN compares unequal to everything, so the first sample always
		// renders — including an initial value of exactly 0
		value: math.NaN(),
	}
	sw.ExtendBaseWidget(sw)
	sw.symbolName = widget.NewLabel(sw.symbol.Name)
	sw.symbolName.Selectable = true
	sw.symbolValue = widget.NewLabel("---")
	/*
		sw.symbolNumber = widget.NewLabel(strconv.Itoa(sw.symbol.Number))
		sw.symbolCorrectionfactor = widget.NewEntry()
		sw.symbolCorrectionfactor.OnChanged = func(s string) {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return
			}
			sw.symbol.Correctionfactor = f
		}
	*/

	// sw.SetCorrectionFactor(sym.Correctionfactor)

	sw.deleteBTN = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		if sw.deleteFunc != nil {
			sw.deleteFunc(sw)
		}
	})

	sw.valueBar = canvas.NewRectangle(color.RGBA{0, 0, 0, 0})
	sw.valueBar.BottomRightCornerRadius = 4
	sw.valueBar.TopRightCornerRadius = 4

	layout := xlayout.NewHPortion(headerSizes)
	sw.body = container.New(layout,
		sw.symbolName,
		sw.symbolValue,
		// sw.symbolNumber,
		// sw.symbolCorrectionfactor,
		sw.deleteBTN,
	)
	sw.container = container.NewStack(
		container.NewWithoutLayout(sw.valueBar),
		sw.body,
	)

	return sw
}

type SymbolWidgetEntry struct {
	widget.BaseWidget

	w *Widget

	symbol      *symbol.Symbol
	symbolName  *widget.Label
	symbolValue *widget.Label
	// symbolNumber           *widget.Label
	// symbolCorrectionfactor *widget.Entry
	deleteBTN      *widget.Button
	valueBar       *canvas.Rectangle
	valueBarFactor float32

	deleteFunc func(*SymbolWidgetEntry)

	value    float64
	min, max float64
	prec     int
	lastText string
	buf      []byte // scratch for AppendFloat, avoids an allocation per update

	oldSize fyne.Size

	body      *fyne.Container
	container *fyne.Container
}

/*
func (sw *SymbolWidgetEntry) SetCorrectionFactor(f float64) {
	sw.symbol.Correctionfactor = f
	switch f {
	case 1:
		sw.symbolCorrectionfactor.SetText(strconv.Itoa(int(f)))
	case 0.1:
		sw.symbolCorrectionfactor.SetText(strconv.FormatFloat(f, 'f', 1, 64))
	case 0.01:
		sw.symbolCorrectionfactor.SetText(strconv.FormatFloat(f, 'f', 2, 64))
	case 0.001:
		sw.symbolCorrectionfactor.SetText(strconv.FormatFloat(f, 'f', 3, 64))
	default:
		sw.symbolCorrectionfactor.SetText(strconv.FormatFloat(f, 'f', 4, 64))
	}
}
*/

func (sw *SymbolWidgetEntry) CreateRenderer() fyne.WidgetRenderer {
	return &symbolWidgetEntryRenderer{
		e:       sw,
		objects: []fyne.CanvasObject{sw.container},
	}
	// return widget.NewSimpleRenderer(sw.container)
}

type symbolWidgetEntryRenderer struct {
	e       *SymbolWidgetEntry
	objects []fyne.CanvasObject
}

func (s *symbolWidgetEntryRenderer) Destroy() {
}

func (s *symbolWidgetEntryRenderer) Layout(size fyne.Size) {
	if s.e.oldSize != size {
		s.e.oldSize = size
		s.e.container.Resize(size)
		s.e.valueBar.Move(fyne.NewPos(0, 6))
		s.e.valueBar.Resize(fyne.Size{Width: s.e.valueBarFactor * s.e.symbolName.Size().Width, Height: 26})
	}
}

func (s *symbolWidgetEntryRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 36)
}

func (s *symbolWidgetEntryRenderer) Refresh() {
	col := colors.GetColorInterpolation(s.e.min, s.e.max, s.e.value, s.e.w.cfg.ColorBlindMode)
	col.A = barAlpha
	s.e.valueBar.FillColor = col
	s.e.valueBar.StrokeColor = col
	s.e.container.Refresh()
}

func (s *symbolWidgetEntryRenderer) Objects() []fyne.CanvasObject {
	return s.objects
}
