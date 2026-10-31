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
	// mu guards cfg, entryMap, entries, subs, updateBars and the entries'
	// value state. SetValue takes it on publisher goroutines; never take it
	// in anything fyne.Do runs or in the renderers.
	mu sync.Mutex
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

// SetColorBlindMode recolours the value bars. It may be called from any
// goroutine; the bars are redrawn through fyne.Do.
func (s *Widget) SetColorBlindMode(mode colors.ColorBlindMode) {
	s.mu.Lock()
	defer s.mu.Unlock() // held across fyne.Do so updates reach the UI in order
	s.cfg.ColorBlindMode = mode
	if !s.updateBars {
		return // UpdateBars colours them when they come back on
	}
	for _, e := range s.entries {
		factor, col := e.bar(mode)
		fyne.Do(func() { e.setBar(factor, col) })
	}
}

// UpdateBars turns the value bars on or off. It may be called from any
// goroutine; the bars are redrawn through fyne.Do, in order with SetValue's
// updates so an older one still queued can't redraw a bar after this.
func (s *Widget) UpdateBars(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateBars = enabled
	for _, e := range s.entries {
		// SetValue stops touching the bars when they're off, so shrink the
		// ones already drawn away instead of leaving them frozen at their
		// last value
		var factor float32
		var col color.RGBA
		if enabled {
			factor, col = e.bar(s.cfg.ColorBlindMode)
		}
		fyne.Do(func() { e.setBar(factor, col) })
	}
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

// SetValue may be called from any goroutine. The min/max, the bar and the text
// are worked out here; only the canvas update goes through fyne.Do.
func (s *Widget) SetValue(name string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock() // held across fyne.Do so updates reach the UI in order
	val, found := s.entryMap[name]
	if !found || value == val.value {
		return
	}
	val.value = value
	if value < val.min {
		val.min = value
	} else if value > val.max {
		val.max = value
	}
	bars := s.updateBars
	var factor float32
	var col color.RGBA
	if bars {
		factor, col = val.bar(s.cfg.ColorBlindMode)
	}
	var text string // stays empty when the formatted value didn't change
	val.buf = strconv.AppendFloat(val.buf[:0], value, 'f', val.prec, 64)
	if string(val.buf) != val.lastText {
		val.lastText = string(val.buf)
		text = val.lastText
	}
	if !bars && text == "" {
		return
	}
	fyne.Do(func() {
		if bars {
			val.setBar(factor, col)
		}
		if text != "" {
			val.symbolValue.SetText(text)
		}
	})
}

// Disable must be called on the UI goroutine.
func (s *Widget) Disable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		// e.symbolCorrectionfactor.Disable()
		e.deleteBTN.Disable()
	}
}

// Enable must be called on the UI goroutine.
func (s *Widget) Enable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		// e.symbolCorrectionfactor.Enable()
		e.deleteBTN.Enable()
	}
}

// Add lists and subscribes the symbols not listed yet. It must be called on
// the UI goroutine.
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

// Clear resets every value to "---". It may be called from any goroutine.
func (s *Widget) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock() // held across fyne.Do so updates reach the UI in order
	for _, e := range s.entries {
		// NaN sentinel so the next sample always renders, even if it equals
		// the last value seen before the clear
		e.value = math.NaN()
		e.min = 0
		e.max = 0
		e.lastText = "---"
		fyne.Do(func() {
			e.setBar(0, color.RGBA{})
			e.symbolValue.SetText("---")
		})
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

// LoadSymbols replaces the listed symbols. It must be called on the UI
// goroutine.
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

	symbol      *symbol.Symbol
	symbolName  *widget.Label
	symbolValue *widget.Label
	// symbolNumber           *widget.Label
	// symbolCorrectionfactor *widget.Entry
	deleteBTN      *widget.Button
	valueBar       *canvas.Rectangle
	valueBarFactor float32 // bar width relative to the name column, UI goroutine only

	deleteFunc func(*SymbolWidgetEntry)

	// SetValue side, guarded by Widget.mu
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

// bar works out the value bar's size factor and colour from the entry's
// current value relative to the min and max seen so far. Call with
// Widget.mu held.
func (sw *SymbolWidgetEntry) bar(mode colors.ColorBlindMode) (float32, color.RGBA) {
	var factor float32
	if span := sw.max - sw.min; span > 0 {
		factor = float32((sw.value - sw.min) / span)
	}
	col := colors.GetColorInterpolation(sw.min, sw.max, sw.value, mode)
	col.A = barAlpha
	return factor, col
}

// setBar draws the value bar. UI goroutine only.
func (sw *SymbolWidgetEntry) setBar(factor float32, col color.RGBA) {
	sw.valueBarFactor = factor
	recolor := sw.valueBar.FillColor != col
	sw.valueBar.FillColor = col
	if size := (fyne.Size{Width: factor * sw.symbolName.Size().Width, Height: 26}); size != sw.valueBar.Size() {
		sw.valueBar.Resize(size) // refreshes
	} else if recolor {
		sw.valueBar.Refresh()
	}
}

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

// Refresh leaves the bar's colour to setBar: the value state it comes from
// belongs to SetValue's side.
func (s *symbolWidgetEntryRenderer) Refresh() {
	s.e.container.Refresh()
}

func (s *symbolWidgetEntryRenderer) Objects() []fyne.CanvasObject {
	return s.objects
}
