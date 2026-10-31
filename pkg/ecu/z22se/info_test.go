package z22se

import (
	"strings"
	"testing"

	"github.com/roffe/txlogger/pkg/model"
)

// Info reads every header in table order, formats each by its type and
// skips (reporting through OnError) the identifiers the ECU refuses.
// Progress starts at minus the header count and climbs to the count.
func TestInfo(t *testing.T) {
	f := newFake(nil)
	f.dids = map[byte][]byte{
		0x90: []byte("YS3FD49Y281012345"),          // VIN, a multi-frame answer
		0x74: []byte("AB\x00CD"),                   // NULs are dropped
		0x95: {0x01, 0x02, 0xAB},                   // hex
		0x02: {0x09, 0xC4},                         // 2500 / 10 km/h
		0x25: {0x00, 0x00, 0x32, 0x00},             // 0x3200 / 256 %
		0x7C: {0x00, 0xBC, 0x61, 0x4E},             // big endian int32
		0x9A: {0x0A, 0x0B},                         // ddi
		0xCB: {0x4E, 0x61, 0xBC, 0x00},             // little endian int32
		0x98: append([]byte("TESTER"), 0, 0, 0x00), // last header
	}
	c, rec := newClient(t, f)
	got, err := c.Info(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id    byte
		desc  string
		value string
	}{
		{0x90, "VIN", "YS3FD49Y281012345"},
		{0x74, "Calibration set ", "ABCD"},
		{0x95, "ECU s/w number", "0102AB"},
		{0x02, "Speed limiter", "250 km/h"},
		{0x25, "Oil quality", "50.00%"},
		{0x7C, "SAAB partnumber", "12345678"},
		{0x9A, "Diagnostic Data Identifier", "0x0A 0x0B"},
		{0xCB, "End model partnumber", "12345678"},
		{0x98, "Tester Serial", "TESTER"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results %+v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Desc != w.desc || got[i].Value != w.value {
			t.Errorf("result %d = {%02X %q %q}, want {%02X %q %q}", i, got[i].ID, got[i].Desc, got[i].Value, w.id, w.desc, w.value)
		}
	}

	if n := len(T8Headers) - len(want); len(rec.errs) != n {
		t.Errorf("got %d errors, want one per refused header (%d): %v", len(rec.errs), n, rec.errs)
	}
	for _, err := range rec.errs {
		if !strings.HasPrefix(err.Error(), "failed to read ") {
			t.Errorf("error %q", err)
		}
	}

	n := float64(len(T8Headers))
	p := rec.progress
	if len(p) < 2 || p[0] != -n || p[len(p)-1] != n {
		t.Errorf("progress = %v, want -%v first and %v last", p, n, n)
	}
	for i := 2; i < len(p); i++ {
		if p[i] <= p[i-1] {
			t.Errorf("progress goes backwards: %v", p)
			break
		}
	}
	if !rec.hasMessage("Fetching ECU info") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// Progress reaches the header count even when the last header is refused,
// so the progress bar fills once every header has been tried.
func TestInfoProgressCompletesWhenLastHeaderRefused(t *testing.T) {
	f := newFake(nil)
	for _, h := range T8Headers[:len(T8Headers)-1] {
		f.dids[h.ID] = []byte{0, 0, 0, 1}
	}
	c, rec := newClient(t, f)
	if _, err := c.Info(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := float64(len(T8Headers)); rec.progress[len(rec.progress)-1] != n {
		t.Fatalf("last progress = %v, want %v", rec.progress[len(rec.progress)-1], n)
	}
}

// Each header type formats the raw identifier bytes its own way; answers
// too short for the type are skipped, reported through OnError except for
// the little endian partnumber, which only logs.
func TestInfoTypes(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		data    []byte // nil: the ECU refuses the identifier
		result  bool   // a result is returned
		want    string // its value
		wantErr bool   // reported through OnError
	}{
		{"string", "string", []byte("Z22SE\x00"), true, "Z22SE", false},
		{"string refused", "string", nil, false, "", true},
		{"hex", "hex", []byte{0xDE, 0xAD}, true, "DEAD", false},
		{"hex refused", "hex", nil, false, "", true},
		{"int32 negative", "int32", []byte{0xFF, 0xFF, 0xFF, 0xFE}, true, "-2", false},
		{"int32 short", "int32", []byte{0x01, 0x02}, false, "", true},
		{"uint32", "uint32", []byte{0x00, 0x01, 0x00, 0x00}, true, "65536", false},
		{"uint32 refused", "uint32", nil, false, "", true},
		{"int64", "int64", []byte{0, 0, 0, 0x01, 0, 0, 0, 0}, true, "4294967296", false},
		{"int64 wrong length", "int64", []byte{0, 0, 0, 1}, false, "", true},
		{"rint32", "rint32", []byte{0x01, 0x00, 0x00, 0x00}, true, "1", false},
		{"rint32 short", "rint32", []byte{0x01}, false, "", false},
		{"rint32 refused", "rint32", nil, false, "", false},
		{"km/h", "km/h", []byte{0x00, 0x64}, true, "10 km/h", false},
		{"km/h wrong length", "km/h", []byte{0x00, 0x64, 0x00}, true, "0 km/h", false},
		{"km/h refused", "km/h", nil, false, "", true},
		{"oil quality", "oilquality", []byte{0x00, 0x00, 0x00, 0x80}, true, "0.50%", false},
		{"oil quality short", "oilquality", []byte{0x80}, false, "", true},
		{"ddi wrong length", "ddi", []byte{0x01}, true, "", false},
		{"ddi refused", "ddi", nil, false, "", true},
		{"unknown type", "bogus", []byte{0x01}, false, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := T8Headers
			T8Headers = []model.Header{{Desc: "Field", ID: 0x42, Type: tc.typ}}
			t.Cleanup(func() { T8Headers = saved })

			f := newFake(nil)
			if tc.data != nil {
				f.dids[0x42] = tc.data
			}
			c, rec := newClient(t, f)
			got, err := c.Info(testCtx(t))
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tc.result && (len(got) != 1 || got[0].Value != tc.want || got[0].ID != 0x42 || got[0].Desc != "Field"):
				t.Errorf("results = %+v, want one with value %q", got, tc.want)
			case !tc.result && len(got) != 0:
				t.Errorf("results = %+v, want none", got)
			}
			if hasErr := len(rec.errs) > 0; hasErr != tc.wantErr {
				t.Errorf("errors = %v, want error %v", rec.errs, tc.wantErr)
			}
			if tc.wantErr && !strings.HasPrefix(rec.errs[0].Error(), "failed to read Field: ") {
				t.Errorf("error %q", rec.errs[0])
			}
		})
	}
}

