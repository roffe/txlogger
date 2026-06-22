// Package rescaler resamples a 2D map onto new axis support points while
// preserving the underlying surface — the local reimplementation of the
// Trionic Map Scaler (gray-plant-037f86003.3.azurestaticapps.net), which did
// the same thing through a server-side API and required pasting values by hand.
// Here the values are read straight from the loaded binary instead.
package rescaler

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
)

// Config describes one map to rescale. Axes and data are in engineering units
// (correction factor already applied). ZData is row-major: index = y*len(X)+x,
// matching MapViewer and interpolate.Interpolate64.
type Config struct {
	Name                   string
	XLabel, YLabel, ZLabel string

	XData, YData, ZData []float64

	XPrecision, YPrecision, ZPrecision int

	// Apply writes the rescaled result back (e.g. to the binary symbols + disk).
	// newX/newY have the same length as the originals; newZ keeps the same dims.
	Apply func(newX, newY, newZ []float64) error
}

// Rescale resamples z (defined on oldX × oldY, row-major row=Y col=X) onto the
// newX × newY grid with clamped bilinear interpolation. Points outside the old
// axis range clamp to the nearest edge — no extrapolation. newX/newY may hold
// different values than the originals but interpolate.Interpolate64 wants the
// same Z layout, so callers keep the breakpoint counts unchanged.
func Rescale(oldX, oldY, z, newX, newY []float64) []float64 {
	out := make([]float64, len(newX)*len(newY))
	for yi, yv := range newY {
		for xi, xv := range newX {
			_, _, v, _ := interpolate.Interpolate64(oldX, oldY, z, xv, yv)
			out[yi*len(newX)+xi] = v
		}
	}
	return out
}

// Rescaler is the editable axis + live preview widget.
type Rescaler struct {
	widget.BaseWidget
	cfg *Config

	xEntry, yEntry *widget.Entry
	preview        *fyne.Container
	status         *widget.Label

	newX, newY, newZ []float64
}

func New(cfg *Config) *Rescaler {
	r := &Rescaler{
		cfg:     cfg,
		preview: container.NewStack(),
		status:  widget.NewLabel(""),
	}
	r.ExtendBaseWidget(r)

	r.xEntry = widget.NewEntry()
	r.xEntry.SetText(floatsToText(cfg.XData, cfg.XPrecision))
	r.yEntry = widget.NewEntry()
	r.yEntry.SetText(floatsToText(cfg.YData, cfg.YPrecision))

	r.rescale() // identity preview on open
	return r
}

func (r *Rescaler) rescale() {
	newX, err := parseAxis(r.xEntry.Text, len(r.cfg.XData))
	if err != nil {
		r.status.SetText("X axis: " + err.Error())
		return
	}
	newY, err := parseAxis(r.yEntry.Text, len(r.cfg.YData))
	if err != nil {
		r.status.SetText("Y axis: " + err.Error())
		return
	}

	newZ := Rescale(r.cfg.XData, r.cfg.YData, r.cfg.ZData, newX, newY)
	r.newX, r.newY, r.newZ = newX, newY, newZ

	mv, err := mapviewer.New(&mapviewer.Config{
		Name:       r.cfg.Name,
		XData:      newX,
		YData:      newY,
		ZData:      newZ,
		XPrecision: r.cfg.XPrecision,
		YPrecision: r.cfg.YPrecision,
		ZPrecision: r.cfg.ZPrecision,
		XLabel:     r.cfg.XLabel,
		YLabel:     r.cfg.YLabel,
		ZLabel:     r.cfg.ZLabel,
	})
	if err != nil {
		r.status.SetText(err.Error())
		return
	}
	r.preview.Objects = []fyne.CanvasObject{mv}
	r.preview.Refresh()
	r.status.SetText("Rescaled — review the preview, then Apply & Save")
}

func (r *Rescaler) apply() {
	if r.cfg.Apply == nil || r.newZ == nil {
		return
	}
	dialog.ShowConfirm("Apply & Save",
		fmt.Sprintf("Overwrite %s and its axes in the binary and save to disk?", r.cfg.Name),
		func(ok bool) {
			if !ok {
				return
			}
			if err := r.cfg.Apply(r.newX, r.newY, r.newZ); err != nil {
				r.status.SetText("Apply failed: " + err.Error())
				return
			}
			r.status.SetText("Applied and saved")
		}, fyne.CurrentApp().Driver().AllWindows()[0])
}

func (r *Rescaler) CreateRenderer() fyne.WidgetRenderer {
	rescaleBtn := widget.NewButtonWithIcon("Rescale", theme.ViewRefreshIcon(), r.rescale)
	rescaleBtn.Importance = widget.HighImportance
	applyBtn := widget.NewButtonWithIcon("Apply & Save", theme.DocumentSaveIcon(), r.apply)

	form := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("New X axis (%s) — %d points:", r.cfg.XLabel, len(r.cfg.XData))),
		r.xEntry,
		widget.NewLabel(fmt.Sprintf("New Y axis (%s) — %d points:", r.cfg.YLabel, len(r.cfg.YData))),
		r.yEntry,
		container.NewHBox(rescaleBtn, applyBtn),
		r.status,
	)

	content := container.NewBorder(form, nil, nil, nil, r.preview)
	return widget.NewSimpleRenderer(content)
}

func floatsToText(vals []float64, prec int) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatFloat(v, 'f', prec, 64)
	}
	return strings.Join(parts, ", ")
}

// parseAxis parses comma/space/newline separated numbers, requiring exactly want
// strictly-ascending values (interpolate.Interpolate64 binary-searches the axis,
// and SetData rejects a changed table length).
func parseAxis(text string, want int) ([]float64, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) != want {
		return nil, fmt.Errorf("need %d values, got %d", want, len(fields))
	}
	out := make([]float64, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", f)
		}
		if i > 0 && v <= out[i-1] {
			return nil, fmt.Errorf("values must strictly ascend (%g after %g)", v, out[i-1])
		}
		out[i] = v
	}
	return out, nil
}
