package symbollist

import (
	"image/color"
	"sort"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/eventbus"
	"github.com/roffe/txlogger/pkg/widgets"
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
	EBus    *eventbus.Controller
	Symbols []*symbol.Symbol
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

func (s *Widget) UpdateBars(enabled bool) {
	s.updateBars = enabled
}

func (s *Widget) Names() []string {
	names := make([]string, len(s.cfg.Symbols)+1)
	for i, s := range s.cfg.Symbols {
		names[i] = s.Name
	}
	names[len(names)-1] = datalogger.EXTERNALWBLSYM
	sort.Strings(names)
	return names
}

func (s *Widget) SetValue(name string, value float64) {
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
			factor := float32((value - val.min) / (val.max - val.min))
			col := widgets.GetColorInterpolation(val.min, val.max, value)
			col.A = 30
			val.valueBar.FillColor = col
			val.valueBar.Resize(fyne.Size{Width: factor * 100, Height: 26})
		}
		prec := symbol.GetPrecision(val.symbol.Correctionfactor)
		textValue := strconv.FormatFloat(value, 'f', prec, 64)
		val.symbolValue.SetText(textValue)
	}
}

func (s *Widget) Disable() {
	for _, e := range s.entries {
		e.symbolCorrectionfactor.Disable()
		e.deleteBTN.Disable()
	}
}

func (s *Widget) Enable() {
	for _, e := range s.entries {
		e.symbolCorrectionfactor.Enable()
		e.deleteBTN.Enable()
	}
}

func (s *Widget) Add(symbols ...*symbol.Symbol) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sym := range symbols {
		if _, found := s.entryMap[sym.Name]; found {
			continue
		}

		cancel := s.cfg.EBus.SubscribeFunc(sym.Name, func(value float64) {
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
		entry := NewSymbolWidgetEntry(sym, deleteFunc)
		s.cfg.Symbols = append(s.cfg.Symbols, sym)
		s.entries = append(s.entries, entry)
		s.container.Add(entry)
		s.entryMap[sym.Name] = entry
	}
}

func (s *Widget) Clear() {
	for _, e := range s.entries {
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
	return fyne.Size{Width: 480, Height: 221}
}

func (s *Widget) CreateRenderer() fyne.WidgetRenderer {
	name := widget.NewLabel("Name")
	name.TextStyle = fyne.TextStyle{Bold: true}

	value := widget.NewLabel("Value")
	value.TextStyle = fyne.TextStyle{Bold: true}

	num := widget.NewLabel("#")
	num.TextStyle = fyne.TextStyle{Bold: true}

	typ := widget.NewLabel("Type")
	typ.TextStyle = fyne.TextStyle{Bold: true}

	factor := widget.NewLabel("Factor")
	factor.TextStyle = fyne.TextStyle{Bold: true}

	header := container.New(&headerLayout{}, widget.NewLabel(""), name, value, num /* typ,*/, factor, widget.NewLabel(""))

	return widget.NewSimpleRenderer(container.NewBorder(
		header,
		nil,
		nil,
		nil,
		s.scroll,
	))
}

type headerLayout struct {
	oldSize fyne.Size
}

func (h *headerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if h.oldSize == size {
		return
	}
	h.oldSize = size

	var x float32
	padd := size.Width * ((1.0 - sumFloat32(sz)) / float32(len(sz)))
	for i, o := range objects {
		o.Resize(fyne.NewSize(size.Width*sz[i], size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += o.Size().Width + padd
	}

}

func (h *headerLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 30)
}

type SymbolWidgetEntry struct {
	widget.BaseWidget
	symbol       *symbol.Symbol
	copyName     *widget.Button
	symbolName   *widget.Label
	symbolValue  *widget.Label
	symbolNumber *widget.Label
	//symbolType             *widget.Label
	symbolCorrectionfactor *widget.Entry
	deleteBTN              *widget.Button
	valueBar               *canvas.Rectangle

	deleteFunc func(*SymbolWidgetEntry)

	//valueSet bool
	value    float64
	min, max float64

	oldSize fyne.Size
}

func NewSymbolWidgetEntry(sym *symbol.Symbol, deleteFunc func(*SymbolWidgetEntry)) *SymbolWidgetEntry {
	sw := &SymbolWidgetEntry{
		symbol:     sym,
		deleteFunc: deleteFunc,
	}
	sw.ExtendBaseWidget(sw)
	sw.copyName = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(sym.Name)
		//fyne.CurrentApp().Driver().AllWindows()[0].Clipboard().SetContent(sym.Name)
	})
	sw.symbolName = widget.NewLabel(sw.symbol.Name)
	sw.symbolValue = widget.NewLabel("---")
	sw.symbolNumber = widget.NewLabel(strconv.Itoa(sw.symbol.Number))
	//sw.symbolType = widget.NewLabel(fmt.Sprintf("%02X", sw.symbol.Type))
	sw.symbolCorrectionfactor = widget.NewEntry()

	sw.symbolCorrectionfactor.OnChanged = func(s string) {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return
		}
		sw.symbol.Correctionfactor = f
	}

	sw.SetCorrectionFactor(sym.Correctionfactor)

	sw.deleteBTN = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		if sw.deleteFunc != nil {
			sw.deleteFunc(sw)
		}
	})

	sw.valueBar = canvas.NewRectangle(color.RGBA{255, 0, 0, 255})

	return sw
}

