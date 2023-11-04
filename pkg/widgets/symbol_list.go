package widgets

import (
	"fmt"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/symbol"
)

type SymbolListWidget struct {
	widget.BaseWidget
	symbols   []*symbol.Symbol
	entryMap  map[string]*SymbolWidgetEntry
	entrys    []*SymbolWidgetEntry
	container *fyne.Container
	scroll    fyne.CanvasObject
	mu        sync.Mutex
	border    *fyne.Container
}

func NewSymbolListWidget(symbols ...*symbol.Symbol) *SymbolListWidget {
	sl := &SymbolListWidget{
		entryMap: make(map[string]*SymbolWidgetEntry),
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
		container.New(&layout.RatioContainer{Widths: sz},
			name,
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

func (s *SymbolListWidget) SetValue(name string, value float64) {
	val, found := s.entryMap[name]
	if found {
		f := val.symbol.Correctionfactor
		switch f {
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
	for _, e := range s.entrys {
		e.symbolName.Disable()
		e.symbolCorrectionfactor.Disable()
		e.deleteBTN.Disable()
	}
}

func (s *SymbolListWidget) Enable() {
	for _, e := range s.entrys {
		e.symbolName.Enable()
		e.symbolCorrectionfactor.Enable()
		e.deleteBTN.Enable()
	}
}

func (s *SymbolListWidget) Add(symbols ...*symbol.Symbol) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.symbols = append(s.symbols, symbols...)
	for _, sym := range symbols {
		if _, found := s.entryMap[sym.Name]; found {
			continue
		}
		entry := NewSymbolWidgetEntry(sym, func(sw *SymbolWidgetEntry) {
			for i, e := range s.entrys {
				if e == sw {
					s.mu.Lock()
					defer s.mu.Unlock()
					s.symbols = append(s.symbols[:i], s.symbols[i+1:]...)
					s.entrys = append(s.entrys[:i], s.entrys[i+1:]...)
					delete(s.entryMap, sw.symbol.Name)
					s.container.Remove(sw)
					s.scroll.Refresh()
					break
				}
			}
			//canvas := fyne.CurrentApp().Driver().CanvasForObject(entry)
			//canvas.Refresh(entry)
		})
		s.entrys = append(s.entrys, entry)
		s.container.Add(entry)
		s.entryMap[sym.Name] = entry
	}
	s.border.Refresh()
}

func (s *SymbolListWidget) LoadSymbols(symbols ...*symbol.Symbol) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.symbols = symbols
	s.entrys = []*SymbolWidgetEntry{}
	s.container.RemoveAll()
	for _, sym := range s.symbols {
		entry := NewSymbolWidgetEntry(sym, func(sw *SymbolWidgetEntry) {
			for i, e := range s.entrys {
				if e == sw {
					s.mu.Lock()
					defer s.mu.Unlock()
					s.symbols = append(s.symbols[:i], s.symbols[i+1:]...)
					s.entrys = append(s.entrys[:i], s.entrys[i+1:]...)
					delete(s.entryMap, sw.symbol.Name)
					s.container.Remove(sw)
					s.scroll.Refresh()
					break
				}
			}
			//canvas := fyne.CurrentApp().Driver().CanvasForObject(entry)
			//canvas.Refresh(entry)
		})
		s.entrys = append(s.entrys, entry)
		s.container.Add(entry)
		s.entryMap[sym.Name] = entry
	}
	s.border.Refresh()
}

func (s *SymbolListWidget) SetSymbols(symbols ...*symbol.Symbol) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.symbols = symbols
	s.render()
	s.Refresh()
}

func (s *SymbolListWidget) Symbols() []*symbol.Symbol {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*symbol.Symbol, len(s.symbols))
	copy(out, s.symbols)
	return s.symbols
}

func (s *SymbolListWidget) CreateRenderer() fyne.WidgetRenderer {
	swr := &SymbolListWidgetRenderer{
		sl: s,
	}
	return swr
}

type SymbolListWidgetRenderer struct {
	sl *SymbolListWidget
}

func (sr *SymbolListWidgetRenderer) Layout(size fyne.Size) {
	sr.sl.border.Resize(size)
}

func (sr *SymbolListWidgetRenderer) MinSize() fyne.Size {
	var width float32
	var height float32
	for _, en := range sr.sl.entrys {
		sz := en.MinSize()
		if sz.Width > width {
			width = sz.Width
		}
		height += sz.Height
	}
	return fyne.NewSize(width, min(height, 200))
}

func (sr *SymbolListWidgetRenderer) Refresh() {
	for _, e := range sr.sl.entrys {
		e.Refresh()
	}
}

func (sr *SymbolListWidgetRenderer) Destroy() {
}

func (sr *SymbolListWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{sr.sl.border}
}

type SymbolWidgetEntry struct {
	widget.BaseWidget

	symbol                 *symbol.Symbol
	symbolName             *widget.Entry
	symbolValue            *widget.Label
	symbolNumber           *widget.Label
	symbolType             *widget.Label
	symbolCorrectionfactor *widget.Entry
	deleteBTN              *widget.Button

	container *fyne.Container

	deleteFunc func(*SymbolWidgetEntry)
}

func NewSymbolWidgetEntry(sym *symbol.Symbol, deleteFunc func(*SymbolWidgetEntry)) *SymbolWidgetEntry {
	sw := &SymbolWidgetEntry{
		symbol:     sym,
		deleteFunc: deleteFunc,
	}
	sw.ExtendBaseWidget(sw)

	sw.symbolName = widget.NewEntry()
	sw.symbolName.SetText(sw.symbol.Name)
	sw.symbolValue = widget.NewLabel("---")
	sw.symbolNumber = widget.NewLabel(strconv.Itoa(sw.symbol.Number))
	sw.symbolType = widget.NewLabel(fmt.Sprintf("%02X", sw.symbol.Type))
	sw.symbolCorrectionfactor = widget.NewEntry()
	sw.SetCorrectionFactor(sym.Correctionfactor)
	sw.deleteBTN = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		if sw.deleteFunc != nil {
			sw.deleteFunc(sw)
		}
	})
	sw.container = container.NewWithoutLayout(
		sw.symbolName,
		sw.symbolValue,
		sw.symbolNumber,
		sw.symbolType,
		sw.symbolCorrectionfactor,
		sw.deleteBTN,
	)

	return sw
}

