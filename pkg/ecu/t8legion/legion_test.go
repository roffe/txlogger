package t8legion

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
)

// pattern returns n bytes that never form an all-FF block.
func pattern(n, seed int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((i*31 + seed*17) % 251)
	}
	return b
}

// sameBytes fails the test at the first offset where got and want differ.
func sameBytes(t *testing.T, what string, got, want []byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d bytes, want %d", what, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: first difference at 0x%X: got %02X, want %02X", what, i, got[i], want[i])
		}
	}
}

func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if sub == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("error = %v, want one containing %q", err, sub)
	}
}

// Command names what each IDemand command asks of the loader.
func TestCommandString(t *testing.T) {
	cases := map[Command]string{
		SetInterFrameLatency:     "configure inter frame latency",
		GetCRC32:                 "full crc-32",
		GetTrionic8MD5:           "Trionic8 MD5",
		GetTrionic8MCPMD5:        "Trionic8 MCP MD5",
		StartSecondaryBootloader: "start secondary bootloader",
		MarrySecondaryProcessor:  "marry secondary processor",
		ReadADCPin:               "read ADC pin",
		7:                        "unknown command",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("Command(%d).String() = %q, want %q", byte(c), got, want)
		}
	}
}

// partitionList renders the partitions 1..9 set in a format mask in order
// and ignores bits above partition 9.
func TestPartitionList(t *testing.T) {
	cases := []struct {
		mask uint64
		want string
	}{
		{0, ""},
		{0x001, "1"},
		{0x105, "1, 3, 9"},
		{0x1FF, "1, 2, 3, 4, 5, 6, 7, 8, 9"},
		{0xFE00, ""},
	}
	for _, c := range cases {
		if got := partitionList(c.mask); got != c.want {
			t.Errorf("partitionList(%#x) = %q, want %q", c.mask, got, c.want)
		}
	}
}

// checkErr rejects the replies the loader sends instead of data to a
// readDataByLocalIdentifier request and accepts a positive 0x61 response
// in either byte 1 or byte 2.
func TestCheckErr(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"7E", frame8(0x7E), "got 0x7E message"},
		{"blank", frame8(), "got blank response"},
		{"no security access", frame8(0x03, 0x7F, 0x23, 0x31), "no security access granted"},
		{"keep alive answer", frame8(0x01, 0x7E), "incorrect response"},
		{"first frame", frame8(0x10, 0x82, 0x61, 0x00, 1, 2, 3, 4), ""},
		{"single frame", frame8(0x05, 0x61, 0x00, 1, 2, 3, 4), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantErr(t, checkErr(gocan.NewFrame(0x7E8, c.data)), c.want)
		})
	}
}

// A negative response to readDataByLocalIdentifier is an error, not data:
// taken for data its response code lands where the loader reports how many
// erased blocks to skip, and the read returns FF.
func TestReadDataByLocalIdentifierRejectsNegativeResponse(t *testing.T) {
	if err := checkErr(gocan.NewFrame(0x7E8, frame8(0x03, 0x7F, 0x21, 0x31))); err == nil {
		t.Error("checkErr accepted 03 7F 21 31")
	}
	cl, f, _ := runningClient(t)
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		return [][]byte{frame8(0x03, 0x7F, 0x21, 0x31)}, true
	}
	b, skip, err := cl.ReadDataByLocalIdentifier(testCtx(t), true, EcuByte_T8, 0, 0x80)
	if err == nil {
		t.Fatalf("read of a refused block returned %d bytes, skip %d and no error", len(b), skip)
	}
}

// demandErr turns the status byte of an IDemand reply into "done" (nil) or
// the reason to ask again, per command.
func TestDemandErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cmd    Command
		status byte
		want   string
	}{
		{SetInterFrameLatency, 1, ""},
		{SetInterFrameLatency, 0, "failed to set inter frame latency"},
		{GetCRC32, 1, ""},
		{GetCRC32, 0, "crc-32 not ready yet"},
		{GetTrionic8MD5, 1, ""},
		{GetTrionic8MCPMD5, 0, "MD5 not ready yet"},
		{StartSecondaryBootloader, 1, ""},
		{StartSecondaryBootloader, 0, "secondary loader not ready yet"},
		{MarrySecondaryProcessor, 1, ""},
		{MarrySecondaryProcessor, 0xFF, "failed to start the secondary loader"},
		{MarrySecondaryProcessor, 0xFD, "retrying write"},
		{MarrySecondaryProcessor, 0xFE, "retrying format"},
		{MarrySecondaryProcessor, 0x02, "busy marrying"},
		{ReadADCPin, 1, ""},
		{ReadADCPin, 0, "ADC read not ready yet"},
		{7, 0, ""},
		{7, 0xFF, "bootloader did what it could and failed"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d/%02X", byte(c.cmd), c.status), func(t *testing.T) {
			t.Parallel() // some statuses sleep before asking again
			wantErr(t, demandErr(c.cmd, []byte{0x33, 0x55, byte(c.cmd), c.status, 0, 0, 0, 0}), c.want)
		})
	}
}

// New picks the inter-frame latency for the adapter it runs on, and
// EnableHighSpeed hands it to the loader as the wish of IDemand(0), big
// endian in bytes 6-7.
func TestEnableHighSpeedLatencyPerAdapter(t *testing.T) {
	cases := []struct {
		adapter string
		want    uint16
	}{
		{"txbridge wifi", 1100},
		{"OBDLink Tech2", 580},
		{"Just4Trionic", 580},
		{"STN2120", 60},
		{"CANUSB VCP", 50},
		{"CombiAdapter", 680},
		{"J2534 #1", 100},
		{"YACA", 10},
		{"CANlib #0", 10},
		{"SLCAN", 10},
		{"rCAN", 10},
		{"SocketCAN", 50},
		{"something else", 200},
	}
	for _, c := range cases {
		t.Run(c.adapter, func(t *testing.T) {
			f := newFake()
			f.name = c.adapter
			f.running = true
			cl, _ := newTestClient(t, f)
			if err := cl.EnableHighSpeed(testCtx(t)); err != nil {
				t.Fatal(err)
			}
			var got uint16
			f.with(func() { got = f.ifl })
			if got != c.want {
				t.Errorf("loader latency = %d, want %d", got, c.want)
			}
			sent := f.framesOn(0x7E0)
			want := []byte{0x02, 0xA5, 0x00, 0x00, 0x00, 0x00, byte(c.want >> 8), byte(c.want)}
			if len(sent) != 1 || !bytes.Equal(sent[0].Bytes(), want) {
				t.Errorf("sent %v, want one frame % X", sent, want)
			}
		})
	}
}

