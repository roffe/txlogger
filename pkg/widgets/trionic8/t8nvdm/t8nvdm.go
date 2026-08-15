// Package t8nvdm edits the Trionic 8 NVDM (Non Volatile Data Memory) block of
// a loaded binary: the immobilizer keys and the identity DIDs (VIN, part
// numbers, tester serial). It edits the file, not a running ECU.
//
// NVDM lives in the flash file system at 0x4000-0x8000 as a ring of 304 byte
// slots, obfuscated per byte (StoreNVDMDataInFlash in NVDM.c):
//
//	store:  out = (in ^ 0xA4) - 0x53
//	read:   in  = (out + 0x53) ^ 0xA4
//
// The plaintext crypt[] field is "Cryp", which stores as 94 83 8A 81 — that is
// how a slot is located. There is no checksum over NVDM: the obfuscation is the
// only integrity marker, and the binary's own L1/L2 checksums start at 0x20000,
// so nothing else needs fixing after an edit.
package t8nvdm

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
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

const (
	slotSize = 304
	cryptOff = 80     // offset of crypt[] within the slot
	bankSize = 0x2000 // one MFS bank, 0x4000 and 0x6000

	// srcVIN is the VIN offset in the NVDM.h struct. Shipped firmware places
	// the identity block a couple of bytes later than the source tree, so the
	// real offset is found by anchoring on the VIN; shippedDelta is the shift
	// seen in every real dump and is the fallback when the VIN is blank.
	srcVIN       = 222
	shippedDelta = 2
)

var magic = [4]byte{0x94, 0x83, 0x8A, 0x81} // "Cryp" obfuscated

type kind int

const (
	kASCII kind = iota // space padded text
	kHex               // raw bytes as hex
	kByte              // single byte as decimal
)

// field is one editable NVDM variable. off is its offset in NVDM.h
// coordinates; drift marks the ones that move with the firmware delta (only
// the block from ECUHardwareName onwards does).
type field struct {
	name  string
	off   int
	n     int
	kind  kind
	drift bool
}

// Fields before crypt[] are at fixed offsets in every build; the identity
// block after it drifts. Everything between securityLevel (125) and
// ECUHardwareName (173) is where the drift is introduced, so those variables
// (oil counters, revs, odometer, EOL settings) cannot be placed reliably and
// are deliberately not offered.
var (
	fVIN = field{"VIN (DID 90)", srcVIN, 17, kASCII, true}
	fSK  = field{"powerTrainSK", 109, 16, kHex, false}
)

var immoFields = []field{
	{"securityCode", 64, 4, kHex, false},
	{"powerTrainIdentifier", 68, 2, kHex, false},
	fSK,
	{"transponderSK", 97, 12, kHex, false},
	{"remoteControlSK", 85, 12, kHex, false},
	{"securityLevel (10 = unlocked)", 125, 1, kByte, false},
	{"securityCodeInReset", 59, 1, kByte, false},
	{"ST_ImmoEnabled", 296, 1, kByte, true},
	{"N_FreeStarts", 294, 1, kByte, true},
	{"N_SecurityCodeReset", 295, 1, kByte, true},
}

var idFields = []field{
	fVIN,
	{"ECU HW name (DID 71)", 173, 10, kASCII, true},
	{"ECU HW level (DID 72)", 183, 5, kASCII, true},
	{"ECU SW version (DID 73)", 188, 10, kASCII, true},
	{"ECU data version (DID 74)", 198, 10, kASCII, true},
	{"CAN dictionary (DID 75)", 208, 10, kASCII, true},
	{"Tester serial (DID 98)", 255, 10, kASCII, true},
	{"Programming date (DID 99)", 265, 4, kHex, true},
	{"Bosch part no (DID 9F)", 269, 10, kASCII, true},
	{"Manufacturers enable counter", 279, 1, kByte, true},
	{"Shift indication (DID 04)", 300, 1, kByte, true},
}

