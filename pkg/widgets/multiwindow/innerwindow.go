package multiwindow

import (
	"image/color"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	_ fyne.Draggable     = (*InnerWindow)(nil)
	_ fyne.Widget        = (*InnerWindow)(nil)
	_ desktop.Mouseable  = (*InnerWindow)(nil)
	_ desktop.Hoverable  = (*InnerWindow)(nil)
	_ desktop.Cursorable = (*InnerWindow)(nil)
)

type titleBarButtonMode int

const (
	modeClose titleBarButtonMode = iota
	modeMinimize
	modeMaximize
	modeIcon
)

// InnerWindow defines a container that wraps content in a window border - that can then be placed inside
// a regular container/canvas.
type InnerWindow struct {
	widget.BaseWidget

	// ButtonAlignment specifies where the window buttons (close, minimize, maximize) should be placed.
	// The default is widget.ButtonAlignCenter which will auto select based on the OS.
	//	- On Darwin this will be `widget.ButtonAlignLeading`
	//	- On all other OS this will be `widget.ButtonAlignTrailing`
	Alignment                                           widget.ButtonAlign
	OnClose                                             func()                `json:"-"`
	OnDragged, OnResized                                func(*fyne.DragEvent) `json:"-"`
	OnMinimized, OnMaximized, OnTappedBar, OnTappedIcon func()                `json:"-"`
	OnMouseDown                                         func()                `json:"-"`
	Icon                                                fyne.Resource

	DisableResize bool // Allow resizing
	Persist       bool // Persist through layout changes
	IgnoreSave    bool // Ignore saving to layout

	//minBtn, maxBtn, closeBtn *borderButton

	title       string
	bg          *canvas.Rectangle
	bgFillColor fyne.ThemeColorName
	content     *fyne.Container

	maximized        bool
	active           bool
	leftDrag         bool
	preMaximizedSize fyne.Size

	preMaximizedPos fyne.Position

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

func (w *InnerWindow) Container() *fyne.Container {
	return w.content
}

func (w *InnerWindow) Cursor() desktop.Cursor {
	return desktop.DefaultCursor
}

func (w *InnerWindow) Content() fyne.CanvasObject {
	return w.content.Objects[0]
}

// Dragged is called when the user drags the window.
func (w *InnerWindow) Dragged(ev *fyne.DragEvent) {}

// DragEnd is called when the user stops dragging the window.
func (w *InnerWindow) DragEnd() {}

// MouseIn is called when the mouse enters the window.
func (w *InnerWindow) MouseIn(*desktop.MouseEvent) {}

// MouseOut is called when the mouse leaves the window.
func (w *InnerWindow) MouseOut() {}

// MouseMoved is called when the mouse moves over the window.
func (w *InnerWindow) MouseMoved(*desktop.MouseEvent) {}

// MouseDown is called when the user presses the mouse button on the draggable corner.
func (w *InnerWindow) MouseDown(*desktop.MouseEvent) {
	if w.OnMouseDown != nil {
		w.OnMouseDown()
	}
}

// MouseUp is called when the user releases the mouse button on the draggable corner.
func (w *InnerWindow) MouseUp(ev *desktop.MouseEvent) {
	// log.Println("MouseUp", ev)
	//if o, ok := w.Content().(desktop.Mouseable); ok {
	//	o.MouseUp(ev)
	//}
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
	if w.OnClose != nil {
		w.OnClose()
	}

	// Call system defined close intercept
	if w.onClose != nil {
		w.onClose()
	}
}

func (w *InnerWindow) CreateRenderer() fyne.WidgetRenderer {
	th := w.Theme()

	min := newBorderButton(theme.WindowMinimizeIcon(), modeMinimize, th, w.OnMinimized)
	if w.OnMinimized == nil {
		min.Disable()
	}
	max := newBorderButton(theme.WindowMaximizeIcon(), modeMaximize, th, w.OnMaximized)
	if w.OnMaximized == nil {
		max.Disable()
	}

	close := newBorderButton(theme.WindowCloseIcon(), modeClose, th, func() {
		w.Close()
	})

	borderIcon := newBorderButton(w.Icon, modeIcon, th, func() {
		if f := w.OnTappedIcon; f != nil {
			f()
		}
	})
	if w.OnTappedIcon == nil {
		borderIcon.Disable()
	}

	if w.Icon == nil {
		borderIcon.Hide()
	}

	title := newDraggableLabel(w.title, w)
	title.Truncation = fyne.TextTruncateEllipsis

	isLeading := w.Alignment == widget.ButtonAlignLeading || (w.Alignment == widget.ButtonAlignCenter && runtime.GOOS == "darwin")

	var buttons *fyne.Container
	var bar *fyne.Container
	height := w.Theme().Size(theme.SizeNameWindowTitleBarHeight)
	topPad := (height - title.labelMinSize().Height) / 2

	if isLeading {
		// Left side (darwin default or explicit left alignment)
		buttons = container.NewHBox(close, min, max)
		//bar = container.NewBorder(nil, nil, buttons, borderIcon, title)
		bar = container.NewBorder(nil, nil, buttons, borderIcon, container.New(layout.NewCustomPaddedLayout(topPad, 0, 0, 0), title))
	} else {
		// Right side (Windows/Linux default and explicit right alignment)
		buttons = container.NewHBox(min, max, close)
		//bar = container.NewBorder(nil, nil, borderIcon, buttons, title)
		bar = container.NewBorder(nil, nil, borderIcon, buttons, container.New(layout.NewCustomPaddedLayout(topPad, 0, 0, 0), title))
	}

	v := fyne.CurrentApp().Settings().ThemeVariant()
	w.bg = canvas.NewRectangle(th.Color(theme.ColorNameOverlayBackground, v))
	contentBG := canvas.NewRectangle(th.Color(theme.ColorNameBackground, v))

	var leftCorner, rightCorner *draggableCorner

	objects := []fyne.CanvasObject{w.bg, contentBG, bar, w.content}

	if !w.DisableResize {
		leftCorner = newDraggableCorner(w, true)
		rightCorner = newDraggableCorner(w, false)
		objects = append(objects, leftCorner, rightCorner)
	}

	r := &innerWindowRenderer{
		ShadowingRenderer: NewShadowingRenderer(objects, DialogLevel),
		win:               w,
		bar:               bar,
		buttons:           []*borderButton{min, max, close},
		bg:                w.bg,
		leftCorner:        leftCorner,
		rightCorner:       rightCorner,
		contentBG:         contentBG}
	r.Layout(w.Size())
	return r
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
	buttons       []*borderButton
	bg, contentBG *canvas.Rectangle
	leftCorner    fyne.CanvasObject
	rightCorner   fyne.CanvasObject
}

func (i *innerWindowRenderer) Layout(size fyne.Size) {
	// Calculate padding and base size
	padding := i.win.Theme().Size(theme.SizeNamePadding)

	contentSize := size.Subtract(fyne.NewSquareSize(padding))

	// Pre-calculate commonly used dimensions
	adjustedWidth := contentSize.Width - padding*2

	// Layout shadow and background
	i.LayoutShadow(size, fyne.Position{})
	i.bg.Resize(contentSize)

	// Layout title bar
	barHeight := i.win.Theme().Size(theme.SizeNameWindowTitleBarHeight)
	i.bar.Move(fyne.NewPos(padding, 0))
	i.bar.Resize(fyne.NewSize(size.Width-(padding*2), barHeight))

	// Layout main content area
	contentPos := fyne.NewPos(padding, barHeight)
	contentDimensions := fyne.NewSize(adjustedWidth, contentSize.Height-padding-barHeight)

	i.contentBG.Move(contentPos)
	i.contentBG.Resize(contentDimensions)
	i.win.content.Move(contentPos)
	i.win.content.Resize(contentDimensions)

	// Layout corners
	if !i.win.DisableResize {
		i.layoutCorners(size, padding/2)
	}
}

// Helper method to handle corner layout
func (i *innerWindowRenderer) layoutCorners(size fyne.Size, pad float32) {
	rightSize := i.rightCorner.MinSize()
	rightPos := fyne.Position{X: size.Width - rightSize.Width + pad, Y: size.Height - rightSize.Height}
	i.rightCorner.Move(rightPos)
	i.rightCorner.Resize(rightSize)

	leftSize := i.leftCorner.MinSize()
	leftPos := fyne.Position{X: -(pad + 2), Y: size.Height - leftSize.Height}
	i.leftCorner.Move(leftPos)
	i.leftCorner.Resize(leftSize)
}

func (i *innerWindowRenderer) MinSize() fyne.Size {
	th := i.win.Theme()
	pad := th.Size(theme.SizeNamePadding)
	contentMin := i.win.content.MinSize()
	barHeight := th.Size(theme.SizeNameWindowTitleBarHeight)

	innerWidth := fyne.Max(i.bar.MinSize().Width, contentMin.Width)

	return fyne.NewSize(innerWidth+pad*2, contentMin.Height+pad+barHeight)
}

func (i *innerWindowRenderer) Refresh() {
	th := i.win.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	i.bg.FillColor = th.Color(i.win.bgFillColor, v)
	i.bg.Refresh()
	i.contentBG.FillColor = th.Color(theme.ColorNameBackground, v)
	i.contentBG.Refresh()

	for _, b := range i.buttons {
		b.setTheme(th, i.win.active)
	}
	i.bar.Refresh()
	title := i.bar.Objects[0].(*fyne.Container).Objects[0].(*draggableLabel)
	title.SetText(i.win.title)
	i.ShadowingRenderer.RefreshShadow()
}

var _ desktop.Mouseable = (*draggableLabel)(nil)
var _ fyne.Draggable = (*draggableLabel)(nil)
var _ fyne.Focusable = (*draggableLabel)(nil)

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

func (d *draggableLabel) MinSize() fyne.Size {
	width := d.Label.MinSize().Width
	height := d.Label.Theme().Size(theme.SizeNameWindowButtonHeight)
	return fyne.NewSize(width, height)
}

func (d *draggableLabel) FocusGained() {
}

func (d *draggableLabel) FocusLost() {
}

func (d *draggableLabel) TypedKey(ev *fyne.KeyEvent) {
	if obj, ok := d.win.content.Objects[0].(fyne.Focusable); ok {
		obj.TypedKey(ev)
	}
}

func (d *draggableLabel) TypedRune(r rune) {
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

func (d *draggableLabel) labelMinSize() fyne.Size {
	return d.Label.MinSize()
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
	prop.ScaleMode = canvas.ImageScaleFastest
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

type borderButton struct {
	widget.BaseWidget

	b    *widget.Button
	c    *container.ThemeOverride
	mode titleBarButtonMode
}

func newBorderButton(icon fyne.Resource, mode titleBarButtonMode, th fyne.Theme, fn func()) *borderButton {
	buttonImportance := widget.MediumImportance
	if mode == modeIcon {
		buttonImportance = widget.LowImportance
	}
	b := &widget.Button{Icon: icon, Importance: buttonImportance, OnTapped: fn}
	c := container.NewThemeOverride(b, &buttonTheme{Theme: th, mode: mode})

	ret := &borderButton{b: b, c: c, mode: mode}
	ret.ExtendBaseWidget(ret)
	return ret
}

func (b *borderButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.c)
}

func (b *borderButton) Disable() {
	b.b.Disable()
}

func (b *borderButton) MinSize() fyne.Size {
	height := b.Theme().Size(theme.SizeNameWindowButtonHeight)
	return fyne.NewSquareSize(height)
}

func (b *borderButton) setTheme(th fyne.Theme, active bool) {
	b.c.Theme = &buttonTheme{Theme: th, mode: b.mode, active: active}
}

type buttonTheme struct {
	fyne.Theme
	mode   titleBarButtonMode
	active bool
}

func (b *buttonTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameButton, theme.ColorNameDisabledButton:
		if b.active {
			n = theme.ColorNamePrimary
		} else {
			n = theme.ColorNameOverlayBackground
		}
	case theme.ColorNameHover:
		if b.mode == modeClose {
			n = theme.ColorNameError
		} else {
			if b.active {
				n = fyne.ThemeColorName("primary-hover")
			} else {
				n = theme.ColorNameHover
			}
		}
	}
	return b.Theme.Color(n, v)
}

func (b *buttonTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameInputRadius:
		if b.mode == modeIcon {
			return 0
		}
		n = theme.SizeNameWindowButtonRadius
	case theme.SizeNameInlineIcon:
		n = theme.SizeNameWindowButtonIcon
	}

	return b.Theme.Size(n)
}