// Ping succeeds on the loader's DE AD reply and reports a negative
// response, any other reply or silence as an error.
func TestPing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		reply []byte
		want  string
	}{
		{"alive", frame8(0xDE, 0xAD, 0xF0, 0x0F), ""},
		{"negative", frame8(0x03, 0x7F, 0xEF, 0x11), "LegionPing: Unknown - Service not supported"},
		{"other", frame8(0x01, 0x02), "LegionPing: no response"},
		{"silent", nil, "LegionPing: context deadline exceeded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFake()
			f.hook = func(gocan.Frame) ([][]byte, bool) {
				if c.reply == nil {
					return nil, true
				}
				return [][]byte{c.reply}, true
			}
			cl, _ := newTestClient(t, f)
			wantErr(t, cl.Ping(testCtx(t)), c.want)
			sent := f.framesOn(0x7E0)
			want := []byte{0xEF, 0xBE, 0x00, 0x00, 0x00, 0x00, 0x33, 0x66}
			if len(sent) != 1 || !bytes.Equal(sent[0].Bytes(), want) {
				t.Errorf("sent %v, want one frame % X", sent, want)
			}
		})
	}
}

// Exit sends 01 20 and succeeds when the loader answers 01 60; a negative
// response, any other reply or silence is an error.
func TestExit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		reply []byte
		want  string
	}{
		{"ok", nil, ""},
		{"negative", frame8(0x03, 0x7F, 0x20, 0x12), "LegionExit: ReturnToNormalMode - SubFunction not supported"},
		{"other", frame8(0x01, 0x61), "LegionExit: invalid response 0161"},
		{"silent", []byte{}, "LegionExit: context deadline exceeded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cl, f, _ := runningClient(t)
			switch {
			case c.reply == nil: // the fake loader answers
			case len(c.reply) == 0:
				f.hook = func(gocan.Frame) ([][]byte, bool) { return nil, true }
			default:
				f.hook = func(gocan.Frame) ([][]byte, bool) { return [][]byte{c.reply}, true }
			}
			wantErr(t, cl.Exit(testCtx(t)), c.want)
			sent := f.framesOn(0x7E0)
			if len(sent) != 1 || !bytes.Equal(sent[0].Bytes(), []byte{0x01, 0x20}) {
				t.Errorf("sent %v, want one frame 01 20", sent)
			}
			if c.want == "" {
				f.with(func() {
					if f.running {
						t.Error("loader still running after Exit")
					}
				})
			}
		})
	}
}

// IDemand(GetCRC32) carries the command in byte 2 and the wish in bytes 6-7
// and returns the four checksum bytes of the loader's reply.
func TestIDemandCRC32(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.crc = [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	got, err := cl.IDemand(testCtx(t), GetCRC32, 0x0102)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, f.crc[:]) {
		t.Errorf("crc = % X, want % X", got, f.crc)
	}
	sent := f.framesOn(0x7E0)
	want := []byte{0x02, 0xA5, 0x01, 0x00, 0x00, 0x00, 0x01, 0x02}
	if len(sent) != 1 || !bytes.Equal(sent[0].Bytes(), want) {
		t.Errorf("sent %v, want one frame % X", sent, want)
	}
}

// IDemand for an md5 waits for the loader to hash the partition and reads
// the 16 byte digest back with readDataByLocalIdentifier on pci 7; GetMD5
// does the same for either md5 command and refuses any other.
func TestIDemandMD5(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() {
		copy(f.flash[EcuByte_T8][0x20000:], pattern(0x20000, 1))
		copy(f.flash[EcuByte_MCP][0x8000:], pattern(0x8000, 2))
	})
	got, err := cl.IDemand(testCtx(t), GetTrionic8MD5, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := md5.Sum(f.mem(EcuByte_T8, 0x20000, 0x40000))
	if !bytes.Equal(got, want[:]) {
		t.Errorf("md5 = % X, want % X", got, want)
	}

	got, err = cl.GetMD5(testCtx(t), GetTrionic8MCPMD5, 2)
	if err != nil {
		t.Fatal(err)
	}
	want = md5.Sum(f.mem(EcuByte_MCP, 0x8000, 0x10000))
	if !bytes.Equal(got, want[:]) {
		t.Errorf("MCP md5 = % X, want % X", got, want)
	}

	f.with(func() {
		wantReads := []readReq{{7, 0, 16}, {7, 0, 16}}
		if !slices.Equal(f.reads, wantReads) {
			t.Errorf("reads = %v, want %v", f.reads, wantReads)
		}
		f.sent = nil
	})

	if _, err := cl.GetMD5(testCtx(t), GetCRC32, 1); err == nil || err.Error() != "invalid md5 type" {
		t.Errorf("GetMD5(GetCRC32) error = %v, want invalid md5 type", err)
	}
	if n := len(f.framesOn(0x7E0)); n != 0 {
		t.Errorf("GetMD5 with an invalid type sent %d frames", n)
	}
}

