package j1979

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/dtc"
)

// fakeT7 is a virtual CAN adapter answering J1979 requests the way the T7
// firmware frames them on its KWP transport: the response is split into
// chunks on the response ID (first chunk: length byte + 5 payload bytes,
// later ones 6 bytes, byte 0 counting down the chunks still to come with
// 0x40 on the first and 0x80 on all), the first delivered at once and each
// further one after the tester's 0x266 ack. Replies are delivered from
// inside Send, so they are on the bus before Request starts waiting.
type fakeT7 struct {
	respID   uint32                   // ID responses go out on
	start    []byte                   // 0x238 reply to startCommunication; nil = no reply
	answers  map[string][]byte        // J1979 request bytes -> KWP response payload; missing = NRC serviceNotSupported
	raw      map[string][]gocan.Frame // request bytes -> verbatim reply frames (first now, rest on acks)
	dropAcks bool                     // ignore 0x266 acks, so continuation chunks never come

	mu      sync.Mutex
	bus     *gocan.Bus
	pending []gocan.Frame
	sent    []string
}

func newFake(answers map[string][]byte) *fakeT7 {
	return &fakeT7{respID: RESP_MSG_ID, answers: answers}
}

func (f *fakeT7) Open(_ context.Context, bus *gocan.Bus) error { f.bus = bus; return nil }
func (f *fakeT7) Close() error                                 { return nil }

func (f *fakeT7) Send(_ context.Context, fr gocan.Frame) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, wire(fr))
	switch fr.ID {
	case INIT_MSG_ID:
		if f.start != nil {
			f.bus.Deliver(gocan.NewFrame(INIT_RESP_ID, f.start))
		}
	case REQ_MSG_ID:
		r := string(fr.Data[3 : 3+int(fr.Data[2])])
		if r[0] == stopCommunicationReq {
			return nil
		}
		if frames, ok := f.raw[r]; ok {
			f.queue(frames)
			return nil
		}
		payload, ok := f.answers[r]
		if !ok {
			payload = []byte{negativeResponse, r[0], t7kwp.SERVICE_NOT_SUPPORTED}
		}
		f.queue(chunk(f.respID, payload))
	case RESP_CHUNK_CONF_ID:
		if !f.dropAcks && len(f.pending) > 0 {
			next := f.pending[0]
			f.pending = f.pending[1:]
			f.bus.Deliver(next)
		}
	}
	return nil
}

func (f *fakeT7) queue(frames []gocan.Frame) {
	f.pending = nil
	if len(frames) == 0 {
		return
	}
	f.pending = append(f.pending, frames[1:]...)
	f.bus.Deliver(frames[0])
}

// wireLog returns every frame the client sent, as "ID: bytes".
func (f *fakeT7) wireLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func wire(fr gocan.Frame) string { return fmt.Sprintf("%03X: % X", fr.ID, fr.Data[:fr.Length]) }

// chunk frames a KWP response payload the way the T7 does.
func chunk(id uint32, payload []byte) []gocan.Frame {
	msg := append([]byte{byte(len(payload))}, payload...)
	n := (len(msg) + 5) / 6
	frames := make([]gocan.Frame, n)
	for i := range n {
		d := make([]byte, 8)
		d[0] = 0x80 | byte(n-1-i)
		if i == 0 {
			d[0] |= 0x40
		}
		d[1] = 0xA1
		copy(d[2:], msg[i*6:min(i*6+6, len(msg))])
		frames[i] = gocan.NewFrame(id, d)
	}
	return frames
}

func req(b ...byte) string { return string(b) }

func newClient(t *testing.T, f *fakeT7) *Client {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	return New(bus)
}

// shortTimeout lowers the per-request timeout for tests that wait for a
// reply that never comes.
func shortTimeout(t *testing.T) {
	old := DefaultTimeout
	DefaultTimeout = 20 * time.Millisecond
	t.Cleanup(func() { DefaultTimeout = old })
}

