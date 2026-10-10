package t8

import (
	"strings"
	"testing"
)

// Info reads every T8 header identifier and formats it by type: strings
// without NUL padding, hex dumps, big- and little-endian integers, the speed
// limiter in km/h, the oil quality in percent and the diagnostic data
// identifier as two hex bytes. Identifiers the ECU does not answer are
// reported through OnError and left out.
func TestInfo(t *testing.T) {
	ecu := newFakeT8()
	for _, h := range T8Headers {
		if h.Type == "string" {
			ecu.dids[h.ID] = []byte("STR" + strings.ToUpper(h.Desc[:3]))
		}
	}
	ecu.dids[0x90] = []byte("YS3FB49F741000001")    // multi-frame answer
	ecu.dids[0x92] = []byte("GMPT\x00\x00")         // NUL padded
	ecu.dids[0x95] = []byte{0x00, 0x04}             // hex
	ecu.dids[0x99] = []byte{0x20, 0x04, 0x08, 0x12} // hex
	ecu.dids[0x02] = []byte{0x09, 0x60}             // 2400 = 240.0 km/h
	ecu.dids[0x25] = []byte{0x00, 0x00, 0x34, 0x0D} // 13325/256 %
	ecu.dids[0x7C] = []byte{0x03, 0x4F, 0xC5, 0x0C} // 55559436
	ecu.dids[0x9A] = []byte{0x01, 0x08}             // ddi
	ecu.dids[0xCB] = []byte{0x0C, 0xC5, 0x4F, 0x03} // little endian 55559436
	ecu.dids[0xCC] = []byte{0xFF, 0xFF, 0xFF, 0xFE} // signed
	delete(ecu.dids, 0x0F)                          // unanswered string
	delete(ecu.dids, 0x75)                          // unanswered string
	c, rec := newClient(t, ecu)

	got, err := c.Info(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	values := map[byte]string{}
	for _, r := range got {
		values[r.ID] = r.Value
	}

	for id, want := range map[byte]string{
		0x90: "YS3FB49F741000001",
		0x92: "GMPT",
		0x74: "STRCAL",
		0x95: "0004",
		0x99: "20040812",
		0x02: "240 km/h",
		0x25: "52.05%",
		0x7C: "55559436",
		0x9A: "0x01 0x08",
		0xCB: "55559436",
		0xCC: "-2",
	} {
		if values[id] != want {
			t.Errorf("ID %02X = %q, want %q", id, values[id], want)
		}
	}
	for _, id := range []byte{0x0F, 0x75, 0xA0} { // 0xA0 is hex typed and also unanswered
		if v, ok := values[id]; ok {
			t.Errorf("unanswered ID %02X reported as %q", id, v)
		}
	}
	if len(got) != len(T8Headers)-3 {
		t.Errorf("got %d results, want %d", len(got), len(T8Headers)-3)
	}
	// results keep the header order and carry the header description
	if got[0].ID != 0x90 || got[0].Desc != "VIN" {
		t.Errorf("first result = %02X %q, want 90 VIN", got[0].ID, got[0].Desc)
	}

	if len(rec.errs) != 3 {
		t.Fatalf("OnError called %d times, want 3: %v", len(rec.errs), rec.errs)
	}
	for i, desc := range []string{"0F identifier", "75 identifier", "ManufacturersEnableCounter"} {
		if !strings.Contains(rec.errs[i].Error(), "failed to read "+desc) {
			t.Errorf("error %d = %v, want it to name %q", i, rec.errs[i], desc)
		}
	}
	if len(rec.progress) == 0 || rec.progress[0] != -float64(len(T8Headers)) {
		t.Errorf("first progress = %v, want %v", rec.progress, -float64(len(T8Headers)))
	}
	if last := rec.progress[len(rec.progress)-1]; last != float64(len(T8Headers)) {
		t.Errorf("last progress = %v, want %v", last, float64(len(T8Headers)))
	}
	if !rec.hasMessage("Fetching ECU info") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// Progress reaches the header count even when the last header is refused,
// so the progress bar fills once every header has been tried.
func TestInfoProgressCompletesWhenLastHeaderRefused(t *testing.T) {
	ecu := newFakeT8()
	for _, h := range T8Headers[:len(T8Headers)-1] {
		ecu.dids[h.ID] = []byte{0, 0, 0, 1}
	}
	c, rec := newClient(t, ecu)
	if _, err := c.Info(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := float64(len(T8Headers)); rec.progress[len(rec.progress)-1] != n {
		t.Fatalf("last progress = %v, want %v", rec.progress[len(rec.progress)-1], n)
	}
}

// Info reports a speed limiter or diagnostic data identifier of unexpected
// size as zero and empty, and skips integers too short to decode.
func TestInfoOddSizes(t *testing.T) {
	ecu := newFakeT8()
	ecu.dids[0x02] = []byte{0x09, 0x60, 0x00}
	ecu.dids[0x9A] = []byte{0x01}
	ecu.dids[0x25] = []byte{0x01}
	ecu.dids[0x7C] = []byte{0x01, 0x02}
	ecu.dids[0xCB] = []byte{0x01, 0x02}
	c, _ := newClient(t, ecu)

	got, err := c.Info(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	values := map[byte]string{}
	for _, r := range got {
		values[r.ID] = r.Value
	}
	if v, ok := values[0x02]; !ok || v != "0 km/h" {
		t.Errorf("speed limiter = %q (present %v), want \"0 km/h\"", v, ok)
	}
	if v, ok := values[0x9A]; !ok || v != "" {
		t.Errorf("ddi = %q (present %v), want empty", v, ok)
	}
	for _, id := range []byte{0x25, 0x7C, 0xCB} {
		if v, ok := values[id]; ok {
			t.Errorf("ID %02X with a short answer reported as %q", id, v)
		}
	}
}