// IDemand asks again while the loader answers "not ready".
func TestIDemandRetriesUntilReady(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	f.crc = [4]byte{1, 2, 3, 4}
	f.demandStatus[GetCRC32] = []byte{0x00}
	got, err := cl.IDemand(testCtx(t), GetCRC32, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, f.crc[:]) {
		t.Errorf("crc = % X, want % X", got, f.crc)
	}
	f.with(func() {
		if len(f.demands) != 2 {
			t.Errorf("demands = %v, want the command twice", f.demands)
		}
	})
}

// IDemand gives up at once on a command the loader acknowledges but the
// client has no reply format for.
func TestIDemandUnknownCommand(t *testing.T) {
	cl, f, _ := runningClient(t)
	_, err := cl.IDemand(testCtx(t), 7, 0)
	wantErr(t, err, "command unknown to the legion")
	f.with(func() {
		if len(f.demands) != 1 {
			t.Errorf("demands = %v, want one", f.demands)
		}
	})
}

// IDemand asks a silent loader again after the reply times out, and stops
// with the context's error as soon as the context ends.
func TestIDemandHonoursContext(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	asks := 0
	f.hook = func(gocan.Frame) ([][]byte, bool) {
		asks++
		if asks == 2 {
			cancel() // end the context while the second ask waits for its reply
		}
		return nil, true
	}
	_, err := cl.IDemand(ctx, SetInterFrameLatency, 1200)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if n := len(f.framesOn(0x7E0)); n != 2 {
		t.Errorf("asked %d times, want 2: once timed out, once cancelled", n)
	}
}

// IDemand for an md5 asks again when the digest cannot be read back.
func TestIDemandMD5ReadBackRetried(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	failed := false
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		if !failed && fr.Data[1] == 0x21 {
			failed = true
			return [][]byte{frame8(0x7E)}, true
		}
		return nil, false
	}
	got, err := cl.IDemand(testCtx(t), GetTrionic8MD5, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := md5.Sum(f.mem(EcuByte_T8, 0, t8Size))
	if !bytes.Equal(got, want[:]) {
		t.Errorf("md5 = % X, want % X", got, want)
	}
	f.with(func() {
		if len(f.demands) != 2 {
			t.Errorf("demands %v, want the md5 asked for twice", f.demands)
		}
	})
}

// IDemand gives up at once when the loader reports a hard failure (status
// 0xFF), as the C# reference does, instead of retrying with backoff.
func TestIDemandHardFailureNotRetried(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	f.demandStatus[MarrySecondaryProcessor] = bytes.Repeat([]byte{0xFF}, 20)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := cl.IDemand(ctx, MarrySecondaryProcessor, 0)
	wantErr(t, err, "failed to start the secondary loader")
	f.with(func() {
		if len(f.demands) != 1 {
			t.Errorf("asked %d times, want once", len(f.demands))
		}
	})
}

// IDemand(ReadADCPin) sends the pin as the wish and returns the reading
// from reply byte 4 on.
func TestIDemandADC(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.adc = [2]byte{0x03, 0x1F}
	got, err := cl.IDemand(testCtx(t), ReadADCPin, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0] != 0x03 {
		t.Errorf("ADC = % X, want it to start with 03", got)
	}
	f.with(func() {
		if want := []demandReq{{ReadADCPin, 11}}; !slices.Equal(f.demands, want) {
			t.Errorf("demands %v, want %v", f.demands, want)
		}
	})
}

// IDemand(ReadADCPin) returns both bytes of the loader's 10-bit ADC reading
// (the C# reference reads (b[0]<<8 | b[1]) & 0x3FF).
func TestIDemandADCReturnsTwoBytes(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.adc = [2]byte{0x03, 0x1F}
	got, err := cl.IDemand(testCtx(t), ReadADCPin, 11)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, f.adc[:]) {
		t.Errorf("ADC = % X, want % X", got, f.adc)
	}
}

// flowControls counts the 0x30 frames the client sent to ask for the
// consecutive frames of a read.
func flowControls(f *fakeLegion) int {
	n := 0
	for _, fr := range f.framesOn(0x7E0) {
		if fr.Length == 1 && fr.Data[0] == 0x30 {
			n++
		}
	}
	return n
}

// ReadDataByLocalIdentifier sends pci, 0x21, the length and a big-endian
// address, takes four bytes from the first reply and, after a flow control
// 0x30, the rest from consecutive frames 0x21, 0x22, ...
func TestReadDataByLocalIdentifierMultiFrame(t *testing.T) {
	cl, f, _ := runningClient(t)
	want := pattern(0x80, 3)
	f.with(func() { copy(f.flash[EcuByte_T8][0x1234:], want) })
	got, skip, err := cl.ReadDataByLocalIdentifier(testCtx(t), true, EcuByte_T8, 0x1234, 0x80)
	if err != nil {
		t.Fatal(err)
	}
	if skip != 0 {
		t.Errorf("skip = %d, want 0", skip)
	}
	sameBytes(t, "block", got, want)
	sent := f.framesOn(0x7E0)
	if wantReq := []byte{0x06, 0x21, 0x80, 0x00, 0x00, 0x12, 0x34, 0x00}; !bytes.Equal(sent[0].Bytes(), wantReq) {
		t.Errorf("request % X, want % X", sent[0].Bytes(), wantReq)
	}
	if n := flowControls(f); n != 1 {
		t.Errorf("sent %d flow controls, want 1", n)
	}
}

// A read of at most four bytes is answered by the first frame alone.
func TestReadDataByLocalIdentifierShort(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() { copy(f.flash[EcuByte_T8][0x10:], []byte{1, 2, 3, 4}) })
	got, _, err := cl.ReadDataByLocalIdentifier(testCtx(t), true, EcuByte_T8, 0x10, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Errorf("got % X, want 01 02 03 04", got)
	}
	if n := flowControls(f); n != 0 {
		t.Errorf("sent %d flow controls, want none", n)
	}
}

