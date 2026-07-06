package dtcreader

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/dtc/wis"
)

var _ fyne.Widget = (*DTCReader)(nil)

type DTCReader struct {
	widget.BaseWidget

	list *widget.List

	dtcs []dtc.DTC

	readBtn  *widget.Button
	clearBtn *widget.Button

	getFW      func() symbol.SymbolCollection
	getECU     func() string
	getAdapter func() (gocan.Adapter, error)
	log        func(string)
	err        func(error)
}

func New(getFW func() symbol.SymbolCollection, getECU func() string, getAdapter func() (gocan.Adapter, error), log func(string), err func(error)) *DTCReader {
	d := &DTCReader{
		getFW:      getFW,
		getECU:     getECU,
		getAdapter: getAdapter,
		log:        log,
		err:        err,
	}
	d.ExtendBaseWidget(d)
	return d
}

func (d *DTCReader) render() {
	d.list = widget.NewList(
		func() int {
			return len(d.dtcs)
		},
		func() fyne.CanvasObject {
			return NewDTCEntry()
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			code := d.dtcs[i]
			entry := o.(*DTCEntry)
			info := code.Info()

			var results []wis.Result
			if models := wisModels(code.ECU); models != nil {
				results = wis.Search(code.Code, models...)
			}

			dtcTitle := code.String()
			switch code.ECU {
			case dtc.ECU_T5:
				dtcTitle += fmt.Sprintf(": %d", code.Status)
			case dtc.ECU_T7:
				// full wire code (status byte included), then its meaning
				dtcTitle += fmt.Sprintf(" %02X (%s)", code.Status, dtc.T7StatusString(code.Status))
			case dtc.ECU_T8:
				// Code already carries the failure type ("B0165 02"); add
				// the GMW3110 meaning of that suffix
				dtcTitle += " (" + gmlan.FailureTypeString(code.FailureType) + ")"
			}
			switch {
			case info.Name != "":
				dtcTitle += " - " + info.Name
			case len(results) > 0 && results[0].Title != "":
				dtcTitle += " - " + results[0].Title
			}
			entry.SetTitle(dtcTitle)

			desc := "No description available."
			switch {
			case info.Description != "":
				desc = info.Description
			case len(results) > 0 && results[0].Symptom != "":
				desc = strings.ReplaceAll(results[0].Symptom, "\n\n", "\n")
			}
			entry.SetDescription(desc)

			if len(results) > 0 {
				entry.SetOnTapped(func() { d.openWIS(entry, results) })
			} else {
				entry.SetOnTapped(nil)
			}

			// rows vary in height with the description text
			d.list.SetItemHeight(i, entry.MinSize().Height)
		},
	)

	d.readBtn = widget.NewButtonWithIcon("Read DTCS", theme.SearchIcon(), func() {
		d.Disable()
		d.log("Reading DTCs...")
		if err := d.ReadDTCS(); err != nil {
			d.err(err)
		}
	})

	d.clearBtn = widget.NewButtonWithIcon("Clear DTCS", theme.DeleteIcon(), func() {
		d.Disable()
		d.log("Clearing DTCs...")
		if err := d.ClearDTCS(); err != nil {
			d.err(err)
		}
	})

}

func (d *DTCReader) Enable() {
	d.readBtn.Enable()
	d.clearBtn.Enable()
}

func (d *DTCReader) Disable() {
	d.readBtn.Disable()
	d.clearBtn.Disable()
}

func (d *DTCReader) CreateRenderer() fyne.WidgetRenderer {
	d.render()
	return widget.NewSimpleRenderer(container.NewBorder(
		nil,
		container.NewGridWithColumns(2,
			d.readBtn,
			d.clearBtn,
		),
		nil,
		nil,
		container.NewVScroll(d.list),
	))
}

func (d *DTCReader) Refresh() {
	d.list.Refresh()
}

func (d *DTCReader) ReadDTCS() error {
	ecu := d.getECU()

	var readDTCSFunc func(context.Context, *gocan.Bus)
	switch ecu {
	case "T5":
		readDTCSFunc = d.readT5DTCS
	case "T7":
		readDTCSFunc = d.readT7DTCS
	case "T8":
		readDTCSFunc = d.readT8DTCS
	default:
		d.readBtn.Enable()
		d.clearBtn.Enable()
		return fmt.Errorf("DTC reading not supported for ECU %s", ecu)
	}

	go func() {
		defer fyne.Do(d.Enable)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		dev, err := d.getAdapter()
		if err != nil {
			d.err(err)
			return
		}

		d.log("Connecting to device " + gocan.AdapterName(dev))

		// Events (incl. the final fatal) stream to the log; a fatal adapter
		// failure also aborts any in-flight call below with that error.
		cl, err := gocan.OpenAdapter(ctx, dev, gocan.WithEventFunc(func(e gocan.Event) {
			d.log(e.String())
		}))
		if err != nil {
			d.err(err)
			return
		}
		defer cl.Close()

		readDTCSFunc(ctx, cl)
	}()
	return nil
}

