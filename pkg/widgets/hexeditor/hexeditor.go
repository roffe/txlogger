// Package hexeditor is a hex viewer/editor over the raw bytes of the loaded
// binary. Rows are virtualized through widget.List so a 1MB T8 bin only
// renders what is on screen. Editing works in both columns: in the hex
// column type hex digits to change a byte nibble by nibble, in the ascii
// column type characters directly. The cursor byte is highlighted in both
// columns; click a column (or press Tab) to switch which one edits — the
// active column's highlight carries a frame. Backspace
// undoes one byte at a time. The bytes-per-row width is selectable.
package hexeditor

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const defaultBytesPerRow = 16

// line layout: "00000000  " (10) + bpr×"XX " with an extra space after every
// 8th byte + "|" + bpr ascii chars + "|".

// hexX returns the character column of byte i's hex pair in a rendered line.
func hexX(i int) int { return 10 + 3*i + i/8 }

// asciiStartFor returns the character column of the first ascii char for a
// given bytes-per-row width.
func asciiStartFor(bpr int) int { return 10 + 3*bpr + (bpr-1)/8 + 1 }

func lineCharsFor(bpr int) int { return asciiStartFor(bpr) + bpr + 1 }

type Config struct {
	Data     []byte                   // live backing slice of the binary
	OnEdit   func(offset int, b byte) // called after every byte change, undo included
	OnSave   func() error             // persist Data
	SymbolAt func(offset int) string  // optional symbol name for the status bar
}

type undoEntry struct {
	off int
	old byte
}

type HexEditor struct {
	widget.BaseWidget
	cfg Config

	list    *widget.List
	status  *widget.Label
	header  *canvas.Text
	content fyne.CanvasObject

	bpr    int // bytes per row
	charW  float32
	rowMin fyne.Size

	cursor    int
	nibble    int // 0 = editing high nibble
	asciiMode bool
	focused   bool
	dirty     bool
	undo      []undoEntry
}

var _ fyne.Focusable = (*HexEditor)(nil)

func New(cfg Config) *HexEditor {
	he := &HexEditor{cfg: cfg, bpr: defaultBytesPerRow}
	he.ExtendBaseWidget(he)
	he.recalcRowMin()

	he.status = widget.NewLabel("")

	he.list = widget.NewList(
		func() int { return (len(he.cfg.Data) + he.bpr - 1) / he.bpr },
		func() fyne.CanvasObject { return newHexRow(he) },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			r := o.(*hexRow)
			r.row = id
			r.Refresh()
		},
	)

	gotoEntry := widget.NewEntry()
	gotoEntry.SetPlaceHolder("goto addr (hex)")
	gotoEntry.OnSubmitted = he.gotoAddr

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("find hex bytes or text")
	searchEntry.OnSubmitted = he.search

	searchBtn := widget.NewButtonWithIcon("", theme.SearchIcon(), func() { he.search(searchEntry.Text) })
	undoBtn := widget.NewButtonWithIcon("Undo", theme.ContentUndoIcon(), he.undoOne)
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), he.save)

	widthSelect := widget.NewSelect([]string{"8", "16", "24", "32"}, func(s string) {
		if n, err := strconv.Atoi(s); err == nil {
			he.setBytesPerRow(n)
		}
	})
	widthSelect.SetSelected(strconv.Itoa(he.bpr))

	bar := container.NewBorder(nil, nil, nil,
		container.NewHBox(widthSelect, undoBtn, saveBtn),
		container.NewGridWithColumns(2,
			gotoEntry,
			container.NewBorder(nil, nil, nil, searchBtn, searchEntry),
		),
	)

	he.header = canvas.NewText(he.columnHeader(), theme.Color(theme.ColorNameForeground))
	he.header.TextStyle.Monospace = true
	he.header.TextSize = theme.TextSize()

	he.content = container.NewBorder(container.NewVBox(bar, he.header), he.status, nil, nil, he.list)
	he.updateStatus()
	return he
}