func wantErrContains(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Errorf("err = %v, want one containing %q", err, sub)
	}
}

func wantWire(t *testing.T, f *fakeT7, want ...string) {
	t.Helper()
	if got := f.wireLog(); !reflect.DeepEqual(got, want) {
		t.Errorf("wire =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// StartSession sends startCommunication on 0x220 announcing 0x240 as the
// request ID and switches to the response ID the ECU assigns, which later
// requests then listen on.
func TestStartSession(t *testing.T) {
	f := newFake(map[string][]byte{req(testerPresentReq): {0x7E}})
	f.respID = 0x25A
	f.start = []byte{0xC0, 0xA1, 0x03, 0xC1, 0xEA, 0x8F, 0x02, 0x5A}
	c := newClient(t, f)

	if err := c.StartSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.TesterPresent(context.Background()); err != nil {
		t.Fatalf("TesterPresent on the assigned ID: %v", err)
	}
	wantWire(t, f, "220: 3F 81 00 11 02 40", "240: 40 A1 01 3E")
}

// StartSession turns a negative startCommunication reply into the KWP error
// for its response code, and a missing reply into a deadline error.
func TestStartSessionFailures(t *testing.T) {
	f := newFake(nil)
	f.start = []byte{0xC0, 0xA1, 0x03, 0x7F, 0x81, t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR, 0, 0}
	err := newClient(t, f).StartSession(context.Background())
	if !errors.Is(err, t7kwp.ErrConditionsNotCorrectOrRequestSequenceError) {
		t.Errorf("rejected: err = %v", err)
	}

	shortTimeout(t)
	err = newClient(t, newFake(nil)).StartSession(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) || !strings.HasPrefix(err.Error(), "StartSession") {
		t.Errorf("silent ECU: err = %v", err)
	}
}

// StopSession sends stopCommunication on 0x240 without waiting for a reply.
func TestStopSession(t *testing.T) {
	f := newFake(nil)
	if err := newClient(t, f).StopSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantWire(t, f, "240: 40 A1 01 82")
}

// TesterPresent reports a negative response as the matching KWP error.
func TestTesterPresentRejected(t *testing.T) {
	f := newFake(map[string][]byte{req(testerPresentReq): {0x7F, 0x3E, t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT}})
	err := newClient(t, f).TesterPresent(context.Background())
	if !errors.Is(err, t7kwp.ErrSubFunctionNotSupportedOrInvalidFormat) || !strings.HasPrefix(err.Error(), "TesterPresent") {
		t.Errorf("err = %v", err)
	}
	shortTimeout(t)
	if err := newClient(t, &fakeT7{respID: 0x300}).TesterPresent(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("reply on another ID: err = %v, want deadline", err)
	}
}

// SetResponseID makes requests listen on a session's non-default response ID.
func TestSetResponseID(t *testing.T) {
	f := newFake(map[string][]byte{req(0x01, 0x0D): {0x41, 0x0D, 0x50}})
	f.respID = 0x300
	c := newClient(t, f)
	c.SetResponseID(0x300)
	got, err := c.ReadPID(context.Background(), 0x0D)
	if err != nil || !reflect.DeepEqual(got, []byte{0x50}) {
		t.Errorf("ReadPID = % X, %v", got, err)
	}
}

// ReadPID sends mode $01 with the PID and returns the data after the echoed
// PID, rejecting negative responses, wrong SIDs, wrong echoes and the
// echo-only answer the T7 gives for unsupported PIDs.
func TestReadPID(t *testing.T) {
	cases := []struct {
		name    string
		answer  []byte
		want    []byte
		wantErr string
		is      error
	}{
		{name: "rpm", answer: []byte{0x41, 0x0C, 0x1F, 0x40}, want: []byte{0x1F, 0x40}},
		{name: "negative", answer: []byte{0x7F, 0x01, t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT}, wantErr: "mode $01", is: t7kwp.ErrSubFunctionNotSupportedOrInvalidFormat},
		{name: "unsupported", answer: []byte{0x41, 0x0C}, wantErr: "PID $0C not supported"},
		{name: "wrong echo", answer: []byte{0x41, 0x0D, 0x00}, wantErr: "expected PID $0C echo"},
		{name: "wrong SID", answer: []byte{0x42, 0x0C, 0x00}, wantErr: "unexpected response"},
		{name: "empty", answer: []byte{}, wantErr: "unexpected response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(map[string][]byte{req(0x01, 0x0C): tc.answer})
			got, err := newClient(t, f).ReadPID(context.Background(), 0x0C)
			if tc.wantErr != "" {
				wantErrContains(t, err, tc.wantErr)
				if tc.is != nil && !errors.Is(err, tc.is) {
					t.Errorf("err = %v, want %v", err, tc.is)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ReadPID = % X, %v; want % X", got, err, tc.want)
			}
			wantWire(t, f, "240: 40 A1 02 01 0C")
		})
	}
}

// ReadPIDValue decodes the PID data with the SAE scaling.
func TestReadPIDValue(t *testing.T) {
	f := newFake(map[string][]byte{req(0x01, 0x05): {0x41, 0x05, 130}})
	v, err := newClient(t, f).ReadPIDValue(context.Background(), 0x05)
	if err != nil {
		t.Fatal(err)
	}
	if v.PID != 0x05 || v.Value != 90 || v.Unit != "°C" || v.Name != "Engine coolant temperature" {
		t.Errorf("got %+v", v)
	}
	if _, err := newClient(t, newFake(nil)).ReadPIDValue(context.Background(), 0x05); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected PID (serviceNotSupported)", err)
	}
}

// A request that is never answered fails with a deadline error naming the
// mode.
func TestRequestTimeout(t *testing.T) {
	shortTimeout(t)
	f := newFake(nil)
	f.raw = map[string][]gocan.Frame{req(0x01, 0x0C): nil}
	_, err := newClient(t, f).ReadPID(context.Background(), 0x0C)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline", err)
	}
	wantErrContains(t, err, "mode $01")
}

// A first chunk announcing more bytes than it carries, with no further
// chunks to come, is reported as truncated.
func TestTruncatedResponse(t *testing.T) {
	f := newFake(nil)
	f.raw = map[string][]gocan.Frame{req(0x09, 0x02): {gocan.NewFrame(RESP_MSG_ID, []byte{0xC0, 0xA1, 0x0A, 0x49, 1, 2, 3, 4})}}
	_, err := newClient(t, f).VIN(context.Background())
	wantErrContains(t, err, "truncated response, 5 bytes missing")
}

// A continuation chunk that never arrives after the ack fails the request.
func TestChunkTimeout(t *testing.T) {
	shortTimeout(t)
	f := newFake(map[string][]byte{req(0x09, 0x02): vinPayload("YS3DF78K537000001")})
	f.dropAcks = true
	_, err := newClient(t, f).VIN(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline", err)
	}
	wantErrContains(t, err, "mode $09 chunk")
}

// SupportedPIDs walks the supported-PID pages while each advertises the
// next, returning every supported PID including the page PIDs; the T7's
// two pages give the documented list.
func TestSupportedPIDs(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x01, 0x00): {0x41, 0x00, 0xBE, 0x3F, 0xB8, 0x11},
		req(0x01, 0x20): {0x41, 0x20, 0x80, 0x00, 0x00, 0x00},
	})
	got, err := newClient(t, f).SupportedPIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x01, 0x03, 0x04, 0x05, 0x06, 0x07, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x13, 0x14, 0x15, 0x1C, 0x20, 0x21}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SupportedPIDs = % X, want % X", got, want)
	}
	// Each 6-byte page reply spans two chunks, so every page is acked once.
	wantWire(t, f, "240: 40 A1 02 01 00", "266: 40 A1 3F 81", "240: 40 A1 02 01 20", "266: 40 A1 3F 81")
}

