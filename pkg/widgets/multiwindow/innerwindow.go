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

// minimizedWidth is the fixed width of a window collapsed into the bottom tray.
const minimizedWidth float32 = 200

type resizeDirection int

const (
	resizeUp resizeDirection = iota
	resizeDown
	resizeLeft
	resizeRight
	resizeDownLeft
	resizeDownRight
	resizeUpLeft
	resizeUpRight
)

// InnerWindow defines a container that wraps content in a window border - that can then be placed inside
// a regular container/canvas.
type InnerWindow struct {
	widget.BaseWidget

	// ButtonAlignment specifies where the window buttons (close, minimize, maximize) should be placed.
	// The default is widget.ButtonAlignCenter which will auto select based on the OS.
	//	- On Darwin this will be `widget.ButtonAlignLeading`
	//	- On all other OS this will be `widget.ButtonAlignTrailing`
	Alignment widget.ButtonAlign
	OnClose   func()                `json:"-"`
	OnDragged func(*fyne.DragEvent) `json:"-"`
	// OnResized is called while a border resize drag is in progress. The
	// event's Dragged field holds the TOTAL delta since the drag started, not
	// the per-event delta; combine it with dragStartPos/dragStartSize.
	OnResized                                           func(resizeDirection, *fyne.DragEvent) `json:"-"`
	OnMinimized, OnMaximized, OnTappedBar, OnTappedIcon func()                                 `json:"-"`
	OnMouseDown                                         func()                                 `json:"-"`
	Icon                                                fyne.Resource

	DisableResize bool // Allow resizing
	Persist       bool // Persist through layout changes
	IgnoreSave    bool // Ignore saving to layout

	// minBtn, maxBtn, closeBtn *borderButton

	title       string
	bg          *canvas.Rectangle
	bgFillColor fyne.ThemeColorName
	content     *fyne.Container

	maximized bool
	minimized bool
	active    bool

	preMaximizedSize fyne.Size
	preMaximizedPos  fyne.Position

	preMinimizedSize fyne.Size
	preMinimizedPos  fyne.Position

	// window rect at the start of the current border-resize drag; OnResized
	// computes the new rect from these plus the total drag delta.
	dragStartPos  fyne.Position
	dragStartSize fyne.Size

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
		// bar = container.NewBorder(nil, nil, buttons, borderIcon, title)
		bar = container.NewBorder(nil, nil, buttons, borderIcon, container.New(layout.NewCustomPaddedLayout(topPad, 0, 0, 0), title))
	} else {
		// Right side (Windows/Linux default and explicit right alignment)
		buttons = container.NewHBox(min, max, close)
		// bar = container.NewBorder(nil, nil, borderIcon, buttons, title)
		bar = container.NewBorder(nil, nil, borderIcon, buttons, container.New(layout.NewCustomPaddedLayout(topPad, 0, 0, 0), title))
	}

	v := fyne.CurrentApp().Settings().ThemeVariant()
	w.bg = canvas.NewRectangle(th.Color(theme.ColorNameOverlayBackground, v))
	w.bg.CornerRadius = 4
	contentBG := canvas.NewRectangle(th.Color(theme.ColorNameBackground, v))

	var topBorder, bottomBorder, leftBorder, rightBorder *draggableBorder
	var borders []fyne.CanvasObject

	objects := []fyne.CanvasObject{w.bg, contentBG, bar, w.content}

	if !w.DisableResize {
		topBorder = newDraggableBorder(w, resizeUp)
		bottomBorder = newDraggableBorder(w, resizeDown)
		leftBorder = newDraggableBorder(w, resizeLeft)
		rightBorder = newDraggableBorder(w, resizeRight)

		borders = []fyne.CanvasObject{topBorder, bottomBorder, leftBorder, rightBorder}
		objects = append(objects, borders...)
	}

	r := &innerWindowRenderer{
		ShadowingRenderer: NewShadowingRenderer(objects, SubmergedContentLevel),
		win:               w,
		bar:               bar,
		title:             title,
		buttons:           []*borderButton{min, max, close},
		bg:                w.bg,
		topBorder:         topBorder,
		bottomBorder:      bottomBorder,
		leftBorder:        leftBorder,
		rightBorder:       rightBorder,
		borders:           borders,
		contentBG:         contentBG,
	}
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
	win     *InnerWindow
	bar     *fyne.Container
	title   *draggableLabel
	buttons []*borderButton

	bg, contentBG *canvas.Rectangle

	topBorder    fyne.CanvasObject
	bottomBorder fyne.CanvasObject
	leftBorder   fyne.CanvasObject
	rightBorder  fyne.CanvasObject

	borders []fyne.CanvasObject // all border handles, for show/hide

	*ShadowingRenderer
}

