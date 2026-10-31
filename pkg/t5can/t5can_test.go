package t5can

import (
	"context"
	"testing"

	"github.com/roffe/gocan/v2"
)

// fakeT5 acks every command with [echo, 0x00], but puts a stray 0xFF frame on
// the bus ahead of the first reply, like an ECU fresh out of reset.
type fakeT5 struct {
	bus   *gocan.Bus
	stray bool
}

func (e *fakeT5) Open(_ context.Context, bus *gocan.Bus) error { e.bus = bus; return nil }
func (e *fakeT5) Close() error                                 { return nil }

func (e *fakeT5) Send(_ context.Context, f gocan.Frame) error {
	if !e.stray {
		e.stray = true
		e.bus.Deliver(gocan.NewFrame(replyID, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}))
	}
	e.bus.Deliver(gocan.NewFrame(replyID, []byte{f.Data[0], respOK, 0, 0, 0, 0, 0, 0}))
	return nil
}

func TestWriteRamSkipsStrayReply(t *testing.T) {
	bus, err := gocan.OpenAdapter(context.Background(), &fakeT5{})
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if err := NewClient(bus).WriteRam(context.Background(), 0x7700, make([]byte, 20)); err != nil {
		t.Fatal(err)
	}
}
