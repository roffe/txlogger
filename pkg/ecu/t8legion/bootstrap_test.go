package t8legion

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
)

const (
	chunk      = 238 // image bytes behind one 0xF0 TransferData: 34 frames of 7
	chunkData  = 234 // of which the transfer carries 0xF0 - 6
	loaderBase = 0x102400
	z22seBase  = 0xFF2000
)

// uploadedImage returns what an upload of the first n chunks of blob
// programs: 234 data bytes per chunk followed by the 4 data bytes of the
// final 0x0A transfer.
func uploadedImage(blob []byte, n int) []byte {
	var out []byte
	for i := range n {
		out = append(out, blob[i*chunk:i*chunk+chunkData]...)
	}
	return append(out, blob[n*chunk:n*chunk+4]...)
}

// The loader images are laid out the way the upload framing consumes them:
// 70 chunks of 238 bytes (34 frames of 7) holding 234 data bytes padded with
// DE AD BE EF, then one 7 byte final frame. The Z22SE preloader fits in the
// six chunks UploadZ22sePreloader sends; the rest of its image is FF.
func TestLoaderImageLayout(t *testing.T) {
	for name, blob := range map[string][]byte{"loader": bootloaderBytes, "z22se preloader": bootloaderz22seBytes} {
		if len(blob) != 70*chunk+7 {
			t.Errorf("%s image is %d bytes, want %d", name, len(blob), 70*chunk+7)
			continue
		}
		for i := range 70 {
			if pad := blob[i*chunk+chunkData : (i+1)*chunk]; !bytes.Equal(pad, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
				t.Errorf("%s chunk %d ends % X, want DE AD BE EF", name, i, pad)
			}
		}
	}
	for i := 6; i < 70; i++ {
		if !allFF(bootloaderz22seBytes[i*chunk : i*chunk+chunkData]) {
			t.Errorf("z22se preloader chunk %d holds data past the six chunks that are uploaded", i)
		}
	}
}

// UploadBootloader sends the loader image from 0x102400 as 70 TransferData
// blocks of 0xF0 (234 data bytes in consecutive frames 0x21..0x2F, 0x20, ...)
// plus a final 4 byte block, keeping the ECU awake with a tester present
// every ten blocks and once at the end.
func TestUploadBootloader(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.unlocked = true
	cl, ev := newTestClient(t, f)
	if err := cl.UploadBootloader(testCtx(t), false); err != nil {
		t.Fatal(err)
	}
	want := uploadedImage(bootloaderBytes, 70)
	sameBytes(t, "RAM at 0x102400", f.ramBytes(loaderBase, len(want)), want)

	var wantAddrs []uint32
	for i := range 71 {
		wantAddrs = append(wantAddrs, uint32(loaderBase+i*0xEA))
	}
	f.with(func() {
		if !slices.Equal(f.transfers, wantAddrs) {
			t.Errorf("transfers %X, want %X", f.transfers, wantAddrs)
		}
		if !bytes.Equal(f.downloadPCI, []byte{0x06}) {
			t.Errorf("requestDownload PCIs % X, want 06", f.downloadPCI)
		}
	})
	tp := f.framesOn(0x101)
	if len(tp) != 8 {
		t.Errorf("sent %d tester presents, want 8", len(tp))
	}
	for _, fr := range tp {
		if !bytes.Equal(fr.Bytes(), []byte{0xFE, 0x01, 0x3E}) {
			t.Errorf("tester present % X, want FE 01 3E", fr.Bytes())
		}
	}
	if !ev.hasMessage("Uploading bootloader 16667 bytes") || !ev.hasMessage("Done, took:") {
		t.Errorf("messages %v", ev.messages)
	}
}

// UploadBootloader never reports more progress than the total it announced
// with a negative value.
func TestUploadBootloaderProgressWithinTotal(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.unlocked = true
	cl, ev := newTestClient(t, f)
	if err := cl.UploadBootloader(testCtx(t), false); err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, v := range ev.progressValues() {
		if v < 0 {
			total = -v
		} else if v > total {
			t.Fatalf("progress %v exceeds the announced total %v", v, total)
		}
	}
}

