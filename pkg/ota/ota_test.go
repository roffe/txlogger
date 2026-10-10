package ota

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
)

// frame encodes a txbridge serial command: command, length, data, checksum.
func frame(cmd byte, data []byte) []byte {
	b, err := serialcommand.NewSerialCommand(cmd, data).MarshalBinary()
	if err != nil {
		panic(err)
	}
	return b
}

// hangup as a device reply closes the connection instead of answering.
var hangup = []byte{}

// fakeBridge is a txbridge on a loopback TCP port that speaks the OTA
// protocol. A nil reply field means the normal answer; hangup closes the
// connection.
type fakeBridge struct {
	version    string
	verReply   []byte
	silent     chan struct{} // when set, hold the connection open unanswered until closed
	uReply     []byte
	chunkReply func(n int) []byte // reply to the n'th firmware chunk, from 0
	finReply   []byte

	// Recorded by the device goroutine; read only after wait.
	verReq  []byte
	gotU    bool
	gotF    bool
	chunks  [][]byte
	image   bytes.Buffer
	readErr error

	done chan struct{}
}

func (d *fakeBridge) start(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d.done = make(chan struct{})
	go func() {
		defer close(d.done)
		conn, err := ln.Accept()
		ln.Close()
		if err != nil {
			return
		}
		defer conn.Close()
		// Unblock both ends instead of hanging the test if the protocol stalls.
		conn.SetDeadline(time.Now().Add(30 * time.Second))
		d.serve(conn)
	}()
	t.Cleanup(func() {
		ln.Close()
		d.wait(t)
	})
	return "tcp://" + ln.Addr().String()
}

func (d *fakeBridge) wait(t *testing.T) {
	t.Helper()
	select {
	case <-d.done:
	case <-time.After(10 * time.Second):
		t.Fatal("fake txbridge did not finish")
	}
}

// reply sends r, or the default def when r is nil; it reports false when the
// device should stop.
func reply(conn io.Writer, r, def []byte) bool {
	if r == nil {
		r = def
	}
	if len(r) == 0 {
		return false
	}
	_, err := conn.Write(r)
	return err == nil
}

// readFrame reads the rest of a frame whose command byte was already read and
// returns the whole frame.
func readFrame(r *bufio.Reader, cmd byte) ([]byte, error) {
	size, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	data := make([]byte, int(size)+1)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	sc := serialcommand.NewSerialCommand(cmd, data[:size])
	if sc.Checksum() != data[size] {
		return nil, fmt.Errorf("bad checksum on %c frame", cmd)
	}
	return append([]byte{cmd, size}, data...), nil
}

func (d *fakeBridge) serve(conn io.ReadWriter) {
	r := bufio.NewReader(conn)
	fail := func(err error) {
		if err != io.EOF {
			d.readErr = err
		}
	}

	// Version request: a 'v' frame.
	c, err := r.ReadByte()
	if err != nil {
		fail(err)
		return
	}
	if c != 'v' {
		fail(fmt.Errorf("first byte %q, want the 'v' version request", c))
		return
	}
	if d.verReq, err = readFrame(r, 'v'); err != nil {
		fail(err)
		return
	}
	if d.silent != nil {
		<-d.silent
		return
	}
	if !reply(conn, d.verReply, frame('v', []byte(d.version))) {
		return
	}

	// Start of upload: a bare 'u'.
	if c, err = r.ReadByte(); err != nil {
		fail(err)
		return
	}
	if c != 'u' {
		fail(fmt.Errorf("byte %q where the upload start 'u' belongs", c))
		return
	}
	d.gotU = true
	if !reply(conn, d.uReply, frame('u', nil)) {
		return
	}

	// Firmware chunks as 'U' frames, ended by a bare 'F'.
	for {
		c, err := r.ReadByte()
		if err != nil {
			fail(err)
			return
		}
		if c == 'F' {
			d.gotF = true
			reply(conn, d.finReply, frame('F', []byte("ok")))
			return
		}
		if c != 'U' {
			fail(fmt.Errorf("unexpected byte %q during upload", c))
			return
		}
		f, err := readFrame(r, 'U')
		if err != nil {
			fail(err)
			return
		}
		payload := f[2 : len(f)-1]
		d.chunks = append(d.chunks, payload)
		d.image.Write(payload)
		var rep []byte
		if d.chunkReply != nil {
			rep = d.chunkReply(len(d.chunks) - 1)
		}
		if !reply(conn, rep, frame('U', nil)) {
			return
		}
	}
}

type recorder struct {
	mu       sync.Mutex
	logs     []string
	progress []float64
}

