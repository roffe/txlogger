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

	Persist    bool // Persist through layout changes
	IgnoreSave bool // Ignore saving to layout

	icon                     fyne.CanvasObject
	minBtn, maxBtn, closeBtn *widget.Button

	title       string
	bg          *canvas.Rectangle
	bgFillColor fyne.ThemeColorName
	content     *fyne.Container

	maximized        bool
	active           bool
	leftDrag         bool
	preMaximizedSize fyne.Size
	preMaximizedPos  fyne.Position

	onClose func() `json:"-"`
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

func NewSystemWindow(title string, content fyne.CanvasObject) *InnerWindow {
	w := &InnerWindow{
		title:       title,
		content:     container.NewPadded(content),
		bgFillColor: theme.ColorNameOverlayBackground,
		Persist:     true,
		IgnoreSave:  true,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *InnerWindow) Content() fyne.CanvasObject {
	return w.content.Objects[0]
}

// MouseDown is called when the user presses the mouse button on the draggable corner.
func (w *InnerWindow) MouseDown(*desktop.MouseEvent) {
	if w.OnMouseDown != nil {
		w.OnMouseDown()
	}
}

// MouseUp is called when the user releases the mouse button on the draggable corner.
func (w *InnerWindow) MouseUp(*desktop.MouseEvent) {
}

func (w *InnerWindow) Maximized() bool {
	return w.maximized
}

func (w *InnerWindow) SetMaximized(maximized bool, prePos fyne.Position, preSize fyne.Size) {
	w.maximized = maximized
	w.preMaximizedPos = prePos
	w.preMaximizedSize = preSize
}

func (w *InnerWindow) PreMaximizedSize() fyne.Size {
	return w.preMaximizedSize
}

func (w *InnerWindow) PreMaximizedPos() fyne.Position {
	return w.preMaximizedPos
}

func (w *InnerWindow) Close() {
	// Call user defined close intercept
	if f := w.CloseIntercept; f != nil {
		f()
	}

	// Call system defined close intercept
	if f := w.onClose; f != nil {
		f()
	}
}

func (w *InnerWindow) CreateRenderer() fyne.WidgetRenderer {
	w.minBtn = &widget.Button{Icon: theme.WindowMinimizeIcon(), Importance: widget.LowImportance, OnTapped: w.OnMinimized}
	if w.OnMinimized == nil {
		w.minBtn.Disable()
	}
	w.maxBtn = &widget.Button{Icon: theme.WindowMaximizeIcon(), Importance: widget.LowImportance, OnTapped: w.OnMaximized}
	if w.OnMaximized == nil {
		w.maxBtn.Disable()
	}
	w.closeBtn = &widget.Button{Icon: theme.WindowCloseIcon(), Importance: widget.LowImportance, OnTapped: func() {
		w.Close()
	}}

	if w.Icon != nil {
		w.icon = &widget.Button{Icon: w.Icon, Importance: widget.LowImportance, OnTapped: func() {
			if f := w.OnTappedIcon; f != nil {
				f()
			}
		}}
		if w.OnTappedIcon == nil {
			w.icon.(*widget.Button).Disable()
		}
	}

	title := newDraggableLabel(w.title, w)
	title.Truncation = fyne.TextTruncateEllipsis

	isLeading := w.ButtonAlignment == widget.ButtonAlignLeading || (w.ButtonAlignment == widget.ButtonAlignCenter && runtime.GOOS == "darwin")

	var buttons *fyne.Container
	var bar *fyne.Container
	if isLeading {
		// Left side (darwin default or explicit left alignment)
		buttons = container.NewHBox(w.closeBtn, w.minBtn, w.maxBtn)
		bar = container.NewBorder(nil, nil, buttons, w.icon, title)
	} else {
		// Right side (Windows/Linux default and explicit right alignment)
		buttons = container.NewHBox(w.minBtn, w.maxBtn, w.closeBtn)
		bar = container.NewBorder(nil, nil, w.icon, buttons, title)
	}

	th := w.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	w.bg = canvas.NewRectangle(th.Color(theme.ColorNameOverlayBackground, v))
	contentBG := canvas.NewRectangle(th.Color(theme.ColorNameBackground, v))
	leftCorner := newDraggableCorner(w, true)
	rightCorner := newDraggableCorner(w, false)

	objects := []fyne.CanvasObject{w.bg, contentBG, bar, w.content, leftCorner, rightCorner}
	return &innerWindowRenderer{ShadowingRenderer: NewShadowingRenderer(objects, DialogLevel),
		win: w, bar: bar, bg: w.bg, leftCorner: leftCorner, rightCorner: rightCorner, contentBG: contentBG}
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
	leftCorner    fyne.CanvasObject
	rightCorner   fyne.CanvasObject
}

func (i *innerWindowRenderer) Layout(size fyne.Size) {
	// Calculate padding and base size
	padding := float32(2.0) //i.win.Theme().Size(theme.SizeNamePadding)
	basePos := fyne.NewSquareOffsetPos(padding / 2)
	contentSize := size.Subtract(fyne.NewSquareSize(padding))

	// Pre-calculate commonly used dimensions
	adjustedWidth := contentSize.Width - padding*2

	// Layout shadow and background
	i.LayoutShadow(contentSize, basePos)
	i.bg.Move(basePos)
	i.bg.Resize(contentSize)

	// Layout title bar
	barHeight := i.bar.MinSize().Height
	i.bar.Resize(fyne.NewSize(adjustedWidth, barHeight))
	i.bar.Move(basePos.AddXY(padding, 0))

	// Layout main content area
	contentPos := basePos.AddXY(padding, barHeight)
	contentDimensions := fyne.NewSize(adjustedWidth, contentSize.Height-padding-barHeight)

	i.contentBG.Move(contentPos)
	i.contentBG.Resize(contentDimensions)
	i.win.content.Move(contentPos)
	i.win.content.Resize(contentDimensions)

	// Layout corners
	i.layoutCorners(basePos, contentSize)
}

// Helper method to handle corner layout
func (i *innerWindowRenderer) layoutCorners(basePos fyne.Position, size fyne.Size) {
	rightSize := i.rightCorner.MinSize()
	i.rightCorner.Move(basePos.Add(size).Subtract(rightSize).AddXY(1, 1))
	i.rightCorner.Resize(rightSize)

	leftSize := i.leftCorner.MinSize()
	leftPos := basePos.AddXY(0, size.Height-leftSize.Height).AddXY(-1, 1)
	i.leftCorner.Move(leftPos)
	i.leftCorner.Resize(leftSize)
}

func (i *innerWindowRenderer) MinSize() fyne.Size {
	pad := i.win.Theme().Size(theme.SizeNamePadding)
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
	win     *InnerWindow
	leading bool
}

func newDraggableCorner(w *InnerWindow, leading bool) *draggableCorner {
	d := &draggableCorner{win: w, leading: leading}
	d.ExtendBaseWidget(d)
	return d
}

func (c *draggableCorner) CreateRenderer() fyne.WidgetRenderer {
	var prop *canvas.Image
	th := fyne.CurrentApp().Settings().Theme()
	if c.leading {
		prop = canvas.NewImageFromResource(th.Icon(fyne.ThemeIconName("drag-corner-indicator-left")))
	} else {
		prop = canvas.NewImageFromResource(th.Icon(theme.IconNameDragCornerIndicator))
	}
	prop.SetMinSize(fyne.NewSquareSize(16))
	return widget.NewSimpleRenderer(prop)
}

func (c *draggableCorner) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (c *draggableCorner) Dragged(ev *fyne.DragEvent) {
	if f := c.win.OnResized; f != nil {
		c.win.leftDrag = c.leading
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