// SupportedPIDs fails only when page $00 fails; a later page that errors or
// is short ends the walk with what was collected.
func TestSupportedPIDsPartial(t *testing.T) {
	page0 := []byte{0x41, 0x00, 0x80, 0x00, 0x00, 0x01} // $01 and $20
	cases := []struct {
		name  string
		page1 []byte // nil = NRC
	}{
		{"page $20 rejected", nil},
		{"page $20 unsupported", []byte{0x41, 0x20}},
		{"page $20 short", []byte{0x41, 0x20, 0x80, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans := map[string][]byte{req(0x01, 0x00): page0}
			if tc.page1 != nil {
				ans[req(0x01, 0x20)] = tc.page1
			}
			got, err := newClient(t, newFake(ans)).SupportedPIDs(context.Background())
			if err != nil || !reflect.DeepEqual(got, []byte{0x01, 0x20}) {
				t.Errorf("SupportedPIDs = % X, %v; want 01 20", got, err)
			}
		})
	}
	if _, err := newClient(t, newFake(nil)).SupportedPIDs(context.Background()); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("page $00 rejected: err = %v", err)
	}
}

// SupportedPIDs stops after page $E0 even if it advertises another page.
func TestSupportedPIDsStopsAtE0(t *testing.T) {
	ans := map[string][]byte{}
	var want []string
	for p := 0x00; p <= 0xE0; p += 0x20 {
		ans[req(0x01, byte(p))] = []byte{0x41, byte(p), 0x00, 0x00, 0x00, 0x01}
		want = append(want, fmt.Sprintf("240: 40 A1 02 01 %02X", p), "266: 40 A1 3F 81")
	}
	ans[req(0x01, 0xE0)] = []byte{0x41, 0xE0, 0x80, 0x00, 0x00, 0x01}
	f := newFake(ans)
	got, err := newClient(t, f).SupportedPIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantWire(t, f, want...)
	// Only the first 8 IDs are checked: page $E0's "next page" bit maps to
	// ID 0x100, which wraps to 0x00 in the byte-sized result.
	if len(got) < 8 || !reflect.DeepEqual(got[:8], []byte{0x20, 0x40, 0x60, 0x80, 0xA0, 0xC0, 0xE0, 0xE1}) {
		t.Errorf("SupportedPIDs = % X", got)
	}
}

