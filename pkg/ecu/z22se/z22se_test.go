package z22se

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

// The client registers as "Z22SE" at 500 kbit/s listening on the UUDT and
// diagnostic reply IDs, and ecu.New builds it from that name.
func TestRegistration(t *testing.T) {
	info := ecu.Info("Z22SE")
	if info.CANRate != 500 || !slices.Equal(info.Filter, []uint32{0x5E8, 0x7E8}) || info.ManualReset {
		t.Fatalf("registration = %+v", info)
	}
	bus, err := gocan.OpenAdapter(context.Background(), newFake(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c, err := ecu.New(bus, &ecu.Config{Name: "Z22SE"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*Client); !ok {
		t.Fatalf("ecu.New built a %T", c)
	}
}

// Marrying and recovery are refused, and the DTC read and ECU info print
// are no-ops that send nothing.
func TestUnsupportedOperations(t *testing.T) {
	f := newFake(nil)
	c, _ := newClient(t, f)
	ctx := testCtx(t)
	if err := c.MarryECU(ctx, "x"); err == nil || err.Error() != "not supported" {
		t.Errorf("MarryECU = %v", err)
	}
	if err := c.RecoverECU(ctx, nil); err == nil || err.Error() != "not supported" {
		t.Errorf("RecoverECU = %v", err)
	}
	if dtcs, err := c.ReadDTC(ctx); dtcs != nil || err != nil {
		t.Errorf("ReadDTC = %v, %v", dtcs, err)
	}
	if err := c.PrintECUInfo(ctx); err != nil {
		t.Errorf("PrintECUInfo = %v", err)
	}
	f.with(func() {
		if len(f.sent) != 0 {
			t.Errorf("sent %v", f.sent)
		}
	})
}

// RequestSecurityAccess unlocks level 01 with the T8 seed/key algorithm:
// the fake's seed 4A21 rotated right by five bits is 0A51, plus B988 gives
// the key C3D9, sent with sub-function 02.
func TestRequestSecurityAccess(t *testing.T) {
	f := newFake(nil)
	c, _ := newClient(t, f)
	if err := c.RequestSecurityAccess(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if len(f.keys) != 1 || !bytes.Equal(f.keys[0], []byte{0x02, 0xC3, 0xD9}) {
			t.Fatalf("key requests = % X, want 02 C3 D9", f.keys)
		}
	})
}

// A rejected key fails RequestSecurityAccess.
func TestRequestSecurityAccessRejected(t *testing.T) {
	f := newFake(nil)
	f.rejectKeys = true
	c, _ := newClient(t, f)
	if err := c.RequestSecurityAccess(testCtx(t)); err == nil || !strings.Contains(err.Error(), "SecurityAccess") {
		t.Fatalf("RequestSecurityAccess = %v, want a security access error", err)
	}
}

// SendAckMessageT8 sends a bare flow control byte to 0x7E0.
func TestSendAckMessageT8(t *testing.T) {
	f := newFake(nil)
	c, _ := newClient(t, f)
	c.SendAckMessageT8()
	f.with(func() {
		if len(f.sent) != 1 || f.sent[0] != gocan.NewFrame(0x7E0, []byte{0x30}) {
			t.Fatalf("sent %v", f.sent)
		}
	})
}