func (he *HexEditor) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(he.content)
}

func (he *HexEditor) recalcRowMin() {
	ts := theme.TextSize()
	n := lineCharsFor(he.bpr)
	sz := fyne.MeasureText(strings.Repeat("0", n), ts, fyne.TextStyle{Monospace: true})
	he.charW = sz.Width / float32(n)
	he.rowMin = fyne.NewSize(sz.Width+theme.Padding(), sz.Height+2)
}

func (he *HexEditor) setBytesPerRow(n int) {
	if n <= 0 || n == he.bpr {
		return
	}
	off := he.cursor
	he.bpr = n
	he.recalcRowMin()
	he.header.Text = he.columnHeader()
	he.header.Refresh()
	he.list.Refresh() // re-measures the item template, so rows pick up the new width
	he.setCursor(off)
}

func (he *HexEditor) columnHeader() string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", 10))
	for i := range he.bpr {
		if i > 0 && i%8 == 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02X ", i)
	}
	return b.String()
}

// line renders one row of the hex dump.
func (he *HexEditor) line(row int) string {
	data := he.cfg.Data
	off := row * he.bpr
	if off < 0 || off >= len(data) {
		return ""
	}
	var b strings.Builder
	b.Grow(lineCharsFor(he.bpr))
	fmt.Fprintf(&b, "%08X  ", off)
	for i := range he.bpr {
		if i > 0 && i%8 == 0 {
			b.WriteByte(' ')
		}
		if off+i < len(data) {
			fmt.Fprintf(&b, "%02X ", data[off+i])
		} else {
			b.WriteString("   ")
		}
	}
	b.WriteByte('|')
	for i := 0; i < he.bpr && off+i < len(data); i++ {
		c := data[off+i]
		if c < 0x20 || c > 0x7E {
			c = '.'
		}
		b.WriteByte(c)
	}
	b.WriteByte('|')
	return b.String()
}

func (he *HexEditor) setCursor(off int) {
	if len(he.cfg.Data) == 0 {
		return
	}
	off = min(max(off, 0), len(he.cfg.Data)-1)
	oldRow := he.cursor / he.bpr
	he.cursor = off
	he.nibble = 0
	if oldRow != off/he.bpr {
		he.list.RefreshItem(oldRow)
	}
	he.list.ScrollTo(off / he.bpr)
	he.list.RefreshItem(off / he.bpr)
	he.updateStatus()
}

func (he *HexEditor) updateStatus() {
	if len(he.cfg.Data) == 0 {
		he.status.SetText("no data")
		return
	}
	mode := "hex"
	if he.asciiMode {
		mode = "ascii"
	}
	b := he.cfg.Data[he.cursor]
	s := fmt.Sprintf("[%s]  $%06X  %02X (%d)", mode, he.cursor, b, b)
	if he.cfg.SymbolAt != nil {
		if n := he.cfg.SymbolAt(he.cursor); n != "" {
			s += "  " + n
		}
	}
	if he.dirty {
		s += "  •unsaved"
	}
	he.status.SetText(s)
}

func (he *HexEditor) gotoAddr(in string) {
	s := strings.TrimSpace(strings.ToLower(in))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "$")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		he.status.SetText("bad address: " + in)
		return
	}
	he.setCursor(int(v))
}

// parsePattern turns the search input into bytes: hex pairs when the input
// (spaces stripped) is valid hex, the literal ascii bytes otherwise.
func parsePattern(q string) []byte {
	s := strings.ReplaceAll(strings.TrimSpace(q), " ", "")
	if s == "" {
		return nil
	}
	if b, err := hex.DecodeString(s); err == nil {
		return b
	}
	return []byte(q)
}

