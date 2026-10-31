package datalogger

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// ConvertByteStringToDouble reads the bytes as one unsigned big-endian number.
func TestConvertByteStringToDouble(t *testing.T) {
	tests := []struct {
		in   []byte
		want float64
	}{
		{nil, 0},
		{[]byte{0x12}, 0x12},
		{[]byte{0x01, 0x00}, 256},
		{[]byte{0x12, 0x34, 0x56}, 0x123456},
		{[]byte{0xFF, 0xFF, 0xFF, 0xFF}, 4294967295},
	}
	for _, tt := range tests {
		if got := ConvertByteStringToDouble(tt.in); got != tt.want {
			t.Errorf("% X: got %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ConvertByteToI16 reads one or two bytes as a big-endian int16, so two
// bytes with the top bit set come out negative.
func TestConvertByteToI16(t *testing.T) {
	tests := []struct {
		in   []byte
		want int16
	}{
		{nil, 0},
		{[]byte{0xFF}, 255},
		{[]byte{0x12, 0x34}, 0x1234},
		{[]byte{0x7F, 0xFF}, 32767},
		{[]byte{0x80, 0x00}, -32768},
		{[]byte{0xFF, 0xFF}, -1},
	}
	for _, tt := range tests {
		if got := ConvertByteToI16(tt.in); got != tt.want {
			t.Errorf("% X: got %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ConvertByteStringToDoubleStatus reads up to eight bytes as one unsigned
// little-endian number (the T5 status word layout).
func TestConvertByteStringToDoubleStatus(t *testing.T) {
	tests := []struct {
		in   []byte
		want float64
	}{
		{nil, 0},
		{[]byte{0x01, 0x02}, 0x0201},
		{[]byte{0x01, 0x02, 0x03}, 0x030201},
		{[]byte{0x01, 0x00, 0x00, 0x80}, 0x80000001},
		{[]byte{0, 0, 0, 0, 0, 0, 0, 0x01}, 1 << 56},
		{[]byte{0x01, 0, 0, 0, 0, 0, 0, 0, 0xFF}, 1}, // ninth byte ignored
	}
	for _, tt := range tests {
		if got := ConvertByteStringToDoubleStatus(tt.in); got != tt.want {
			t.Errorf("% X: got %v, want %v", tt.in, got, tt.want)
		}
	}
}

// newT5Converter scales each known T5 symbol to engineering units: pressures
// to bar over atmospheric, temperatures and angles to signed values, rpm by
// ten, tenths to units. Unknown symbols pass through as unsigned values.
func TestT5Converter(t *testing.T) {
	conv := newT5Converter()
	tests := []struct {
		name string
		in   []byte
		want float64
	}{
		{"P_medel", []byte{150}, 0.5},
		{"P_Manifold10", []byte{0x00, 0x64}, 0},
		{"Max_tryck", []byte{50}, -0.5},
		{"Regl_tryck", []byte{200}, 1},
		{"Lambdaint", []byte{120}, 120},
		{"Lufttemp", []byte{20}, 20},
		{"Lufttemp", []byte{128}, 128},
		{"Kyl_temp", []byte{250}, -6},
		{"Rpm", []byte{0x01, 0x2C}, 3000},
		{"AD_sond", []byte{0x80}, 128},
		{"AD_EGR", []byte{0x02, 0x00}, 512},
		{"Pgm_status", []byte{0x00, 0x01, 0x00, 0x00}, 256},
		{"Insptid_ms10", []byte{0x00, 0x7B}, 12.3},
		{"Lacc_mangd", []byte{0x10}, 16},
		{"Ret_mangd", []byte{0x01, 0x02}, 258},
		{"Ign_angle", []byte{0x00, 0x96}, 15},
		{"Ign_angle", []byte{0xFF, 0xCE}, -5},
		{"Knock_offset1234", []byte{0xFF, 0xF6}, -1},
		{"Knock_offset3", []byte{0x00, 0x0A}, 1},
		{"Medeltrot", []byte{64}, 30},
		{"Apc_decrese", []byte{0x00, 0xC8}, 2},
		// P/I/D_fak mirror T5Suite's DifGenerator, which negates with
		// 65535 (one off from two's complement).
		{"P_fak", []byte{0x80, 0x00}, -32767},
		{"I_fak", []byte{0x01, 0x00}, 256},
		{"D_fak", []byte{0xFF, 0xFF}, 0},
		{"PWM_ut10", []byte{0x02}, 2},
		{"Knock_count_cyl4", []byte{3}, 3},
		{"Knock_average", []byte{4}, 4},
		{"Bil_hast", []byte{90}, 90},
		{"TQ", []byte{0x01, 0x00}, 256},
		{"Batt_volt", []byte{123}, 12.3},
		{"Unknown_sym", []byte{0x01, 0x02}, 258},
	}
	for _, tt := range tests {
		if got := conv(tt.name, tt.in); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s % X: got %v, want %v", tt.name, tt.in, got, tt.want)
		}
	}
}

// T5 logs every symbol as a sysvar column in symbol order (all formatted as
// one-decimal values) followed by the active wideband columns, and the
// publisher stores each converted symbol there and feeds the AD scanner.
func TestT5ChannelsAndPublisher(t *testing.T) {
	syms := []*symbol.Symbol{
		{Name: "Rpm", SramOffset: 0x1000, Length: 2, Correctionfactor: 1},
		{Name: "AD_EGR", SramOffset: 0x1010, Length: 2},
	}
	bl := newTestBase(t, Config{Symbols: syms, WidebandConfig: WidebandConfig{
		Name: "ECU", ADScanner: true, ADScannerSymbol: "AD_EGR",
		SupportPoints: []int{0, 1000}, LambdaValues: []float64{0.5, 1.5},
	}})
	chans := bl.t5Channels()
	var names []string
	for _, ch := range chans {
		names = append(names, ch.Name)
	}
	if !slices.Equal(names, []string{"Rpm", "AD_EGR", LAMBDAADSCANNER}) {
		t.Fatalf("channels %v", names)
	}
	for _, s := range syms {
		if s.Correctionfactor != 0.1 {
			t.Fatalf("%s correction factor %v, want 0.1", s.Name, s.Correctionfactor)
		}
	}

	rpm := collectEbus(t, "Rpm")
	publish := bl.t5Publisher()
	_ = syms[0].SetData([]byte{0x01, 0x2C})
	_ = syms[1].SetData([]byte{0x01, 0xF4}) // 500
	publish(syms[0])
	publish(syms[1])
	if got := chans[0].String(); got != "3000" {
		t.Fatalf("Rpm column %q", got)
	}
	if got, _ := rpm.last(); got != 3000 {
		t.Fatalf("Rpm published %v", got)
	}
	if got := chans[2].Value(); got != 1.0 {
		t.Fatalf("AD scanner lambda %v", got)
	}
}

// t5Hook wraps a fake T5 so tests can fail C7 reads of addresses in
// [failFrom, failTo) (the ECU answers with status 04) and drop console writes.
func t5Hook(sim *ecusim.T5, failFrom, failTo uint32, dropWrites bool) *hookAdapter {
	return newHook(sim, "", func(bus *gocan.Bus, f gocan.Frame) bool {
		if f.ID != 0x05 {
			return false
		}
		switch f.Data[0] {
		case 0xC7:
			if addr := uint32(f.Data[3])<<8 | uint32(f.Data[4]); addr >= failFrom && addr < failTo {
				bus.Deliver(gocan.NewFrame(0x0C, []byte{0xC7, 0x04, 0, 0, 0, 0, 0, 0}))
				return true
			}
		case 0xC4:
			return dropWrites
		}
		return false
	})
}

func t5Symbols() []*symbol.Symbol {
	return []*symbol.Symbol{
		{Name: "Rpm", SramOffset: 0x1000, Length: 2},
		{Name: "Batt_volt", SramOffset: 0x1010, Length: 1},
	}
}

// A RAM write the ECU does not acknowledge fails SetRAM, and logging carries
// on afterwards.
func TestT5SetRAMFailure(t *testing.T) {
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "classic", true: "gather"}[fast], func(t *testing.T) {
			sim := &ecusim.T5{}
			sim.Poke(0x1010, []byte{123})
			msgs := newMsgLog(t)
			cfg := testCfg(t, "T5", t5Hook(sim, 0, 0, true), t5Symbols(), msgs)
			rec := newRecWriter()
			var cl IClient
			if fast {
				cl, _ = NewT5Fast(cfg, rec)
			} else {
				cl, _ = NewT5(cfg, rec)
			}
			runLogger(t, cl)
			rec.waitRows(t, 1, 10*time.Second)
			if err := cl.SetRAM(0x2000, []byte{1, 2}); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("SetRAM = %v, want the ack timeout", err)
			}
			n := len(rec.snapshot())
			rec.waitRows(t, n+3, 10*time.Second)
			if got := rec.snapshot()[n+2].text["Batt_volt"]; got != "12.30" {
				t.Fatalf("Batt_volt %q", got)
			}
		})
	}
}

// A RAM read the ECU rejects fails GetRAM promptly with the read error,
// as the T7/T8 loggers do, instead of leaving the caller to time out.
func TestT5GetRAMFailure(t *testing.T) {
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "classic", true: "gather"}[fast], func(t *testing.T) {
			sim := &ecusim.T5{}
			msgs := newMsgLog(t)
			cfg := testCfg(t, "T5", t5Hook(sim, 0x3000, 0x3100, false), t5Symbols(), msgs)
			rec := newRecWriter()
			var cl IClient
			if fast {
				cl, _ = NewT5Fast(cfg, rec)
			} else {
				cl, _ = NewT5(cfg, rec)
			}
			runLogger(t, cl)
			rec.waitRows(t, 1, 10*time.Second)
			done := make(chan error, 1)
			go func() {
				_, err := cl.GetRAM(0x3000, 4)
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil || err.Error() == "RAM read timeout" {
					t.Fatalf("GetRAM = %v, want the read error", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("GetRAM did not return within 3s")
			}
		})
	}
}

// When every symbol read fails the T5 logger counts the errors and gives up
// after a second with more than five of them.
func TestT5AbortsOnTooManyErrors(t *testing.T) {
	sim := &ecusim.T5{}
	msgs := newMsgLog(t)
	cl, _ := NewT5(testCfg(t, "T5", t5Hook(sim, 0, 0x10000, false), t5Symbols(), msgs), newRecWriter())
	s := runLogger(t, cl)
	if err := s.wait(t, 10*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	if !msgs.has("too many errors, aborting logging") || msgs.errs.Load() <= 5 {
		t.Fatalf("errors %d, messages %v", msgs.errs.Load(), msgs.all())
	}
}

// A log write failure ends a T5 session.
func TestT5StopsOnLogWriteError(t *testing.T) {
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "classic", true: "gather"}[fast], func(t *testing.T) {
			msgs := newMsgLog(t)
			cfg := testCfg(t, "T5", &ecusim.T5{}, t5Symbols(), msgs)
			rec := newRecWriter()
			rec.err = errors.New("disk full")
			var cl IClient
			if fast {
				cl, _ = NewT5Fast(cfg, rec)
			} else {
				cl, _ = NewT5(cfg, rec)
			}
			s := runLogger(t, cl)
			if err := s.wait(t, 10*time.Second); err != nil {
				t.Fatalf("Start = %v", err)
			}
			if !msgs.has("failed to write log: disk full") || len(rec.snapshot()) != 1 {
				t.Fatalf("%d rows, messages %v", len(rec.snapshot()), msgs.all())
			}
			waitClosed(t, rec.closed, time.Second, "log writer close")
		})
	}
}

