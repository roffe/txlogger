package symbolcompare

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Item is one differing symbol. OtherLength is the symbol's size in the other
// binary, so size mismatches (which side-by-side can't render) are visible up
// front.
type Item struct {
	Name        string
	Number      int
	Length      uint16
	OtherLength uint16
	Address     uint32
}

func (it Item) details() string {
	size := fmt.Sprintf("%d B", it.Length)
	if it.OtherLength != it.Length {
		size = fmt.Sprintf("%d→%d B", it.Length, it.OtherLength)
	}
	return fmt.Sprintf("#%d · %s · $%X", it.Number, size, it.Address)
}

// Config drives the compare list. Diffs is the (already sorted) list of
// symbols that differ between the two binaries. OnPreview fires on single
// click, OnOpen on double click; both get the symbol name and open the actual
// map view, which lives in the windows package so it can reuse MapViewer.
type Config struct {
	Diffs     []Item
	OnPreview func(name string)
	OnOpen    func(name string)
}

// New returns a list of differing symbols. Single-click previews a symbol,
// double-click opens it in its own tab.
func New(cfg *Config) fyne.CanvasObject {
	// Double-click is detected manually: if the row implemented DoubleTappable,
	// Fyne would hold every single tap for DoubleTapDelay before delivering it,
	// making previews feel sluggish. Previewing on the first click of a
	// double-click is harmless, so preview fires immediately and a quick second
	// click on the same row means "open".
	var lastName string
	var lastTap time.Time
	list := widget.NewList(
		func() int { return len(cfg.Diffs) },
		func() fyne.CanvasObject { return newCompareRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			r := o.(*compareRow)
			r.set(cfg.Diffs[i])
			name := cfg.Diffs[i].Name
			r.onTap = func() {
				if name == lastName && time.Since(lastTap) <= fyne.CurrentApp().Driver().DoubleTapDelay() {
					lastName = ""
					if cfg.OnOpen != nil {
						cfg.OnOpen(name)
					}
					return
				}
				lastName, lastTap = name, time.Now()
				if cfg.OnPreview != nil {
					cfg.OnPreview(name)
				}
			}
		},
	)

	header := widget.NewLabel(fmt.Sprintf("%d symbols differ · click to preview, double-click to open", len(cfg.Diffs)))
	return container.NewBorder(header, nil, nil, nil, list)
}

// compareRow shows a symbol name plus its number/size/address details. It
// handles taps itself (instead of List.OnSelected) so single and double click
// can do different things. Deliberately NOT DoubleTappable — see New.
type compareRow struct {
	widget.BaseWidget
	name   *widget.Label
	detail *widget.Label
	onTap  func()
}

func newCompareRow() *compareRow {
	r := &compareRow{
		name:   widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		detail: widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true}),
	}
	r.detail.Importance = widget.LowImportance
	r.ExtendBaseWidget(r)
	return r
}

func (r *compareRow) set(item Item) {
	r.name.SetText(item.Name)
	r.detail.SetText(item.details())
}

func (r *compareRow) Tapped(*fyne.PointEvent) {
	if r.onTap != nil {
		r.onTap()
	}
}

func (r *compareRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, nil, r.detail, r.name))
}
