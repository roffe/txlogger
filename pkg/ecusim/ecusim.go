// Package ecusim holds fake ECUs that sit behind a virtual gocan adapter, so
// the code that talks to them can be run end to end without hardware.
//
// Every sim answers from inside Send, so a reply is on the bus before Send
// returns unless Latency is set, in which case replies arrive from another
// goroutine after that delay, the way a real adapter hands them over.
package ecusim

import (
	"time"

	"github.com/roffe/gocan/v2"
)

// deliver hands frames to the bus in order, after latency if set.
func deliver(bus *gocan.Bus, latency time.Duration, trace func(string), frames ...gocan.Frame) {
	if trace != nil {
		for _, f := range frames {
			trace("-> " + f.String())
		}
	}
	if latency == 0 {
		for _, f := range frames {
			bus.Deliver(f)
		}
		return
	}
	go func() {
		time.Sleep(latency)
		for _, f := range frames {
			bus.Deliver(f)
		}
	}()
}

// pad8 returns b as an 8-byte frame payload, zero padded.
func pad8(b []byte) []byte {
	out := make([]byte, 8)
	copy(out, b)
	return out
}
