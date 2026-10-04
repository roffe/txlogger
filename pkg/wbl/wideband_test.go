package wbl

import (
	"slices"
	"testing"

	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
)

// The adapter filter must carry exactly the ids the selected wideband
// subscribes to, and nothing when it isn't on CAN.
func TestCANIDs(t *testing.T) {
	for _, c := range []struct {
		typ, port string
		want      []uint32
	}{
		{ecumaster.ProductString, "", []uint32{0x664, 0x665}},
		{aem.ProductString, "CAN", []uint32{0x180}},
		{aem.ProductString, "/dev/ttyUSB0", nil},
		{innovate.ProductString, "CAN", nil},
		{"None", "CAN", nil},
	} {
		if got := CANIDs(c.typ, c.port); !slices.Equal(got, c.want) {
			t.Errorf("CANIDs(%q, %q) = %X, want %X", c.typ, c.port, got, c.want)
		}
	}
}