func (r *recorder) cfg(port string) Config {
	return Config{
		Port: port,
		Logfunc: func(v ...any) {
			r.mu.Lock()
			r.logs = append(r.logs, fmt.Sprint(v...))
			r.mu.Unlock()
		},
		ProgressFunc: func(p float64) {
			r.mu.Lock()
			r.progress = append(r.progress, p)
			r.mu.Unlock()
		},
	}
}

// UpdateOTA asks the bridge for its version, then streams the embedded
// firmware in 250 byte 'U' frames, each acknowledged, and finishes with 'F'
// answered by "ok". Progress runs from 0 to 100.
func TestUpdateOTAUploadsFirmware(t *testing.T) {
	dev := &fakeBridge{version: "1.1.3"}
	port := dev.start(t)
	var rec recorder
	if err := UpdateOTA(rec.cfg(port)); err != nil {
		t.Fatalf("UpdateOTA: %v", err)
	}
	dev.wait(t)
	if dev.readErr != nil {
		t.Fatalf("device: %v", dev.readErr)
	}

	if !bytes.Equal(dev.verReq, []byte{'v', 0x01, 0x10, 0x10}) {
		t.Errorf("version request = % X, want 76 01 10 10", dev.verReq)
	}
	if !dev.gotU || !dev.gotF {
		t.Errorf("gotU %v gotF %v, want both", dev.gotU, dev.gotF)
	}
	if !bytes.Equal(dev.image.Bytes(), firmware) {
		t.Fatalf("device received %d bytes, not the %d byte embedded firmware", dev.image.Len(), len(firmware))
	}
	wantChunks := (len(firmware) + 249) / 250
	if len(dev.chunks) != wantChunks {
		t.Errorf("%d chunks, want %d", len(dev.chunks), wantChunks)
	}
	for i, c := range dev.chunks[:len(dev.chunks)-1] {
		if len(c) != 250 {
			t.Fatalf("chunk %d is %d bytes, want 250", i, len(c))
		}
	}

	if len(rec.progress) != wantChunks+1 || rec.progress[0] != 0 || rec.progress[len(rec.progress)-1] != 100 {
		t.Fatalf("progress: %d reports, first %v last %v", len(rec.progress), rec.progress[0], rec.progress[len(rec.progress)-1])
	}
	for i := 1; i < len(rec.progress); i++ {
		if rec.progress[i] <= rec.progress[i-1] {
			t.Fatalf("progress went from %v to %v", rec.progress[i-1], rec.progress[i])
		}
	}
	logs := strings.Join(rec.logs, "\n")
	for _, want := range []string{"Opening " + port + " port", "Device firmware: 1.1.3", "OTA finished in "} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %q:\n%s", want, logs)
		}
	}
}

// UpdateOTA stops with an error at the first unexpected or error reply, and
// sends nothing further once the bridge has refused: no 'u' after a bad
// version reply, no chunk after a refused one and never the final 'F'.
func TestUpdateOTADeviceErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dev        fakeBridge
		want       string
		wantU      bool
		wantChunks int
	}{
		{
			name: "version error",
			dev:  fakeBridge{verReply: frame('e', []byte{0x01})},
			want: "OTA update failed: error: 65 01",
		},
		{
			name: "version wrong command",
			dev:  fakeBridge{verReply: frame('x', []byte("1.0"))},
			want: "unexpected response: 78 312E30",
		},
		{
			name: "no version reply",
			dev:  fakeBridge{verReply: hangup},
			want: "read error: EOF",
		},
		{
			name:  "not ready",
			dev:   fakeBridge{version: "1.0.0", uReply: frame('e', []byte{0x07})},
			want:  "device is not ready for OTA: 07",
			wantU: true,
		},
		{
			name:  "upload start wrong command",
			dev:   fakeBridge{version: "1.0.0", uReply: frame('q', nil)},
			want:  "unexpected response: 71",
			wantU: true,
		},
		{
			name:       "chunk refused",
			dev:        fakeBridge{version: "1.0.0", chunkReply: func(n int) []byte { return frame('e', []byte{byte(n)}) }},
			want:       "error: 65 00",
			wantU:      true,
			wantChunks: 1,
		},
		{
			name: "chunk wrong command",
			dev: fakeBridge{version: "1.0.0", chunkReply: func(n int) []byte {
				if n == 2 {
					return frame('V', nil)
				}
				return nil
			}},
			want:       "unexpected response OTA update: 56",
			wantU:      true,
			wantChunks: 3,
		},
		{
			name:       "chunk not acknowledged",
			dev:        fakeBridge{version: "1.0.0", chunkReply: func(int) []byte { return hangup }},
			want:       "finish firmware upload failed: read error: EOF",
			wantU:      true,
			wantChunks: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := tc.dev
			port := dev.start(t)
			var rec recorder
			err := UpdateOTA(rec.cfg(port))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			dev.wait(t)
			if dev.gotU != tc.wantU {
				t.Errorf("upload started = %v, want %v", dev.gotU, tc.wantU)
			}
			if len(dev.chunks) != tc.wantChunks {
				t.Errorf("sent %d chunks, want %d (none after the refusal)", len(dev.chunks), tc.wantChunks)
			}
			if dev.gotF {
				t.Error("finish sent after a failure")
			}
		})
	}
}

