package t8file

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// topObjects returns every object of the topmost overlay on c.
func topObjects(t *testing.T, c fyne.Canvas) []fyne.CanvasObject {
	t.Helper()
	top := c.Overlays().Top()
	if top == nil {
		t.Fatal("no dialog shown")
	}
	return test.LaidOutObjects(top)
}

func entriesOf(objs []fyne.CanvasObject) (out []*widget.Entry) {
	for _, o := range objs {
		if e, ok := o.(*widget.Entry); ok {
			out = append(out, e)
		}
	}
	return out
}

func buttonOf(t *testing.T, objs []fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	for _, o := range objs {
		if b, ok := o.(*widget.Button); ok && b.Text == text {
			return b
		}
	}
	t.Fatalf("no %q button", text)
	return nil
}

// openEditDialog shows the edit dialog for the T8 file at path and returns
// its seven entries: VIN, ECU description, part number, software version,
// PIN, PSK and ISK.
func openEditDialog(t *testing.T, tf *T8File) (fyne.Window, []*widget.Entry) {
	t.Helper()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(800, 800))
	tf.ShowEditT8Dialog(win)
	entries := entriesOf(topObjects(t, win.Canvas()))
	if len(entries) != 7 {
		t.Fatalf("dialog has %d entries, want 7", len(entries))
	}
	return win, entries
}

// The edit dialog starts out with the file's values and validates each
// editable field's length.
func TestEditDialogFields(t *testing.T) {
	test.NewTempApp(t)
	tf := new(T8File)
	tf.GetInfo(writeBin(t, testBin()))
	_, e := openEditDialog(t, tf)

	want := []string{blockC.vin, blockC.desc, "55353231", "FA5I_C_FME2_65_FIEF_82h       ", blockC.pin, "A0A1A2A3A4A5", "B0B1B2B3B4B5"}
	for i, w := range want {
		if e[i].Text != w {
			t.Errorf("entry %d = %q, want %q", i, e[i].Text, w)
		}
	}

	lengths := map[int]int{0: 17, 1: 15, 4: 4, 5: 12, 6: 12}
	for i, n := range lengths {
		if err := e[i].Validator(strings.Repeat("A", n)); err != nil {
			t.Errorf("entry %d rejects %d characters: %v", i, n, err)
		}
		if err := e[i].Validator(strings.Repeat("A", n-1)); err == nil {
			t.Errorf("entry %d accepts %d characters", i, n-1)
		}
	}
}

// The ISK entry's validation message names the ISK.
func TestEditDialogISKMessage(t *testing.T) {
	test.NewTempApp(t)
	tf := new(T8File)
	tf.GetInfo(writeBin(t, testBin()))
	_, e := openEditDialog(t, tf)
	if err := e[6].Validator("short"); err == nil || !strings.Contains(err.Error(), "ISK") {
		t.Errorf("ISK validation error = %v, want it to name the ISK", err)
	}
}

// Part number and software version are read-only: SaveFlashBlock does not
// write them, so the dialog does not offer to change them.
func TestEditDialogReadOnlyFields(t *testing.T) {
	test.NewTempApp(t)
	path := writeBin(t, testBin())
	tf := new(T8File)
	th := tf.GetInfo(path)
	win, e := openEditDialog(t, tf)

	for _, i := range []int{2, 3} {
		if !e[i].Disabled() {
			t.Errorf("entry %d (%q) is editable", i, e[i].Text)
		}
	}
	for _, i := range []int{0, 1, 4, 5, 6} {
		if e[i].Disabled() {
			t.Errorf("entry %d is disabled", i)
		}
	}

	e[2].SetText("12345678")
	e[3].SetText("FA5I_C_FME2_65_FIEF_99z       ")
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Save changes"))
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Yes"))

	if th.PartNumber() != "55353231" || th.SoftwareVersion() != "FA5I_C_FME2_65_FIEF_82h       " {
		t.Errorf("header PartNumber %q SoftwareVersion %q, want them unchanged", th.PartNumber(), th.SoftwareVersion())
	}
	again := new(T8File).GetInfo(path)
	if again.PartNumber() != "55353231" || again.SoftwareVersion() != "FA5I_C_FME2_65_FIEF_82h       " {
		t.Errorf("saved PartNumber %q SoftwareVersion %q, want them unchanged", again.PartNumber(), again.SoftwareVersion())
	}
}

