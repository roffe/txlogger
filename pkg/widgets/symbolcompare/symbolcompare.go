package symbolcompare

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Config drives the compare list. Diffs is the (already sorted) list of symbol
// names that differ between the two binaries. The callbacks open the actual
// map views, which live in the windows package so they can reuse MapViewer.
type Config struct {
	Diffs            []string
	OnShowDiff       func(name string)
	OnShowSideBySide func(name string)
}

// New returns a list of differing symbols. Double-click a row to compare it side
// by side; right-click for a context menu to show the differences map.
func New(cfg *Config) fyne.CanvasObject {
	list := widget.NewList(
		func() int { return len(cfg.Diffs) },
		func() fyne.CanvasObject { return newCompareRow(cfg) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*compareRow).setName(cfg.Diffs[i])
		},
	)

	header := widget.NewLabel(fmt.Sprintf("%d symbols differ", len(cfg.Diffs)))
	return container.NewBorder(header, nil, nil, nil, list)
}

// compareRow is a list row that reacts to double-tap and right-click. It does
// not implement Tapped, so single taps still propagate to the List for
// selection highlighting.
type compareRow struct {
	widget.BaseWidget
	label *widget.Label
	name  string
	cfg   *Config
}

func newCompareRow(cfg *Config) *compareRow {
	r := &compareRow{label: widget.NewLabel(""), cfg: cfg}
	r.ExtendBaseWidget(r)
	return r
}

func (r *compareRow) setName(name string) {
	r.name = name
	r.label.SetText(name)
}

func (r *compareRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.label)
}

func (r *compareRow) DoubleTapped(_ *fyne.PointEvent) {
	if r.name != "" && r.cfg.OnShowSideBySide != nil {
		r.cfg.OnShowSideBySide(r.name)
	}
}

func (r *compareRow) TappedSecondary(e *fyne.PointEvent) {
	if r.name == "" || r.cfg.OnShowDiff == nil {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(r)
	if c == nil {
		return
	}
	menu := fyne.NewMenu("", fyne.NewMenuItem("Show differences map", func() {
		r.cfg.OnShowDiff(r.name)
	}))
	widget.NewPopUpMenu(menu, c).ShowAtPosition(e.AbsolutePosition)
}
