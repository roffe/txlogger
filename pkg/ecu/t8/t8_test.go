package t8

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

// The package registers the Trionic 8 flasher with its bus settings, and
// ecu.New builds a T8 client for it.
func TestRegistration(t *testing.T) {
	info := ecu.Info("Trionic 8")
	if info.CANRate != 500 {
		t.Errorf("CANRate = %v, want 500", info.CANRate)
	}
	if len(info.Filter) != 2 || info.Filter[0] != 0x5E8 || info.Filter[1] != 0x7E8 {
		t.Errorf("Filter = %#x, want [0x5e8 0x7e8]", info.Filter)
	}
	bus, err := gocan.OpenAdapter(context.Background(), newFakeT8())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	cl, err := ecu.New(bus, &ecu.Config{Name: "Trionic 8"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cl.(*Client); !ok {
		t.Errorf("ecu.New returned %T, want *t8.Client", cl)
	}
}

// writesOf returns the $3B requests the ECU received.
func writesOf(ecu *fakeT8) []didWrite {
	defer ecu.lock()()
	return append([]didWrite(nil), ecu.writes...)
}

// The parameter setters write the scaled values to their identifiers, in
// the wire format TrionicCANLib documents.
func TestParameterWrites(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(*Client, context.Context) error
		pid  byte
		data []byte
	}{
		{"top speed 270 km/h", func(c *Client, ctx context.Context) error { return c.SetTopSpeed(ctx, 270) }, 0x02, []byte{0x0A, 0x8C}},
		{"rpm limit 6150", func(c *Client, ctx context.Context) error { return c.SetRPMLimit(ctx, 6150) }, 0x29, []byte{0x18, 0x06}},
		{"oil quality 52.05%", func(c *Client, ctx context.Context) error { return c.SetOilQuality(ctx, 52.05078125) }, 0x25, []byte{0x00, 0x00, 0x34, 0x0D}},
		{"VIN", func(c *Client, ctx context.Context) error { return c.SetVehicleVIN(ctx, "YS3FB49F741000001") }, 0x90, []byte("YS3FB49F741000001")},
	} {
		t.Run(c.name, func(t *testing.T) {
			ecu := newFakeT8()
			cl, _ := newClient(t, ecu)
			if err := c.do(cl, testCtx(t)); err != nil {
				t.Fatal(err)
			}
			w := writesOf(ecu)
			if len(w) != 1 || w[0].pid != c.pid || !bytes.Equal(w[0].data, c.data) {
				t.Errorf("writes = %+v, want one %02X % X", w, c.pid, c.data)
			}
		})
	}
}

// SetVehicleVIN rejects a VIN that is not 17 characters without talking to
// the ECU.
func TestSetVehicleVINLength(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	for _, vin := range []string{"", "YS3FB49F74100000", "YS3FB49F7410000011"} {
		if err := c.SetVehicleVIN(testCtx(t), vin); err == nil {
			t.Errorf("SetVehicleVIN(%q) succeeded", vin)
		}
	}
	if n := len(ecu.sentTo(0x7E0)); n != 0 {
		t.Errorf("%d frames sent for invalid VINs", n)
	}
}

// The parameter getters read and scale their identifiers.
func TestParameterReads(t *testing.T) {
	ecu := newFakeT8()
	ecu.dids[0x02] = []byte{0x0A, 0x8C}
	ecu.dids[0x29] = []byte{0x18, 0x06}
	ecu.dids[0x90] = []byte("YS3FB49F741000001\x00\x00")
	c, _ := newClient(t, ecu)
	ctx := testCtx(t)

	if v, err := c.GetTopSpeed(ctx); err != nil || v != 270 {
		t.Errorf("GetTopSpeed = %d, %v, want 270", v, err)
	}
	if v, err := c.GetRPMLimiter(ctx); err != nil || v != 6150 {
		t.Errorf("GetRPMLimiter = %d, %v, want 6150", v, err)
	}
	if v, err := c.GetVehicleVIN(ctx); err != nil || v != "YS3FB49F741000001" {
		t.Errorf("GetVehicleVIN = %q, %v, want YS3FB49F741000001", v, err)
	}
}

// The getters report an ECU that cannot answer.
func TestParameterReadErrors(t *testing.T) {
	c, _ := newClient(t, newFakeT8()) // no identifiers: every read is out of range
	ctx := testCtx(t)
	if _, err := c.GetTopSpeed(ctx); err == nil {
		t.Error("GetTopSpeed succeeded")
	}
	if _, err := c.GetOilQuality(ctx); err == nil {
		t.Error("GetOilQuality succeeded")
	}
	if _, err := c.GetE85Percent(ctx); err == nil {
		t.Error("GetE85Percent succeeded")
	}
}

// GetOilQuality reads the 4-byte value the ECU stores (and SetOilQuality
// writes) as 1/256 percent.
func TestGetOilQuality(t *testing.T) {
	ecu := newFakeT8()
	ecu.dids[0x25] = []byte{0x00, 0x00, 0x34, 0x0D}
	c, _ := newClient(t, ecu)
	v, err := c.GetOilQuality(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if v != 52.05078125 {
		t.Errorf("GetOilQuality = %v, want 52.05078125", v)
	}
}

// GetE85Percent requests DPID 0x7A once and reads the percentage from the
// packet the ECU sends on 0x5E8.
func TestGetE85Percent(t *testing.T) {
	ecu := newFakeT8()
	ecu.dpids[0x7A] = []byte{0x7A, 0x00, 0x52, 0x00, 0x00, 0x00, 0xDC, 0x00}
	c, _ := newClient(t, ecu)
	v, err := c.GetE85Percent(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if v != 82 {
		t.Errorf("GetE85Percent = %v, want 82", v)
	}
	if !ecu.sentWith(0x03, 0xAA, 0x01, 0x7A) {
		t.Errorf("no AA 01 7A request in % X", ecu.sentTo(0x7E0))
	}
}

// GetE85Percent reads only the percentage byte: the packet's other bytes
// carry unrelated values (sample answer from TrionicCANLib).
func TestGetE85PercentOtherBytes(t *testing.T) {
	ecu := newFakeT8()
	ecu.dpids[0x7A] = []byte{0x7A, 0x00, 0x52, 0x04, 0x00, 0x16, 0xDC, 0x00}
	c, _ := newClient(t, ecu)
	v, err := c.GetE85Percent(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if v != 82 {
		t.Errorf("GetE85Percent = %v, want 82", v)
	}
}

// SetE85Percent sends DeviceControl CPID 0x18 with the percentage.
func TestSetE85Percent(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	if err := c.SetE85Percent(testCtx(t), 85); err != nil {
		t.Fatal(err)
	}
	if !ecu.sentWith(0x04, 0xAE, 0x18, 0x00, 0x55) {
		t.Errorf("no AE 18 00 55 request in % X", ecu.sentTo(0x7E0))
	}
	unlock := ecu.lock()
	got := ecu.devctl
	unlock()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x18, 0x00, 0x55}) {
		t.Errorf("device control requests = % X, want [18 00 55]", got)
	}

	ecu.nrc[0xAE] = 0x31
	if err := c.SetE85Percent(testCtx(t), 10); err == nil {
		t.Error("SetE85Percent succeeded with the request refused")
	}
}

// RequestSecurityAccess unlocks level 01 with the T8 key for the seed.
func TestRequestSecurityAccess(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	if err := c.RequestSecurityAccess(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if !ecu.sentWith(0x02, 0x27, 0x01) || !ecu.sentWith(0x04, 0x27, 0x02) {
		t.Errorf("seed/key exchange missing in % X", ecu.sentTo(0x7E0))
	}
}

// The RequestECUInfoAs helpers decode an identifier's bytes big endian, as
// a NUL-stripped string, and reject answers of the wrong size.
func TestRequestECUInfoAs(t *testing.T) {
	ecu := newFakeT8()
	ecu.dids[0x01] = []byte("GMPT\x000100\x00")
	ecu.dids[0x02] = []byte{0xFF, 0xFF, 0xFF, 0xFE}
	ecu.dids[0x03] = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	ecu.dids[0x04] = []byte{0x01, 0x02}
	c, _ := newClient(t, ecu)
	ctx := testCtx(t)

	if v, err := c.RequestECUInfoAsString(ctx, 0x01); err != nil || v != "GMPT0100" {
		t.Errorf("AsString = %q, %v, want GMPT0100", v, err)
	}
	if v, err := c.RequestECUInfoAsInt32(ctx, 0x02); err != nil || v != -2 {
		t.Errorf("AsInt32 = %d, %v, want -2", v, err)
	}
	if v, err := c.RequestECUInfoAsUint32(ctx, 0x02); err != nil || v != 0xFFFFFFFE {
		t.Errorf("AsUint32 = %#x, %v, want 0xfffffffe", v, err)
	}
	if v, err := c.RequestECUInfoAsInt64(ctx, 0x03); err != nil || v != 0x0102030405060708 {
		t.Errorf("AsInt64 = %#x, %v, want 0x0102030405060708", v, err)
	}
	if v, err := c.RequestECUInfoAsUint64(ctx, 0x03); err != nil || v != 0x0102030405060708 {
		t.Errorf("AsUint64 = %#x, %v, want 0x0102030405060708", v, err)
	}

	for name, f := range map[string]func() error{
		"AsInt32 short":  func() error { _, err := c.RequestECUInfoAsInt32(ctx, 0x04); return err },
		"AsUint32 short": func() error { _, err := c.RequestECUInfoAsUint32(ctx, 0x04); return err },
		"AsInt64 4":      func() error { _, err := c.RequestECUInfoAsInt64(ctx, 0x02); return err },
		"AsUint64 4":     func() error { _, err := c.RequestECUInfoAsUint64(ctx, 0x02); return err },
		"AsString none":  func() error { _, err := c.RequestECUInfoAsString(ctx, 0x77); return err },
		"AsInt32 none":   func() error { _, err := c.RequestECUInfoAsInt32(ctx, 0x77); return err },
		"AsUint32 none":  func() error { _, err := c.RequestECUInfoAsUint32(ctx, 0x77); return err },
		"AsInt64 none":   func() error { _, err := c.RequestECUInfoAsInt64(ctx, 0x77); return err },
	} {
		if f() == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// SendAckMessageT8 sends a bare flow-control frame to the ECU.
func TestSendAckMessageT8(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	c.SendAckMessageT8()
	sent := ecu.sentTo(0x7E0)
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{0x30}) {
		t.Errorf("sent % X, want [30]", sent)
	}
}

// PrintECUInfo does nothing on a T8.
func TestPrintECUInfo(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	if err := c.PrintECUInfo(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := len(ecu.sentTo(0x7E0)); n != 0 {
		t.Errorf("%d frames sent", n)
	}
}

// ResetECU has nothing to do while the legion bootloader is not running,
// and tells a running one to exit.
func TestResetECU(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	ctx := testCtx(t)
	if err := c.ResetECU(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(ecu.sentTo(0x7E0)); n != 0 {
		t.Fatalf("%d frames sent without legion running", n)
	}

	func() { defer ecu.lock()(); ecu.legion = true }()
	if !c.legion.Alive(ctx) {
		t.Fatal("legion not alive")
	}
	if err := c.ResetECU(ctx); err != nil {
		t.Fatal(err)
	}
	if !ecu.sentWith(0x01, 0x20) {
		t.Errorf("no legion exit (01 20) in % X", ecu.sentTo(0x7E0))
	}
}

// EraseECU needs the legion bootloader running.
func TestEraseECUWithoutLegion(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	if err := c.EraseECU(testCtx(t)); err == nil {
		t.Fatal("EraseECU succeeded without legion")
	}
	if n := len(ecu.sentTo(0x7E0)); n != 0 {
		t.Errorf("%d frames sent", n)
	}
}

// EraseECU erases every partition of the T8 and waits for the loader to
// report completion.
func TestEraseECU(t *testing.T) {
	ecu := newFakeT8()
	ecu.legion = true
	ecu.flash = make([]byte, 0x100000) // all zero: fully programmed
	c, rec := newClient(t, ecu)
	ctx := testCtx(t)
	if !c.legion.Alive(ctx) {
		t.Fatal("legion not alive")
	}
	if err := c.EraseECU(ctx); err != nil {
		t.Fatal(err)
	}
	// device 6, 34 01, ~mask, mask: all of 0xFFFF
	if !ecu.sentWith(0x06, 0x34, 0x01, 0x00, 0x00, 0xFF, 0xFF, 0x00) {
		t.Errorf("no erase-all command in % X", ecu.sentTo(0x7E0))
	}
	// the loader's erase status is polled until it reports done (1)
	if !ecu.sentWith(0xF0, 0x21) {
		t.Errorf("no erase status poll in % X", ecu.sentTo(0x7E0))
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !slices.ContainsFunc(rec.msgs, func(m string) bool { return strings.HasPrefix(m, "Erase completed") }) {
		t.Errorf("no completion message in %q", rec.msgs)
	}
	if rec.errs != nil {
		t.Errorf("errors reported: %v", rec.errs)
	}
}
