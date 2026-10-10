package dtc

import (
	"regexp"
	"slices"
	"testing"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/dtc/wis"
)

// StatusBytetoString lists the set ISO 14229 statusOfDTC bits from bit 7
// down to bit 0, comma separated, and returns "" when no bit is set.
func TestStatusBytetoString(t *testing.T) {
	tests := []struct {
		status byte
		want   string
	}{
		{0x00, ""},
		{0x01, "failed at the time of the request"},
		{0x02, "failed on the current operation cycle"},
		{0x04, "failed on the current or previous operation cycle"},
		{0x08, "confirmed at the time of the request"},
		{0x10, "test not completed since the last code clear"},
		{0x20, "test failed at least once since last code clear"},
		{0x40, "test not completed this operation cycle"},
		{0x80, "CEL illuminated"},
		{0x09, "confirmed at the time of the request, failed at the time of the request"},
		{0xFF, "CEL illuminated, test not completed this operation cycle, " +
			"test failed at least once since last code clear, " +
			"test not completed since the last code clear, " +
			"confirmed at the time of the request, " +
			"failed on the current or previous operation cycle, " +
			"failed on the current operation cycle, " +
			"failed at the time of the request"},
	}
	for _, tt := range tests {
		if got := StatusBytetoString(tt.status); got != tt.want {
			t.Errorf("StatusBytetoString(%#02x) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// T7StatusString decodes bits 5-6 as the storage state (always reported
// first) and the low nibble as the fault kinds; bits 4 and 7 are ignored.
func TestT7StatusString(t *testing.T) {
	tests := []struct {
		status byte
		want   string
	}{
		{0x00, "not detected"},
		{0x20, "not present"},
		{0x40, "intermittent"},
		{0x60, "present"},
		{0x61, "present, above max"},
		{0x22, "not present, below min"},
		{0x44, "intermittent, no signal"},
		{0x08, "not detected, invalid signal"},
		{0x6F, "present, above max, below min, no signal, invalid signal"},
		{0x90, "not detected"}, // bits 4 and 7 carry no T7 meaning
		{0xE1, "present, above max"},
	}
	for _, tt := range tests {
		if got := T7StatusString(tt.status); got != tt.want {
			t.Errorf("T7StatusString(%#02x) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// Title formats the code the way each ECU reports it: T5 with the decimal
// error counter, T7 with the hex status byte and its decoded meaning, T8
// with the GMLAN failure type meaning, and any other ECU as the bare code.
func TestDTCTitle(t *testing.T) {
	tests := []struct {
		name string
		dtc  DTC
		want string
	}{
		{"T5 counter", DTC{ECU: symbol.ECU_T5, Code: "Kyl_fel", Status: 12}, "Kyl_fel: 12"},
		{"T5 zero", DTC{ECU: symbol.ECU_T5, Code: "ROM_error"}, "ROM_error: 0"},
		{"T7 present", DTC{ECU: symbol.ECU_T7, Code: "P1460", Status: 0x64}, "P1460 64 (present, no signal)"},
		{"T7 not detected", DTC{ECU: symbol.ECU_T7, Code: "P0100", Status: 0x01}, "P0100 01 (not detected, above max)"},
		{"T8 known failure type", DTC{ECU: symbol.ECU_T8, Code: "B0165 02", FailureType: 0x02, Status: 0xAF}, "B0165 02 (short to ground)"},
		{"T8 system specific", DTC{ECU: symbol.ECU_T8, Code: "P0420 F5", FailureType: 0xF5}, "P0420 F5 (system specific)"},
		{"unknown ECU", DTC{ECU: symbol.ECU_UNKNOWN, Code: "U0100", Status: 0x08, FailureType: 0x02}, "U0100"},
		{"ME9.6", DTC{ECU: symbol.ECU_ME96, Code: "P0300", Status: 0x60}, "P0300"},
	}
	for _, tt := range tests {
		if got := tt.dtc.Title(); got != tt.want {
			t.Errorf("%s: Title() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// String is the bare code and StatusString the generic ISO 14229 decoding
// of the status byte, whatever the ECU.
func TestDTCStringAndStatusString(t *testing.T) {
	d := DTC{ECU: symbol.ECU_T7, Code: "P1460", Status: 0x88}
	if got := d.String(); got != "P1460" {
		t.Errorf("String() = %q, want P1460", got)
	}
	if got, want := d.StatusString(), "CEL illuminated, confirmed at the time of the request"; got != want {
		t.Errorf("StatusString() = %q, want %q", got, want)
	}
}

// WISModels returns the WIS car models an ECU appears in, nil for ECUs the
// WIS archive does not cover.
func TestDTCWISModels(t *testing.T) {
	tests := []struct {
		ecu  symbol.ECUType
		want []string
	}{
		{symbol.ECU_T5, nil},
		{symbol.ECU_T7, []string{"9400", "9600"}},
		{symbol.ECU_T8, []string{"9440"}},
		{symbol.ECU_ME96, nil},
		{symbol.ECU_UNKNOWN, nil},
	}
	for _, tt := range tests {
		got := DTC{ECU: tt.ecu, Code: "P0100"}.WISModels()
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: WISModels() = %q, want %q", tt.ecu, got, tt.want)
		}
	}
}

// Every model WISModels hands out is a model the wis package knows, so a
// search for it can hit the archive.
func TestWISModelsAreKnownToWIS(t *testing.T) {
	for ecu, models := range wisModels {
		for _, m := range models {
			if wis.ModelName(m) == m {
				t.Errorf("%s: model %q has no wis display name", ecu, m)
			}
			if len(wis.Years(m)) == 0 {
				t.Errorf("%s: model %q has no years in the wis archive", ecu, m)
			}
		}
	}
}

// Info looks the code up in the per-ECU knowledge table and returns the
// zero DTCInfo for unknown codes or ECUs without a table.
func TestDTCInfo(t *testing.T) {
	tests := []struct {
		name string
		dtc  DTC
		want DTCInfo
	}{
		{"T7 known", DTC{ECU: symbol.ECU_T7, Code: "P1460"},
			DTCInfo{"Immobilizer Active", "If new T7, MIU or TWICE module, program code in TWICE"}},
		{"T5 known", DTC{ECU: symbol.ECU_T5, Code: "Kyl_fel"},
			DTCInfo{"", "Water temp sensor error counter"}},
		{"T7 unknown code", DTC{ECU: symbol.ECU_T7, Code: "P9999"}, DTCInfo{}},
		{"T5 code on T7", DTC{ECU: symbol.ECU_T7, Code: "Kyl_fel"}, DTCInfo{}},
		{"T7 code on T5", DTC{ECU: symbol.ECU_T5, Code: "P1460"}, DTCInfo{}},
		{"T8 empty table", DTC{ECU: symbol.ECU_T8, Code: "B0165 02"}, DTCInfo{}},
		{"ECU without table", DTC{ECU: symbol.ECU_ME96, Code: "P1460"}, DTCInfo{}},
	}
	for _, tt := range tests {
		if got := tt.dtc.Info(); got != tt.want {
			t.Errorf("%s: Info() = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// The knowledge tables are keyed the way the ECU clients produce codes: T7
// uses the SAE "P1234" form, and every entry carries some text to show.
func TestDTCTables(t *testing.T) {
	saeCode := regexp.MustCompile(`^[PBCU][0-9A-F]{4}$`)
	for code, info := range T7DTCS {
		if !saeCode.MatchString(code) {
			t.Errorf("T7DTCS: key %q is not an SAE DTC code", code)
		}
		if info.Name == "" && info.Description == "" {
			t.Errorf("T7DTCS[%q]: no name or description", code)
		}
	}
	for code, info := range T5DTCS {
		if info.Description == "" {
			t.Errorf("T5DTCS[%q]: no description", code)
		}
	}
}