// New builds the editor for the NVDM block of fw.
func New(filename string, fw *symbol.T8File) fyne.CanvasObject {
	data := fw.Bytes()
	slots := Scan(data)
	if len(slots) == 0 {
		return widget.NewLabel("No NVDM block found in the loaded binary.\n\n" +
			"NVDM lives at 0x4000-0x8000 and is only present in a full 1MB ECU dump.")
	}

	cur := Decrypt(data, Current(data, slots))
	// The layout shift is a property of the firmware, not of the slot, so it is
	// taken once from the current record and used for every slot.
	delta := Delta(cur)

	type row struct {
		fd    *field
		entry *widget.Entry
		orig  string
	}
	var rows []*row
	form := func(fields []field) *widget.Form {
		f := widget.NewForm()
		for i := range fields {
			fd := &fields[i]
			e := widget.NewEntry()
			r := &row{fd: fd, entry: e}
			if b, err := fd.get(cur, delta); err == nil {
				r.orig = fd.encode(b)
				e.SetText(r.orig)
			} else {
				e.Disable()
			}
			rows = append(rows, r)
			f.Append(fd.name, e)
		}
		return f
	}

	win := func() fyne.Window { return fyne.CurrentApp().Driver().AllWindows()[0] }

	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		// Only fields you actually edited are written: untouched ones keep their
		// raw bytes, which matters for blank fields padded with 0xFF that render
		// as dots and would otherwise be saved as dots.
		values := make(map[*field][]byte, len(rows))
		for _, r := range rows {
			if r.entry.Disabled() || r.entry.Text == r.orig {
				continue
			}
			b, err := r.fd.decode(r.entry.Text)
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", r.fd.name, err), win())
				return
			}
			values[r.fd] = b
		}
		if len(values) == 0 {
			dialog.ShowInformation("NVDM", "Nothing changed", win())
			return
		}
		for _, s := range slots {
			d := Decrypt(data, s)
			for fd, b := range values {
				if err := fd.set(d, delta, b); err != nil {
					dialog.ShowError(fmt.Errorf("%s: %w", fd.name, err), win())
					return
				}
			}
			Encrypt(data[s:s+slotSize], d)
		}
		if err := fw.Save(filename); err != nil {
			dialog.ShowError(err, win())
			return
		}
		dialog.ShowInformation("NVDM", fmt.Sprintf("%d fields written to %d slots in %s",
			len(values), len(slots), filename), win())
	})

	txt := fmt.Sprintf(
		"%d NVDM slots found, showing the newest. Saving writes the edited fields to every slot.\n"+
			"Reflash the ECU with \"Unlock systems partition\" enabled for changes to take effect.", len(slots))
	if n := identities(data, slots, delta); n > 1 {
		txt += fmt.Sprintf("\n\nThis ECU carries %d different identities (it has been in more than one car). "+
			"Saving overwrites the older records too.", n)
	}
	head := widget.NewLabel(txt)
	head.Wrapping = fyne.TextWrapWord

	body := container.NewVBox(
		widget.NewLabelWithStyle("Identity", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		form(idFields),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Immobilizer", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		form(immoFields),
	)

	return container.NewBorder(head, container.NewHBox(layout.NewSpacer(), save), nil, nil,
		container.NewVScroll(body))
}

// Scan returns the start offset of every obfuscated NVDM slot in buf.
func Scan(buf []byte) []int {
	var out []int
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] == magic[0] && buf[i+1] == magic[1] && buf[i+2] == magic[2] && buf[i+3] == magic[3] {
			if start := i - cryptOff; start >= 0 && start+slotSize <= len(buf) {
				out = append(out, start)
			}
		}
	}
	return out
}

// Current returns the start of the newest slot. NVDM is double banked (0x4000
// and 0x6000): each bank starts with an "MFS*" header carrying a generation
// counter at +4, the higher one is the live bank, and within a bank the ring is
// written from the low slot up. Falls back to the last slot found when there is
// no bank header to go by.
func Current(buf []byte, slots []int) int {
	best, bestGen := slots[len(slots)-1], int64(-1)
	for _, s := range slots {
		base := s &^ (bankSize - 1)
		if base+8 > len(buf) || string(buf[base:base+4]) != "MFS*" {
			continue
		}
		gen := int64(binary.BigEndian.Uint32(buf[base+4 : base+8]))
		if gen >= bestGen {
			best, bestGen = s, gen
		}
	}
	return best
}