// The RequestECUInfoAs helpers decode a raw identifier: strings without
// NULs, big endian 32 and 64 bit integers, the 64 bit ones only from
// exactly eight bytes.
func TestRequestECUInfoAs(t *testing.T) {
	f := newFake(nil)
	f.dids = map[byte][]byte{
		0x01: []byte("A\x00B\x00"),
		0x02: {0xFF, 0xFF, 0xFF, 0xFE},
		0x03: {0x80, 0, 0, 0, 0, 0, 0, 0x01},
		0x04: {0x01, 0x02, 0x03},
		0x05: {0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	c, _ := newClient(t, f)
	ctx := testCtx(t)

	if s, err := c.RequestECUInfoAsString(ctx, 0x01); err != nil || s != "AB" {
		t.Errorf("AsString = %q, %v", s, err)
	}
	if v, err := c.RequestECUInfoAsInt32(ctx, 0x02); err != nil || v != -2 {
		t.Errorf("AsInt32 = %d, %v", v, err)
	}
	if v, err := c.RequestECUInfoAsUint32(ctx, 0x02); err != nil || v != 0xFFFFFFFE {
		t.Errorf("AsUint32 = %d, %v", v, err)
	}
	if v, err := c.RequestECUInfoAsInt64(ctx, 0x03); err != nil || v != -0x7FFFFFFFFFFFFFFF {
		t.Errorf("AsInt64 = %d, %v", v, err)
	}
	if v, err := c.RequestECUInfoAsUint64(ctx, 0x03); err != nil || v != 0x8000000000000001 {
		t.Errorf("AsUint64 = %d, %v", v, err)
	}

	for _, pid := range []byte{0x04, 0x05} {
		if _, err := c.RequestECUInfoAsInt64(ctx, pid); err == nil || !strings.Contains(err.Error(), "invalid response length for int64") {
			t.Errorf("AsInt64(%02X) error = %v", pid, err)
		}
		if _, err := c.RequestECUInfoAsUint64(ctx, pid); err == nil || !strings.Contains(err.Error(), "invalid response length for uint64") {
			t.Errorf("AsUint64(%02X) error = %v", pid, err)
		}
	}
	if _, err := c.RequestECUInfoAsInt32(ctx, 0x04); err == nil {
		t.Error("AsInt32 decoded three bytes")
	}
	if _, err := c.RequestECUInfoAsUint32(ctx, 0x04); err == nil {
		t.Error("AsUint32 decoded three bytes")
	}

	// refused identifiers fail every helper
	for name, fn := range map[string]func() error{
		"AsString": func() error { _, err := c.RequestECUInfoAsString(ctx, 0x99); return err },
		"AsInt32":  func() error { _, err := c.RequestECUInfoAsInt32(ctx, 0x99); return err },
		"AsUint32": func() error { _, err := c.RequestECUInfoAsUint32(ctx, 0x99); return err },
		"AsInt64":  func() error { _, err := c.RequestECUInfoAsInt64(ctx, 0x99); return err },
		"AsUint64": func() error { _, err := c.RequestECUInfoAsUint64(ctx, 0x99); return err },
	} {
		if err := fn(); err == nil {
			t.Errorf("%s of a refused identifier succeeded", name)
		}
	}
}