// In legion mode a first frame tagging N erased blocks (byte 3) ends the
// read: the block comes back as FF without a flow control and N is
// returned so the caller can skip ahead. Outside legion mode the tag is
// ignored and the block is transferred.
func TestReadDataByLocalIdentifierErasedBlocks(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() { f.flash[EcuByte_T8][0x300] = 0x00 }) // six erased blocks from 0

	got, skip, err := cl.ReadDataByLocalIdentifier(testCtx(t), true, EcuByte_T8, 0, 0x80)
	if err != nil {
		t.Fatal(err)
	}
	if skip != 6 || !allFF(got) || len(got) != 0x80 {
		t.Errorf("legion mode: skip %d, %d bytes all FF %v; want skip 6 and 128 FF bytes", skip, len(got), allFF(got))
	}
	if n := flowControls(f); n != 0 {
		t.Errorf("legion mode sent %d flow controls, want none", n)
	}

	got, skip, err = cl.ReadDataByLocalIdentifier(testCtx(t), false, EcuByte_T8, 0, 0x80)
	if err != nil {
		t.Fatal(err)
	}
	if skip != 0 || !allFF(got) || len(got) != 0x80 {
		t.Errorf("plain mode: skip %d, %d bytes all FF %v; want skip 0 and 128 FF bytes", skip, len(got), allFF(got))
	}
	if n := flowControls(f); n != 1 {
		t.Errorf("plain mode sent %d flow controls, want 1", n)
	}
}

// ReadDataByLocalIdentifier reports the loader's refusals, a broken frame
// sequence, missing consecutive frames and silence as errors.
func TestReadDataByLocalIdentifierErrors(t *testing.T) {
	t.Parallel()
	first := frame8(0x10, 0x82, 0x61, 0x00, 1, 2, 3, 4)
	cases := []struct {
		name  string
		first []byte   // reply to the request, nil = silence
		cfs   [][]byte // replies to the flow control
		want  string
	}{
		{"7E", frame8(0x7E), nil, "got 0x7E message"},
		{"blank", frame8(), nil, "got blank response"},
		{"no security access", frame8(0x03, 0x7F, 0x23, 0x31), nil, "no security access granted"},
		{"keep alive answer", frame8(0x01, 0x7E), nil, "incorrect response"},
		{"out of sequence", first, [][]byte{frame8(0x22)}, "received invalid sequenced frame 0x22, expected 0x21"},
		{"no consecutive frames", first, nil, "timeout waiting for data"},
		{"silent", nil, nil, "ReadDataByLocalIdentifier: context deadline exceeded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cl, f, _ := runningClient(t)
			f.hook = func(fr gocan.Frame) ([][]byte, bool) {
				if fr.Data[0] == 0x30 {
					return c.cfs, true
				}
				if c.first == nil {
					return nil, true
				}
				return [][]byte{c.first}, true
			}
			got, _, err := cl.ReadDataByLocalIdentifier(testCtx(t), true, EcuByte_T8, 0, 0x80)
			wantErr(t, err, c.want)
			if got != nil {
				t.Errorf("got % X alongside the error", got)
			}
		})
	}
}

// ReadDataByLocalIdentifier stops waiting for consecutive frames when the
// context ends.
func TestReadDataByLocalIdentifierContextEnds(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() { copy(f.flash[EcuByte_T8], pattern(0x80, 21)) })
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		return nil, fr.Data[0] == 0x30 // swallow the flow control
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err := cl.ReadDataByLocalIdentifier(ctx, true, EcuByte_T8, 0, 0x80)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

// ReadFlashRange dumps flash in 0x80 byte blocks, jumps over runs of erased
// blocks the loader tags instead of transferring them, and reports progress
// from -lastAddress up to lastAddress.
func TestReadFlashRange(t *testing.T) {
	cl, f, ev := runningClient(t)
	f.with(func() {
		mem := f.flash[EcuByte_T8]
		copy(mem[0:], pattern(0x2000, 4))
		copy(mem[0x5000:], pattern(0x1000, 5))
	})
	got, err := cl.ReadFlashRange(testCtx(t), EcuByte_T8, 0, 0x6000)
	if err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "dump", got, f.mem(EcuByte_T8, 0, 0x6000))

	var addrs []int
	f.with(func() {
		for _, r := range f.reads {
			addrs = append(addrs, r.addr)
		}
	})
	if len(addrs) != 64+1+32 || addrs[64] != 0x2000 || addrs[65] != 0x5000 {
		t.Errorf("read %d blocks (65th at %X, 66th at %X), want 64 data blocks, one skip at 0x2000 and 32 from 0x5000",
			len(addrs), addrs[min(64, len(addrs)-1)], addrs[min(65, len(addrs)-1)])
	}
	if n := flowControls(f); n != 96 {
		t.Errorf("sent %d flow controls, want 96 (none for the erased run)", n)
	}
	p := ev.progressValues()
	if p[0] != -0x6000 || p[len(p)-1] != 0x6000 {
		t.Errorf("progress runs %v .. %v, want -24576 .. 24576", p[0], p[len(p)-1])
	}
	if !ev.hasMessage("Downloading 24576 bytes") {
		t.Errorf("messages %v lack the download size", ev.messages)
	}
}

// ReadFlashRange from a start address returns a lastAddress sized buffer
// holding the flash from startAddress up.
func TestReadFlashRangeFromStartAddress(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() { copy(f.flash[EcuByte_T8][0x4000:], pattern(0x4000, 22)) })
	got, err := cl.ReadFlashRange(testCtx(t), EcuByte_T8, 0x4000, 0x8000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0x8000 {
		t.Fatalf("got %d bytes, want 0x8000", len(got))
	}
	sameBytes(t, "requested range", got[0x4000:], f.mem(EcuByte_T8, 0x4000, 0x8000))
}

// ReadFlashRange refuses to run before the loader is up.
func TestReadFlashRangeNotRunning(t *testing.T) {
	f := newFake()
	cl, _ := newTestClient(t, f)
	_, err := cl.ReadFlashRange(testCtx(t), EcuByte_T8, 0, 0x100)
	wantErr(t, err, "legion not running")
	if n := len(f.framesOn(0x7E0)); n != 0 {
		t.Errorf("sent %d frames", n)
	}
}

