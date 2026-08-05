// Package t7piarea is a raw editor for the T7 PI area (the footer at the top
// of flash). It edits the loaded binary, not the ECU: writing PI fields to a
// live ECU is EOL-mode only and 0x98 can trigger a full flash erase.
package t7piarea

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
)

type row struct {
	id    *widget.Entry
	name  *widget.Label
	asHex *widget.Check
	value *widget.Entry
}

// New builds the editor. Rows are shown in footer order, top of flash first.
func New(filename string, fw *symbol.T7File) fyne.CanvasObject {
	list := container.NewVBox()
	rows := []*row{}

	var addRow func(f *symbol.T7HeaderField)
	var rebuild func()
	rebuild = func() {
		list.RemoveAll()
		for _, r := range rows {
			list.Add(container.NewBorder(nil, nil,
				container.NewHBox(r.id, r.name),
				container.NewHBox(r.asHex, deleteButton(r, &rows, rebuild)),
				r.value,
			))
		}
		list.Refresh()
	}

	addRow = func(f *symbol.T7HeaderField) {
		r := &row{
			id:    widget.NewEntry(),
			name:  widget.NewLabel(""),
			asHex: widget.NewCheck("hex", nil),
			value: widget.NewEntry(),
		}
		r.id.Resize(fyne.NewSize(60, 0))
		r.id.OnChanged = func(s string) { r.name.SetText(fieldName(s)) }
		if f != nil {
			r.id.SetText(fmt.Sprintf("%02X", f.ID))
			r.asHex.SetChecked(!printable(f.Data))
			r.value.SetText(encode(f.ID, f.Data, r.asHex.Checked))
		}
		r.asHex.OnChanged = func(hexMode bool) {
			id, _ := parseID(r.id.Text)
			if b, err := decode(id, r.value.Text, !hexMode); err == nil {
				r.value.SetText(encode(id, b, hexMode))
			}
		}
		rows = append(rows, r)
		rebuild()
	}

	for _, f := range fw.GetHeaders() {
		addRow(f)
	}

	win := func() fyne.Window { return fyne.CurrentApp().Driver().AllWindows()[0] }

	add := widget.NewButtonWithIcon("Add field", theme.ContentAddIcon(), func() { addRow(nil) })
	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		fields, err := collect(rows)
		if err != nil {
			dialog.ShowError(err, win())
			return
		}
		if err := fw.SetPIArea(fields); err != nil {
			dialog.ShowError(err, win())
			return
		}
		if err := fw.Save(filename); err != nil {
			dialog.ShowError(err, win())
			return
		}
		dialog.ShowInformation("PI area", "Saved to "+filename, win())
	})

	head := widget.NewLabel("ID (hex), value. Uncheck hex to edit as text. Reflash the ECU for changes to take effect")
	return container.NewBorder(head, container.NewHBox(add, layout.NewSpacer(), save), nil, nil,
		container.NewVScroll(list))
}

func deleteButton(r *row, rows *[]*row, rebuild func()) *widget.Button {
	return widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		for i, x := range *rows {
			if x == r {
				*rows = append((*rows)[:i], (*rows)[i+1:]...)
				break
			}
		}
		rebuild()
	})
}

func collect(rows []*row) ([]*symbol.T7HeaderField, error) {
	fields := make([]*symbol.T7HeaderField, 0, len(rows))
	for _, r := range rows {
		id, err := parseID(r.id.Text)
		if err != nil {
			return nil, err
		}
		data, err := decode(id, r.value.Text, r.asHex.Checked)
		if err != nil {
			return nil, fmt.Errorf("field 0x%02X: %w", id, err)
		}
		fields = append(fields, &symbol.T7HeaderField{ID: id, Data: data})
	}
	return fields, nil
}

func parseID(s string) (byte, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(strings.ToLower(s)), "0x"), 16, 8)
	if err != nil {
		return 0, fmt.Errorf("bad field id %q, expected two hex digits", s)
	}
	return byte(v), nil
}

func fieldName(s string) string {
	id, err := parseID(s)
	if err != nil {
		return ""
	}
	return symbol.T7PIAreaNames[id]
}

func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return len(b) > 0
}

// 0x9B/0x9C are the only fields stored little-endian in ECU read order (the
// linker emits them, the ECU never reads them), so their bytes come out of the
// footer walk backwards. Show them value order, 00064486 rather than 86440600.
var littleEndianIDs = map[byte]bool{0x9B: true, 0x9C: true}

func swapped(id byte, b []byte) []byte {
	if !littleEndianIDs[id] {
		return b
	}
	out := make([]byte, len(b))
	for i, c := range b {
		out[len(b)-1-i] = c
	}
	return out
}

func encode(id byte, b []byte, asHex bool) string {
	if !asHex {
		return string(b)
	}
	return strings.ToUpper(hex.EncodeToString(swapped(id, b)))
}

func decode(id byte, s string, asHex bool) ([]byte, error) {
	if !asHex {
		return []byte(s), nil
	}
	b, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if err != nil {
		return nil, fmt.Errorf("value must be hex bytes: %w", err)
	}
	return swapped(id, b), nil
}