// The final 'F' must be answered with exactly "ok"; an error or any other
// answer fails the update after the whole image was sent.
func TestUpdateOTAFinishReply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply []byte
		want  string
	}{
		{"error", frame('e', []byte{0x02}), "OTA update failed: error: 65 02"},
		{"not ok", frame('F', []byte("no")), "unexpected response end OTA: 46 6E6F"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := &fakeBridge{version: "1.0.0", finReply: tc.reply}
			var rec recorder
			err := UpdateOTA(rec.cfg(dev.start(t)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			dev.wait(t)
			if !dev.gotF || dev.image.Len() != len(firmware) {
				t.Errorf("gotF %v after %d bytes, want the full image first", dev.gotF, dev.image.Len())
			}
		})
	}
}

// UpdateOTA refuses an empty port and reports a port that cannot be opened.
func TestUpdateOTAPortErrors(t *testing.T) {
	if err := UpdateOTA(Config{}); err == nil || err.Error() != "port is empty" {
		t.Errorf("empty port err = %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	var rec recorder
	if err := UpdateOTA(rec.cfg("tcp://" + addr)); err == nil {
		t.Error("UpdateOTA to a closed TCP port succeeded")
	}

	missing := filepath.Join(t.TempDir(), "ttyUSB9")
	if err := UpdateOTA(rec.cfg(missing)); err == nil {
		t.Errorf("UpdateOTA on missing serial port %s succeeded", missing)
	}
}

// UpdateOTA gives up with a timeout error when a tcp:// bridge stops
// answering, as it does on a serial port, instead of waiting forever.
func TestUpdateOTATCPReplyTimeout(t *testing.T) {
	dev := &fakeBridge{silent: make(chan struct{})}
	port := dev.start(t)
	var rec recorder
	result := make(chan error, 1)
	go func() { result <- UpdateOTA(rec.cfg(port)) }()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Errorf("err = %v, want a timeout", err)
		}
		close(dev.silent)
	case <-time.After(15 * time.Second):
		close(dev.silent) // hang up so UpdateOTA returns
		<-result
		t.Fatal("UpdateOTA still waiting 15s after a 5s reply timeout")
	}
}

// queueApp routes fyne.Do to a channel so a test can run UI work, like the
// confirm dialog UpdateOTA shows, on its own goroutine.
type queueApp struct {
	fyne.App
	drv *queueDriver
}

func (a *queueApp) Driver() fyne.Driver { return a.drv }

type queueDriver struct {
	fyne.Driver
	do chan func()
}

func (d *queueDriver) DoFromGoroutine(fn func(), _ bool) { d.do <- fn }

func findButton(t *testing.T, o fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	for _, obj := range test.LaidOutObjects(o) {
		if b, ok := obj.(*widget.Button); ok && b.Text == text {
			return b
		}
	}
	t.Fatalf("no %q button in the dialog", text)
	return nil
}

// A bridge already on the bundled firmware version gets a confirm dialog:
// "No" aborts before the upload starts and "Yes" goes ahead with it.
func TestUpdateOTASameVersionAsks(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   string
		wantU  bool
	}{
		{"No", "OTA update aborted", false},
		{"Yes", "device is not ready for OTA", true},
	} {
		t.Run(tc.answer, func(t *testing.T) {
			a := test.NewTempApp(t)
			// UpdateOTA parents the dialog on the app's first window.
			if len(a.Driver().AllWindows()) == 0 {
				a.NewWindow("txlogger")
			}
			w := a.Driver().AllWindows()[0]
			w.Resize(fyne.NewSize(600, 400))
			drv := &queueDriver{Driver: a.Driver(), do: make(chan func(), 8)}
			fyne.SetCurrentApp(&queueApp{App: a, drv: drv})

			dev := &fakeBridge{version: MinimumtxbridgeVersion, uReply: frame('e', nil)}
			port := dev.start(t)
			var rec recorder
			result := make(chan error, 1)
			go func() { result <- UpdateOTA(rec.cfg(port)) }()

			select {
			case fn := <-drv.do:
				fn()
			case err := <-result:
				t.Fatalf("UpdateOTA returned %v without asking", err)
			case <-time.After(10 * time.Second):
				t.Fatal("no confirm dialog")
			}
			test.Tap(findButton(t, w.Canvas().Overlays().Top(), lang.L(tc.answer)))

			select {
			case err := <-result:
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("err = %v, want %q", err, tc.want)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("UpdateOTA did not return after the answer")
			}
			dev.wait(t)
			if dev.gotU != tc.wantU {
				t.Errorf("upload started = %v, want %v", dev.gotU, tc.wantU)
			}
		})
	}
}