// Layout arranges the window chrome. The visible window (bg) fills the widget
// rect exactly: the resize strips straddle its edges, the shadow hugs it, and
// the title bar and content sit inside with symmetric padding insets.
func (i *innerWindowRenderer) Layout(size fyne.Size) {
	th := i.win.Theme()
	padding := th.Size(theme.SizeNamePadding)
	barHeight := th.Size(theme.SizeNameWindowTitleBarHeight)

	// Shadow and background wrap the full widget rect.
	i.LayoutShadow(size, fyne.Position{})
	i.bg.Resize(size)

	// Title bar: full width minus a padding inset on each side.
	i.bar.Move(fyne.NewPos(padding, 0))
	i.bar.Resize(fyne.NewSize(size.Width-2*padding, barHeight))

	// When minimized only the title bar is shown (tray-style).
	if i.win.minimized {
		i.contentBG.Hide()
		i.win.content.Hide()
		i.setBordersVisible(false)
		return
	}
	i.contentBG.Show()
	i.win.content.Show()

	// Content: below the bar, padding inset on the left, right and bottom.
	contentPos := fyne.NewPos(padding, barHeight)
	contentDimensions := fyne.NewSize(size.Width-2*padding, size.Height-barHeight-padding)

	i.contentBG.Move(contentPos)
	i.contentBG.Resize(contentDimensions)
	i.win.content.Move(contentPos)
	i.win.content.Resize(contentDimensions)

	// Layout resize handles
	if !i.win.DisableResize {
		i.setBordersVisible(true)
		i.layoutResizeHandles(size)
	}
}

func (i *innerWindowRenderer) setBordersVisible(visible bool) {
	for _, b := range i.borders { // empty when DisableResize
		if visible {
			b.Show()
		} else {
			b.Hide()
		}
	}
}

// Resize handle geometry. Each window edge gets one strip, centered on the
// edge. Like native Windows, the last resizeCornerZone units at either end of
// a strip act as a diagonal (corner) resize. These two constants are the only
// knobs — everything in layoutResizeHandles is derived from them.
const (
	resizeBorderThickness float32 = 8
	resizeCornerZone      float32 = 20
)

// layoutResizeHandles positions the four edge drag handles. Top and bottom
// strips span the full width; left and right fill the space between them so
// no two strips overlap.
func (i *innerWindowRenderer) layoutResizeHandles(size fyne.Size) {
	const (
		t        = resizeBorderThickness
		overhang = t / 2 // strips straddle the window edge
	)
	hEdge := fyne.NewSize(size.Width, t)
	vEdge := fyne.NewSize(t, size.Height-2*t)

	i.topBorder.Move(fyne.Position{X: 0, Y: -overhang})
	i.topBorder.Resize(hEdge)

	i.bottomBorder.Move(fyne.Position{X: 0, Y: size.Height - overhang})
	i.bottomBorder.Resize(hEdge)

	i.leftBorder.Move(fyne.Position{X: -overhang, Y: t - overhang})
	i.leftBorder.Resize(vEdge)

	i.rightBorder.Move(fyne.Position{X: size.Width - overhang, Y: t - overhang})
	i.rightBorder.Resize(vEdge)
}

// MinSize mirrors Layout: bar height plus content height plus the bottom
// padding inset, and the wider of bar/content plus the side insets. At this
// size the content gets exactly its own MinSize.
func (i *innerWindowRenderer) MinSize() fyne.Size {
	th := i.win.Theme()
	pad := th.Size(theme.SizeNamePadding)
	barHeight := th.Size(theme.SizeNameWindowTitleBarHeight)
	if i.win.minimized {
		return fyne.NewSize(minimizedWidth, barHeight)
	}
	contentMin := i.win.content.MinSize()
	innerWidth := max(i.bar.MinSize().Width, contentMin.Width)
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
	if i.title.Text != i.win.title {
		i.title.SetText(i.win.title)
	}
	i.ShadowingRenderer.RefreshShadow()
}

var (
	_ desktop.Mouseable = (*draggableLabel)(nil)
	_ fyne.Draggable    = (*draggableLabel)(nil)
	_ fyne.Focusable    = (*draggableLabel)(nil)
)

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
		//if b.mode == modeIcon {
		//	return 4
		//}
		//n = theme.SizeNameWindowButtonRadius
		return 4
	case theme.SizeNameInlineIcon:
		// n = theme.SizeNameWindowButtonIcon
		return 20
	}

	return b.Theme.Size(n)
}

