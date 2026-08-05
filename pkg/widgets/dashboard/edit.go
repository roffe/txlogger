package dashboard

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
)

// --- edit mode ------------------------------------------------------------

func (db *Dashboard) toggleEditMode() {
	if db.editMode {
		db.exitEdit()
	} else {
		db.enterEdit()
	}
}

// Edit mode never touches item visibility: warning lights stay owned by the
// metric router, and each item's labeled handle shows where it sits even
// when the element itself is currently hidden.
func (db *Dashboard) enterEdit() {
	db.editMode = true
	db.toolbar = db.buildToolbar()
	db.rebuildHandles()
	db.rebuildGridLines()
	db.applyLayout()
	db.Refresh()
}

func (db *Dashboard) exitEdit() {
	db.editMode = false
	db.handles = nil
	db.gridLines = nil
	db.toolbar = nil
	db.layout.save()
	db.gen++
	db.Refresh()
}

func (db *Dashboard) rebuildHandles() {
	db.handles = db.handles[:0]
	for i := range db.layout.Items {
		it := &db.layout.Items[i]
		if db.itemObject(it) == nil {
			continue
		}
		db.handles = append(db.handles, newItemHandle(db, it.ID))
	}
	db.gen++
}

func (db *Dashboard) rebuildGridLines() {
	if db.size.Width <= 0 || db.size.Height <= 0 {
		return
	}
	cw, ch := db.cellSize()
	lineColor := color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x50}
	lines := make([]fyne.CanvasObject, 0, db.layout.Cols+db.layout.Rows+2)
	for i := 0; i <= db.layout.Cols; i++ {
		l := canvas.NewLine(lineColor)
		x := float32(i) * cw
		l.Position1 = fyne.NewPos(x, 0)
		l.Position2 = fyne.NewPos(x, db.size.Height)
		lines = append(lines, l)
	}
	for i := 0; i <= db.layout.Rows; i++ {
		l := canvas.NewLine(lineColor)
		y := float32(i) * ch
		l.Position1 = fyne.NewPos(0, y)
		l.Position2 = fyne.NewPos(db.size.Width, y)
		lines = append(lines, l)
	}
	db.gridLines = lines
	db.gen++
}

// layoutEditOverlay positions the handles and toolbar; called from applyLayout.
func (db *Dashboard) layoutEditOverlay() {
	cw, ch := db.cellSize()
	for _, h := range db.handles {
		it := db.layout.item(h.id)
		if it == nil {
			continue
		}
		h.Resize(fyne.NewSize(float32(it.W)*cw, float32(it.H)*ch))
		h.Move(fyne.NewPos(float32(it.X)*cw, float32(it.Y)*ch))
	}
	if db.toolbar != nil {
		ms := db.toolbar.MinSize()
		db.toolbar.Resize(ms)
		db.toolbar.Move(fyne.NewPos(0, db.size.Height-ms.Height))
	}
}

func (db *Dashboard) buildToolbar() *fyne.Container {
	addBtn := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), nil)
	addBtn.OnTapped = func() { db.showAddMenu(addBtn) }

	presetSel := widget.NewSelect(listPresets(), nil)
	presetSel.PlaceHolder = "Load preset"
	presetSel.OnChanged = func(name string) {
		if name == "" {
			return
		}
		db.applyPreset(name)
		presetSel.ClearSelected()
	}

	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		db.savePresetDialog(presetSel)
	})
	resetBtn := widget.NewButtonWithIcon("Reset", theme.ViewRefreshIcon(), db.resetLayout)
	doneBtn := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), db.toggleEditMode)

	return container.NewHBox(addBtn, presetSel, saveBtn, resetBtn, doneBtn)
}

func (db *Dashboard) showAddMenu(anchor fyne.CanvasObject) {
	var items []*fyne.MenuItem
	for i := range itemDefs {
		def := itemDefs[i]
		if db.layout.item(def.id) != nil {
			continue
		}
		probe := Item{ID: def.id}
		if db.itemObject(&probe) == nil {
			continue
		}
		items = append(items, fyne.NewMenuItem(def.label, func() { db.addItem(def) }))
	}
	if len(items) == 0 {
		empty := fyne.NewMenuItem("(everything is placed)", nil)
		empty.Disabled = true
		items = append(items, empty)
	}
	drv := fyne.CurrentApp().Driver()
	c := drv.CanvasForObject(db)
	if c == nil {
		return
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), c, drv.AbsolutePositionForObject(anchor))
}

