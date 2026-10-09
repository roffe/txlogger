package stag

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

const ProductString = "Stag AFR"

// reconnectDelay is how long run() waits between reconnect attempts after the
// serial port drops (a common occurrence on Windows).
const reconnectDelay = time.Second

// replyTimeout is how long session() waits for a valid frame before it
// restarts the handshake. The STAG only answers requests, and every request
// is sent in reaction to a reply, so one lost reply stalls both sides.
const replyTimeout = time.Second

// initRequest starts the handshake; the STAG answers with a 0x80 frame.
var initRequest = []byte{0xAC, 0x00, 0x00, 0x04, 0x00, 0x00, 0x32, 0xE2}

type STAG struct {
	port string
	sp   serial.Port

	lambda float64
	oxygen float64
	status byte // last reported sensor status, logged on change

	log func(string)

	closeOnce sync.Once
	closed    atomic.Bool
	done      chan struct{}
	mu        sync.Mutex
}

func NewSTAGClient(port string, logFunc func(string)) (*STAG, error) {
	return &STAG{
		port:   port,
		done:   make(chan struct{}),
		log:    logFunc,
		status: 0xFF, // unknown, so the first status gets logged
	}, nil
}

func (a *STAG) Start(ctx context.Context) error {
	// Fail fast if we can't open the port at all on the initial attempt so the
	// caller gets immediate feedback. Later drops are handled by run().
	if err := a.openPort(); err != nil {
		return err
	}

	go a.run(ctx)

	return nil
}

// openPort opens the serial port and stores it on the client.
func (a *STAG) openPort() error {
	mode := &serial.Mode{
		BaudRate: 57600,
	}
	sp, err := serial.Open(a.port, mode)
	if err != nil {
		return err
	}
	sp.SetReadTimeout(5 * time.Millisecond)

	a.mu.Lock()
	a.sp = sp
	a.mu.Unlock()
	return nil
}

// getPort returns the current serial port, or nil if none is open.
func (a *STAG) getPort() serial.Port {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sp
}

// closePort closes the current serial port if open. It is safe to call
// multiple times.
func (a *STAG) closePort() {
	a.mu.Lock()
	sp := a.sp
	a.sp = nil
	a.mu.Unlock()
	if sp != nil {
		if err := sp.Close(); err != nil {
			a.log(err.Error())
		}
	}
}

// reconnect keeps trying to reopen the serial port until it succeeds or the
// client is stopped. It returns true when the port was reopened, false when
// the client is shutting down.
func (a *STAG) reconnect(ctx context.Context) bool {
	for {
		if ctx.Err() != nil || a.closed.Load() {
			return false
		}
		a.log("Stag: reconnecting to " + a.port)
		if err := a.openPort(); err != nil {
			a.log("Stag: reconnect failed: " + err.Error())
			select {
			case <-ctx.Done():
				return false
			case <-a.done:
				return false
			case <-time.After(reconnectDelay):
			}
			continue
		}
		a.log("Stag: reconnected to " + a.port)
		return true
	}
}

func (a *STAG) GetLambda() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lambda
}

func (a *STAG) run(ctx context.Context) {
	// Make sure any port we (re)opened gets cleaned up when run() exits.
	defer a.closePort()

	for {
		if ctx.Err() != nil || a.closed.Load() {
			return
		}

		sp := a.getPort()
		if sp == nil {
			if !a.reconnect(ctx) {
				return
			}
			continue
		}

		// session() runs until the port errors or the client is stopped.
		a.session(ctx, sp)

		if ctx.Err() != nil || a.closed.Load() {
			return
		}
		a.closePort()
		if !a.reconnect(ctx) {
			return
		}
	}
}

