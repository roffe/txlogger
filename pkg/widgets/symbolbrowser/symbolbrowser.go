package symbolbrowser

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xlayout "fyne.io/x/fyne/layout"
	symbol "github.com/roffe/ecusymbol"
)

var _ fyne.Widget = (*Widget)(nil)

// Column proportions, shared by the header and every row so they line up.
// Name, Address, SRAM offset, Length, Type, Action(Open).
var columnSizes = []float64{0.34, 0.15, 0.15, 0.09, 0.15, 0.12}

// Widget lists every symbol in the loaded binary together with its address,
// sram offset, length and type, and offers a button to open each one as a map.
type Widget struct {
	widget.BaseWidget

	getFW   func() symbol.SymbolCollection
	getECU  func() symbol.ECUType
	openMap func(symbol.ECUType, string, string)
	err     func(error)

	search   *widget.Entry
	countLbl *widget.Label
	list     *widget.List

	all      []*symbol.Symbol
	filtered []*symbol.Symbol
}

func New(getFW func() symbol.SymbolCollection, getECU func() symbol.ECUType, openMap func(symbol.ECUType, string, string), err func(error)) *Widget {
	w := &Widget{
		getFW:   getFW,
		getECU:  getECU,
		openMap: openMap,
		err:     err,
	}
	w.loadSymbols()
	w.ExtendBaseWidget(w)
	return w
}

// loadSymbols pulls the current symbol collection from the main window.
func (w *Widget) loadSymbols() {
	fw := w.getFW()
	if fw == nil {
		w.all = nil
		w.filtered = nil
		return
	}
	w.all = fw.Symbols()
	w.filtered = w.all
}

func (w *Widget) applyFilter(q string) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		w.filtered = w.all
	} else {
		out := make([]*symbol.Symbol, 0, len(w.all))
		for _, s := range w.all {
			if strings.Contains(strings.ToLower(s.Name), q) {
				out = append(out, s)
			}
		}
		w.filtered = out
	}
	if w.countLbl != nil {
		w.countLbl.SetText(w.countText())
	}
	if w.list != nil {
		w.list.Refresh()
		w.list.ScrollToTop()
	}
}

func (w *Widget) countText() string {
	if len(w.filtered) == len(w.all) {
		return fmt.Sprintf("%d symbols", len(w.all))
	}
	return fmt.Sprintf("%d / %d symbols", len(w.filtered), len(w.all))
}

func (w *Widget) openSymbol(sym *symbol.Symbol) {
	if w.openMap == nil || sym == nil {
		return
	}
	w.openMap(w.getECU(), "", sym.Name)
}

func (w *Widget) header() fyne.CanvasObject {
	mk := func(s string) *widget.Label {
		return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	return container.New(
		xlayout.NewHPortion(columnSizes),
		mk("Name"),
		mk("Address"),
		mk("SRAM offset"),
		mk("Length"),
		mk("Type"),
		mk("Action"),
	)
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	w.search = widget.NewEntry()
	w.search.SetPlaceHolder("Filter by name...")
	w.search.OnChanged = w.applyFilter

	w.countLbl = widget.NewLabel(w.countText())

	refresh := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		w.loadSymbols()
		w.applyFilter(w.search.Text)
	})

	w.list = widget.NewList(
		func() int { return len(w.filtered) },
		func() fyne.CanvasObject { return newSymbolRow(w.openSymbol) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(w.filtered) {
				return
			}
			o.(*symbolRow).set(w.filtered[i])
		},
	)

	top := container.NewBorder(nil, nil, nil, container.NewHBox(w.countLbl, refresh), w.search)

	return widget.NewSimpleRenderer(container.NewBorder(
		container.NewVBox(top, w.header()),
		nil,
		nil,
		nil,
		w.list,
	))
}

// typeString renders the Trionic type bitfield as readable flags.
func typeString(t uint8) string {
	var parts []string
	if t&symbol.SIGNED != 0 {
		parts = append(parts, "signed")
	}
	if t&symbol.KONST != 0 {
		parts = append(parts, "const")
	}
	if t&symbol.CHAR != 0 {
		parts = append(parts, "char")
	}
	if t&symbol.LONG != 0 {
		parts = append(parts, "long")
	}
	if t&symbol.BITFIELD != 0 {
		parts = append(parts, "bitfield")
	}
	if t&symbol.STRUCT != 0 {
		parts = append(parts, "struct")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0x%02X", t)
	}
	return strings.Join(parts, "|")
}

var _ fyne.Widget = (*symbolRow)(nil)

type symbolRow struct {
	widget.BaseWidget

	name   *widget.Label
	addr   *widget.Label
	sram   *widget.Label
	length *widget.Label
	typ    *widget.Label
	open   *widget.Button

	sym    *symbol.Symbol
	onOpen func(*symbol.Symbol)
}

func newSymbolRow(onOpen func(*symbol.Symbol)) *symbolRow {
	r := &symbolRow{
		name:   widget.NewLabel(""),
		addr:   widget.NewLabel(""),
		sram:   widget.NewLabel(""),
		length: widget.NewLabel(""),
		typ:    widget.NewLabel(""),
		onOpen: onOpen,
	}
	r.name.Truncation = fyne.TextTruncateEllipsis
	r.name.Selectable = true
	r.typ.Truncation = fyne.TextTruncateEllipsis
	r.open = widget.NewButtonWithIcon("Open", theme.GridIcon(), func() {
		if r.sym != nil && r.onOpen != nil {
			r.onOpen(r.sym)
		}
	})
	r.ExtendBaseWidget(r)
	return r
}

func (r *symbolRow) set(sym *symbol.Symbol) {
	r.sym = sym
	r.name.SetText(sym.Name)
	r.addr.SetText(fmt.Sprintf("$%06X", sym.Address))
	r.sram.SetText(fmt.Sprintf("$%06X", sym.SramOffset))
	r.length.SetText(strconv.Itoa(int(sym.Length)))
	r.typ.SetText(typeString(sym.Type))
}

func (r *symbolRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.New(
		xlayout.NewHPortion(columnSizes),
		r.name,
		r.addr,
		r.sram,
		r.length,
		r.typ,
		r.open,
	))
}