// MonitorStatus decodes PID $01 and rejects a reply shorter than 4 bytes.
func TestMonitorStatus(t *testing.T) {
	f := newFake(map[string][]byte{req(0x01, 0x01): {0x41, 0x01, 0x82, 0x07, 0x65, 0x04}})
	m, err := newClient(t, f).MonitorStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !m.MILOn || m.DTCCount != 2 || !m.Evap.Supported || m.Evap.Complete || !m.Catalyst.Complete {
		t.Errorf("got %+v", m)
	}

	f = newFake(map[string][]byte{req(0x01, 0x01): {0x41, 0x01, 0x82, 0x07}})
	_, err = newClient(t, f).MonitorStatus(context.Background())
	wantErrContains(t, err, "short response")

	if _, err := newClient(t, newFake(nil)).MonitorStatus(context.Background()); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected request (serviceNotSupported)", err)
	}
}

// ReadFreezeFramePID sends mode $02 with PID and frame number and returns
// the data after the echoed PID.
func TestReadFreezeFramePID(t *testing.T) {
	cases := []struct {
		name    string
		answer  []byte
		want    []byte
		wantErr string
	}{
		{name: "rpm", answer: []byte{0x42, 0x0C, 0x0F, 0xA0}, want: []byte{0x0F, 0xA0}},
		{name: "unsupported", answer: []byte{0x42, 0x0C}, wantErr: "mode $02: PID $0C not supported"},
		{name: "wrong echo", answer: []byte{0x42, 0x05, 0x10}, wantErr: "expected PID $0C echo"},
		{name: "negative", answer: []byte{0x7F, 0x02, t7kwp.REQUEST_OUT_OF_RANGE}, wantErr: "mode $02"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(map[string][]byte{req(0x02, 0x0C, 0x01): tc.answer})
			got, err := newClient(t, f).ReadFreezeFramePID(context.Background(), 0x0C, 0x01)
			if tc.wantErr != "" {
				wantErrContains(t, err, tc.wantErr)
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got % X, %v; want % X", got, err, tc.want)
			}
			wantWire(t, f, "240: 40 A1 03 02 0C 01")
		})
	}
}