// A block that fails to read is reported through OnError and retried after
// raising the loader's inter-frame latency by 70.
func TestReadFlashRangeRetryBacksOff(t *testing.T) {
	t.Parallel()
	cl, f, ev := runningClient(t)
	f.with(func() { copy(f.flash[EcuByte_T8], pattern(0x100, 6)) })
	failed := false
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		if !failed && fr.Data[1] == 0x21 {
			failed = true
			return [][]byte{frame8(0x7E)}, true
		}
		return nil, false
	}
	got, err := cl.ReadFlashRange(testCtx(t), EcuByte_T8, 0, 0x100)
	if err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "dump", got, f.mem(EcuByte_T8, 0, 0x100))
	f.with(func() {
		if want := []demandReq{{SetInterFrameLatency, 50 + 70}}; !slices.Equal(f.demands, want) {
			t.Errorf("demands = %v, want %v", f.demands, want)
		}
	})
	errs := ev.errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "retrying read flash: #0") {
		t.Errorf("OnError got %v, want one retry report", errs)
	}
}

// ReadFlashRange stops retrying a block that keeps failing once the context
// ends, and returns the context's error and no data.
func TestReadFlashRangeGivesUpWithContext(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		if fr.Data[1] == 0x21 {
			return [][]byte{frame8(0x7E)}, true
		}
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	got, err := cl.ReadFlashRange(ctx, EcuByte_T8, 0, 0x100)
	if !errors.Is(err, context.DeadlineExceeded) || got != nil {
		t.Fatalf("ReadFlashRange = %d bytes, %v; want no data and context deadline exceeded", len(got), err)
	}
}

// ReadFlashRange starts reading at the block holding startAddress.
func TestReadFlashRangeStartsAtStartAddress(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() { copy(f.flash[EcuByte_T8], pattern(0x8000, 7)) })
	got, err := cl.ReadFlashRange(testCtx(t), EcuByte_T8, 0x4000, 0x8000)
	if err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "requested range", got[0x4000:], f.mem(EcuByte_T8, 0x4000, 0x8000))
	f.with(func() {
		if f.reads[0].addr != 0x4000 || len(f.reads) != 0x80 {
			t.Errorf("first read at %X, %d reads; want 128 reads from 0x4000", f.reads[0].addr, len(f.reads))
		}
	})
}

// t8Image returns a 0x8000 byte image whose boot partition and parts of
// both NVDM partitions hold data, the rest FF.
func t8Image() []byte {
	file := ffBytes(0x8000)
	copy(file[0:], pattern(0x4000, 8))
	copy(file[0x4000:], pattern(0x800, 9))
	copy(file[0x6000:], pattern(0x1000, 10))
	return file
}

// WriteFlash sends every non-FF 0x80 byte block inside the partitions of
// the format mask as an XOR-coded TransferData of 0x88 bytes with a
// checksum-16, leaving blocks outside the mask and all-FF blocks alone.
func TestWriteFlash(t *testing.T) {
	cl, f, ev := runningClient(t)
	boot := pattern(0x4000, 11)
	f.with(func() { copy(f.flash[EcuByte_T8], boot) }) // partitions 2 and 3 are erased
	file := t8Image()
	if err := cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, file, 0b110); err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "boot partition", f.mem(EcuByte_T8, 0, 0x4000), boot)
	sameBytes(t, "NVDM partitions", f.mem(EcuByte_T8, 0x4000, 0x8000), file[0x4000:])

	var want []uint32
	for a := uint32(0x4000); a < 0x4800; a += 0x80 {
		want = append(want, a)
	}
	for a := uint32(0x6000); a < 0x7000; a += 0x80 {
		want = append(want, a)
	}
	f.with(func() {
		if !slices.Equal(f.transfers, want) {
			t.Errorf("transferred %d blocks %X, want the 48 data blocks of partitions 2 and 3", len(f.transfers), f.transfers)
		}
	})
	p := ev.progressValues()
	if p[0] != -255 || p[len(p)-1] != 256 {
		t.Errorf("progress runs %v .. %v, want -255 .. 256", p[0], p[len(p)-1])
	}
}

// swapPairs returns b with every byte pair swapped, the layout of early
// MCP dumps.
func swapPairs(b []byte) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b); i += 2 {
		out[i], out[i+1] = b[i+1], b[i]
	}
	return out
}

// WriteFlash of a byteswapped MCP image (it starts 08 00 00 20) programs
// the unswapped bytes, and VerifyFlash then matches the file partition by
// partition with MCP md5s.
func TestWriteFlashMCPByteswappedThenVerify(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.with(func() { f.device = EcuByte_MCP }) // as left by the erase
	want := ffBytes(mcpSize)
	copy(want[0:], pattern(0x80, 12))
	copy(want[0:], []byte{0x00, 0x08, 0x20, 0x00})
	copy(want[0x8000:], pattern(0x100, 13))
	copy(want[0x40000:], pattern(0x80, 14))
	file := swapPairs(want)

	if err := cl.WriteFlash(testCtx(t), EcuByte_MCP, mcpSize, file, 0x1FF); err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "MCP flash", f.mem(EcuByte_MCP, 0, mcpSize), want)

	ok, err := cl.VerifyFlash(testCtx(t), file, EcuByte_MCP, 0x1FF)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("VerifyFlash = false after writing the image")
	}
	f.with(func() {
		var wishes []uint16
		for _, d := range f.demands {
			if d.cmd != GetTrionic8MCPMD5 {
				t.Errorf("demand %v, want MCP md5", d)
			}
			wishes = append(wishes, d.wish)
		}
		if want := []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9}; !slices.Equal(wishes, want) {
			t.Errorf("md5 of partitions %v, want %v", wishes, want)
		}
	})
}

