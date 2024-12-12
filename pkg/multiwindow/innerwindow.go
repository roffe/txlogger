package multiwindow

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

var _ fyne.Widget = (*InnerWindow)(nil)

// InnerWindow defines a container that wraps content in a window border - that can then be placed inside
// a regular container/canvas.
type InnerWindow struct {
	widget.BaseWidget

	// ButtonAlignment specifies where the window buttons (close, minimize, maximize) should be placed.
	// The default is widget.ButtonAlignCenter which will auto select based on the OS.
	//	- On Darwin this will be `widget.ButtonAlignLeading`
	//	- On all other OS this will be `widget.ButtonAlignTrailing`
	ButtonAlignment                                     widget.ButtonAlign
	CloseIntercept                                      func()                `json:"-"`
	OnDragged, OnResized                                func(*fyne.DragEvent) `json:"-"`
	OnMinimized, OnMaximized, OnTappedBar, OnTappedIcon func()                `json:"-"`
	OnMouseDown                                         func()                `json:"-"`
	Icon                                                fyne.Resource

	title       string
	bg          *canvas.Rectangle
	bgFillColor fyne.ThemeColorName
	content     *fyne.Container

	maximized        bool
	active           bool
	preMaximizedSize fyne.Size
	preMaximizedPos  fyne.Position
}