// identities counts the distinct VIN + powerTrainSK pairs across the slots. A
// donor ECU that has been in several cars still carries the older ones.
func identities(buf []byte, slots []int, delta int) int {
	seen := map[string]bool{}
	for _, s := range slots {
		d := Decrypt(buf, s)
		vin, err := fVIN.get(d, delta)
		if err != nil {
			continue
		}
		sk, err := fSK.get(d, delta)
		if err != nil {
			continue
		}
		seen[string(vin)+string(sk)] = true
	}
	return len(seen)
}

// Decrypt returns the plaintext slot at start.
func Decrypt(buf []byte, start int) []byte {
	d := make([]byte, slotSize)
	for i := range d {
		d[i] = (buf[start+i] + 0x53) ^ 0xA4
	}
	return d
}

// Encrypt writes the obfuscated form of d into dst.
func Encrypt(dst, d []byte) {
	for i := range d {
		dst[i] = (d[i] ^ 0xA4) - 0x53
	}
}

// Delta returns the identity block shift of this firmware, taken from the VIN
// when there is one and from shippedDelta when there is not.
func Delta(d []byte) int {
	if vin := findVIN(d); vin >= 0 {
		return vin - srcVIN
	}
	return shippedDelta
}

func (f *field) offset(delta int) int {
	if f.drift {
		return f.off + delta
	}
	return f.off
}

func (f *field) get(d []byte, delta int) ([]byte, error) {
	o := f.offset(delta)
	if o < 0 || o+f.n > len(d) {
		return nil, errors.New("offset outside NVDM slot")
	}
	return d[o : o+f.n], nil
}

func (f *field) set(d []byte, delta int, b []byte) error {
	o := f.offset(delta)
	if o < 0 || o+f.n > len(d) {
		return errors.New("offset outside NVDM slot")
	}
	copy(d[o:o+f.n], b)
	return nil
}

func (f *field) encode(b []byte) string {
	switch f.kind {
	case kASCII:
		return strings.TrimRight(ascii(b), " ")
	case kByte:
		return strconv.Itoa(int(b[0]))
	default:
		return strings.ToUpper(hex.EncodeToString(b))
	}
}

func (f *field) decode(s string) ([]byte, error) {
	switch f.kind {
	case kASCII:
		if len(s) > f.n {
			return nil, fmt.Errorf("max %d characters", f.n)
		}
		b := []byte(s)
		for _, c := range b {
			if c < 0x20 || c > 0x7E {
				return nil, errors.New("only printable ascii allowed")
			}
		}
		return append(b, []byte(strings.Repeat(" ", f.n-len(b)))...), nil
	case kByte:
		v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 8)
		if err != nil {
			return nil, errors.New("expected a number 0-255")
		}
		return []byte{byte(v)}, nil
	default:
		b, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
		if err != nil {
			return nil, errors.New("expected hex bytes")
		}
		if len(b) != f.n {
			return nil, fmt.Errorf("expected %d bytes, got %d", f.n, len(b))
		}
		return b, nil
	}
}

// findVIN returns the offset of the 17 char VIN, or -1. A VIN uses
// [A-HJ-NPR-Z0-9] (no I/O/Q); we look for a 17 char window that starts with a
// letter and is followed by a non-VIN character, which is the field's trailing
// space padding. Anchoring on the trailing boundary rather than a maximal run
// tolerates a VIN-valid byte bleeding in from the field in front of it.
func findVIN(d []byte) int {
	const afterImmo = 126 // securityLevel is the last immo byte, at 125
	for i := afterImmo; i+17 <= len(d); i++ {
		if !vinLetter(d[i]) {
			continue
		}
		ok := true
		for j := 1; j < 17; j++ {
			if !vinChar(d[i+j]) {
				ok = false
				break
			}
		}
		if ok && (i+17 == len(d) || !vinChar(d[i+17])) {
			return i
		}
	}
	return -1
}

func vinChar(c byte) bool {
	return (c >= '0' && c <= '9') || vinLetter(c)
}

func vinLetter(c byte) bool {
	return c >= 'A' && c <= 'Z' && c != 'I' && c != 'O' && c != 'Q'
}

func ascii(b []byte) string {
	out := make([]byte, len(b))
	for i, v := range b {
		if v >= 0x20 && v < 0x7F {
			out[i] = v
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}