func (he *HexEditor) search(q string) {
	pat := parsePattern(q)
	if len(pat) == 0 || len(he.cfg.Data) == 0 {
		return
	}
	start := he.cursor + 1
	if start < len(he.cfg.Data) {
		if idx := bytes.Index(he.cfg.Data[start:], pat); idx >= 0 {
			he.setCursor(start + idx)
			return
		}
	}
	// wrap around
	if idx := bytes.Index(he.cfg.Data[:min(len(he.cfg.Data), start+len(pat)-1)], pat); idx >= 0 {
		he.setCursor(idx)
		return
	}
	he.status.SetText("not found: " + q)
}

func (he *HexEditor) edit(off int, b byte) {
	he.cfg.Data[off] = b
	he.dirty = true
	if he.cfg.OnEdit != nil {
		he.cfg.OnEdit(off, b)
	}
}

func (he *HexEditor) undoOne() {
	n := len(he.undo)
	if n == 0 {
		return
	}
	e := he.undo[n-1]
	he.undo = he.undo[:n-1]
	he.edit(e.off, e.old)
	he.setCursor(e.off)
}

func (he *HexEditor) save() {
	if he.cfg.OnSave == nil {
		return
	}
	if err := he.cfg.OnSave(); err != nil {
		he.status.SetText("save failed: " + err.Error())
		return
	}
	he.dirty = false
	he.updateStatus()
}

func (he *HexEditor) FocusGained() { he.focused = true }
func (he *HexEditor) FocusLost()   { he.focused = false }

func (he *HexEditor) TypedRune(r rune) {
	if len(he.cfg.Data) == 0 {
		return
	}
	off := he.cursor

	if he.asciiMode {
		if r < 0x20 || r > 0x7E {
			return
		}
		he.undo = append(he.undo, undoEntry{off, he.cfg.Data[off]})
		he.edit(off, byte(r))
		he.setCursor(off + 1)
		return
	}

	var v byte
	switch {
	case r >= '0' && r <= '9':
		v = byte(r - '0')
	case r >= 'a' && r <= 'f':
		v = byte(r-'a') + 10
	case r >= 'A' && r <= 'F':
		v = byte(r-'A') + 10
	default:
		return
	}
	b := he.cfg.Data[off]
	if he.nibble == 0 {
		he.undo = append(he.undo, undoEntry{off, b})
		he.edit(off, b&0x0F|v<<4)
		he.nibble = 1
		he.list.RefreshItem(off / he.bpr)
		he.updateStatus()
		return
	}
	he.edit(off, b&0xF0|v)
	he.setCursor(off + 1) // low nibble typed: advance (also resets nibble)
}

func (he *HexEditor) TypedKey(k *fyne.KeyEvent) {
	ctrl := false
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		ctrl = d.CurrentKeyModifiers()&fyne.KeyModifierControl != 0
	}
	rowStart := he.cursor / he.bpr * he.bpr
	switch k.Name {
	case fyne.KeyLeft:
		he.setCursor(he.cursor - 1)
	case fyne.KeyRight:
		he.setCursor(he.cursor + 1)
	case fyne.KeyUp:
		he.setCursor(he.cursor - he.bpr)
	case fyne.KeyDown:
		he.setCursor(he.cursor + he.bpr)
	case fyne.KeyPageUp:
		he.setCursor(he.cursor - he.bpr*32)
	case fyne.KeyPageDown:
		he.setCursor(he.cursor + he.bpr*32)
	case fyne.KeyHome:
		if ctrl {
			he.setCursor(0)
		} else {
			he.setCursor(rowStart)
		}
	case fyne.KeyEnd:
		if ctrl {
			he.setCursor(len(he.cfg.Data) - 1)
		} else {
			he.setCursor(rowStart + he.bpr - 1)
		}
	case fyne.KeyTab:
		he.asciiMode = !he.asciiMode
		he.setCursor(he.cursor) // refreshes cursor row and status
	case fyne.KeyBackspace:
		he.undoOne()
	}
}

// hexRow is one virtualized dump line; it reads everything from its editor.
type hexRow struct {
	widget.BaseWidget
	he  *HexEditor
	row int

	text     *canvas.Text
	selHex   *canvas.Rectangle
	selASCII *canvas.Rectangle
}

