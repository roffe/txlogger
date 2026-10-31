package ecumaster

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
)

// frame664 is a status frame: 12.80 V supply, heater 0x80/255, 800 °C,
// λ 0.987, calibration finished on an LSU 4.9, no faults.
var frame664 = []byte{0x05, 0x00, 0x80, 0xC8, 0x03, 0xDB, 0x28, 0x00}

// frame665 is a sensor frame: -1.000 mA pump current, -2.00 % O2, 50.0 Ω,
// heater regulating, lambda valid.
var frame665 = []byte{0xFC, 0x18, 0xFF, 0x38, 0x01, 0xF4, 0x07, 0x01}

// decodeCAN maps frame 0x664 and 0x665 onto the status, scaled to volts,
// percent, °C, λ, mA and Ω.
func TestDecodeCANFrames(t *testing.T) {
	l := NewLambdaToCAN(nil)
	if err := l.decodeCAN(0x664, frame664); err != nil {
		t.Fatal(err)
	}
	if err := l.decodeCAN(0x665, frame665); err != nil {
		t.Fatal(err)
	}
	// 0x80 of 255 heater duty is 50.196 %, checked with a tolerance.
	const heaterPower = 50.19607843
	want := LambdaToCANStatus{
		SupplyVoltage:    12.80,
		SensorTemp:       800,
		Lambda:           0.987,
		CalibrationState: CalibrationFinished,
		DeviceVersion:    LSU_49,
		IpCurrent:        -1,
		OxygenConc:       -2,
		Resistance:       50,
		HeaterState:      HeaterRegulation,
		LambdaValid:      true,
	}
	got := l.Status()
	if math.Abs(got.HeaterPower-heaterPower) > 1e-8 {
		t.Errorf("HeaterPower = %v, want %v", got.HeaterPower, heaterPower)
	}
	got.HeaterPower = 0
	if got != want {
		t.Errorf("Status() =\n%+v\nwant\n%+v", got, want)
	}
	if got := l.GetLambda(); got != 0.987 {
		t.Errorf("GetLambda() = %v, want 0.987", got)
	}
	if got := l.GetSupplyVoltage(); got != 12.80 {
		t.Errorf("GetSupplyVoltage() = %v, want 12.80", got)
	}
	if got := l.GetHeaterPower(); math.Abs(got-heaterPower) > 1e-8 {
		t.Errorf("GetHeaterPower() = %v, want %v", got, heaterPower)
	}
	if got := l.GetSensorTemp(); got != 800 {
		t.Errorf("GetSensorTemp() = %v, want 800", got)
	}
}

// faultFlags lists the 0x664 fault flags in the bit order they are packed:
// byte 7 bits 0-7, then byte 6 bits 0-1.
func faultFlags(st LambdaToCANStatus) []bool {
	return []bool{
		st.VmShortVcc, st.VmShortGnd, st.UnShortVcc, st.UnShortGnd,
		st.IaipShortVcc, st.IaipShortGnd, st.VubLowVoltage, st.HeaterShortVcc,
		st.HeaterShortGnd, st.HeaterOpenLoad,
	}
}

// Each fault bit of 0x664 (byte 7 bits 0-7, byte 6 bits 0-1) raises exactly
// its own flag.
func TestDecodeCAN664FaultBits(t *testing.T) {
	for i := range 10 {
		data := make([]byte, 8)
		if i < 8 {
			data[7] = 1 << i
		} else {
			data[6] = 1 << (i - 8)
		}
		l := NewLambdaToCAN(nil)
		if err := l.decodeCAN664Data(data); err != nil {
			t.Fatal(err)
		}
		for j, set := range faultFlags(l.Status()) {
			if set != (i == j) {
				t.Errorf("fault bit %d: flag %d = %v, want %v", i, j, set, i == j)
			}
		}
	}
}