// FreezeFrame reads the DTC that stored the frame, then every PID the
// frame's supported mask lists (skipping the already read $02), decoded.
func TestFreezeFrame(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x02, 0x02, 0x00): {0x42, 0x02, 0x03, 0x00},
		req(0x02, 0x00, 0x00): {0x42, 0x00, 0x7C, 0x10, 0x00, 0x00}, // 02-06, 0C
		req(0x02, 0x03, 0x00): {0x42, 0x03, 0x02},
		req(0x02, 0x04, 0x00): {0x42, 0x04, 0xFF},
		req(0x02, 0x05, 0x00): {0x42, 0x05, 0x5A},
		req(0x02, 0x06, 0x00): {0x42, 0x06, 0x80},
		req(0x02, 0x0C, 0x00): {0x42, 0x0C, 0x0F, 0xA0},
	})
	ff, err := newClient(t, f).FreezeFrame(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if ff == nil {
		t.Fatal("FreezeFrame = nil, want a frame")
	}
	if ff.DTC != (dtc.DTC{ECU: symbol.ECU_T7, Code: "P0300"}) {
		t.Errorf("DTC = %+v", ff.DTC)
	}
	type pv struct {
		pid byte
		val float64
	}
	var got []pv
	for _, v := range ff.Values {
		got = append(got, pv{v.PID, v.Value})
	}
	if len(got) != 5 {
		t.Fatalf("values = %+v, want PIDs 03 04 05 06 0C", got)
	}
	if got[0].pid != 0x03 || !math.IsNaN(got[0].val) {
		t.Errorf("first value = %+v, want PID 03 (non-numeric)", got[0])
	}
	want := []pv{{0x04, 100}, {0x05, 50}, {0x06, 0}, {0x0C, 1000}}
	if !reflect.DeepEqual(got[1:], want) {
		t.Errorf("values = %+v, want %+v", got[1:], want)
	}
	if s := ff.Values[0].String(); s != "Fuel system status: closed loop" {
		t.Errorf("PID 03 string = %q", s)
	}
}

// FreezeFrame returns nil without error when the stored DTC is 0000 (no
// frame stored), and fails on short or rejected replies.
func TestFreezeFrameEdges(t *testing.T) {
	f := newFake(map[string][]byte{req(0x02, 0x02, 0x01): {0x42, 0x02, 0x00, 0x00}})
	ff, err := newClient(t, f).FreezeFrame(context.Background(), 1)
	if ff != nil || err != nil {
		t.Errorf("empty frame: got %+v, %v; want nil, nil", ff, err)
	}
	wantWire(t, f, "240: 40 A1 03 02 02 01")

	dtcAns := []byte{0x42, 0x02, 0x01, 0x05}
	cases := []struct {
		name    string
		answers map[string][]byte
		wantErr string
	}{
		{"short DTC", map[string][]byte{req(0x02, 0x02, 0x00): {0x42, 0x02, 0x01}}, "short DTC response"},
		{"DTC rejected", map[string][]byte{}, "mode $02"},
		{"mask rejected", map[string][]byte{req(0x02, 0x02, 0x00): dtcAns}, "mode $02"},
		{"short mask", map[string][]byte{req(0x02, 0x02, 0x00): dtcAns, req(0x02, 0x00, 0x00): {0x42, 0x00, 0x60}}, "short supported-PID response"},
		{"value rejected", map[string][]byte{req(0x02, 0x02, 0x00): dtcAns, req(0x02, 0x00, 0x00): {0x42, 0x00, 0x60, 0x00, 0x00, 0x00}}, "mode $02"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ff, err := newClient(t, newFake(tc.answers)).FreezeFrame(context.Background(), 0)
			if ff != nil {
				t.Errorf("frame = %+v, want nil", ff)
			}
			wantErrContains(t, err, tc.wantErr)
		})
	}
}