// UploadZ22sePreloader requests a Z22SE download (PCI 05) and sends the six
// chunks of the preloader image from 0xFF2000 plus a final 4 byte block.
func TestUploadZ22sePreloader(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.unlocked = true
	cl, _ := newTestClient(t, f)
	if err := cl.UploadZ22sePreloader(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	want := uploadedImage(bootloaderz22seBytes, 6)
	sameBytes(t, "RAM at 0xFF2000", f.ramBytes(z22seBase, len(want)), want)
	f.with(func() {
		if len(f.transfers) != 7 || f.transfers[6] != z22seBase+6*0xEA {
			t.Errorf("transfers %X, want 7 from 0xFF2000", f.transfers)
		}
		if !bytes.Equal(f.downloadPCI, []byte{0x05}) {
			t.Errorf("requestDownload PCIs % X, want 05", f.downloadPCI)
		}
	})
}

// Both uploads stop at the first download step the ECU refuses or does not
// acknowledge and return why.
func TestUploadErrors(t *testing.T) {
	t.Parallel()
	uploads := []struct {
		name   string
		base   uint32
		blocks int
		run    func(*Client, context.Context) error
	}{
		{"loader", loaderBase, 70, func(c *Client, ctx context.Context) error { return c.UploadBootloader(ctx, false) }},
		{"z22se preloader", z22seBase, 6, (*Client).UploadZ22sePreloader},
	}
	cases := []struct {
		name  string
		setup func(f *fakeLegion, base, final uint32)
		want  string
	}{
		{"download refused", func(f *fakeLegion, _, _ uint32) { f.unlocked = false }, "RequestDownload[2]"},
		{"transfer refused", func(f *fakeLegion, base, _ uint32) { f.tdReject[base] = 1 }, "TransferData[2]"},
		{"block nack", func(f *fakeLegion, base, _ uint32) {
			f.blockReplies[base] = [][]byte{frame8(0x03, 0x7F, 0x36, 0x77)}
		}, "TransferData - Block transfer data checksum error"},
		{"block bad ack", func(f *fakeLegion, base, _ uint32) { f.blockReplies[base] = [][]byte{frame8(0x01, 0x02)} }, "invalid transfer data response"},
		{"block silent", func(f *fakeLegion, base, _ uint32) { f.blockReplies[base] = [][]byte{nil} }, "block response: context deadline exceeded"},
		{"final transfer refused", func(f *fakeLegion, _, final uint32) { f.tdReject[final] = 1 }, "TransferData[2]"},
		{"final nack", func(f *fakeLegion, _, final uint32) {
			f.blockReplies[final] = [][]byte{frame8(0x03, 0x7F, 0x36, 0x77)}
		}, "TransferData - Block transfer data checksum error"},
		{"final bad ack", func(f *fakeLegion, _, final uint32) { f.blockReplies[final] = [][]byte{frame8(0x01, 0x02)} }, "invalid transfer data response"},
		{"final silent", func(f *fakeLegion, _, final uint32) { f.blockReplies[final] = [][]byte{nil} }, "context deadline exceeded"},
	}
	for _, u := range uploads {
		for _, c := range cases {
			t.Run(u.name+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				f := newFake()
				f.unlocked = true
				c.setup(f, u.base, u.base+uint32(u.blocks*0xEA))
				cl, _ := newTestClient(t, f)
				wantErr(t, u.run(cl, testCtx(t)), c.want)
			})
		}
	}
}