func (sw *SymbolWidgetEntry) SetCorrectionFactor(f float64) {
	sw.symbol.Correctionfactor = f
	switch f {
	case 1:
		sw.symbolCorrectionfactor.SetText(fmt.Sprintf("%.0f", f))
	case 0.1:
		sw.symbolCorrectionfactor.SetText(fmt.Sprintf("%.01f", f))
	case 0.01:
		sw.symbolCorrectionfactor.SetText(fmt.Sprintf("%.02f", f))
	case 0.001:
		sw.symbolCorrectionfactor.SetText(fmt.Sprintf("%.03f", f))
	default:
		sw.symbolCorrectionfactor.SetText(fmt.Sprintf("%.04f", f))
	}
}

func (sw *SymbolWidgetEntry) CreateRenderer() fyne.WidgetRenderer {
	swr := &SymbolWidgetEntryRenderer{
		sw: sw,
	}
	return swr
}

type SymbolWidgetEntryRenderer struct {
	sw *SymbolWidgetEntry
}

var sz = []float32{
	.32, // name
	.18, // value
	.12, // number
	.16, // correctionfactor
	.08, // type
	.08, // deletebtn
}

func sumFloat32(a []float32) float32 {
	var sum float32
	for _, v := range a {
		sum += v
	}
	return sum
}

func (sr *SymbolWidgetEntryRenderer) Layout(size fyne.Size) {
	sw := sr.sw
	sw.container.Resize(size)

	padd := size.Width * ((1.0 - sumFloat32(sz)) / float32(len(sz)))
	sw.symbolName.Resize(fyne.NewSize(size.Width*sz[0], size.Height))
	sw.symbolValue.Resize(fyne.NewSize(size.Width*sz[1], size.Height))
	sw.symbolNumber.Resize(fyne.NewSize(size.Width*sz[2], size.Height))
	sw.symbolType.Resize(fyne.NewSize(size.Width*sz[3], size.Height))
	sw.symbolCorrectionfactor.Resize(fyne.NewSize(size.Width*sz[4], size.Height))
	sw.deleteBTN.Resize(fyne.NewSize(size.Width*sz[5], size.Height))

	var x float32

	sw.symbolName.Move(fyne.NewPos(x, 0))
	x += sw.symbolName.Size().Width + padd

	sw.symbolValue.Move(fyne.NewPos(x, 0))
	x += sw.symbolValue.Size().Width + padd

	sw.symbolNumber.Move(fyne.NewPos(x, 0))
	x += sw.symbolNumber.Size().Width + padd

	sw.symbolType.Move(fyne.NewPos(x, 0))
	x += sw.symbolType.Size().Width + padd

	sw.symbolCorrectionfactor.Move(fyne.NewPos(x, 0))
	x += sw.symbolCorrectionfactor.Size().Width + padd

	sw.deleteBTN.Move(fyne.NewPos(x, 0))
}

func (sr *SymbolWidgetEntryRenderer) MinSize() fyne.Size {
	sw := sr.sw
	var width float32
	var height float32 = sw.symbolName.MinSize().Height
	width += sw.symbolName.MinSize().Width
	width += sw.symbolValue.MinSize().Width
	width += sw.symbolNumber.MinSize().Width
	width += sw.symbolCorrectionfactor.MinSize().Width
	width += sw.deleteBTN.MinSize().Width
	return fyne.NewSize(width, height)
}

func (sr *SymbolWidgetEntryRenderer) Refresh() {
	sr.sw.symbolName.SetText(sr.sw.symbol.Name)
	sr.sw.symbolNumber.SetText(strconv.Itoa(sr.sw.symbol.Number))
	sr.sw.symbolType.SetText(fmt.Sprintf("%02X", sr.sw.symbol.Type))
	sr.sw.SetCorrectionFactor(sr.sw.symbol.Correctionfactor)
	sr.sw.symbolValue.SetText(sr.sw.symbol.StringValue())
}

func (sr *SymbolWidgetEntryRenderer) Destroy() {
}

func (sr *SymbolWidgetEntryRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{sr.sw.container}
}
