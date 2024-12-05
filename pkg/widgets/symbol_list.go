package widgets

import (
	"fmt"
	"image/color"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
)

type SymbolListWidget struct {
	widget.BaseWidget
	symbols    []*symbol.Symbol
	entryMap   map[string]*SymbolWidgetEntry
	entries    []*SymbolWidgetEntry
	container  *fyne.Container
	border     *fyne.Container
	scroll     *container.Scroll
	mu         sync.Mutex
	updateBars bool
	onUpdate   func([]*symbol.Symbol)
	w          fyne.Window
}

func NewSymbolListWidget(w fyne.Window, updateFunc func([]*symbol.Symbol), symbols ...*symbol.Symbol) *SymbolListWidget {
	sl := &SymbolListWidget{
		entryMap: make(map[string]*SymbolWidgetEntry),
		onUpdate: updateFunc,
		w:        w,
	}
	sl.ExtendBaseWidget(sl)
	sl.render()
	sl.LoadSymbols(symbols...)
	return sl
}

func (s *SymbolListWidget) render() {
	s.container = container.NewVBox()
	s.scroll = container.NewVScroll(s.container)

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

	s.border = container.NewBorder(
		container.NewGridWithColumns(7,
			name,
			widget.NewLabel(""),
			widget.NewLabel(""),
			value,
			num,
			typ,
			factor,
		),
		nil,
		nil,
		nil,
		s.scroll,
	)
}

func (s *SymbolListWidget) UpdateBars(enabled bool) {
	s.updateBars = enabled
}

func (s *SymbolListWidget) SetValue(name string, value float64) {
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
			col := GetColorInterpolation(val.min, val.max, value)
			col.A = 30
			val.valueBar.FillColor = col
			val.valueBar.Resize(fyne.NewSize(factor*100, 26))
		}
		switch val.symbol.Correctionfactor {
		case 1:
			val.symbolValue.SetText(strconv.FormatFloat(value, 'f', 0, 64))
			return
		case 0.1:
			val.symbolValue.SetText(strconv.FormatFloat(value, 'f', 1, 64))
			return
		case 0.01:
			val.symbolValue.SetText(strconv.FormatFloat(value, 'f', 2, 64))
			return
		case 0.001:
			val.symbolValue.SetText(strconv.FormatFloat(value, 'f', 3, 64))
			return
		default:
			val.symbolValue.SetText(strconv.FormatFloat(value, 'f', 2, 64))
			return
		}
	}
}

func (s *SymbolListWidget) Disable() {
	for _, e := range s.entries {
		e.symbolCorrectionfactor.Disable()
		e.deleteBTN.Disable()
	}
}

func (s *SymbolListWidget) Enable() {
	for _, e := range s.entries {
		e.symbolCorrectionfactor.Enable()
		e.deleteBTN.Enable()
	}
}

func (s *SymbolListWidget) Add(symbols ...*symbol.Symbol) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sym := range symbols {
		if _, found := s.entryMap[sym.Name]; found {
			continue
		}
		deleteFunc := func(sw *SymbolWidgetEntry) {
			s.mu.Lock()
			defer s.mu.Unlock()
			for i, e := range s.entries {
				if e == sw {
					s.symbols = append(s.symbols[:i], s.symbols[i+1:]...)
					s.entries = append(s.entries[:i], s.entries[i+1:]...)
					delete(s.entryMap, sw.symbol.Name)
					s.container.Remove(sw)
					s.onUpdate(s.symbols)
					break
				}
			}
		}
		entry := NewSymbolWidgetEntry(sym, deleteFunc)
		s.symbols = append(s.symbols, sym)
		s.entries = append(s.entries, entry)
		s.container.Add(entry)
		s.entryMap[sym.Name] = entry
	}
	s.onUpdate(s.symbols)
}

func (s *SymbolListWidget) Clear() {
	for _, e := range s.entries {
		e.symbolValue.SetText("---")
	}
}

func (s *SymbolListWidget) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.container.RemoveAll()
	s.symbols = []*symbol.Symbol{}
	s.entries = []*SymbolWidgetEntry{}
	s.entryMap = make(map[string]*SymbolWidgetEntry)
	s.onUpdate(s.symbols)
}

func (s *SymbolListWidget) LoadSymbols(symbols ...*symbol.Symbol) {
	s.clear()
	s.Add(symbols...)
}