// Alive pings the loader and marks the client running when it answers;
// a silent ECU is pinged three times and reported through OnError.
func TestAlive(t *testing.T) {
	t.Parallel()
	t.Run("running", func(t *testing.T) {
		f := newFake()
		f.running = true
		cl, ev := newTestClient(t, f)
		if !cl.Alive(testCtx(t)) || !cl.IsRunning() {
			t.Fatal("Alive = false with the loader running")
		}
		if !ev.hasMessage("legion is running") || len(ev.errors()) != 0 {
			t.Errorf("messages %v, errors %v", ev.messages, ev.errors())
		}
	})
	t.Run("silent", func(t *testing.T) {
		t.Parallel()
		f := newFake()
		cl, ev := newTestClient(t, f)
		if cl.Alive(testCtx(t)) || cl.IsRunning() {
			t.Fatal("Alive = true with stock firmware")
		}
		if n := len(f.framesOn(0x7E0)); n != 3 {
			t.Errorf("pinged %d times, want 3", n)
		}
		if errs := ev.errors(); len(errs) != 1 {
			t.Errorf("OnError got %v, want the ping failure", errs)
		}
	})
}

// Bootstrap on an ECU whose loader already runs skips the upload and only
// hands the loader the inter-frame latency.
func TestBootstrapAlreadyRunning(t *testing.T) {
	f := newFake()
	f.running = true
	cl, ev := newTestClient(t, f)
	if err := cl.Bootstrap(testCtx(t), false); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if len(f.stockLog) != 0 || len(f.transfers) != 0 {
			t.Errorf("stock requests %v, transfers %d; want none", f.stockLog, len(f.transfers))
		}
		if f.ifl != 50 {
			t.Errorf("loader latency %d, want 50", f.ifl)
		}
	})
	if !ev.hasMessage("enabling high speed mode") {
		t.Errorf("messages %v", ev.messages)
	}
}

// Bootstrap takes a stock ECU into programming mode, unlocks it at level 1,
// uploads the loader to 0x102400, starts it and hands it the inter-frame
// latency.
func TestBootstrapUploadsLoader(t *testing.T) {
	t.Parallel()
	f := newFake()
	cl, ev := newTestClient(t, f)
	if err := cl.Bootstrap(testCtx(t), false); err != nil {
		t.Fatal(err)
	}
	if !cl.IsRunning() {
		t.Error("IsRunning = false after Bootstrap")
	}
	want := []string{"10 02", "28", "A2", "A5 01", "A5 03", "27 01", "27 02", "34 (06)", "36 80 102400"}
	f.with(func() {
		if !slices.Equal(f.stockLog, want) {
			t.Errorf("stock requests %q, want %q", f.stockLog, want)
		}
		if f.accessLevel != 0x01 || f.ifl != 50 {
			t.Errorf("access level %02X, loader latency %d; want 01 and 50", f.accessLevel, f.ifl)
		}
	})
	img := uploadedImage(bootloaderBytes, 70)
	sameBytes(t, "RAM at 0x102400", f.ramBytes(loaderBase, len(img)), img)
	for _, m := range []string{"ECU Programmed state: fully programmed", "starting bootloader", "legion is running"} {
		if !ev.hasMessage(m) {
			t.Errorf("messages %v lack %q", ev.messages, m)
		}
	}
}

// Bootstrap of a Z22SE first uploads and starts the preloader at 0xFF2000,
// and both downloads use the Z22SE PCI.
func TestBootstrapZ22SE(t *testing.T) {
	t.Parallel()
	f := newFake()
	cl, ev := newTestClient(t, f)
	if err := cl.Bootstrap(testCtx(t), true); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if want := []uint32{z22seBase, loaderBase}; !slices.Equal(f.executed, want) {
			t.Errorf("executed %X, want %X", f.executed, want)
		}
		if !bytes.Equal(f.downloadPCI, []byte{0x05, 0x05}) {
			t.Errorf("requestDownload PCIs % X, want 05 05", f.downloadPCI)
		}
	})
	pre := uploadedImage(bootloaderz22seBytes, 6)
	sameBytes(t, "RAM at 0xFF2000", f.ramBytes(z22seBase, len(pre)), pre)
	img := uploadedImage(bootloaderBytes, 70)
	sameBytes(t, "RAM at 0x102400", f.ramBytes(loaderBase, len(img)), img)
	if !ev.hasMessage("starting z22se preloader") {
		t.Errorf("messages %v", ev.messages)
	}
}

