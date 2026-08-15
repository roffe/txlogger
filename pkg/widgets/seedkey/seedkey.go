// Package seedkey patches the Trionic 7 SecurityAccess (KWP2000 service 0x27)
// seed->key algorithm directly in a .bin image.
//
// The ECU computes the expected key from the 2-byte seed with this exact
// 68332 code (found identically in every T7 image sampled):
//
//	E548        lsl.w  #2,d0            ; seed * 4
//	323C <xor>  move.w #<xor>,d1        ; XOR constant  (editable, +4)
//	B340        eor.w  d1,d0            ; key ^= xor
//	0640 <neg>  addi.w #<neg>,d0        ; key += (0x10000 - sub)  (editable, +10)
//
// so key = ((seed<<2) ^ xor) - sub. The subtract is stored as an add of the
// two's-complement, which is why searching for the raw "sub" constant (the
// value tools like txlogger's calcen use) never finds it in the ROM.
package seedkey

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets"
)

// ErrNotFound is returned by Find when the routine signature is absent.
var errNotFound = fmt.Errorf("seed/key routine signature not found")

const (
	xorOff = 4  // XOR immediate, relative to signature start
	subOff = 10 // addi.w immediate (= 0x10000 - sub)
	sigLen = 12
)

// Method is one of the known txlogger calcen variants (methods 0-4).
type Method struct {
	Name     string
	XOR, Sub uint16
}

// Methods are the known-good algorithm variants. "Custom" is offered on top.
var Methods = []Method{
	{"Method 0", 0x8142, 0x2356},
	{"Method 1", 0x4081, 0x1F6F},
}

const customName = "Custom"

// MethodName returns the matching method name, or "Custom".
func MethodName(xor, sub uint16) string {
	for _, m := range Methods {
		if m.XOR == xor && m.Sub == sub {
			return m.Name
		}
	}
	return customName
}

// Find locates the seed->key routine and returns its offset and current
// XOR / SUB values. 68k code is word-aligned so the signature is scanned on
// even offsets; the 8 fixed bytes make a false positive effectively impossible.
func Find(data []byte) (offset int, xor, sub uint16, err error) {
	for i := 0; i+sigLen <= len(data); i += 2 {
		if data[i] == 0xE5 && data[i+1] == 0x48 && data[i+2] == 0x32 && data[i+3] == 0x3C &&
			data[i+6] == 0xB3 && data[i+7] == 0x40 && data[i+8] == 0x06 && data[i+9] == 0x40 {
			xor = binary.BigEndian.Uint16(data[i+xorOff:])
			neg := binary.BigEndian.Uint16(data[i+subOff:])
			return i, xor, uint16(-neg), nil // sub = 0x10000 - neg (mod 2^16)
		}
	}
	return 0, 0, 0, errNotFound
}

// Apply writes new XOR and SUB values at a known offset (from Find).
func Apply(data []byte, offset int, xor, sub uint16) {
	binary.BigEndian.PutUint16(data[offset+xorOff:], xor)
	binary.BigEndian.PutUint16(data[offset+subOff:], uint16(-sub)) // store two's-complement
}

// SavePatched patches a copy-safe []byte and writes it with checksums fixed.
// data must be an unpatched, valid T7 image; it is mutated in place.
func SavePatched(dst string, data []byte, offset int, xor, sub uint16) error {
	t7, err := symbol.NewT7File(data) // parses the still-valid original
	if err != nil {
		return fmt.Errorf("not a valid T7 image: %w", err)
	}
	Apply(t7.Bytes(), offset, xor, sub)
	if err := t7.UpdateChecksum(); err != nil {
		return fmt.Errorf("update checksum: %w", err)
	}
	return os.WriteFile(dst, t7.Bytes(), 0o644)
}

func parseHex16(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	v, err := strconv.ParseUint(s, 16, 16)
	return uint16(v), err
}

// New builds the T7 Seed/Key patcher widget.
func New() fyne.CanvasObject {
	var (
		data   []byte // unpatched, as loaded
		offset int
		loaded bool
	)

	pathLabel := widget.NewLabel("No binary loaded")
	info := widget.NewLabel("")
	info.Wrapping = fyne.TextWrapWord

	xorEntry := widget.NewEntry()
	xorEntry.SetPlaceHolder("0x8142")
	subEntry := widget.NewEntry()
	subEntry.SetPlaceHolder("0x2356")

	names := make([]string, 0, len(Methods)+1)
	for _, m := range Methods {
		names = append(names, m.Name)
	}
	names = append(names, customName)

	methodSelect := widget.NewSelect(names, func(name string) {
		for _, m := range Methods {
			if m.Name == name {
				xorEntry.SetText(fmt.Sprintf("0x%04X", m.XOR))
				subEntry.SetText(fmt.Sprintf("0x%04X", m.Sub))
				return
			}
		}
	})

	openBtn := widget.NewButtonWithIcon("Open binary…", theme.DocumentIcon(), func() {
		widgets.SelectFile(func(r fyne.URIReadCloser) {
			defer r.Close()
			b, err := io.ReadAll(r)
			if err != nil {
				info.SetText("read error: " + err.Error())
				return
			}
			pathLabel.SetText(r.URI().Name())
			off, xor, sub, err := Find(b)
			if err != nil {
				loaded = false
				info.SetText("Seed/key routine not found in this file")
				return
			}
			data, offset, loaded = b, off, true
			xorEntry.SetText(fmt.Sprintf("0x%04X", xor))
			subEntry.SetText(fmt.Sprintf("0x%04X", sub))
			methodSelect.SetSelected(MethodName(xor, sub))
			info.SetText(fmt.Sprintf("Routine @ 0x%X — current: XOR 0x%04X, SUB 0x%04X", off, xor, sub))
		}, "T7 binary", "bin")
	})

	saveBtn := widget.NewButtonWithIcon("Save patched binary…", theme.DocumentSaveIcon(), func() {
		if !loaded {
			info.SetText("Load a binary first")
			return
		}
		xor, err := parseHex16(xorEntry.Text)
		if err != nil {
			info.SetText("bad XOR value")
			return
		}
		sub, err := parseHex16(subEntry.Text)
		if err != nil {
			info.SetText("bad SUB value")
			return
		}
		widgets.SaveFile(func(dst string) {
			cp := make([]byte, len(data))
			copy(cp, data)
			if err := SavePatched(dst, cp, offset, xor, sub); err != nil {
				info.SetText("save error: " + err.Error())
				return
			}
			info.SetText(fmt.Sprintf("Saved with XOR 0x%04X, SUB 0x%04X (%s) → %s", xor, sub, MethodName(xor, sub), dst))
		}, "T7 binary", "bin")
	})

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Method"), methodSelect,
		widget.NewLabel("XOR"), xorEntry,
		widget.NewLabel("SUB"), subEntry,
	)

	return container.NewVBox(
		container.NewHBox(openBtn, saveBtn),
		pathLabel,
		form,
		info,
		widget.NewLabel("key = (seed<<2) XOR value − SUB value  (KWP2000 SecurityAccess 0x27)"),
	)
}