var (
	_ desktop.Cursorable = (*draggableBorder)(nil)
	_ desktop.Hoverable  = (*draggableBorder)(nil)
	_ fyne.Draggable     = (*draggableBorder)(nil)
)

// draggableBorder is one resize strip along a window edge. The ends of the
// strip (resizeCornerZone units) resize diagonally, like native Windows
// borders; the middle resizes along the edge's axis only.
type draggableBorder struct {
	widget.BaseWidget
	win      *InnerWindow
	rect     *canvas.Rectangle
	edge     resizeDirection // resizeUp/Down/Left/Right: which window edge this strip sits on
	dir      resizeDirection // current direction, updated on hover, locked while dragging
	dragging bool
	pressAbs fyne.Position // mouse position at drag start, anchor for total-delta resizing
}

func newDraggableBorder(w *InnerWindow, edge resizeDirection) *draggableBorder {
	d := &draggableBorder{win: w, edge: edge, dir: edge}
	d.ExtendBaseWidget(d)
	d.rect = canvas.NewRectangle(color.RGBA{0, 0, 0, 0})
	return d
}

// dirAt maps a position within the strip to a resize direction: the edge's own
// direction in the middle, a diagonal within resizeCornerZone of either end.
func (d *draggableBorder) dirAt(pos fyne.Position) resizeDirection {
	switch d.edge {
	case resizeUp, resizeDown:
		if pos.X < resizeCornerZone {
			if d.edge == resizeUp {
				return resizeUpLeft
			}
			return resizeDownLeft
		}
		if pos.X > d.Size().Width-resizeCornerZone {
			if d.edge == resizeUp {
				return resizeUpRight
			}
			return resizeDownRight
		}
	case resizeLeft, resizeRight:
		if pos.Y < resizeCornerZone {
			if d.edge == resizeLeft {
				return resizeUpLeft
			}
			return resizeUpRight
		}
		if pos.Y > d.Size().Height-resizeCornerZone {
			if d.edge == resizeLeft {
				return resizeDownLeft
			}
			return resizeDownRight
		}
	}
	return d.edge
}

func (d *draggableBorder) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(d.rect)
}

func (d *draggableBorder) Cursor() desktop.Cursor {
	switch d.dir {
	case resizeUp, resizeDown:
		return desktop.VResizeCursor
	case resizeLeft, resizeRight:
		return desktop.HResizeCursor
	case resizeDownLeft, resizeUpRight:
		return desktop.NESWResizeCursor
	case resizeDownRight, resizeUpLeft:
		return desktop.NWSEResizeCursor
	}
	return desktop.DefaultCursor
}

func (d *draggableBorder) MouseIn(ev *desktop.MouseEvent) {
	d.MouseMoved(ev)
}

func (d *draggableBorder) MouseMoved(ev *desktop.MouseEvent) {
	if !d.dragging {
		d.dir = d.dirAt(ev.Position)
	}
}

func (d *draggableBorder) MouseOut() {
}

func (d *draggableBorder) Dragged(ev *fyne.DragEvent) {
	if !d.dragging {
		d.dragging = true // lock dir so the corner grab survives leaving the zone
		// Anchor the resize at the press point and the window rect at that
		// moment. Reporting total travel from this anchor (instead of
		// per-event deltas) makes min-size clamping absorb overshoot: after
		// shrinking past the limit the edge won't move again until the mouse
		// comes back to where the limit was hit, like native window borders.
		d.pressAbs = fyne.NewPos(ev.AbsolutePosition.X-ev.Dragged.DX, ev.AbsolutePosition.Y-ev.Dragged.DY)
		d.win.dragStartPos = d.win.Position()
		d.win.dragStartSize = d.win.Size()
	}
	if f := d.win.OnResized; f != nil {
		total := *ev
		total.Dragged = fyne.Delta{
			DX: ev.AbsolutePosition.X - d.pressAbs.X,
			DY: ev.AbsolutePosition.Y - d.pressAbs.Y,
		}
		f(d.dir, &total)
	}
}

// MouseDown is called when the user presses the mouse button on the border.
func (d *draggableBorder) MouseDown(*desktop.MouseEvent) {
	if f := d.win.OnMouseDown; f != nil {
		f()
	}
}

func (d *draggableBorder) MouseUp(*desktop.MouseEvent) {
}

func (d *draggableBorder) DragEnd() {
	d.dragging = false
}