// StoredDTCs (mode $03) and PendingDTCs (mode $07) decode 2-byte codes,
// skipping 0000 padding and a dangling odd byte, tagged as T7 DTCs.
func TestReadDTCs(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x03): {0x43, 0x01, 0x05, 0x00, 0x00, 0xC1, 0x55, 0x07},
		req(0x07): {0x47, 0x03, 0x00},
	})
	c := newClient(t, f)
	stored, err := c.StoredDTCs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []dtc.DTC{{ECU: symbol.ECU_T7, Code: "P0105"}, {ECU: symbol.ECU_T7, Code: "U0155"}}
	if !reflect.DeepEqual(stored, want) {
		t.Errorf("StoredDTCs = %+v, want %+v", stored, want)
	}
	pending, err := c.PendingDTCs(context.Background())
	if err != nil || !reflect.DeepEqual(pending, []dtc.DTC{{ECU: symbol.ECU_T7, Code: "P0300"}}) {
		t.Errorf("PendingDTCs = %+v, %v", pending, err)
	}
	wantWire(t, f, "240: 40 A1 01 03", "266: 40 A1 3F 81", "240: 40 A1 01 07")

	none, err := newClient(t, newFake(map[string][]byte{req(0x03): {0x43}})).StoredDTCs(context.Background())
	if err != nil || len(none) != 0 {
		t.Errorf("no DTCs: got %+v, %v", none, err)
	}
	if _, err := newClient(t, newFake(nil)).PendingDTCs(context.Background()); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected request (serviceNotSupported)", err)
	}
}

// ClearDTCs sends mode $04 and reports a negative response as its KWP error.
func TestClearDTCs(t *testing.T) {
	f := newFake(map[string][]byte{req(0x04): {0x44}})
	if err := newClient(t, f).ClearDTCs(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantWire(t, f, "240: 40 A1 01 04")

	f = newFake(map[string][]byte{req(0x04): {0x7F, 0x04, t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR}})
	if err := newClient(t, f).ClearDTCs(context.Background()); !errors.Is(err, t7kwp.ErrConditionsNotCorrectOrRequestSequenceError) {
		t.Errorf("err = %v", err)
	}
}

// O2SensorTest returns value/min/max of a mode $05 test and rejects a reply
// without all three.
func TestO2SensorTest(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x05, 0x01, 0x01): {0x45, 0x5A, 0x50, 0x64},
		req(0x05, 0x01, 0x02): {0x45},
	})
	c := newClient(t, f)
	r, err := c.O2SensorTest(context.Background(), 0x01, 0x01)
	if err != nil {
		t.Fatal(err)
	}
	if r != (O2TestResult{TestID: 1, Sensor: 1, Value: 0x5A, Min: 0x50, Max: 0x64}) {
		t.Errorf("got %+v", r)
	}
	if v := Volts(r.Value); math.Abs(v-0.45) > 1e-9 {
		t.Errorf("Volts(0x5A) = %v, want 0.45", v)
	}
	_, err = c.O2SensorTest(context.Background(), 0x01, 0x02)
	wantErrContains(t, err, "test $01 sensor 2 not supported")
	if _, err := c.O2SensorTest(context.Background(), 0x09, 0x01); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected test (serviceNotSupported)", err)
	}
}