// WriteFlash resends a block the loader did not program (a checksum error
// reply, a refused TransferData, no acknowledgement) until it is
// acknowledged.
func TestWriteFlashRetriesBlock(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	file := ffBytes(0x8000)
	copy(file[0x4000:], pattern(0x200, 15))
	f.blockReplies[0x4000] = [][]byte{frame8(0x03, 0x7F, 0x36, 0x77)}
	f.tdReject[0x4080] = 1
	f.blockReplies[0x4100] = [][]byte{nil}
	if err := cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, file, 0b110); err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "NVDM partitions", f.mem(EcuByte_T8, 0x4000, 0x8000), file[0x4000:])
	f.with(func() {
		if want := []uint32{0x4000, 0x4000, 0x4080, 0x4080, 0x4100, 0x4100, 0x4180}; !slices.Equal(f.transfers, want) {
			t.Errorf("transfers %X, want %X", f.transfers, want)
		}
	})
}

// WriteFlash does nothing for an empty format mask and refuses to run
// before the loader is up.
func TestWriteFlashGuards(t *testing.T) {
	cl, f, _ := runningClient(t)
	if err := cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, t8Image(), 0); err != nil {
		t.Fatal(err)
	}
	if n := len(f.framesOn(0x7E0)); n != 0 {
		t.Errorf("empty mask sent %d frames", n)
	}
	cl.legionRunning = false
	wantErr(t, cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, t8Image(), 0b110), "legion not running")
}

// WriteFlash resends a block the loader answers 01 7F 36 12 (the loader's
// "flash is slow to respond" reply) instead of 01 76.
func TestWriteFlashRetriesSlowFlashReply(t *testing.T) {
	cl, f, _ := runningClient(t)
	file := ffBytes(0x8000)
	copy(file[0x4000:], pattern(0x80, 16))
	f.blockReplies[0x4000] = [][]byte{frame8(0x01, 0x7F, 0x36, 0x12)}
	if err := cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, file, 0b110); err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "block at 0x4000", f.mem(EcuByte_T8, 0x4000, 0x4080), file[0x4000:0x4080])
}

// WriteFlash gives up on a block the loader keeps refusing (the C#
// reference stops after 20 tries) instead of retrying it forever.
func TestWriteFlashGivesUpOnRefusedBlock(t *testing.T) {
	cl, f, _ := runningClient(t)
	file := ffBytes(0x8000)
	copy(file[0x4000:], pattern(0x80, 17))
	f.tdReject[0x4000] = 30 // then it accepts, so the buggy loop still ends
	if err := cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, file, 0b110); err == nil {
		t.Error("WriteFlash succeeded after 30 refusals of the same block, want it to give up")
	}
}

// "Flash is slow to respond" replies do not count towards the 20 tries:
// WriteFlash resends the block until the loader takes it.
func TestWriteFlashSlowRepliesNotCounted(t *testing.T) {
	cl, f, _ := runningClient(t)
	file := ffBytes(0x8000)
	copy(file[0x4000:], pattern(0x80, 19))
	for range 25 {
		f.blockReplies[0x4000] = append(f.blockReplies[0x4000], frame8(0x01, 0x7F, 0x36, 0x33))
	}
	if err := cl.WriteFlash(testCtx(t), EcuByte_T8, 0x8000, file, 0b110); err != nil {
		t.Fatal(err)
	}
	sameBytes(t, "block at 0x4000", f.mem(EcuByte_T8, 0x4000, 0x4080), file[0x4000:0x4080])
}

// EraseFlash sends dev 34 01 with the format mask and its complement packed
// into bytes 3-6, waits for the status poll to echo the device, then polls
// until the loader reports 1; the loader erases exactly the masked
// partitions.
func TestEraseFlash(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		dev   byte
		mask  uint64
		polls []int
		frame []byte
		parts []int
	}{
		{"t8 app", EcuByte_T8, 0x110, nil, []byte{0x06, 0x34, 0x01, 0xFE, 0xEF, 0x01, 0x10, 0x00}, []int{5, 9}},
		{"t8 all", EcuByte_T8, math.MaxUint64, nil, []byte{0x06, 0x34, 0x01, 0x00, 0x00, 0xFF, 0xFF, 0x00}, []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"mcp busy", EcuByte_MCP, 0x105, []int{5, 3, 2}, []byte{0x05, 0x34, 0x01, 0xFE, 0xFA, 0x01, 0x05, 0x00}, []int{1, 3, 9}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cl, f, ev := runningClient(t)
			size := len(f.flash[c.dev])
			before := pattern(size, 18)
			f.with(func() {
				copy(f.flash[c.dev], before)
				f.erasePolls = c.polls
			})
			if err := cl.EraseFlash(testCtx(t), c.dev, c.mask); err != nil {
				t.Fatal(err)
			}
			want := bytes.Clone(before)
			for _, p := range c.parts {
				start, end := partitionRange(c.dev, p)
				copy(want[start:end], ffBytes(end-start))
			}
			sameBytes(t, "flash after erase", f.mem(c.dev, 0, size), want)
			f.with(func() {
				if len(f.erases) != 1 || !bytes.Equal(f.erases[0].frame[:], c.frame) {
					t.Errorf("erase requests %v, want one % X", f.erases, c.frame)
				}
				wantPolls := max(len(c.polls), 1) + 1
				if len(f.reads) != wantPolls {
					t.Errorf("polled %d times, want %d", len(f.reads), wantPolls)
				}
			})
			for _, m := range []string{fmt.Sprintf("Erasing device %d...", c.dev), "Erase completed after"} {
				if !ev.hasMessage(m) {
					t.Errorf("messages %v lack %q", ev.messages, m)
				}
			}
		})
	}
}