// Bootstrap returns the error of a refused programming step without
// uploading anything.
func TestBootstrapPreflightRefused(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		if fr.Data[1] == 0x10 {
			return [][]byte{frame8(0x03, 0x7F, 0x10, 0x22)}, true
		}
		return nil, false
	}
	cl, _ := newTestClient(t, f)
	wantErr(t, cl.Bootstrap(testCtx(t), false), "InitiateDiagnosticOperation[2]")
	f.with(func() {
		if len(f.stockLog) != 0 || len(f.transfers) != 0 {
			t.Errorf("stock requests %v, transfers %d after the refusal", f.stockLog, len(f.transfers))
		}
	})
}

// bootstrapPreFlight stops at the first step the ECU refuses and returns
// its error.
func TestBootstrapPreFlightSteps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		refuse func(d [8]byte) bool
		want   string
		before []string
	}{
		{"diagnostic session", func(d [8]byte) bool { return d[1] == 0x10 }, "InitiateDiagnosticOperation[2]", nil},
		{"disable communication", func(d [8]byte) bool { return d[1] == 0x28 }, "DisableNormalCommunication - Conditions not correct", []string{"10 02"}},
		{"programmed state", func(d [8]byte) bool { return d[1] == 0xA2 }, "ReportProgrammedState[2]", []string{"10 02", "28"}},
		{"programming mode", func(d [8]byte) bool { return d[1] == 0xA5 }, "ProgrammingMode[2]", []string{"10 02", "28", "A2"}},
		{"seed", func(d [8]byte) bool { return d[0] == 0x02 && d[1] == 0x27 }, "SecurityAccessRequestSeed[2]", []string{"10 02", "28", "A2", "A5 01", "A5 03"}},
		{"key", func(d [8]byte) bool { return d[0] == 0x04 && d[1] == 0x27 }, "SecurityAccessSendKey[2]", []string{"10 02", "28", "A2", "A5 01", "A5 03", "27 01"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFake()
			f.hook = func(fr gocan.Frame) ([][]byte, bool) {
				if c.refuse(fr.Data) {
					return [][]byte{frame8(0x03, 0x7F, fr.Data[1], 0x22)}, true
				}
				return nil, false
			}
			cl, _ := newTestClient(t, f)
			wantErr(t, cl.bootstrapPreFlight(testCtx(t)), c.want)
			f.with(func() {
				if !slices.Equal(f.stockLog, c.before) {
					t.Errorf("stock requests %q, want %q", f.stockLog, c.before)
				}
			})
		})
	}
}

// BootstrapRecovery talks on the recovery IDs, carries on when the ECU
// ignores the diagnostic session requests, unlocks at level FD and uploads
// and starts the loader.
func TestBootstrapRecovery(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.reqID, f.respID = 0x011, 0x311
	f.ignore[0x10] = true
	f.ignore[0x28] = true
	cl, _ := newTestClient(t, f)
	if err := cl.BootstrapRecovery(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"A2", "A5 01", "A5 03", "27 FD", "27 FE", "34 (06)", "36 80 102400"}
	f.with(func() {
		if !slices.Equal(f.stockLog, want) {
			t.Errorf("stock requests %q, want %q", f.stockLog, want)
		}
		if f.accessLevel != 0xFD || !f.running {
			t.Errorf("access level %02X, loader running %v; want FD and running", f.accessLevel, f.running)
		}
	})
	img := uploadedImage(bootloaderBytes, 70)
	sameBytes(t, "RAM at 0x102400", f.ramBytes(loaderBase, len(img)), img)
}