var _ fyne.Tappable = (*hexRow)(nil)

func newHexRow(he *HexEditor) *hexRow {
	r := &hexRow{
		he:       he,
		row:      -1,
		text:     canvas.NewText("", theme.Color(theme.ColorNameForeground)),
		selHex:   canvas.NewRectangle(theme.Color(theme.ColorNameSelection)),
		selASCII: canvas.NewRectangle(theme.Color(theme.ColorNameSelection)),
	}
	r.text.TextStyle.Monospace = true
	r.ExtendBaseWidget(r)
	return r
}

// tapCol maps a tapped character column to a byte column, and reports whether
// the tap landed in the ascii section. ok is false outside both sections.
func tapCol(ci, bpr int) (col int, ascii, ok bool) {
	as := asciiStartFor(bpr)
	switch {
	case ci >= as && ci < as+bpr:
		return ci - as, true, true
	case ci >= 10 && ci < as-1:
		adj := ci - 10
		group := adj / 25         // 8 bytes + gap = 25 chars
		within := min(adj%25, 23) // a tap on the gap counts as the group's last byte
		return group*8 + within/3, false, true
	}
	return 0, false, false
}

func (r *hexRow) Tapped(e *fyne.PointEvent) {
	he := r.he
	if r.row < 0 || len(he.cfg.Data) == 0 {
		return
	}
	col, ascii, ok := tapCol(int(e.Position.X/he.charW), he.bpr)
	if !ok {
		return
	}
	off := r.row*he.bpr + col
	if off >= len(he.cfg.Data) {
		return
	}
	he.asciiMode = ascii
	if c := fyne.CurrentApp().Driver().CanvasForObject(r); c != nil {
		c.Focus(he)
	}
	he.setCursor(off)
}

func (r *hexRow) CreateRenderer() fyne.WidgetRenderer {
	return &hexRowRenderer{r: r}
}

type hexRowRenderer struct{ r *hexRow }

func (rr *hexRowRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{rr.r.selHex, rr.r.selASCII, rr.r.text}
}
func (rr *hexRowRenderer) MinSize() fyne.Size { return rr.r.he.rowMin }
func (rr *hexRowRenderer) Destroy()           {}
func (rr *hexRowRenderer) Layout(fyne.Size)   { rr.update() }

func (rr *hexRowRenderer) Refresh() {
	rr.update()
	canvas.Refresh(rr.r)
}

func (rr *hexRowRenderer) update() {
	r := rr.r
	he := r.he
	r.text.Text = he.line(r.row)
	r.text.Color = theme.Color(theme.ColorNameForeground)
	r.text.TextSize = theme.TextSize()
	r.text.Move(fyne.NewPos(0, 0))
	r.text.Resize(he.rowMin)

	if len(he.cfg.Data) > 0 && he.cursor/he.bpr == r.row {
		col := he.cursor % he.bpr
		h := he.rowMin.Height
		sel := theme.Color(theme.ColorNameSelection)
		// both columns highlight the cursor byte; the active one is framed
		active, passive := r.selHex, r.selASCII
		if he.asciiMode {
			active, passive = r.selASCII, r.selHex
		}
		active.FillColor = sel
		active.StrokeColor = theme.Color(theme.ColorNamePrimary)
		active.StrokeWidth = 1.5
		passive.FillColor = sel
		passive.StrokeColor = color.Transparent
		passive.StrokeWidth = 0
		r.selHex.Move(fyne.NewPos(he.charW*float32(hexX(col)), 0))
		r.selHex.Resize(fyne.NewSize(he.charW*2, h))
		r.selASCII.Move(fyne.NewPos(he.charW*float32(asciiStartFor(he.bpr)+col), 0))
		r.selASCII.Resize(fyne.NewSize(he.charW, h))
		r.selHex.Show()
		r.selASCII.Show()
	} else {
		r.selHex.Hide()
		r.selASCII.Hide()
	}
}