func (d *DTCReader) ClearDTCS() error {
	ecu := d.getECU()
	var clearDTCSFunc func(context.Context, *gocan.Bus)
	switch ecu {
	case "T5":
		clearDTCSFunc = d.clearT5DTCS
	case "T7":
		clearDTCSFunc = d.clearT7DTCS
	case "T8":
		clearDTCSFunc = d.clearT8DTCS
	default:
		d.clearBtn.Enable()
		return fmt.Errorf("DTC clearing not supported for ECU %s", ecu)
	}
	go func() {
		defer fyne.Do(d.Enable)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		dev, err := d.getAdapter()
		if err != nil {
			d.err(err)
			return
		}
		d.log("Connecting to device " + gocan.AdapterName(dev))

		// Events (incl. the final fatal) stream to the log; a fatal adapter
		// failure also aborts any in-flight call below with that error.
		cl, err := gocan.OpenAdapter(ctx, dev, gocan.WithEventFunc(func(e gocan.Event) {
			d.log(e.String())
		}))
		if err != nil {
			d.err(err)
			return
		}
		defer cl.Close()

		clearDTCSFunc(ctx, cl)
	}()
	return nil
}

// wisModels maps an ECU to the WIS car models it appears in. T5 cars
// (9000/NG900) are not covered by the WIS archive.
func wisModels(ecu dtc.ECU) []string {
	switch ecu {
	case dtc.ECU_T7:
		return []string{"9400", "9600"}
	case dtc.ECU_T8:
		return []string{"9440"}
	}
	return nil
}

// openWIS opens the fault diagnosis document for a DTC, showing a picker
// when the code matches in more than one model/system.
func (d *DTCReader) openWIS(entry fyne.CanvasObject, results []wis.Result) {
	if len(results) == 1 {
		d.openWISDoc(results[0])
		return
	}
	items := make([]*fyne.MenuItem, 0, len(results))
	for _, r := range results {
		label := fmt.Sprintf("%s %s · %s · %s", wis.ModelName(r.Model), r.Year, r.System, r.Code)
		items = append(items, fyne.NewMenuItem(label, func() { d.openWISDoc(r) }))
	}
	drv := fyne.CurrentApp().Driver()
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...),
		drv.CanvasForObject(entry), drv.AbsolutePositionForObject(entry))
}

// openWISDoc renders the document to a temp file and opens it in the
// system browser; fyne has no widget that can display this HTML.
func (d *DTCReader) openWISDoc(r wis.Result) {
	html, err := wis.Render(r.Doc)
	if err != nil {
		d.err(err)
		return
	}
	f, err := os.CreateTemp("", "txlogger-dtc-*.html")
	if err != nil {
		d.err(err)
		return
	}
	_, werr := f.Write(html)
	if err := f.Close(); werr == nil {
		werr = err
	}
	if werr != nil {
		d.err(werr)
		return
	}
	p := filepath.ToSlash(f.Name())
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // windows: file:///C:/...
	}
	d.log("Opening WIS document for " + r.Code + " in browser")
	if err := fyne.CurrentApp().OpenURL(&url.URL{Scheme: "file", Path: p}); err != nil {
		d.err(err)
	}
}

var _ fyne.Widget = (*DTCEntry)(nil)

type DTCEntry struct {
	widget.BaseWidget
	title       *widget.Label
	description *widget.Label
	info        *widget.Button
	onTapped    func()
}

func NewDTCEntry() *DTCEntry {
	d := &DTCEntry{
		title:       widget.NewLabel(""),
		description: widget.NewLabel("No description available."),
	}

	d.title.Selectable = true
	d.description.Selectable = true

	d.info = widget.NewButtonWithIcon("", theme.InfoIcon(), func() {
		if d.onTapped != nil {
			d.onTapped()
		}
	})
	d.info.Disable()

	d.ExtendBaseWidget(d)
	return d
}

func (d *DTCEntry) SetTitle(title string) {
	d.title.SetText(title)
}

func (d *DTCEntry) SetDescription(desc string) {
	d.description.SetText(desc)
}

// SetOnTapped sets the handler for the info button; nil disables it.
func (d *DTCEntry) SetOnTapped(f func()) {
	d.onTapped = f
	if f == nil {
		d.info.Disable()
	} else {
		d.info.Enable()
	}
}

func (d *DTCEntry) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(
		nil,
		canvas.NewLine(color.Black),
		d.info,
		nil,
		container.NewVBox(
			d.title,
			d.description,
		),
	))
}