func (db *Dashboard) addItem(def itemDef) {
	w := common.Clamp(def.addW, 1, db.layout.Cols)
	h := common.Clamp(def.addH, 1, db.layout.Rows)
	it := Item{ID: def.id, X: (db.layout.Cols - w) / 2, Y: (db.layout.Rows - h) / 2, W: w, H: h}
	db.layout.Items = append(db.layout.Items, it)
	db.rebuildHandles()
	db.applyLayout()
	db.layout.save()
	db.Refresh()
}

func (db *Dashboard) removeItem(id string) {
	db.layout.remove(id)
	db.rebuildHandles()
	db.applyLayout()
	db.layout.save()
	db.Refresh()
}

func (db *Dashboard) setTextAlign(id, align string) {
	it := db.layout.item(id)
	if it == nil {
		return
	}
	it.Align = align
	db.applyLayout()
	db.layout.save()
	db.Refresh()
}

func (db *Dashboard) setWBLStyle(style string) {
	it := db.layout.item("wblambda")
	if it == nil {
		return
	}
	it.Style = style
	db.gen++
	db.applyLayout()
	db.layout.save()
	db.Refresh()
}

func (db *Dashboard) applyPreset(name string) {
	l, err := loadPreset(name)
	if err != nil {
		dialog.ShowError(err, db.window())
		return
	}
	db.layout = l
	db.rebuildHandles()
	db.rebuildGridLines()
	db.applyLayout()
	db.layout.save()
	db.Refresh()
}

func (db *Dashboard) resetLayout() {
	db.layout = defaultLayout()
	db.rebuildHandles()
	db.rebuildGridLines()
	db.applyLayout()
	db.layout.save()
	db.Refresh()
}

func (db *Dashboard) savePresetDialog(presetSel *widget.Select) {
	entry := widget.NewEntry()
	dialog.NewForm("Save dashboard preset", "Save", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			if err := db.layout.savePreset(entry.Text); err != nil {
				dialog.ShowError(err, db.window())
				return
			}
			presetSel.SetOptions(listPresets())
		}, db.window()).Show()
}

// window finds the fyne window the dashboard lives in, for dialogs.
func (db *Dashboard) window() fyne.Window {
	drv := fyne.CurrentApp().Driver()
	c := drv.CanvasForObject(db)
	for _, w := range drv.AllWindows() {
		if w.Canvas() == c {
			return w
		}
	}
	if ws := drv.AllWindows(); len(ws) > 0 {
		return ws[0]
	}
	return nil
}

// --- item handle ----------------------------------------------------------

const resizeZone = 20

// resizeCorner is the bottom-right grab area of a handle. It shrinks with the
// handle so the top-left two thirds of even a one-cell item still moves.
func resizeCorner(size fyne.Size) (float32, float32) {
	return min(resizeZone, size.Width/3), min(resizeZone, size.Height/3)
}

// itemHandle is the translucent drag overlay covering one layout item in
// edit mode: drag to move, drag the bottom-right corner to resize, tap for
// the remove/style menu.
type itemHandle struct {
	widget.BaseWidget
	db *Dashboard
	id string

	dragging, resizing             bool
	startX, startY, startW, startH int
	dx, dy                         float32

	fill   *canvas.Rectangle
	label  *canvas.Text
	corner *canvas.Rectangle
}

var (
	_ fyne.Draggable         = (*itemHandle)(nil)
	_ fyne.Tappable          = (*itemHandle)(nil)
	_ fyne.SecondaryTappable = (*itemHandle)(nil)
)

func newItemHandle(db *Dashboard, id string) *itemHandle {
	h := &itemHandle{db: db, id: id}
	h.fill = &canvas.Rectangle{
		FillColor:   color.NRGBA{R: 0x00, G: 0x85, B: 0xff, A: 0x20},
		StrokeColor: color.NRGBA{R: 0x00, G: 0x85, B: 0xff, A: 0xa0},
		StrokeWidth: 1,
	}
	name := id
	if def := itemDefByID(id); def != nil {
		name = def.label
	}
	h.label = canvas.NewText(name, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xc0})
	h.label.TextSize = 12
	h.corner = &canvas.Rectangle{FillColor: color.NRGBA{R: 0x00, G: 0x85, B: 0xff, A: 0xc0}}
	h.ExtendBaseWidget(h)
	return h
}