// A failed save is reported as an error, not as success.
func TestEditDialogSaveError(t *testing.T) {
	test.NewTempApp(t)
	tf := new(T8File)
	tf.GetInfo(writeBin(t, testBin()))
	tf.filePath = t.TempDir() // a directory cannot be opened read-write
	win, e := openEditDialog(t, tf)

	e[0].SetText("YS3FB49F741999999")
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Save changes"))
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Yes"))

	var texts []string
	for _, o := range topObjects(t, win.Canvas()) {
		if l, ok := o.(*widget.Label); ok {
			texts = append(texts, l.Text)
		}
	}
	joined := strings.Join(texts, "\n")
	if strings.Contains(joined, "Data has been stored in T8 bin file.") {
		t.Errorf("success message shown for a failed save: %q", texts)
	}
	if !strings.Contains(joined, tf.filePath) {
		t.Errorf("dialog %q does not report the write error", texts)
	}
}

// Cancel leaves the header as it was.
func TestEditDialogCancel(t *testing.T) {
	test.NewTempApp(t)
	tf := new(T8File)
	th := tf.GetInfo(writeBin(t, testBin()))
	win, e := openEditDialog(t, tf)

	e[0].SetText("YS3FB49F741999999")
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Cancel"))
	if th.VIN() != blockC.vin {
		t.Errorf("VIN = %q after Cancel, want %q", th.VIN(), blockC.vin)
	}
}

// Save changes copies fields of valid length into the header, ignores the
// others, and asks for confirmation; declining leaves the file untouched.
func TestEditDialogSaveDeclined(t *testing.T) {
	test.NewTempApp(t)
	orig := testBin()
	path := writeBin(t, orig)
	tf := new(T8File)
	th := tf.GetInfo(path)
	win, e := openEditDialog(t, tf)

	e[0].SetText("YS3FB49F741999999") // valid VIN
	e[4].SetText("123")               // PIN too short, ignored
	e[5].SetText("CAFEBABE0001")      // valid PSK
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Save changes"))

	if th.VIN() != "YS3FB49F741999999" || th.PSK() != "CAFEBABE0001" {
		t.Errorf("VIN %q PSK %q, want the edited values", th.VIN(), th.PSK())
	}
	if th.PIN() != blockC.pin {
		t.Errorf("PIN = %q, want the invalid edit ignored (%q)", th.PIN(), blockC.pin)
	}

	confirm := topObjects(t, win.Canvas())
	found := false
	for _, o := range confirm {
		if l, ok := o.(*widget.Label); ok && strings.Contains(l.Text, "your car will not start") {
			found = true
		}
	}
	if !found {
		t.Error("no immobilizer warning in the confirmation dialog")
	}
	test.Tap(buttonOf(t, confirm, "No"))

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, orig) {
		t.Error("file changed although the confirmation was declined")
	}
}

// Confirming the save writes the edited fields into the bin file and tells
// the user, both on the dialog's own window.
func TestEditDialogSaveConfirmed(t *testing.T) {
	test.NewTempApp(t)
	path := writeBin(t, testBin())
	tf := new(T8File)
	tf.GetInfo(path)
	win, e := openEditDialog(t, tf)

	e[0].SetText("YS3FB49F741999999")
	e[5].SetText("CAFEBABE0001")
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Save changes"))
	test.Tap(buttonOf(t, topObjects(t, win.Canvas()), "Yes"))

	found := false
	for _, o := range topObjects(t, win.Canvas()) {
		if l, ok := o.(*widget.Label); ok && l.Text == "Data has been stored in T8 bin file." {
			found = true
		}
	}
	if !found {
		t.Error("no success message after saving")
	}

	th := new(T8File).GetInfo(path)
	if th.VIN() != "YS3FB49F741999999" || th.PSK() != "CAFEBABE0001" {
		t.Errorf("saved VIN %q PSK %q, want the edited values", th.VIN(), th.PSK())
	}
}