// O2SupportedTests walks the mode $05 supported-test pages for a sensor;
// the T7 front sensor advertises tests 1 and 2 plus the next page.
func TestO2SupportedTests(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x05, 0x00, 0x01): {0x45, 0xC0, 0x00, 0x00, 0x01},
		req(0x05, 0x20, 0x01): {0x45},
		req(0x05, 0x00, 0x02): {0x45, 0x00, 0x00, 0x00, 0x01},
	})
	c := newClient(t, f)
	got, err := c.O2SupportedTests(context.Background(), 1)
	if err != nil || !reflect.DeepEqual(got, []byte{0x01, 0x02, 0x20}) {
		t.Errorf("sensor 1 = % X, %v; want 01 02 20", got, err)
	}
	got, err = c.O2SupportedTests(context.Background(), 2) // page $20 rejected: walk stops
	if err != nil || !reflect.DeepEqual(got, []byte{0x20}) {
		t.Errorf("sensor 2 = % X, %v; want 20", got, err)
	}
	if _, err := c.O2SupportedTests(context.Background(), 3); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error when page $00 is rejected (serviceNotSupported)", err)
	}
}

// O2SupportedTests stops after page $E0.
func TestO2SupportedTestsStopsAtE0(t *testing.T) {
	ans := map[string][]byte{}
	for p := 0x00; p <= 0xE0; p += 0x20 {
		ans[req(0x05, byte(p), 0x01)] = []byte{0x45, 0x00, 0x00, 0x00, 0x01}
	}
	f := newFake(ans)
	if _, err := newClient(t, f).O2SupportedTests(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if n := len(f.wireLog()); n != 8 {
		t.Errorf("%d requests, want 8 (pages $00-$E0)", n)
	}
}

// TestResults splits a mode $06 reply into 6-byte result blocks, dropping a
// trailing partial block, and errors on an empty reply.
func TestTestResults(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x06, 0x19): {0x46, 0x19, 0x00, 0x01, 0xF4, 0x03, 0xE8, 0x19, 0x01, 0x00, 0x0A, 0x00, 0x05, 0xFF},
		req(0x06, 0x13): {0x46},
	})
	c := newClient(t, f)
	got, err := c.TestResults(context.Background(), 0x19)
	if err != nil {
		t.Fatal(err)
	}
	want := []TestResult{{0x19, 0, 500, 1000}, {0x19, 1, 10, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	_, err = c.TestResults(context.Background(), 0x13)
	wantErrContains(t, err, "test $13 not supported")
	if _, err := c.TestResults(context.Background(), 0x01); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected test (serviceNotSupported)", err)
	}
}

// SupportedTests expands the 5-byte mask after the echoed TID $00 as-is;
// for the T7 OBD2 firmware that yields the misaligned IDs documented on
// T7TestIDs.
func TestSupportedTests(t *testing.T) {
	f := newFake(map[string][]byte{req(0x06, 0x00): {0x46, 0x00, 0xFF, 0x00, 0x00, 0x38, 0x8C}})
	got, err := newClient(t, f).SupportedTests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x1B, 0x1C, 0x1D, 0x21, 0x25, 0x26}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SupportedTests = % X, want % X", got, want)
	}

	for _, bad := range [][]byte{{0x46, 0x00}, {0x46, 0x01, 0xFF}} {
		f := newFake(map[string][]byte{req(0x06, 0x00): bad})
		_, err := newClient(t, f).SupportedTests(context.Background())
		wantErrContains(t, err, "unexpected supported-TID response")
	}
	if _, err := newClient(t, newFake(nil)).SupportedTests(context.Background()); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected request (serviceNotSupported)", err)
	}
}