// session drives one connection: it reads from sp and parses packets until the
// port errors or the client is stopped.
func (a *STAG) session(ctx context.Context, sp serial.Port) {
	// sessionCtx tears down the reader goroutine when this session ends.
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var p parser
	buf := make([]byte, 8)
	stalled := false

	watchdog := time.NewTimer(replyTimeout)
	defer watchdog.Stop()

	// Create a channel to receive bytes
	byteChan := make(chan byte, 100)
	errChan := make(chan error, 1)

	a.sendRequest(initRequest)
	// Start a goroutine to read bytes
	go func() {
		for {
			// read from serial
			n, err := sp.Read(buf)
			if sessionCtx.Err() != nil {
				return
			}
			if err != nil {
				select {
				case errChan <- err:
				case <-sessionCtx.Done():
				}
				return
			}
			if n == 0 {
				continue
			}
			for _, b := range buf[:n] {
				select {
				case byteChan <- b:
				case <-sessionCtx.Done():
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.done:
			return
		case err := <-errChan:
			a.log(err.Error())
			// Port dropped; let run() handle reconnection.
			return
		case <-watchdog.C:
			if !stalled {
				a.log("Stag: no reply, restarting handshake")
				stalled = true
			}
			p = parser{}
			a.sendRequest(initRequest)
			watchdog.Reset(replyTimeout)
		case b := <-byteChan:
			if frame := p.feed(b); frame != nil {
				stalled = false
				watchdog.Reset(replyTimeout)
				a.processPacket(frame)
			}
		}
	}
}

// parser assembles frames: 0x32, two bytes, a length byte, that many bytes and
// a checksum that is the sum of all earlier bytes.
type parser struct {
	frame []byte
	size  int
}

// feed adds b and returns a complete frame with a valid checksum, or nil.
func (p *parser) feed(b byte) []byte {
	if len(p.frame) == 0 && b != 0x32 {
		return nil // junk before the start byte
	}
	p.frame = append(p.frame, b)
	if len(p.frame) == 4 {
		p.size = int(b) + 4
	}
	if len(p.frame) < 4 || len(p.frame) < p.size {
		return nil
	}
	frame := p.frame
	p.frame = nil
	var sum byte
	for _, c := range frame[:len(frame)-1] {
		sum += c
	}
	if sum != frame[len(frame)-1] {
		return nil
	}
	return frame
}

func (a *STAG) Stop() {
	a.closeOnce.Do(func() {
		a.log("Stopping Stag serial client")
		// Signal run()/session()/reconnect() to stop before closing the port so
		// a port drop isn't mistaken for a reconnect opportunity.
		a.closed.Store(true)
		close(a.done)
		a.closePort()
	})
}

func (a *STAG) processPacket(packetContentBuffer []byte) {
	if len(packetContentBuffer) < 5 {
		return
	}

	switch packetContentBuffer[4] {
	case 0x80:
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x03, 0x00, 0x38})
	case 0x83:
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x6D, 0x00, 0xA2})
	case 0xF0:
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x64, 0x00, 0x99})
	case 0xE4:
		if err := a.SetData(packetContentBuffer); err != nil {
			a.log(err.Error())
		}
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x64, 0x00, 0x99})
	default:
		// Not handled
	}
}

func (a *STAG) sendRequest(data []byte) {
	time.Sleep(100 * time.Millisecond)
	if sp := a.getPort(); sp != nil {
		sp.Write(data)
	}
}

func (a *STAG) SetData(data []byte) error {
	if len(data) < 18 {
		return fmt.Errorf("stag: data frame too short: %d bytes", len(data))
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if data[6] != a.status {
		a.status = data[6]
		switch a.status {
		case 0x00:
			a.log("Stag: status_sleep")
		case 0x01:
			a.log("Stag: status_warming")
		case 0x02:
			a.log("Stag: status_work")
		case 0x03:
			a.log("Stag: status_breakdown")
		}
	}
	if a.status == 0x02 {
		a.lambda = float64(uint32(data[12])<<24|uint32(data[13])<<16|uint32(data[14])<<8|uint32(data[15])) * 0.001
		a.oxygen = float64((uint16(data[16])<<8)|uint16(data[17])) * 0.1
	}
	return nil
}

func (a *STAG) String() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return fmt.Sprintf("Lambda: %.4f, Oxygen: %.3f", a.lambda, a.oxygen)
}
