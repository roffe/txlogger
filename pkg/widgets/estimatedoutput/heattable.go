package estimatedoutput

import (
	"image/color"
	"slices"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

const (
	cellTextSize = 10
	cornerSize   = 6
)

// per-limiter corner marker colors, mirroring the suites' table triangles
// but drawn from the validated palette
var limiterColors = map[string]color.RGBA{
	limTorqueEngine:  {0xed, 0xa1, 0x00, 0xff}, // yellow
	limTorqueE85:     {0xe8, 0x7b, 0xa4, 0xff}, // magenta
	limTorqueE85Auto: {0x00, 0x83, 0x00, 0xff}, // green
	limTorqueGear:    {0xeb, 0x68, 0x34, 0xff}, // orange
	limOverboost:     {0x1b, 0xaf, 0x7a, 0xff}, // aqua
	limAirmass:       {0x2a, 0x78, 0xd6, 0xff}, // blue
	limTurboSpeed:    {0x4a, 0x3a, 0xa7, 0xff}, // violet
	limFuelCut:       {0x89, 0x87, 0x81, 0xff}, // gray
}

// heatTable renders a Table as a colored grid with the same value->color
// ramp the mapviewer uses, plus a corner marker per cell naming the active
// limiter.
type heatTable struct {
	widget.BaseWidget
	data     *Table
	mode     colors.ColorBlindMode
	renderer *heatRenderer
}

func newHeatTable(mode colors.ColorBlindMode) *heatTable {
	h := &heatTable{mode: mode}
	h.ExtendBaseWidget(h)
	return h
}

func (h *heatTable) SetData(t *Table) {
	h.data = t
	if h.renderer != nil {
		h.renderer.rebuild()
	}
	h.Refresh()
}

func (h *heatTable) CreateRenderer() fyne.WidgetRenderer {
	r := &heatRenderer{h: h}
	r.bg = &canvas.Rectangle{FillColor: dynoPlotBg}
	r.rebuild()
	h.renderer = r
	return r
}

var _ fyne.WidgetRenderer = (*heatRenderer)(nil)

type heatRenderer struct {
	h *heatTable

	bg        *canvas.Rectangle
	cells     [][]*canvas.Rectangle
	cellTexts [][]*canvas.Text
	corners   [][]*canvas.Rectangle
	colHdrs   []*canvas.Text
	rowHdrs   []*canvas.Text
	legend    []fyne.CanvasObject // swatch/text pairs for limiters present

	objects []fyne.CanvasObject
	size    fyne.Size
}

func (r *heatRenderer) rebuild() {
	d := r.h.data
	r.cells, r.cellTexts, r.corners = nil, nil, nil
	r.colHdrs, r.rowHdrs, r.legend = nil, nil, nil
	r.objects = r.objects[:0]
	r.objects = append(r.objects, r.bg)
	if d == nil || len(d.Cells) == 0 {
		r.size = fyne.Size{}
		return
	}
	rows, cols := len(d.Cells), len(d.Cols)
	var present []string
	for ri := 0; ri < rows; ri++ {
		var cr []*canvas.Rectangle
		var ct []*canvas.Text
		var cc []*canvas.Rectangle
		for ci := 0; ci < cols; ci++ {
			cr = append(cr, &canvas.Rectangle{})
			ct = append(ct, &canvas.Text{TextSize: cellTextSize, Color: dynoInkColor, Alignment: fyne.TextAlignCenter})
			corner := &canvas.Rectangle{}
			corner.Hide()
			cc = append(cc, corner)
			if d.Limiters != nil {
				if lim := d.Limiters[ri][ci]; lim != "" && !slices.Contains(present, lim) {
					present = append(present, lim)
				}
			}
		}
		r.cells = append(r.cells, cr)
		r.cellTexts = append(r.cellTexts, ct)
		r.corners = append(r.corners, cc)
	}
	for ci := 0; ci < cols; ci++ {
		r.colHdrs = append(r.colHdrs, &canvas.Text{TextSize: tickTextSize})
	}
	for ri := 0; ri < rows; ri++ {
		r.rowHdrs = append(r.rowHdrs, &canvas.Text{TextSize: tickTextSize})
	}
	slices.Sort(present)
	for _, lim := range present {
		sw := &canvas.Rectangle{FillColor: limiterColorFor(lim)}
		txt := &canvas.Text{Text: lim, TextSize: tickTextSize}
		r.legend = append(r.legend, sw, txt)
	}

	for ri := range r.cells {
		for ci := range r.cells[ri] {
			r.objects = append(r.objects, r.cells[ri][ci], r.cellTexts[ri][ci], r.corners[ri][ci])
		}
	}
	for _, o := range r.colHdrs {
		r.objects = append(r.objects, o)
	}
	for _, o := range r.rowHdrs {
		r.objects = append(r.objects, o)
	}
	r.objects = append(r.objects, r.legend...)
	r.size = fyne.Size{} // force relayout
}

func limiterColorFor(lim string) color.RGBA {
	if c, ok := limiterColors[lim]; ok {
		return c
	}
	return color.RGBA{0x89, 0x87, 0x81, 0xff}
}

func (r *heatRenderer) MinSize() fyne.Size { return fyne.NewSize(480, 300) }

func (r *heatRenderer) Destroy() {}

func (r *heatRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *heatRenderer) Layout(size fyne.Size) {
	if size == r.size {
		return
	}
	r.size = size
	r.relayout()
	r.refreshObjects()
}

func (r *heatRenderer) Refresh() {
	r.relayout()
	r.refreshObjects()
}

func (r *heatRenderer) refreshObjects() {
	for _, o := range r.objects {
		if o.Visible() {
			o.Refresh()
		}
	}
}

func (r *heatRenderer) relayout() {
	d := r.h.data
	size := r.size
	if d == nil || len(d.Cells) == 0 || size.Width <= 0 || size.Height <= 0 {
		return
	}
	rows, cols := len(d.Cells), len(d.Cols)
	style := fyne.TextStyle{}

	r.bg.Resize(size)

	hdrH := fyne.MeasureText("0", tickTextSize, style).Height
	var rowHdrW float32
	rowTexts := make([]string, rows)
	for ri, v := range d.Rows {
		rowTexts[ri] = strconv.FormatFloat(v, 'f', 0, 64)
		if s := fyne.MeasureText(rowTexts[ri], tickTextSize, style); s.Width > rowHdrW {
			rowHdrW = s.Width
		}
	}
	legendH := float32(0)
	if len(r.legend) > 0 {
		legendH = hdrH + 8
	}
	gridLeft := rowHdrW + 8
	gridTop := hdrH + 6
	gridW := size.Width - gridLeft - 4
	gridH := size.Height - gridTop - legendH - 4
	if gridW < 10 || gridH < 10 {
		return
	}
	cellW := gridW / float32(cols)
	cellH := gridH / float32(rows)

	zMin, zMax := d.Cells[0][0], d.Cells[0][0]
	for _, row := range d.Cells {
		for _, v := range row {
			zMin = min(zMin, v)
			zMax = max(zMax, v)
		}
	}
	if zMax <= zMin {
		zMax = zMin + 1
	}

	// column headers (RPM), skipping when too narrow
	var maxColW float32
	colTexts := make([]string, cols)
	for ci, v := range d.Cols {
		colTexts[ci] = strconv.FormatFloat(v, 'f', 0, 64)
		if s := fyne.MeasureText(colTexts[ci], tickTextSize, style); s.Width > maxColW {
			maxColW = s.Width
		}
	}
	hdrSkip := 1
	if maxColW+4 > cellW {
		hdrSkip = int(float64((maxColW+4)/cellW)) + 1
	}
	labelColor := dynoInkColor
	for ci := 0; ci < cols; ci++ {
		hdr := r.colHdrs[ci]
		if ci%hdrSkip != 0 {
			hdr.Hide()
			continue
		}
		hdr.Text = colTexts[ci]
		hdr.Color = labelColor
		s := fyne.MeasureText(hdr.Text, tickTextSize, style)
		hdr.Resize(s)
		hdr.Move(fyne.NewPos(gridLeft+float32(ci)*cellW+(cellW-s.Width)/2, 2))
		hdr.Show()
	}
	for ri := 0; ri < rows; ri++ {
		hdr := r.rowHdrs[ri]
		hdr.Text = rowTexts[ri]
		hdr.Color = labelColor
		s := fyne.MeasureText(hdr.Text, tickTextSize, style)
		hdr.Resize(s)
		hdr.Move(fyne.NewPos(rowHdrW-s.Width+2, gridTop+float32(ri)*cellH+(cellH-s.Height)/2))
	}

	showText := cellW >= 26 && cellH >= 13
	for ri := 0; ri < rows; ri++ {
		for ci := 0; ci < cols && ci < len(d.Cells[ri]); ci++ {
			x := gridLeft + float32(ci)*cellW
			y := gridTop + float32(ri)*cellH
			v := d.Cells[ri][ci]

			cell := r.cells[ri][ci]
			cell.FillColor = colors.GetColorInterpolation(zMin, zMax, v, r.h.mode)
			cell.Move(fyne.NewPos(x, y))
			cell.Resize(fyne.NewSize(cellW-2, cellH-2))

			txt := r.cellTexts[ri][ci]
			if !showText {
				txt.Hide()
			} else {
				txt.Text = strconv.FormatFloat(v, 'f', d.Precision, 64)
				s := fyne.MeasureText(txt.Text, cellTextSize, style)
				txt.Resize(fyne.NewSize(cellW-2, s.Height))
				txt.Move(fyne.NewPos(x, y+(cellH-2-s.Height)/2))
				txt.Show()
			}

			corner := r.corners[ri][ci]
			if d.Limiters == nil || d.Limiters[ri][ci] == "" {
				corner.Hide()
			} else {
				corner.FillColor = limiterColorFor(d.Limiters[ri][ci])
				corner.Move(fyne.NewPos(x+cellW-2-cornerSize, y))
				corner.Resize(fyne.NewSize(cornerSize, cornerSize))
				corner.Show()
			}
		}
	}

	// limiter legend along the bottom
	lx := gridLeft
	ly := size.Height - legendH + 4
	for i := 0; i < len(r.legend); i += 2 {
		sw := r.legend[i].(*canvas.Rectangle)
		txt := r.legend[i+1].(*canvas.Text)
		txt.Color = labelColor
		s := fyne.MeasureText(txt.Text, tickTextSize, style)
		sw.Move(fyne.NewPos(lx, ly+(s.Height-8)/2))
		sw.Resize(fyne.NewSize(8, 8))
		txt.Resize(s)
		txt.Move(fyne.NewPos(lx+12, ly))
		lx += 12 + s.Width + 16
	}
}