// EraseFlash gives up after 21 failed status polls.
func TestEraseFlashNoResponse(t *testing.T) {
	cl, f, _ := runningClient(t)
	polls := make([]int, 30)
	for i := range polls {
		polls[i] = -1
	}
	f.erasePolls = polls
	wantErr(t, cl.EraseFlash(testCtx(t), EcuByte_T8, 0x110), "erase failed, no response")
	f.with(func() {
		if len(f.reads) != 21 {
			t.Errorf("polled %d times, want 21", len(f.reads))
		}
	})
}

// When the first status poll does not echo the device (the loader missed
// the request) EraseFlash waits and resends the same erase request.
func TestEraseFlashResendsMissedRequest(t *testing.T) {
	t.Parallel()
	cl, f, ev := runningClient(t)
	before := pattern(t8Size, 23)
	missed := false
	f.with(func() {
		copy(f.flash[EcuByte_T8], before)
		f.polls = []int{0} // status of a loader that has no erase running
		f.erasePolls = []int{int(EcuByte_T8), 2}
		f.hook = func(fr gocan.Frame) ([][]byte, bool) {
			if !missed && fr.Data[1] == 0x34 && fr.Data[2] == 0x01 {
				missed = true
				return nil, true // the loader misses the first request
			}
			return nil, false
		}
	})
	if err := cl.EraseFlash(testCtx(t), EcuByte_T8, 0x110); err != nil {
		t.Fatal(err)
	}
	wantFrame := []byte{0x06, 0x34, 0x01, 0xFE, 0xEF, 0x01, 0x10, 0x00}
	var erases int
	for _, fr := range f.framesOn(0x7E0) {
		if fr.Data[1] == 0x34 {
			erases++
			if !bytes.Equal(fr.Bytes(), wantFrame) {
				t.Errorf("erase request % X, want % X", fr.Bytes(), wantFrame)
			}
		}
	}
	if erases != 2 {
		t.Errorf("sent %d erase requests, want 2", erases)
	}
	want := bytes.Clone(before)
	for _, p := range []int{5, 9} {
		start, end := partitionRange(EcuByte_T8, p)
		copy(want[start:end], ffBytes(end-start))
	}
	sameBytes(t, "flash after erase", f.mem(EcuByte_T8, 0, t8Size), want)
	f.with(func() {
		if len(f.reads) != 4 {
			t.Errorf("polled %d times, want 4: missed, echo, busy, done", len(f.reads))
		}
	})
	for _, m := range []string{"Erasing device 6...", "Erase completed after"} {
		if !ev.hasMessage(m) {
			t.Errorf("messages %v lack %q", ev.messages, m)
		}
	}
}

// EraseFlash does nothing for an empty mask and refuses to run before the
// loader is up.
func TestEraseFlashGuards(t *testing.T) {
	cl, f, _ := runningClient(t)
	if err := cl.EraseFlash(testCtx(t), EcuByte_T8, 0); err != nil {
		t.Fatal(err)
	}
	if n := len(f.framesOn(0x7E0)); n != 0 {
		t.Errorf("empty mask sent %d frames", n)
	}
	cl.legionRunning = false
	wantErr(t, cl.EraseFlash(testCtx(t), EcuByte_T8, 0x110), "legion not running")
}

// EraseFlash only gives up after 21 consecutive failed polls: a good poll
// in between starts the count again, as in the C# reference.
func TestEraseFlashRetryCountResets(t *testing.T) {
	cl, f, _ := runningClient(t)
	polls := []int{6}
	for range 20 {
		polls = append(polls, -1)
	}
	polls = append(polls, 2, -1, 1)
	f.erasePolls = polls
	if err := cl.EraseFlash(testCtx(t), EcuByte_T8, 0x110); err != nil {
		t.Fatalf("erase with 21 failed polls, never more than 20 in a row: %v", err)
	}
}

// differingFile returns a copy of mem with one byte changed in each of the
// given partitions of dev.
func differingFile(mem []byte, dev byte, parts []int) []byte {
	file := bytes.Clone(mem)
	for _, p := range parts {
		start, _ := partitionRange(dev, p)
		file[start+0x10] ^= 0x5A
	}
	return file
}

// DeterminePartitionmask tags each partition whose md5 differs between the
// file and the ECU. It only compares boot when formatting boot (or on a
// Z22SE), masks off boot and NVDM unless they are to be formatted, and on
// the MCP mirrors boot onto the shadow (9) and leaves partition 7 to the
// loader unless on a Z22SE.
func TestDeterminePartitionmask(t *testing.T) {
	cases := []struct {
		name              string
		dev               byte
		differ            []int
		boot, nvdm, z22se bool
		want              uint64
		from              uint16 // first partition compared
		msg               string // what is reported about the mask
	}{
		{"t8 app only", EcuByte_T8, []int{1, 2, 5, 9}, false, false, false, 0x110, 2, "Partitions tagged for erase and write: 5, 9"},
		{"t8 boot and nvdm", EcuByte_T8, []int{1, 2, 5, 9}, true, true, false, 0x113, 1, "Partitions tagged for erase and write: 1, 2, 5, 9"},
		{"t8 boot keeps nvdm", EcuByte_T8, []int{1, 2, 5, 9}, true, false, false, 0x111, 1, "Partitions tagged for erase and write: 1, 5, 9"},
		{"t8 nvdm keeps boot", EcuByte_T8, []int{1, 2, 5, 9}, false, true, false, 0x112, 2, "Partitions tagged for erase and write: 2, 5, 9"},
		{"z22se ignores flags", EcuByte_T8, []int{1, 2, 5, 9}, false, false, true, 0x113, 1, "Partitions tagged for erase and write: 1, 2, 5, 9"},
		{"t8 identical", EcuByte_T8, nil, true, true, false, 0, 1, "No partitions differ, nothing to erase"},
		{"mcp boot mirrored to shadow", EcuByte_MCP, []int{1, 3, 7, 9}, true, true, false, 0x105, 1, "Partitions tagged for erase and write: 1, 3, 9"},
		{"mcp without boot", EcuByte_MCP, []int{1, 3, 7, 9}, false, true, false, 0x004, 2, "Partitions tagged for erase and write: 3"},
		{"z22se mcp keeps 7", EcuByte_MCP, []int{1, 3, 7, 9}, true, true, true, 0x145, 1, "Partitions tagged for erase and write: 1, 3, 7, 9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl, f, ev := runningClient(t)
			mem := pattern(len(f.flash[c.dev]), 19)
			f.with(func() { copy(f.flash[c.dev], mem) })
			file := differingFile(mem, c.dev, c.differ)

			got, err := cl.DeterminePartitionmask(testCtx(t), file, c.dev, c.boot, c.nvdm, c.z22se)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("mask = %#03x, want %#03x", got, c.want)
			}
			wantCmd := GetTrionic8MD5
			if c.dev == EcuByte_MCP {
				wantCmd = GetTrionic8MCPMD5
			}
			var wantDemands []demandReq
			for p := c.from; p <= 9; p++ {
				wantDemands = append(wantDemands, demandReq{wantCmd, p})
			}
			f.with(func() {
				if !slices.Equal(f.demands, wantDemands) {
					t.Errorf("demands %v, want %v", f.demands, wantDemands)
				}
			})
			if !ev.hasMessage(c.msg) {
				t.Errorf("messages %v lack %q", ev.messages, c.msg)
			}
		})
	}
}