// Cancelling the context ends a session cleanly and closes the log.
func TestT5StopsOnContextCancel(t *testing.T) {
	msgs := newMsgLog(t)
	rec := newRecWriter()
	cl, _ := NewT5(testCfg(t, "T5", &ecusim.T5{}, t5Symbols(), msgs), rec)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cl.Start(ctx) }()
	rec.waitRows(t, 3, 10*time.Second)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
	waitClosed(t, rec.closed, time.Second, "log writer close")
}

// Start closes the log only after the logging goroutine has stopped writing
// to it, also when the adapter dies in the middle of a frame.
func TestT5DoesNotCloseLogDuringWrite(t *testing.T) {
	sim := &ecusim.T5{}
	hook := newHook(sim, "", nil)
	rec := newRecWriter()
	blocked, release := make(chan struct{}), make(chan struct{})
	rec.onWrite = func(n int) {
		if n == 3 {
			close(blocked)
			<-release
		}
	}
	cl, _ := NewT5(testCfg(t, "T5", hook, t5Symbols(), newMsgLog(t)), rec)
	s := runLogger(t, cl)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	waitClosed(t, blocked, 10*time.Second, "third write")
	hook.fatal(errors.New("usb unplugged"))
	select {
	case <-rec.closed:
		t.Error("log closed while a write was still in progress")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := s.wait(t, 5*time.Second); err == nil {
		t.Fatal("Start did not report the adapter failure")
	}
}
