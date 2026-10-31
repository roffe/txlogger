package shortcuts

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := Binding{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		Key:      fyne.KeyF5,
		Action:   ActionMap,
		Target:   "BstKnkCal.MaxAirmass",
		ECU:      "T7",
	}
	out, ok := decode(in.encode())
	if !ok {
		t.Fatalf("decode(%q) failed", in.encode())
	}
	if out != in {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "1|F5", "notanumber|F5|x|y", "1||x|y"} {
		if _, ok := decode(s); ok {
			t.Errorf("decode(%q) should have failed", s)
		}
	}
}

// Bindings saved before per-ECU support have no ECU field and must stay active
// on every ECU.
func TestDecodeLegacyEntryAppliesToAllECUs(t *testing.T) {
	b, ok := decode("2|1|" + ActionMap + "|Insp_mat!")
	if !ok {
		t.Fatal("legacy entry should decode")
	}
	if b.ECU != ECUAll || !b.AppliesTo("T5") || !b.AppliesTo("T8") {
		t.Fatalf("got %+v, want ECU=%s applying everywhere", b, ECUAll)
	}
}

func TestAppliesTo(t *testing.T) {
	b := Binding{ECU: "T7"}
	if !b.AppliesTo("T7") || b.AppliesTo("T8") {
		t.Fatalf("T7-scoped binding applied to the wrong ECU")
	}
}