// DeterminePartitionmask fails when a partition md5 cannot be fetched.
func TestDeterminePartitionmaskMD5Error(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	f.demandStatus[GetTrionic8MD5] = bytes.Repeat([]byte{0}, 30)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_, err := cl.DeterminePartitionmask(ctx, ffBytes(t8Size), EcuByte_T8, false, false, false)
	wantErr(t, err, "legion: failed to get md5 for block 2")
}

// VerifyFlash compares the md5s of only the partitions in the mask and is
// true when all of them match.
func TestVerifyFlash(t *testing.T) {
	cl, f, _ := runningClient(t)
	mem := pattern(t8Size, 20)
	f.with(func() { copy(f.flash[EcuByte_T8], mem) })
	file := differingFile(mem, EcuByte_T8, []int{3})

	ok, err := cl.VerifyFlash(testCtx(t), file, EcuByte_T8, 0x110)
	if err != nil || !ok {
		t.Fatalf("VerifyFlash over partitions 5 and 9 = %v, %v; want true", ok, err)
	}
	f.with(func() {
		want := []demandReq{{GetTrionic8MD5, 5}, {GetTrionic8MD5, 9}}
		if !slices.Equal(f.demands, want) {
			t.Errorf("demands %v, want %v", f.demands, want)
		}
	})

	ok, err = cl.VerifyFlash(testCtx(t), file, EcuByte_T8, 0x114)
	if err != nil || ok {
		t.Fatalf("VerifyFlash over the differing partition 3 = %v, %v; want false", ok, err)
	}
}

// VerifyFlash fails when a partition md5 cannot be fetched.
func TestVerifyFlashMD5Error(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	f.demandStatus[GetTrionic8MD5] = bytes.Repeat([]byte{0}, 30)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_, err := cl.VerifyFlash(ctx, ffBytes(t8Size), EcuByte_T8, 0x10)
	wantErr(t, err, "legion: failed to get md5 for block 5")
}

// MarryMCP asks the loader to marry the MCP, waiting out "busy" replies,
// and then reports the MCP firmware version stored at 0x810C.
func TestMarryMCP(t *testing.T) {
	t.Parallel() // a busy reply is followed by a 500 ms pause
	cl, f, ev := runningClient(t)
	f.with(func() {
		copy(f.flash[EcuByte_MCP][0x810C:], "FW 1.2.3 X")
		f.demandStatus[MarrySecondaryProcessor] = []byte{0x02}
	})
	if err := cl.MarryMCP(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"MarryMCP...", "MCP Firmware information: FW 1.2.3 X"} {
		if !ev.hasMessage(m) {
			t.Errorf("messages %v lack %q", ev.messages, m)
		}
	}
	f.with(func() {
		if want := []demandReq{{MarrySecondaryProcessor, 0}, {MarrySecondaryProcessor, 0}}; !slices.Equal(f.demands, want) {
			t.Errorf("demands %v, want %v", f.demands, want)
		}
		if want := []readReq{{EcuByte_MCP, 0x8100, 0x80}}; !slices.Equal(f.reads, want) {
			t.Errorf("reads %v, want %v", f.reads, want)
		}
	})
}

// MarryMCP and GetMCPVersion report a version that cannot be read.
func TestMarryMCPVersionError(t *testing.T) {
	cl, f, _ := runningClient(t)
	f.hook = func(fr gocan.Frame) ([][]byte, bool) {
		if fr.Data[1] == 0x21 {
			return [][]byte{frame8(0x7E)}, true
		}
		return nil, false
	}
	wantErr(t, cl.MarryMCP(testCtx(t)), "got 0x7E message")
	_, err := cl.GetMCPVersion(testCtx(t))
	wantErr(t, err, "got 0x7E message")
}

// MarryMCP fails when the loader never confirms the marriage.
func TestMarryMCPNoAnswer(t *testing.T) {
	t.Parallel()
	cl, f, _ := runningClient(t)
	f.hook = func(gocan.Frame) ([][]byte, bool) { return nil, true }
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := cl.MarryMCP(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

// StartSecondaryBootloader asks the loader to start the MCP loader.
func TestStartSecondaryBootloader(t *testing.T) {
	cl, f, ev := runningClient(t)
	if err := cl.StartSecondaryBootloader(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if !f.mcpRunning || !slices.Equal(f.demands, []demandReq{{StartSecondaryBootloader, 0}}) {
			t.Errorf("demands %v, MCP loader running %v", f.demands, f.mcpRunning)
		}
	})
	if !ev.hasMessage("Start MCP bootloader...") {
		t.Errorf("messages %v", ev.messages)
	}
}
