package zeitronix

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

const ProductString = "Zeitronix ZT-2"

// reconnectDelay is how long the serial handler waits between reconnect
// attempts after the serial port drops (a common occurrence on Windows).
const reconnectDelay = time.Second

/*
Zeitronix Packet format, []byte
[0] always 0
[1] always 1
[2] always 2
[3] AFR
[4] EGT Low
[5] EGT High
[6] RPM Low
[7] RPM High
[8] MAP Low
[9] MAP High
[10] TPS
[11] USER1
[12] Config Register1
[13] Config Register2
*/

type Zeitronix struct {
	Port string

	lambdaValue float64
	egtValue    uint16
	rpmValue    uint16
	mapValue    uint16

	p         serial.Port
	closeOnce sync.Once
	closed    atomic.Bool
	done      chan struct{}
	mu        sync.Mutex
	logFunc   func(string)
}

func NewZeitronixClient(port string, logFunc func(string)) (*Zeitronix, error) {
	z := &Zeitronix{
		Port:    port,
		done:    make(chan struct{}),
		logFunc: logFunc,
	}
	return z, nil
}

func (z *Zeitronix) Start(ctx context.Context) error {
	// Fail fast if we can't open the port at all on the initial attempt so the
	// caller gets immediate feedback. Later drops are handled by serialHandler().
	if err := z.openPort(); err != nil {
		return err
	}
	go z.serialHandler(ctx)

	return nil
}

// openPort opens the serial port and stores it on the client.
func (z *Zeitronix) openPort() error {
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
		DataBits: 8,
	}
	sp, err := serial.Open(z.Port, mode)
	if err != nil {
		return err
	}
	sp.SetReadTimeout(5 * time.Millisecond)

	z.mu.Lock()
	z.p = sp
	z.mu.Unlock()
	return nil
}

// getPort returns the current serial port, or nil if none is open.
func (z *Zeitronix) getPort() serial.Port {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.p
}

// closePort closes the current serial port if open. It is safe to call
// multiple times.
func (z *Zeitronix) closePort() {
	z.mu.Lock()
	sp := z.p
	z.p = nil
	z.mu.Unlock()
	if sp != nil {
		if err := sp.Close(); err != nil {
			z.logFunc("Zeitronix: " + err.Error())
		}
	}
}

// reconnect keeps trying to reopen the serial port until it succeeds or the
// client is stopped. It returns true when the port was reopened, false when
// the client is shutting down.
func (z *Zeitronix) reconnect(ctx context.Context) bool {
	for {
		if ctx.Err() != nil || z.closed.Load() {
			return false
		}
		z.logFunc("Zeitronix: reconnecting to " + z.Port)
		if err := z.openPort(); err != nil {
			z.logFunc("Zeitronix: reconnect failed: " + err.Error())
			select {
			case <-ctx.Done():
				return false
			case <-z.done:
				return false
			case <-time.After(reconnectDelay):
			}
			continue
		}
		z.logFunc("Zeitronix: reconnected to " + z.Port)
		return true
	}
}

func (z *Zeitronix) serialHandler(ctx context.Context) {
	// Make sure any port we (re)opened gets cleaned up when this returns.
	defer z.closePort()

	buff := make([]byte, 14)
	cmd := make([]byte, 14)
	step := 0
	for {
		if ctx.Err() != nil || z.closed.Load() {
			return
		}

		sp := z.getPort()
		if sp == nil {
			if !z.reconnect(ctx) {
				return
			}
			step = 0
			continue
		}

		n, err := sp.Read(buff)
		if ctx.Err() != nil || z.closed.Load() {
			return
		}
		if err != nil {
			z.logFunc("Zeitronix read error: " + err.Error())
			z.closePort()
			step = 0
			if !z.reconnect(ctx) {
				return
			}
			continue
		}
		if n == 0 {
			continue
		}
		for _, b := range buff[:n] {
			switch step {
			case 0, 1, 2:
				if b == byte(step) {
					cmd[step] = b
					step++
					continue
				}
				step = 0
				continue
			case 3, 4, 5, 6, 7, 8, 9, 10, 11, 12:
				cmd[step] = b
				step++
				continue
			case 13:
				cmd[13] = b
				// Got full packet parse it
				z.SetData(cmd)
				step = 0
				continue
			default:
				step = 0
			}
		}

	}
}

func (z *Zeitronix) SetData(data []byte) error {
	if len(data) < 14 {
		return errors.New("invalid data length")
	}
	if data[0] != 0 || data[1] != 1 || data[2] != 2 {
		return errors.New("invalid data format")
	}
	z.lambdaValue = float64(data[3]) * 0.01
	z.egtValue = uint16(data[4]) | (uint16(data[5]) << 8)
	z.rpmValue = uint16(data[6]) | (uint16(data[7]) << 8)
	z.mapValue = uint16(data[8]) | (uint16(data[9]) << 8)
	return nil
}

func (z *Zeitronix) Stop() {
	z.closeOnce.Do(func() {
		z.logFunc("Closing Zeitronix client")
		// Signal serialHandler()/reconnect() to stop before closing the port so
		// a port drop isn't mistaken for a reconnect opportunity.
		z.closed.Store(true)
		close(z.done)
		z.closePort()
	})
}

func (z *Zeitronix) GetLambda() float64 {
	return z.lambdaValue
}

func (z *Zeitronix) String() string {
	return fmt.Sprintf("Lambda: %.3f, EGT: %d, RPM: %d, MAP: %d", z.lambdaValue, z.egtValue, z.rpmValue, z.mapValue)
}