func (h *itemHandle) CreateRenderer() fyne.WidgetRenderer {
	return &itemHandleRenderer{h: h}
}

func (h *itemHandle) Dragged(ev *fyne.DragEvent) {
	it := h.db.layout.item(h.id)
	if it == nil {
		return
	}
	if !h.dragging {
		h.dragging = true
		h.startX, h.startY, h.startW, h.startH = it.X, it.Y, it.W, it.H
		sz := h.Size()
		press := ev.Position.Subtract(fyne.NewPos(ev.Dragged.DX, ev.Dragged.DY))
		zx, zy := resizeCorner(sz)
		h.resizing = press.X > sz.Width-zx && press.Y > sz.Height-zy
		h.dx, h.dy = 0, 0
	}
	h.dx += ev.Dragged.DX
	h.dy += ev.Dragged.DY
	cw, ch := h.db.cellSize()
	dxc := int(math.Round(float64(h.dx / cw)))
	dyc := int(math.Round(float64(h.dy / ch)))
	if h.resizing {
		it.W = common.Clamp(h.startW+dxc, 1, h.db.layout.Cols-it.X)
		it.H = common.Clamp(h.startH+dyc, 1, h.db.layout.Rows-it.Y)
	} else {
		it.X = common.Clamp(h.startX+dxc, 0, h.db.layout.Cols-it.W)
		it.Y = common.Clamp(h.startY+dyc, 0, h.db.layout.Rows-it.H)
	}
	h.db.applyLayout()
}

func (h *itemHandle) DragEnd() {
	h.dragging = false
	h.db.layout.save()
}

func (h *itemHandle) Tapped(ev *fyne.PointEvent) {
	h.showMenu(ev.AbsolutePosition)
}

func (h *itemHandle) TappedSecondary(ev *fyne.PointEvent) {
	h.showMenu(ev.AbsolutePosition)
}

func (h *itemHandle) showMenu(pos fyne.Position) {
	var items []*fyne.MenuItem
	if h.id == "wblambda" {
		if it := h.db.layout.item(h.id); it != nil {
			if it.Style == StyleGauge {
				items = append(items, fyne.NewMenuItem("Show as bar", func() { h.db.setWBLStyle(StyleBar) }))
			} else {
				items = append(items, fyne.NewMenuItem("Show as round gauge", func() { h.db.setWBLStyle(StyleGauge) }))
			}
		}
	}
	if it := h.db.layout.item(h.id); it != nil {
		if _, isText := h.db.itemObject(it).(*canvas.Text); isText {
			for _, a := range []struct{ label, val string }{
				{"Align left", AlignLeft},
				{"Align center", AlignCenter},
				{"Align right", AlignRight},
			} {
				mi := fyne.NewMenuItem(a.label, func() { h.db.setTextAlign(h.id, a.val) })
				mi.Checked = it.Align == a.val
				items = append(items, mi)
			}
		}
	}
	items = append(items, fyne.NewMenuItem("Remove", func() { h.db.removeItem(h.id) }))
	c := fyne.CurrentApp().Driver().CanvasForObject(h)
	if c == nil {
		return
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), c, pos)
}

type itemHandleRenderer struct{ h *itemHandle }

func (r *itemHandleRenderer) Layout(size fyne.Size) {
	r.h.fill.Resize(size)
	min := r.h.label.MinSize()
	r.h.label.Resize(min)
	r.h.label.Move(fyne.NewPos((size.Width-min.Width)/2, (size.Height-min.Height)/2))
	cx, cy := resizeCorner(size)
	r.h.corner.Resize(fyne.NewSize(cx, cy))
	r.h.corner.Move(fyne.NewPos(size.Width-cx, size.Height-cy))
}

func (r *itemHandleRenderer) MinSize() fyne.Size { return fyne.NewSize(10, 10) }

func (r *itemHandleRenderer) Refresh() {}

func (r *itemHandleRenderer) Destroy() {}

func (r *itemHandleRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.h.fill, r.h.label, r.h.corner}
}
