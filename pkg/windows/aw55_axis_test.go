package windows

import (
	"os"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

// The map viewer resolves axes with symbol.GetInfo(ecuType, name), and the ecu
// type comes from the ECU selector — so an AW55 file only draws as a grid if
// "AW55" is a selectable ECU. This is the check that it round-trips.
func TestAW55AxisResolution(t *testing.T) {
	data, err := os.ReadFile("/home/roffe/OneDrive/T8 Source/TCM/AW55/NG9-3_TCM_20260807_155756_2006-05-12_B207R.bin")
	if err != nil {
		t.Skip(err)
	}
	ecuType, fw, err := symbol.Load("tcm.bin", data, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	// what LoadSymbols puts in the selector, and what getECU() reads back
	typ := symbol.ECUTypeFromString(ecuType.String())
	if typ != ecuType {
		t.Fatalf("%q does not round-trip through the ECU selector", ecuType)
	}
	axis := symbol.GetInfo(typ, "Symbol-6")
	if axis.X == "" || axis.Y == "" {
		t.Fatalf("Symbol-6 axes not resolved: %+v", axis)
	}
	x, y, z, _, _, _, err := fw.GetXYZ(axis.X, axis.Y, axis.Z)
	if err != nil {
		t.Fatal(err)
	}
	if len(x)*len(y) != len(z) || len(x) < 2 || len(y) < 2 {
		t.Fatalf("Symbol-6 came out %dx%d over %d values", len(y), len(x), len(z))
	}
	t.Logf("Symbol-6: %dx%d, X=%v Y=%v", len(y), len(x), x, y)
}