func (s *SymbolListWidget) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.symbols)
}

func (s *SymbolListWidget) Symbols() []*symbol.Symbol {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*symbol.Symbol, len(s.symbols))
	copy(out, s.symbols)
	return out
}

func (s *SymbolListWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.border)
}

type SymbolWidgetEntry struct {
	widget.BaseWidget
	symbol                 *symbol.Symbol
	copyName               *widget.Button
	symbolName             *widget.Label
	symbolValue            *widget.Label
	symbolNumber           *widget.Label
	symbolType             *widget.Label
	symbolCorrectionfactor *widget.Entry
	deleteBTN              *widget.Button
	valueBar               *canvas.Rectangle

	container *fyne.Container

	deleteFunc func(*SymbolWidgetEntry)

	//valueSet bool
	value    float64
	min, max float64
}

func NewSymbolWidgetEntry(sym *symbol.Symbol, deleteFunc func(*SymbolWidgetEntry)) *SymbolWidgetEntry {
	sw := &SymbolWidgetEntry{
		symbol:     sym,
		deleteFunc: deleteFunc,
	}
	sw.ExtendBaseWidget(sw)
	sw.copyName = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(sym.Name)
	})
	sw.symbolName = widget.NewLabel(sw.symbol.Name)
	sw.symbolValue = widget.NewLabel("---")
	sw.symbolNumber = widget.NewLabel(strconv.Itoa(sw.symbol.Number))
	sw.symbolType = widget.NewLabel(fmt.Sprintf("%02X", sw.symbol.Type))
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

	sw.container = container.NewWithoutLayout(
		sw.copyName,
		sw.valueBar,
		sw.symbolName,
		sw.symbolValue,
		sw.symbolNumber,
		sw.symbolType,
		sw.symbolCorrectionfactor,
		sw.deleteBTN,
	)
	return sw
}

func (sw *SymbolWidgetEntry) MouseMoved(event *desktop.MouseEvent) {
}

func (sw *SymbolWidgetEntry) MouseOut() {
}

func (sw *SymbolWidgetEntry) MouseIn(event *desktop.MouseEvent) {
}

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

func (sw *SymbolWidgetEntry) MinSize() fyne.Size {
	return fyne.NewSize(300, 38)
}

func (sw *SymbolWidgetEntry) Resize(size fyne.Size) {
	padd := size.Width * ((1.0 - sumFloat32(sz)) / float32(len(sz)))
	sw.copyName.Resize(fyne.NewSize(size.Width*sz[0], size.Height))
	sw.symbolName.Resize(fyne.NewSize(size.Width*sz[1], size.Height))
	sw.symbolValue.Resize(fyne.NewSize(size.Width*sz[2], size.Height))
	sw.symbolNumber.Resize(fyne.NewSize(size.Width*sz[3], size.Height))
	sw.symbolType.Resize(fyne.NewSize(size.Width*sz[4], size.Height))
	sw.symbolCorrectionfactor.Resize(fyne.NewSize(size.Width*sz[5], size.Height))
	sw.deleteBTN.Resize(fyne.NewSize(size.Width*sz[6], size.Height))

	var x float32

	sw.copyName.Move(fyne.NewPos(x, 0))
	x += sw.copyName.Size().Width + padd

	sw.symbolName.Move(fyne.NewPos(x, 0))
	x += sw.symbolName.Size().Width + padd

	sw.symbolValue.Move(fyne.NewPos(x, 0))
	sw.valueBar.Move(fyne.NewPos(x, 6))
	x += sw.symbolValue.Size().Width + padd

	sw.symbolNumber.Move(fyne.NewPos(x, 0))
	x += sw.symbolNumber.Size().Width + padd

	sw.symbolType.Move(fyne.NewPos(x, 0))
	x += sw.symbolType.Size().Width + padd

	sw.symbolCorrectionfactor.Move(fyne.NewPos(x, 0))
	x += sw.symbolCorrectionfactor.Size().Width + padd

	sw.deleteBTN.Move(fyne.NewPos(x, 0))
}

func (sw *SymbolWidgetEntry) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(sw.container)
}

var sz = []float32{
	.04, // copy
	.32, // name
	.18, // value
	.12, // number
	.14, // correctionfactor
	.07, // type
	.04, // deletebtn
}

func sumFloat32(a []float32) float32 {
	var sum float32
	for _, v := range a {
		sum += v
	}
	return sum
}
