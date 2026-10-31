package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// The gather image is the stub padded to 0x40, then the descriptor table:
// symbol count, a pad byte and one {addr hi, addr lo, len, pad} per symbol.
func TestBuildT5GatherImage(t *testing.T) {
	img, err := buildT5GatherImage([]*symbol.Symbol{
		{Name: "Rpm", SramOffset: 0x1234, Length: 2},
		{Name: "Batt_volt", SramOffset: 0x7FFF, Length: 255},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), t5GatherStub...)
	want = append(want, make([]byte, 0x40-len(t5GatherStub))...)
	want = append(want, 2, 0, 0x12, 0x34, 2, 0, 0x7F, 0xFF, 255, 0)
	if !bytes.Equal(img, want) {
		t.Fatalf("image\n% X\nwant\n% X", img, want)
	}
}

// Symbols the 16-bit address / 8-bit length table cannot describe, or a
// table too large for the one-shot upload, are refused.
func TestBuildT5GatherImageLimits(t *testing.T) {
	bad := []*symbol.Symbol{
		{Name: "high", SramOffset: 0x8000, Length: 1},
		{Name: "empty", SramOffset: 0x1000, Length: 0},
		{Name: "long", SramOffset: 0x1000, Length: 256},
	}
	for _, s := range bad {
		if _, err := buildT5GatherImage([]*symbol.Symbol{s}); err == nil || !strings.Contains(err.Error(), "not gather-representable") {
			t.Errorf("%s: err = %v", s.Name, err)
		}
	}
	many := func(n int) []*symbol.Symbol {
		out := make([]*symbol.Symbol, n)
		for i := range out {
			out[i] = &symbol.Symbol{Name: fmt.Sprint(i), SramOffset: uint32(0x1000 + i), Length: 1}
		}
		return out
	}
	if img, err := buildT5GatherImage(many(47)); err != nil || len(img) != 254 {
		t.Fatalf("47 symbols: %d bytes, %v", len(img), err)
	}
	if _, err := buildT5GatherImage(many(48)); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("48 symbols: %v", err)
	}
}

// frameLog records the frames sent through a hookAdapter.
type frameLog struct {
	mu     sync.Mutex
	frames []gocan.Frame
}

func (l *frameLog) add(f gocan.Frame) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frames = append(l.frames, f)
}

func (l *frameLog) withFirstByte(id uint32, b byte) []gocan.Frame {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []gocan.Frame
	for _, f := range l.frames {
		if f.ID == id && len(f.Data) > 0 && f.Data[0] == b {
			out = append(out, f)
		}
	}
	return out
}

func openBus(t *testing.T, a gocan.Adapter) *gocan.Bus {
	t.Helper()
	cl, err := gocan.OpenAdapter(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

// armT5Gather sends A5 with the stub address and length 0 (the stream-call
// flag) and wants A5 00 back; callT5Gather sends the reply-less C1 call.
func TestArmAndCallT5Gather(t *testing.T) {
	var log frameLog
	cl := openBus(t, newHook(&ecusim.T5{}, "", func(_ *gocan.Bus, f gocan.Frame) bool {
		log.add(f)
		return false
	}))
	ctx := context.Background()
	if err := armT5Gather(ctx, cl); err != nil {
		t.Fatal(err)
	}
	if err := callT5Gather(ctx, cl); err != nil {
		t.Fatal(err)
	}
	arm := log.withFirstByte(0x05, 0xA5)
	call := log.withFirstByte(0x05, 0xC1)
	if len(arm) != 1 || !bytes.Equal(arm[0].Bytes(), []byte{0xA5, 0, 0, 0x77, 0x00, 0, 0, 0}) {
		t.Fatalf("arm frames %v", arm)
	}
	if len(call) != 1 || !bytes.Equal(call[0].Bytes(), []byte{0xC1, 0, 0, 0x77, 0x00, 0, 0, 0}) {
		t.Fatalf("call frames %v", call)
	}
}

// An A5 answered with a non-zero status (engine running) or not answered at
// all fails the arm.
func TestArmT5GatherFailures(t *testing.T) {
	reject := openBus(t, newHook(&ecusim.T5{}, "", func(bus *gocan.Bus, f gocan.Frame) bool {
		if f.ID == 0x05 && f.Data[0] == 0xA5 {
			bus.Deliver(gocan.NewFrame(0x0C, []byte{0xA5, 0x01, 0, 0, 0, 0, 0, 0}))
			return true
		}
		return false
	}))
	if err := armT5Gather(context.Background(), reject); err == nil || !strings.Contains(err.Error(), "arm rejected") {
		t.Fatalf("rejected arm: %v", err)
	}
	silent := openBus(t, newHook(&ecusim.T5{}, "", func(_ *gocan.Bus, f gocan.Frame) bool {
		return f.ID == 0x05 && f.Data[0] == 0xA5
	}))
	if err := armT5Gather(context.Background(), silent); err == nil {
		t.Fatal("unanswered arm succeeded")
	}
}

// The fast logger uploads the gather image to the stub address before
// logging, and refuses to start when the symbols cannot be gathered or the
// upload or arm is rejected.
func TestT5FastSetup(t *testing.T) {
	t.Run("uploads image", func(t *testing.T) {
		sim := &ecusim.T5{}
		rec := newRecWriter()
		syms := t5Symbols()
		cl, _ := NewT5Fast(testCfg(t, "T5", sim, syms, newMsgLog(t)), rec)
		runLogger(t, cl)
		rec.waitRows(t, 1, 10*time.Second)
		want, _ := buildT5GatherImage(syms)
		if got := sim.Peek(t5GatherStubAddr, len(want)); !bytes.Equal(got, want) {
			t.Fatalf("SRAM at stub\n% X\nwant\n% X", got, want)
		}
	})

	rejectA5 := func(armOnly bool) func(*gocan.Bus, gocan.Frame) bool {
		return func(bus *gocan.Bus, f gocan.Frame) bool {
			if f.ID != 0x05 || f.Data[0] != 0xA5 || (armOnly && f.Data[5] != 0) {
				return false
			}
			bus.Deliver(gocan.NewFrame(0x0C, []byte{0xA5, 0x01, 0, 0, 0, 0, 0, 0}))
			return true
		}
	}
	tests := []struct {
		name    string
		syms    []*symbol.Symbol
		hook    func(*gocan.Bus, gocan.Frame) bool
		wantErr string
	}{
		{"symbol out of range", []*symbol.Symbol{{Name: "X", SramOffset: 0x9000, Length: 2}}, nil, "not gather-representable"},
		{"upload rejected", t5Symbols(), rejectA5(false), "gather upload"},
		{"arm rejected", t5Symbols(), rejectA5(true), "gather arm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := newMsgLog(t)
			rec := newRecWriter()
			cl, _ := NewT5Fast(testCfg(t, "T5", newHook(&ecusim.T5{}, "", tt.hook), tt.syms, msgs), rec)
			err := runLogger(t, cl).wait(t, 10*time.Second)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Start = %v, want %q", err, tt.wantErr)
			}
			if len(rec.snapshot()) != 0 {
				t.Fatal("rows logged despite failed setup")
			}
			waitClosed(t, rec.closed, time.Second, "log writer close")
		})
	}
}
