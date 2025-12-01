package dtcreader

import (
	"context"
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/dtc"
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
			dtcTitle := code.String()
			if code.ECU == dtc.ECU_T5 {
				dtcTitle += fmt.Sprintf(": %d", code.Status)
			}
			if info.Name != "" {
				dtcTitle += " - " + info.Name
			}
			entry.SetTitle(dtcTitle)
			if info.Description != "" {
				entry.SetDescription(info.Description)
			}
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

	var readDTCSFunc func(context.Context, *gocan.Client)
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
		defer d.Enable()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		dev, err := d.getAdapter()
		if err != nil {
			d.err(err)
			return
		}

		eventHandler := func(e gocan.Event) {
			d.log(e.String())
		}

		d.log("Connecting to device " + dev.Name())

		cl, err := gocan.NewWithOpts(ctx, dev, gocan.WithEventHandler(eventHandler))
		if err != nil {
			d.err(err)
			return
		}
		defer cl.Close()

		go func() {
			if err := cl.Wait(ctx); err != nil {
				d.err(err)
				return
			}
		}()

		readDTCSFunc(ctx, cl)
	}()
	return nil
}

func (d *DTCReader) ClearDTCS() error {
	ecu := d.getECU()
	var clearDTCSFunc func(context.Context, *gocan.Client)
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
		defer d.Enable()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		dev, err := d.getAdapter()
		if err != nil {
			d.err(err)
			return
		}
		eventHandler := func(e gocan.Event) {
			d.log(e.String())
		}

		d.log("Connecting to device " + dev.Name())

		cl, err := gocan.NewWithOpts(ctx, dev, gocan.WithEventHandler(eventHandler))
		if err != nil {
			d.err(err)
			return
		}
		defer cl.Close()

		go func() {
			if err := cl.Wait(ctx); err != nil {
				d.err(err)
				return
			}
		}()

		clearDTCSFunc(ctx, cl)
	}()
	return nil
}

var _ fyne.Widget = (*DTCEntry)(nil)

type DTCEntry struct {
	widget.BaseWidget
	title       *widget.Label
	description *widget.Label
}

func NewDTCEntry() *DTCEntry {
	d := &DTCEntry{
		title:       widget.NewLabel(""),
		description: widget.NewLabel("No description available."),
	}

	d.title.Selectable = true
	d.description.Selectable = true

	d.ExtendBaseWidget(d)
	return d
}

func (d *DTCEntry) SetTitle(title string) {
	d.title.SetText(title)
}

func (d *DTCEntry) SetDescription(desc string) {
	d.description.SetText(desc)
}

func (d *DTCEntry) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(
		nil,
		canvas.NewLine(color.Black),
		widget.NewButtonWithIcon("", theme.WarningIcon(), func() {}),
		nil,
		container.NewVBox(
			d.title,
			d.description,
		),
	))
}