// scriptedPort returns one scripted chunk per Read, then err (or zero-byte
// reads forever when err is nil), and records writes.
type scriptedPort struct {
	reads  [][]byte
	err    error
	wrote  bytes.Buffer
	wrErr  error
	closed bool
}

func (p *scriptedPort) Read(b []byte) (int, error) {
	if len(p.reads) == 0 {
		return 0, p.err
	}
	n := copy(b, p.reads[0])
	if n < len(p.reads[0]) {
		p.reads[0] = p.reads[0][n:]
	} else {
		p.reads = p.reads[1:]
	}
	return n, nil
}

func (p *scriptedPort) Write(b []byte) (int, error) {
	if p.wrErr != nil {
		return 0, p.wrErr
	}
	return p.wrote.Write(b)
}

func (p *scriptedPort) Close() error { p.closed = true; return nil }

// readSerialCommand assembles a frame that arrives split across reads,
// accepts an empty payload, and keeps the payload past the 16 byte read size.
func TestReadSerialCommand(t *testing.T) {
	long := bytes.Repeat([]byte{0xA5}, 40)
	for _, tc := range []struct {
		name  string
		reads [][]byte
		cmd   byte
		data  []byte
	}{
		{"one read", [][]byte{frame('v', []byte("1.1.4"))}, 'v', []byte("1.1.4")},
		{"split", [][]byte{{'v'}, {0x05, '1'}, {}, {'.', '1', '.', '4', 0xF2}}, 'v', []byte("1.1.4")},
		{"empty payload", [][]byte{{'u', 0x00, 0x00}}, 'u', []byte{}},
		{"longer than a read", [][]byte{frame('U', long)}, 'U', long},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readSerialCommand(&scriptedPort{reads: tc.reads, err: io.EOF}, time.Second)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got.Command != tc.cmd || !bytes.Equal(got.Data, tc.data) {
				t.Errorf("got %c % X, want %c % X", got.Command, got.Data, tc.cmd, tc.data)
			}
		})
	}
}

// readSerialCommand rejects a frame whose checksum does not add up, wraps read
// errors, and gives up after the timeout when no complete frame arrives.
func TestReadSerialCommandErrors(t *testing.T) {
	_, err := readSerialCommand(&scriptedPort{reads: [][]byte{{'v', 0x02, 0x01, 0x02, 0x04}}}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "checksum error") {
		t.Errorf("bad checksum err = %v", err)
	}

	boom := errors.New("port unplugged")
	_, err = readSerialCommand(&scriptedPort{reads: [][]byte{{'v', 0x02}}, err: boom}, time.Second)
	if !errors.Is(err, boom) {
		t.Errorf("read error = %v, want it to wrap %v", err, boom)
	}

	start := time.Now()
	_, err = readSerialCommand(&scriptedPort{reads: [][]byte{{'v', 0x02, 0x01}}}, 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timeout after 20ms") {
		t.Errorf("timeout err = %v", err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Error("returned before the timeout")
	}
}

// writeSerialCommand puts command, length, data and checksum on the wire, and
// reports oversized payloads and write failures.
func TestWriteSerialCommand(t *testing.T) {
	p := &scriptedPort{}
	if err := writeSerialCommand(p, 'U', []byte{0x01, 0xFF, 0x10}); err != nil {
		t.Fatal(err)
	}
	if want := []byte{'U', 0x03, 0x01, 0xFF, 0x10, 0x10}; !bytes.Equal(p.wrote.Bytes(), want) {
		t.Errorf("wire = % X, want % X", p.wrote.Bytes(), want)
	}

	if err := writeSerialCommand(p, 'U', make([]byte, 256)); err == nil || !strings.Contains(err.Error(), "marshal error") {
		t.Errorf("oversized payload err = %v", err)
	}

	boom := errors.New("cable cut")
	if err := writeSerialCommand(&scriptedPort{wrErr: boom}, 'U', nil); !errors.Is(err, boom) {
		t.Errorf("write err = %v, want it to wrap %v", err, boom)
	}
}

// checkErr turns an 'e' reply into an error and passes anything else.
func TestCheckErr(t *testing.T) {
	if err := checkErr(&serialcommand.SerialCommand{Command: 'e', Data: []byte{0x0A}}); err == nil || err.Error() != "error: 65 0A" {
		t.Errorf("checkErr('e') = %v", err)
	}
	if err := checkErr(&serialcommand.SerialCommand{Command: 'v'}); err != nil {
		t.Errorf("checkErr('v') = %v", err)
	}
}