// Byte 6 of 0x664 carries the calibration state in bits 2-4 and the sensor
// type in bits 5-7; values without a name decode as Unknown.
func TestDecodeCAN664StateAndVersion(t *testing.T) {
	cal := []string{CalibrationStart, CalibrationWaitSPIReset, CalibrationFinished, CalibrationError, Unknown, Unknown, Unknown, Unknown}
	ver := []string{LSU_42, LSU_49, LSU_ADV, Unknown, Unknown, Unknown, Unknown, Unknown}
	for c := range 8 {
		for v := range 8 {
			// The fault bits are set too, they must not leak into either field.
			data := []byte{0, 0, 0, 0, 0, 0, byte(v<<5 | c<<2 | 0x03), 0}
			l := NewLambdaToCAN(nil)
			if err := l.decodeCAN664Data(data); err != nil {
				t.Fatal(err)
			}
			st := l.Status()
			if st.CalibrationState != cal[c] || st.DeviceVersion != ver[v] {
				t.Errorf("byte6 %#02x: calibration %q version %q, want %q %q", data[6], st.CalibrationState, st.DeviceVersion, cal[c], ver[v])
			}
		}
	}
}

// Byte 6 of 0x665 is the heater state and only bit 0 of byte 7 marks the
// lambda value valid.
func TestDecodeCAN665HeaterAndValid(t *testing.T) {
	for _, c := range []struct {
		b6, b7 byte
		heater string
		valid  bool
	}{
		{2, 0x01, HeaterLowPower, true},
		{4, 0xFF, HeaterRampUp, true},
		{7, 0xFE, HeaterRegulation, false},
		{0, 0x00, Unknown, false},
		{3, 0x00, Unknown, false},
		{0xFF, 0x00, Unknown, false},
	} {
		l := NewLambdaToCAN(nil)
		if err := l.decodeCAN665Data([]byte{0, 0, 0, 0, 0, 0, c.b6, c.b7}); err != nil {
			t.Fatal(err)
		}
		st := l.Status()
		if st.HeaterState != c.heater || st.LambdaValid != c.valid {
			t.Errorf("b6 %#02x b7 %#02x: heater %q valid %v, want %q %v", c.b6, c.b7, st.HeaterState, st.LambdaValid, c.heater, c.valid)
		}
	}
}

// Frames shorter than 8 bytes (ErrDataTooShort) and unknown ids are
// rejected without touching the status.
func TestDecodeCANRejects(t *testing.T) {
	l := NewLambdaToCAN(nil)
	if err := l.decodeCAN(0x664, frame664); err != nil {
		t.Fatal(err)
	}
	before := l.Status()
	for _, c := range []struct {
		id   uint32
		data []byte
		want string
	}{
		{0x664, frame664[:7], ErrDataTooShort.Error()},
		{0x665, nil, ErrDataTooShort.Error()},
		{0x666, frame665, "unknown identifier 666"},
	} {
		err := l.decodeCAN(c.id, c.data)
		if err == nil || err.Error() != c.want {
			t.Errorf("decodeCAN(%#x, % X) = %v, want %q", c.id, c.data, err, c.want)
		}
		if c.want == ErrDataTooShort.Error() && !errors.Is(err, ErrDataTooShort) {
			t.Errorf("decodeCAN(%#x) = %v, want ErrDataTooShort itself", c.id, err)
		}
	}
	if got := l.Status(); got != before {
		t.Errorf("status changed by rejected frames:\n%+v\nwant\n%+v", got, before)
	}
}

// String lists every status field, one per line.
func TestString(t *testing.T) {
	l := NewLambdaToCAN(nil)
	l.decodeCAN(0x664, frame664)
	l.decodeCAN(0x665, frame665)
	want := "Supply Voltage:   12.80 V\n" +
		"Heater Power:     50.20 %DC\n" +
		"Sensor Temperature: 800 °C\n" +
		"Lambda:           0.99\n" +
		"VM Short VCC:     false\n" +
		"VM Short GND:     false\n" +
		"UN Short VCC:     false\n" +
		"UN Short GND:     false\n" +
		"IAIP Short VCC:   false\n" +
		"IAIP Short GND:   false\n" +
		"VUB Low Voltage:  false\n" +
		"Heater Short VCC: false\n" +
		"Heater Short GND: false\n" +
		"Heater Open Load: false\n" +
		"Calibration State: Finished\n" +
		"Device Version:    LSU 4.9\n" +
		"Ip Current:   -1.00 mA\n" +
		"Oxygen Conc:  -2.00 %\n" +
		"Resistance:   50.00 Ohm\n" +
		"Heater State: Regulation\n" +
		"Lambda Valid: true\n"
	if got := l.String(); got != want {
		t.Errorf("String() =\n%s\nwant\n%s", got, want)
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog sends the standard logger, which the reader goroutine reports
// to, into a buffer for the rest of the test.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	b := &syncBuffer{}
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(b)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})
	return b
}

