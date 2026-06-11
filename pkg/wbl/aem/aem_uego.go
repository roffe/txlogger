package aem

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

const ProductString = "AEM Uego"

// reconnectDelay is how long run() waits between reconnect attempts after the
// serial port drops (a common occurrence on Windows).
const reconnectDelay = time.Second

type AEMuego struct {
	port string
	sp   serial.Port

	lamba   float64
	oxygen  float64
	voltage float64

	log func(string)

	closeOnce sync.Once
	closed    atomic.Bool
	done      chan struct{}
	mu        sync.Mutex

	dataBuff []byte
	dataPos  int

	//debugLog *os.File
}

func NewAEMuegoClient(port string, logFunc func(string)) (*AEMuego, error) {
	//f, err := os.Create("aem.log")
	//if err != nil {
	//	return nil, err
	//}
	return &AEMuego{
		port:     port,
		log:      logFunc,
		done:     make(chan struct{}),
		dataBuff: make([]byte, 8),
		//debugLog: f,
	}, nil
}

func (a *AEMuego) Start(ctx context.Context) error {
	// Fail fast if we can't open the port at all on the initial attempt so the
	// caller gets immediate feedback. Later drops are handled by run().
	if err := a.openPort(); err != nil {
		return err
	}

	go a.run(ctx)

	return nil
}

// openPort opens the serial port and stores it on the client.
func (a *AEMuego) openPort() error {
	mode := &serial.Mode{
		BaudRate: 9600,
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
func (a *AEMuego) getPort() serial.Port {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sp
}

// closePort closes the current serial port if open. It is safe to call
// multiple times.
func (a *AEMuego) closePort() {
	a.mu.Lock()
	sp := a.sp
	a.sp = nil
	a.mu.Unlock()
	if sp != nil {
		if err := sp.Close(); err != nil {
			a.log("AEM: " + err.Error())
		}
	}
}

// reconnect keeps trying to reopen the serial port until it succeeds or the
// client is stopped. It returns true when the port was reopened, false when
// the client is shutting down.
func (a *AEMuego) reconnect(ctx context.Context) bool {
	for {
		if ctx.Err() != nil || a.closed.Load() {
			return false
		}
		a.log("AEM: reconnecting to " + a.port)
		if err := a.openPort(); err != nil {
			a.log("AEM: reconnect failed: " + err.Error())
			select {
			case <-ctx.Done():
				return false
			case <-a.done:
				return false
			case <-time.After(reconnectDelay):
			}
			continue
		}
		a.log("AEM: reconnected to " + a.port)
		a.dataPos = 0
		return true
	}
}

func (a *AEMuego) GetLambda() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lamba
}

func (a *AEMuego) SetLambda(value float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lamba = value
}

func (a *AEMuego) run(ctx context.Context) {
	// Make sure any port we (re)opened gets cleaned up when run() exits.
	defer a.closePort()

	buf := make([]byte, 8)
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

		// read from serial
		n, err := sp.Read(buf)
		if ctx.Err() != nil || a.closed.Load() {
			return
		}
		if err != nil {
			a.log("AEM: " + err.Error())
			a.closePort()
			if !a.reconnect(ctx) {
				return
			}
			continue
		}
		if n == 0 {
			continue
		}

		//log.Printf("AEM: %s", buf[:n])

		for _, b := range buf[:n] {
			switch b {
			case '\r':
				continue
			case '\n':
				value, err := strconv.ParseFloat(string(a.dataBuff[:a.dataPos]), 64)
				if err != nil {
					a.log("AEM: " + err.Error())
					a.dataPos = 0
					continue
				}

				// log.Printf("AEM: %0.3f", value/10)
				a.mu.Lock()
				a.lamba = value / 10
				a.mu.Unlock()

				a.dataPos = 0
				continue
			}
			a.dataBuff[a.dataPos] = b
			a.dataPos++
			if a.dataPos == 8 {
				a.dataPos = 0
			}
		}

	}
}

func (a *AEMuego) Stop() {
	a.closeOnce.Do(func() {
		a.log("Stopping AEM serial client")
		// Signal run()/reconnect() to stop before closing the port so a port
		// drop isn't mistaken for a reconnect opportunity.
		a.closed.Store(true)
		close(a.done)
		a.closePort()
		//if a.debugLog != nil {
		//	a.debugLog.Sync()
		//	a.debugLog.Close()
		//}
	})
}

/*
| Byte | Bit    | Bitmask | Label               | Data Type       | Scaling             | Offset | Range
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|  0-1 |        |         | Lambda              | 16 bit unsigned | .0001 Lambda/bit    | 0      | 0 to 6.5535 Lambda
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|  2-3 |        |         | Oxygen              | 16 bit signed   | 0.001%/bit          | 0      | -32.768% to 32.767%
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   4  |        |         | System Volts        | 8 bit unsigned  | 0.1 V/bit           | 0      | 0 to 25.5 Volts
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   5  |        |         | Reserved            | ---             | ---                 | ---    | ---
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   6  |0 (lsb) |    0    | Reserved            | ---             | ---                 | ---    | ---
|      |   1    |    2    | AEM/FAE Detected    | Boolean         | 0 = false, 1 = true | 0      | 0/1
|      |  2-4   |    4    | Reserved            | ---             | ---                 | ---    | ---
|      |   5    |   32    | Free-Air cal in use | Boolean         | 0 = false, 1 = true | 0      | 0/1
|      |   6    |   64    | Reserved            | ---             | ---                 | ---    | ---
|      |7 (msb) |  128    | Lambda Data Valid   | Boolean         | 0 = false, 1 = true | 0      | 0/1
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   7  |  0-5   |    0    | Reserved            | ---             | ---                 | ---    | ---
|      |   6    |   64    | Sensor Fault        | Boolean         | 0 = false, 1 = true | 0      |0/1
|      | 7(msb) |  128    | Reserved            | ---             | ---                 | ---    | ---
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
*/

// Set CAN data
func (a *AEMuego) SetData(data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// fmt.Fprintf(a.debugLog, "% 02X\n", data)

	r := bytes.NewReader(data)
	var wbl uint16
	binary.Read(r, binary.BigEndian, &wbl)
	a.lamba = float64(wbl) * 0.0001

	var oxygen uint16
	binary.Read(r, binary.BigEndian, &oxygen)
	a.oxygen = float64(oxygen) * 0.001

	var systemVolt uint8
	binary.Read(r, binary.BigEndian, &systemVolt)
	a.voltage = float64(systemVolt) * 0.1

	return nil
}

func (a *AEMuego) String() string {
	return fmt.Sprintf("Lambda: %.4f, Oxygen: %.3f, Voltage: %.1f", a.lamba, a.oxygen, a.voltage)
}