//func (sw *SymbolWidgetEntry) MouseMoved(event *desktop.MouseEvent) {
//}
//
//func (sw *SymbolWidgetEntry) MouseOut() {
//}
//
//func (sw *SymbolWidgetEntry) MouseIn(event *desktop.MouseEvent) {
//}

func (sw *SymbolWidgetEntry) SetCorrectionFactor(f float64) {
	sw.symbol.Correctionfactor = f
	switch f {
	case 1:
		sw.symbolCorrectionfactor.SetText(strconv.FormatFloat(f, 'f', 0, 64))
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

func (sw *SymbolWidgetEntry) CreateRenderer() fyne.WidgetRenderer {
	return &symbolWidgetEntryRenderer{sw}
}

var sz = []float32{
	.05, // copy
	.32, // name
	.20, // value
	.12, // number
	.14, // correctionfactor
	//.07, // type
	.06, // deletebtn
}

func sumFloat32(a []float32) float32 {
	var sum float32
	for _, v := range a {
		sum += v
	}
	return sum
}

type symbolWidgetEntryRenderer struct {
	e *SymbolWidgetEntry
}

func (s *symbolWidgetEntryRenderer) Destroy() {
}

func (s *symbolWidgetEntryRenderer) Layout(size fyne.Size) {
	if s.e.oldSize == size {
		return
	}
	s.e.oldSize = size
	padd := size.Width * ((1.0 - sumFloat32(sz)) / float32(len(sz)))
	s.e.copyName.Resize(fyne.NewSize(size.Width*sz[0], size.Height))
	s.e.symbolName.Resize(fyne.NewSize(size.Width*sz[1], size.Height))
	s.e.symbolValue.Resize(fyne.NewSize(size.Width*sz[2], size.Height))
	s.e.symbolNumber.Resize(fyne.NewSize(size.Width*sz[3], size.Height))
	//s.symbolType.Resize(fyne.NewSize(size.Width*sz[4], size.Height))
	s.e.symbolCorrectionfactor.Resize(fyne.NewSize(size.Width*sz[4], size.Height))
	s.e.deleteBTN.Resize(fyne.NewSize(size.Width*sz[5], size.Height))

	var x float32

	s.e.copyName.Move(fyne.NewPos(x, 0))
	x += s.e.copyName.Size().Width + padd

	s.e.symbolName.Move(fyne.NewPos(x, 0))
	x += s.e.symbolName.Size().Width + padd

	s.e.symbolValue.Move(fyne.NewPos(x, 0))
	s.e.valueBar.Move(fyne.NewPos(x, 6))
	x += s.e.symbolValue.Size().Width + padd

	s.e.symbolNumber.Move(fyne.NewPos(x, 0))
	x += s.e.symbolNumber.Size().Width + padd

	// s.symbolType.Move(fyne.NewPos(x, 0))
	// x += s.symbolType.Size().Width + padd

	s.e.symbolCorrectionfactor.Move(fyne.NewPos(x, 0))
	x += s.e.symbolCorrectionfactor.Size().Width + padd

	s.e.deleteBTN.Move(fyne.NewPos(x, 0))
}

func (s *symbolWidgetEntryRenderer) MinSize() fyne.Size {
	return fyne.NewSize(400, 36)
}

func (s *symbolWidgetEntryRenderer) Refresh() {
	s.e.copyName.Refresh()
	s.e.symbolName.Refresh()
	s.e.symbolValue.Refresh()
	s.e.symbolNumber.Refresh()
	s.e.symbolCorrectionfactor.Refresh()
}

func (s *symbolWidgetEntryRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		s.e.copyName,
		s.e.valueBar,
		s.e.symbolName,
		s.e.symbolValue,
		s.e.symbolNumber,
		//s.symbolType,
		s.e.symbolCorrectionfactor,
		s.e.deleteBTN,
	}
}