// ControlTest returns the mode $08 reply bytes and errors when the ECU
// answers with none.
func TestControlTest(t *testing.T) {
	f := newFake(map[string][]byte{
		req(0x08, 0x01): {0x48, 0x00, 0x00, 0x00, 0x00, 0x00},
		req(0x08, 0x02): {0x48},
	})
	c := newClient(t, f)
	got, err := c.ControlTest(context.Background(), 0x01)
	if err != nil || !reflect.DeepEqual(got, []byte{0, 0, 0, 0, 0}) {
		t.Errorf("ControlTest(1) = % X, %v", got, err)
	}
	_, err = c.ControlTest(context.Background(), 0x02)
	wantErrContains(t, err, "test $02 not supported")
	if _, err := c.ControlTest(context.Background(), 0x03); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("err = %v, want error for a rejected test (serviceNotSupported)", err)
	}
}

// vinPayload builds the T7 mode $09 infotype $02 reply (Scantool.cpp mode9):
// [count, 00 00 00 v0] then four [02, msgNo, 4 chars] groups.
func vinPayload(vin string) []byte {
	p := []byte{0x49, 0x01, 0x00, 0x00, 0x00, vin[0]}
	for g := range 4 {
		p = append(p, 0x02, byte(g+2))
		p = append(p, vin[1+g*4:5+g*4]...)
	}
	return p
}

// VIN reassembles the six-chunk infotype $02 reply, acking each chunk on
// 0x266, and strips the pre-CAN message structure and leading padding.
func TestVIN(t *testing.T) {
	const vin = "YS3DF78K537000001"
	f := newFake(map[string][]byte{req(0x09, 0x02): vinPayload(vin)})
	got, err := newClient(t, f).VIN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != vin {
		t.Errorf("VIN = %q, want %q", got, vin)
	}
	wantWire(t, f,
		"240: 40 A1 02 09 02",
		"266: 40 A1 3F 85",
		"266: 40 A1 3F 84",
		"266: 40 A1 3F 83",
		"266: 40 A1 3F 82",
		"266: 40 A1 3F 81",
	)
}

// CalibrationID unwraps the infotype $04 reply and trims trailing NULs and
// spaces; CVN returns the 4 checksum bytes of infotype $06.
func TestCalibrationIDAndCVN(t *testing.T) {
	cal := "4980123 \x00\x00\x00\x00\x00\x00\x00\x00"
	p := []byte{0x49, 0x01}
	p = append(p, cal[0:4]...)
	for g := range 3 {
		p = append(p, 0x04, byte(g+2))
		p = append(p, cal[4+g*4:8+g*4]...)
	}
	f := newFake(map[string][]byte{
		req(0x09, 0x04): p,
		req(0x09, 0x06): {0x49, 0x01, 0xDE, 0xAD, 0xBE, 0xEF},
	})
	c := newClient(t, f)
	id, err := c.CalibrationID(context.Background())
	if err != nil || id != "4980123" {
		t.Errorf("CalibrationID = %q, %v; want 4980123", id, err)
	}
	cvn, err := c.CVN(context.Background())
	if err != nil || !reflect.DeepEqual(cvn, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("CVN = % X, %v", cvn, err)
	}
}

// The mode $09 helpers propagate rejected requests, VehicleInfo rejects an
// empty reply, and CVN rejects a reply too short to hold 4 bytes.
func TestVehicleInfoErrors(t *testing.T) {
	c := newClient(t, newFake(map[string][]byte{
		req(0x09, 0x07): {0x49},
		req(0x09, 0x06): {0x49, 0x01, 0xDE, 0xAD},
	}))
	ctx := context.Background()
	_, err := c.VehicleInfo(ctx, 0x07)
	wantErrContains(t, err, "infotype $07 not supported")
	_, err = c.CVN(ctx)
	wantErrContains(t, err, "short CVN")

	c = newClient(t, newFake(nil))
	if _, err := c.VIN(ctx); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("VIN: err = %v", err)
	}
	if _, err := c.CalibrationID(ctx); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("CalibrationID: err = %v", err)
	}
	if _, err := c.CVN(ctx); !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Errorf("CVN: err = %v", err)
	}
}