// eventually polls cond until it holds, failing the test after 5 seconds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !cond() {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// readerStacks returns the stack of every running reader goroutine started
// by Start.
func readerStacks() []string {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	var out []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "ecumaster.(*LambdaToCAN).Start.func1(") {
			out = append(out, g)
		}
	}
	return out
}

// readers counts running reader goroutines.
func readers() int { return len(readerStacks()) }

// readerIdle reports whether a reader is parked waiting for a frame.
func readerIdle() bool {
	for _, g := range readerStacks() {
		if strings.Contains(g, " [select") {
			return true
		}
	}
	return false
}

// openBus opens a loopback bus, which delivers sent frames to subscribers.
func openBus(t *testing.T, ctx context.Context) *gocan.Bus {
	t.Helper()
	cl, err := gocan.OpenAdapter(ctx, &gocan.Loopback{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

// Start subscribes to 0x664/0x665 and decodes each frame into the status;
// a bad frame is logged and the reader keeps going.
func TestStartDecodesBusFrames(t *testing.T) {
	logs := captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, ctx)
	l := NewLambdaToCAN(cl)
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for _, f := range []gocan.Frame{
		gocan.NewFrame(0x664, frame664),
		gocan.NewFrame(0x664, frame664[:4]),          // too short, logged
		gocan.NewFrame(0x123, []byte{1, 2, 3, 4, 5}), // not subscribed
		gocan.NewFrame(0x665, frame665),
	} {
		if err := cl.Send(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	// Frames arrive in order, so once 0x665 is applied the rest were handled.
	eventually(t, "0x665 decoded", func() bool { return l.Status().LambdaValid })
	if got := l.GetLambda(); got != 0.987 {
		t.Errorf("lambda = %v, want 0.987", got)
	}
	if got := l.Status().DeviceVersion; got != LSU_49 {
		t.Errorf("device version = %q, want %q", got, LSU_49)
	}
	if !strings.Contains(logs.String(), ErrDataTooShort.Error()) {
		t.Errorf("log %q, want the short frame error", logs.String())
	}
}

// The reader goroutine ends when the Start context is cancelled.
func TestReaderEndsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, context.Background())
	if err := NewLambdaToCAN(cl).Start(ctx); err != nil {
		t.Fatal(err)
	}
	if readers() == 0 {
		t.Fatal("no reader goroutine after Start")
	}
	cancel()
	eventually(t, "reader to exit", func() bool { return readers() == 0 })
}

// The reader logs and ends when the bus closes under it.
func TestReaderEndsOnBusClose(t *testing.T) {
	logs := captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, context.Background())
	if err := NewLambdaToCAN(cl).Start(ctx); err != nil {
		t.Fatal(err)
	}
	cl.Close()
	eventually(t, "reader to exit", func() bool { return readers() == 0 })
	if !strings.Contains(logs.String(), "channel closed") {
		t.Errorf("log %q, want channel closed", logs.String())
	}
}

// The loggers stop their wideband while its reader goroutine runs; Stop
// must not race with it. Frames keep arriving during Stop so the reader
// re-checks its state with no other synchronization. Run with -race to see
// the failure.
func TestStopIsRaceFree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, ctx)
	l := NewLambdaToCAN(cl)
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stop, sent := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sent)
		f := gocan.NewFrame(0x664, frame664)
		for {
			select {
			case <-stop:
				return
			default:
				cl.Send(ctx, f)
			}
		}
	}()
	defer func() { close(stop); <-sent }()
	eventually(t, "0x664 decoded", func() bool { return l.GetLambda() == 0.987 })
	l.Stop()
	eventually(t, "reader to exit after Stop", func() bool { return readers() == 0 })
}

// Stop ends a reader that is waiting for its next frame, while the Start
// context is still alive and no further frame arrives.
func TestStopEndsReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, ctx)
	l := NewLambdaToCAN(cl)
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cl.Send(ctx, gocan.NewFrame(0x664, frame664)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "0x664 decoded", func() bool { return l.GetLambda() == 0.987 })
	eventually(t, "reader to wait for the next frame", readerIdle)
	l.Stop()
	eventually(t, "reader to exit after Stop", func() bool { return readers() == 0 })
}