// refuseExecute returns a hook that refuses the TransferData $80 (execute)
// of addr.
func refuseExecute(addr uint32) func(gocan.Frame) ([][]byte, bool) {
	return func(fr gocan.Frame) ([][]byte, bool) {
		d := fr.Data
		if d[0] == 0x06 && d[1] == 0x36 && d[2] == 0x80 && binary.BigEndian.Uint32(d[3:7]) == addr {
			return [][]byte{frame8(0x03, 0x7F, 0x36, 0x22)}, true
		}
		return nil, false
	}
}

// refuseService returns a hook that refuses every request whose first two
// bytes are pci and service.
func refuseService(pci, service byte) func(gocan.Frame) ([][]byte, bool) {
	return func(fr gocan.Frame) ([][]byte, bool) {
		if fr.Data[0] == pci && fr.Data[1] == service {
			return [][]byte{frame8(0x03, 0x7F, service, 0x22)}, true
		}
		return nil, false
	}
}

func hasError(errs []error, sub string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), sub) {
			return true
		}
	}
	return false
}

// Bootstrap stops when an upload or the preloader start fails. A failed
// loader start is only reported through OnError, after which Bootstrap
// fails if the loader does not answer, and a loader that never takes the
// inter-frame latency fails it too.
func TestBootstrapFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		z22se   bool
		setup   func(f *fakeLegion)
		timeout time.Duration
		want    string
		onError string
	}{
		{"preloader upload refused", true, func(f *fakeLegion) { f.tdReject[z22seBase] = 1 }, 0, "TransferData[2]", ""},
		{"preloader start refused", true, func(f *fakeLegion) { f.hook = refuseExecute(z22seBase) }, 0,
			"TransferData - Conditions not correct", "TransferData - Conditions not correct"},
		{"loader upload refused", false, func(f *fakeLegion) { f.tdReject[loaderBase] = 1 }, 0, "TransferData[2]", ""},
		{"loader start refused", false, func(f *fakeLegion) { f.hook = refuseExecute(loaderBase) }, 0,
			"legion is not running", "TransferData - Conditions not correct"},
		{"latency refused", false, func(f *fakeLegion) {
			f.running = true
			f.demandStatus[SetInterFrameLatency] = bytes.Repeat([]byte{0}, 30)
		}, 300 * time.Millisecond, "context deadline exceeded", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFake()
			c.setup(f)
			cl, ev := newTestClient(t, f)
			ctx := testCtx(t)
			if c.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, c.timeout)
				defer cancel()
			}
			wantErr(t, cl.Bootstrap(ctx, c.z22se), c.want)
			if c.onError != "" && !hasError(ev.errors(), c.onError) {
				t.Errorf("OnError got %v, want one containing %q", ev.errors(), c.onError)
			}
		})
	}
}

// BootstrapRecovery stops at the first refused programming step or a
// failed upload, and only reports a failed loader start through OnError.
func TestBootstrapRecoveryFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(f *fakeLegion)
		want    string
		onError string
	}{
		{"programmed state refused", func(f *fakeLegion) { f.hook = refuseService(0x01, 0xA2) }, "ReportProgrammedState[2]", ""},
		{"programming mode refused", func(f *fakeLegion) { f.hook = refuseService(0x02, 0xA5) }, "ProgrammingMode[2]", ""},
		{"key refused", func(f *fakeLegion) { f.hook = refuseService(0x04, 0x27) }, "SecurityAccessSendKey[2]", ""},
		{"upload refused", func(f *fakeLegion) { f.tdReject[loaderBase] = 1 }, "TransferData[2]", ""},
		{"start refused", func(f *fakeLegion) { f.hook = refuseExecute(loaderBase) }, "", "TransferData - Conditions not correct"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFake()
			f.reqID, f.respID = 0x011, 0x311
			c.setup(f)
			cl, ev := newTestClient(t, f)
			wantErr(t, cl.BootstrapRecovery(testCtx(t)), c.want)
			if c.onError != "" && !hasError(ev.errors(), c.onError) {
				t.Errorf("OnError got %v, want one containing %q", ev.errors(), c.onError)
			}
		})
	}
}