// NewInnerWindow creates a new window border around the given `content`, displaying the `title` along the top.
// This will behave like a normal contain and will probably want to be added to a `MultipleWindows` parent.
func NewInnerWindow(title string, content fyne.CanvasObject) *InnerWindow {
	w := &InnerWindow{
		title:       title,
		content:     container.NewPadded(content),
		bgFillColor: theme.ColorNameOverlayBackground,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *InnerWindow) Close() {
	w.Hide()
}

func (w *InnerWindow) CreateRenderer() fyne.WidgetRenderer {
	w.ExtendBaseWidget(w)

	min := &widget.Button{Icon: theme.WindowMinimizeIcon(), Importance: widget.LowImportance, OnTapped: w.OnMinimized}
	if w.OnMinimized == nil {
		min.Disable()
	}
	max := &widget.Button{Icon: theme.WindowMaximizeIcon(), Importance: widget.LowImportance, OnTapped: w.OnMaximized}
	if w.OnMaximized == nil {
		max.Disable()
	}

	var icon fyne.CanvasObject
	if w.Icon != nil {
		icon = &widget.Button{Icon: w.Icon, Importance: widget.LowImportance, OnTapped: func() {
			if f := w.OnTappedIcon; f != nil {
				f()
			}
		}}
		if w.OnTappedIcon == nil {
			icon.(*widget.Button).Disable()
		}
	}

	title := newDraggableLabel(w.title, w)
	title.Truncation = fyne.TextTruncateEllipsis

	close := &widget.Button{Icon: theme.WindowCloseIcon(), Importance: widget.DangerImportance, OnTapped: func() {
		if f := w.CloseIntercept; f != nil {
			f()
		} else {
			w.Close()
		}
	}}

	isLeading := w.ButtonAlignment == widget.ButtonAlignLeading || (w.ButtonAlignment == widget.ButtonAlignCenter && runtime.GOOS == "darwin")

	var buttons *fyne.Container
	var bar *fyne.Container
	if isLeading {
		// Left side (darwin default or explicit left alignment)
		buttons = container.NewHBox(close, min, max)
		bar = container.NewBorder(nil, nil, buttons, icon, title)
	} else {
		// Right side (Windows/Linux default and explicit right alignment)
		buttons = container.NewHBox(min, max, close)
		bar = container.NewBorder(nil, nil, icon, buttons, title)
	}

	th := w.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	w.bg = canvas.NewRectangle(th.Color(theme.ColorNameOverlayBackground, v))
	contentBG := canvas.NewRectangle(th.Color(theme.ColorNameBackground, v))
	corner := newDraggableCorner(w)

	objects := []fyne.CanvasObject{w.bg, contentBG, bar, w.content, corner}
	return &innerWindowRenderer{ShadowingRenderer: NewShadowingRenderer(objects, DialogLevel),
		win: w, bar: bar, bg: w.bg, corner: corner, contentBG: contentBG}
}

func (w *InnerWindow) SetContent(obj fyne.CanvasObject) {
	w.content.Objects[0] = obj

	w.content.Refresh()
}

func (w *InnerWindow) SetPadded(pad bool) {
	if pad {
		w.content.Layout = layout.NewPaddedLayout()
	} else {
		w.content.Layout = layout.NewStackLayout()
	}
	w.content.Refresh()
}

// Title returns the current title of the window.
func (w *InnerWindow) Title() string {
	return w.title
}

func (w *InnerWindow) SetTitle(title string) {
	w.title = title
	w.Refresh()
}

var _ fyne.WidgetRenderer = (*innerWindowRenderer)(nil)

type innerWindowRenderer struct {
	*ShadowingRenderer

	win           *InnerWindow
	bar           *fyne.Container
	bg, contentBG *canvas.Rectangle
	corner        fyne.CanvasObject
}

func (i *innerWindowRenderer) Layout(size fyne.Size) {
	th := i.win.Theme()
	pad := th.Size(theme.SizeNamePadding)

	pos := fyne.NewSquareOffsetPos(pad / 2)
	size = size.Subtract(fyne.NewSquareSize(pad))
	i.LayoutShadow(size, pos)

	i.bg.Move(pos)
	i.bg.Resize(size)

	barHeight := i.bar.MinSize().Height
	i.bar.Move(pos.AddXY(pad, 0))
	i.bar.Resize(fyne.NewSize(size.Width-pad*2, barHeight))

	innerPos := pos.AddXY(pad, barHeight)
	innerSize := fyne.NewSize(size.Width-pad*2, size.Height-pad-barHeight)
	i.contentBG.Move(innerPos)
	i.contentBG.Resize(innerSize)
	i.win.content.Move(innerPos)
	i.win.content.Resize(innerSize)

	cornerSize := i.corner.MinSize()
	i.corner.Move(pos.Add(size).Subtract(cornerSize).AddXY(1, 1))
	i.corner.Resize(cornerSize)
}

func (i *innerWindowRenderer) MinSize() fyne.Size {
	th := i.win.Theme()
	pad := th.Size(theme.SizeNamePadding)
	contentMin := i.win.content.MinSize()
	barMin := i.bar.MinSize()

	innerWidth := fyne.Max(barMin.Width, contentMin.Width)

	return fyne.NewSize(innerWidth+pad*2, contentMin.Height+pad+barMin.Height).Add(fyne.NewSquareSize(pad))
}

func (i *innerWindowRenderer) Refresh() {
	th := i.win.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	i.bg.FillColor = th.Color(i.win.bgFillColor, v)
	i.bg.Refresh()
	i.contentBG.FillColor = th.Color(theme.ColorNameBackground, v)
	i.contentBG.Refresh()
	i.bar.Refresh()

	title := i.bar.Objects[0].(*draggableLabel)
	title.SetText(i.win.title)
	i.ShadowingRenderer.RefreshShadow()
}

var _ desktop.Mouseable = (*draggableLabel)(nil)

type draggableLabel struct {
	widget.Label
	win *InnerWindow
}

func newDraggableLabel(title string, win *InnerWindow) *draggableLabel {
	d := &draggableLabel{win: win}
	d.ExtendBaseWidget(d)
	d.Text = title
	return d
}

func (d *draggableLabel) Dragged(ev *fyne.DragEvent) {
	if f := d.win.OnDragged; f != nil {
		f(ev)
	}
}

func (d *draggableLabel) DragEnd() {
}

func (d *draggableLabel) Tapped(ev *fyne.PointEvent) {
	if f := d.win.OnTappedBar; f != nil {
		f()
	}
}

// DoubleTapped is called when the user double taps the label.
func (d *draggableLabel) DoubleTapped(_ *fyne.PointEvent) {
	if d.win.OnMaximized != nil {
		d.win.OnMaximized()
	}
}

// MouseDown is called when the user presses the mouse button on the label.
func (d *draggableLabel) MouseDown(*desktop.MouseEvent) {
	if f := d.win.OnMouseDown; f != nil {
		f()
	}
}

// MouseUp is called when the user releases the mouse button on the label.
func (d *draggableLabel) MouseUp(*desktop.MouseEvent) {
}

var _ desktop.Cursorable = (*draggableCorner)(nil)

type draggableCorner struct {
	widget.BaseWidget
	win *InnerWindow
}

var dragcornerindicatorleftIconRes = &fyne.StaticResource{
	StaticName:    "drag-corner-indicator-left.svg",
	StaticContent: assets.LeftCornerBytes,
}

func newDraggableCorner(w *InnerWindow) *draggableCorner {
	d := &draggableCorner{win: w}
	d.ExtendBaseWidget(d)
	return d
}

func (c *draggableCorner) CreateRenderer() fyne.WidgetRenderer {
	prop := canvas.NewImageFromResource(fyne.CurrentApp().Settings().Theme().Icon(theme.IconNameDragCornerIndicator))
	prop.SetMinSize(fyne.NewSquareSize(16))
	return widget.NewSimpleRenderer(prop)
}

func (c *draggableCorner) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (c *draggableCorner) Dragged(ev *fyne.DragEvent) {
	if f := c.win.OnResized; f != nil {
		c.win.OnResized(ev)
	}
}

// MouseDown is called when the user presses the mouse button on the draggable corner.
func (c *draggableCorner) MouseDown(*desktop.MouseEvent) {
	if f := c.win.OnMouseDown; f != nil {
		f()
	}
}

// MouseUp is called when the user releases the mouse button on the draggable corner.
func (c *draggableCorner) MouseUp(*desktop.MouseEvent) {
}

func (c *draggableCorner) DragEnd() {
}
