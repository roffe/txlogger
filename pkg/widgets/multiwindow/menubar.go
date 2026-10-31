package multiwindow

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// MenuBar shows menus as a row of titles, like the main window menu. Tapping
// a title drops its menu down, and while a menu is open hovering another title
// switches to it. Place it at the top of the window content, e.g.
// container.NewBorder(bar, nil, nil, nil, body).
type MenuBar struct {
	widget.BaseWidget
	items  []*menuBarItem
	drop   *menuBarDropDown // nil while no menu is open
	active int
}

func NewMenuBar(menus ...*fyne.Menu) *MenuBar {
	b := &MenuBar{active: -1}
	for i, m := range menus {
		it := &menuBarItem{menu: m, bar: b, index: i}
		it.ExtendBaseWidget(it)
		b.items = append(b.items, it)
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *MenuBar) CreateRenderer() fyne.WidgetRenderer {
	row := container.NewHBox()
	for _, it := range b.items {
		row.Add(it)
	}
	return widget.NewSimpleRenderer(row)
}

// open drops down menu i below its title, replacing any open menu.
func (b *MenuBar) open(i int) {
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	if b.drop == nil {
		b.drop = &menuBarDropDown{bar: b, box: container.NewWithoutLayout()}
		b.drop.ExtendBaseWidget(b.drop)
		c.Overlays().Add(b.drop)
		c.Focus(b.drop)
	}
	prev := b.active
	b.active = i

	item := b.items[i]
	m := widget.NewMenu(item.menu)
	m.OnDismiss = b.close
	size := m.MinSize()
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(item).Subtract(b.drop.Position())
	pos = pos.AddXY(0, item.Size().Height)
	pos.X = max(0, min(pos.X, b.drop.Size().Width-size.Width))
	m.Resize(size)
	m.Move(pos)
	b.drop.box.Objects = []fyne.CanvasObject{m}
	b.drop.box.Refresh()

	if prev >= 0 {
		b.items[prev].Refresh()
	}
	item.Refresh()
}

func (b *MenuBar) close() {
	if b.drop == nil {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(b.drop); c != nil {
		c.Overlays().Remove(b.drop)
	}
	b.drop = nil
	if b.active >= 0 {
		b.items[b.active].Refresh()
	}
	b.active = -1
}

// menuBarDropDown is the canvas overlay holding the open menu. It covers the
// whole canvas, so it also sees the mouse over the bar titles and closes the
// menu on taps outside it.
type menuBarDropDown struct {
	widget.BaseWidget
	bar *MenuBar
	box *fyne.Container
}

var (
	_ fyne.Tappable     = (*menuBarDropDown)(nil)
	_ fyne.Focusable    = (*menuBarDropDown)(nil)
	_ desktop.Hoverable = (*menuBarDropDown)(nil)
)

func (d *menuBarDropDown) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(d.box)
}

func (d *menuBarDropDown) Tapped(*fyne.PointEvent) { d.bar.close() }

func (d *menuBarDropDown) MouseIn(e *desktop.MouseEvent) { d.MouseMoved(e) }
func (d *menuBarDropDown) MouseOut()                     {}

// MouseMoved switches to the menu whose title is under the mouse.
func (d *menuBarDropDown) MouseMoved(e *desktop.MouseEvent) {
	drv := fyne.CurrentApp().Driver()
	for i, it := range d.bar.items {
		p := e.AbsolutePosition.Subtract(drv.AbsolutePositionForObject(it))
		if p.X >= 0 && p.Y >= 0 && p.X < it.Size().Width && p.Y < it.Size().Height {
			if i != d.bar.active {
				d.bar.open(i)
			}
			return
		}
	}
}

func (d *menuBarDropDown) menu() *widget.Menu { return d.box.Objects[0].(*widget.Menu) }

func (d *menuBarDropDown) TypedKey(e *fyne.KeyEvent) {
	n := len(d.bar.items)
	switch e.Name {
	case fyne.KeyLeft:
		if !d.menu().DeactivateLastSubmenu() {
			d.bar.open((d.bar.active + n - 1) % n)
		}
	case fyne.KeyRight:
		if !d.menu().ActivateLastSubmenu() {
			d.bar.open((d.bar.active + 1) % n)
		}
	case fyne.KeyDown:
		d.menu().ActivateNext()
	case fyne.KeyUp:
		d.menu().ActivatePrevious()
	case fyne.KeyEnter, fyne.KeyReturn, fyne.KeySpace:
		d.menu().TriggerLast()
	case fyne.KeyEscape:
		d.bar.close()
	}
}

func (d *menuBarDropDown) TypedRune(rune) {}
func (d *menuBarDropDown) FocusGained()   {}
func (d *menuBarDropDown) FocusLost()     {}

// menuBarItem is one title in a MenuBar.
type menuBarItem struct {
	widget.BaseWidget
	menu    *fyne.Menu
	bar     *MenuBar
	index   int
	hovered bool
}

var (
	_ fyne.Tappable     = (*menuBarItem)(nil)
	_ desktop.Hoverable = (*menuBarItem)(nil)
)

func (i *menuBarItem) Tapped(*fyne.PointEvent) { i.bar.open(i.index) }

func (i *menuBarItem) MouseIn(*desktop.MouseEvent)    { i.hovered = true; i.Refresh() }
func (i *menuBarItem) MouseMoved(*desktop.MouseEvent) {}
func (i *menuBarItem) MouseOut()                      { i.hovered = false; i.Refresh() }

func (i *menuBarItem) CreateRenderer() fyne.WidgetRenderer {
	r := &menuBarItemRenderer{
		i:    i,
		bg:   canvas.NewRectangle(color.Transparent),
		text: canvas.NewText(i.menu.Label, theme.Color(theme.ColorNameForeground)),
	}
	r.Refresh()
	return r
}

type menuBarItemRenderer struct {
	i    *menuBarItem
	bg   *canvas.Rectangle
	text *canvas.Text
}

func (r *menuBarItemRenderer) Layout(size fyne.Size) {
	p := theme.InnerPadding()
	r.text.Resize(r.text.MinSize())
	r.text.Move(fyne.NewPos(p, p/2))
	r.bg.Resize(size)
}

func (r *menuBarItemRenderer) MinSize() fyne.Size {
	p := theme.InnerPadding()
	return r.text.MinSize().AddWidthHeight(p*2, p)
}

func (r *menuBarItemRenderer) Refresh() {
	r.text.TextSize = theme.TextSize()
	r.text.Color = theme.Color(theme.ColorNameForeground)
	r.bg.CornerRadius = theme.SelectionRadiusSize()
	switch {
	case r.i.bar.active == r.i.index:
		r.bg.FillColor = theme.Color(theme.ColorNameFocus)
	case r.i.hovered && r.i.bar.drop == nil:
		r.bg.FillColor = theme.Color(theme.ColorNameHover)
	default:
		r.bg.FillColor = color.Transparent
	}
	r.text.Refresh()
	r.bg.Refresh()
}

func (r *menuBarItemRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.text} }
func (r *menuBarItemRenderer) Destroy()                     {}
