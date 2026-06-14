package innovate

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

const (
	ProductString = "Innovate Serial Protocol v2"
)

// reconnectDelay is how long run() waits between reconnect attempts after the
// serial port drops (a common occurrence on Windows).
const reconnectDelay = time.Second

const (
	ISP2_NORMAL uint8 = iota
	ISP2_O2
	ISP2_CALIBRATING
	ISP2_NEED_CALIBRATION
	ISP2_WARMING
	ISP2_HEATER_CALIBRATING
	ISP2_LAMBDA_ERROR_CODE
	ISP2_RESERVED
)

const (
	ISP2_HEADER_BITS    = 0xA280
	ISP2_LAMBDA_DIVISOR = 1000
	ISP2_LAMBDA_OFFSET  = 500
)

const (
	ISP2_WORD_HEADER = iota
	ISP2_WORD_STATUS
	ISP2_WORD_LAMBDA
	ISP2_BATTERY_VOLTAGE
)

type ISP2Client struct {
	port string
	sp   serial.Port

	afr           float64
	afrMultiplier float64
	lambda        float64

	buff *bytes.Buffer

	status    uint8
	wordIndex uint8

	wordLength uint8

	syncBuffer []byte // New field for synchronization

	closeOnce sync.Once
	closed    atomic.Bool
	done      chan struct{}
	mu        sync.Mutex

	log func(string)
}

func NewISP2Client(port string, logFunc func(string)) (*ISP2Client, error) {
	return &ISP2Client{
		port: port,
		buff: bytes.NewBuffer(nil),
		done: make(chan struct{}),
		log:  logFunc,
	}, nil
}

func (c *ISP2Client) Start(ctx context.Context) error {
	if c.port == "txbridge" {
		return nil
	}
	c.log("Starting ISP2 client")
	// Fail fast if we can't open the port at all on the initial attempt so the
	// caller gets immediate feedback. Later drops are handled by run().
	if err := c.openPort(); err != nil {
		return err
	}
	go c.run(ctx)
	return nil
}

// openPort opens the serial port and stores it on the client.
func (c *ISP2Client) openPort() error {
	mode := &serial.Mode{
		BaudRate: 19200,
	}
	sp, err := serial.Open(c.port, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %w", err)
	}
	if err := sp.SetReadTimeout(5 * time.Millisecond); err != nil {
		sp.Close()
		return fmt.Errorf("failed to set read timeout: %w", err)
	}

	c.mu.Lock()
	c.sp = sp
	c.mu.Unlock()
	return nil
}

// getPort returns the current serial port, or nil if none is open.
func (c *ISP2Client) getPort() serial.Port {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sp
}

// closePort closes the current serial port if open. It is safe to call
// multiple times.
func (c *ISP2Client) closePort() {
	c.mu.Lock()
	sp := c.sp
	c.sp = nil
	c.mu.Unlock()
	if sp != nil {
		if err := sp.Close(); err != nil {
			c.log("isp2: failed to close serial port: " + err.Error())
		}
	}
}

// reconnect keeps trying to reopen the serial port until it succeeds or the
// client is stopped. It returns true when the port was reopened, false when
// the client is shutting down.
func (c *ISP2Client) reconnect(ctx context.Context) bool {
	for {
		if ctx.Err() != nil || c.closed.Load() {
			return false
		}
		c.log("isp2: reconnecting to " + c.port)
		if err := c.openPort(); err != nil {
			c.log("isp2: reconnect failed: " + err.Error())
			select {
			case <-ctx.Done():
				return false
			case <-c.done:
				return false
			case <-time.After(reconnectDelay):
			}
			continue
		}
		c.log("isp2: reconnected to " + c.port)
		// Drop any partially-parsed frame from before the drop.
		c.mu.Lock()
		c.syncBuffer = nil
		c.wordIndex = 0
		c.mu.Unlock()
		return true
	}
}

func (c *ISP2Client) Stop() {
	c.closeOnce.Do(func() {
		c.log("Stopping ISP2 client")
		// Signal run()/reconnect() to stop before closing the port so a port
		// drop isn't mistaken for a reconnect opportunity.
		c.closed.Store(true)
		close(c.done)
		c.closePort()
	})
}

func (c *ISP2Client) SetData(data []byte) {
	c.processBytes(data)
}

func (c *ISP2Client) GetAFR() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.afr
}

func (c *ISP2Client) GetLambda() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getLambda()
}

func (c *ISP2Client) getLambda() float64 {
	switch c.status {
	case ISP2_NORMAL:
		return c.lambda
	case ISP2_O2:
		return 1.5
	case ISP2_CALIBRATING:
		return 0.502
	case ISP2_NEED_CALIBRATION:
		return 0.503
	case ISP2_WARMING:
		return 0.504
	case ISP2_HEATER_CALIBRATING:
		return 0.505
	case ISP2_LAMBDA_ERROR_CODE:
		return 0.506
	case ISP2_RESERVED:
		return 0.507
	default:
		return 0.666
	}
}

func (c *ISP2Client) GetStatus() uint8 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *ISP2Client) GetAFRMultiplier() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.afrMultiplier
}

func (c *ISP2Client) GetLambdaStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return lambdaStatus(c.status)
}

func (c *ISP2Client) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("AFR stoich: %.01f, AFR: %.03f, λ: %.03f - %s", c.afrMultiplier, c.afr, c.getLambda(), lambdaStatus(c.status))
}

func (c *ISP2Client) run(ctx context.Context) {
	// Make sure any port we (re)opened gets cleaned up when run() exits.
	defer c.closePort()

	buf := make([]byte, 16)
	for {
		if ctx.Err() != nil || c.closed.Load() {
			return
		}

		sp := c.getPort()
		if sp == nil {
			if !c.reconnect(ctx) {
				return
			}
			continue
		}

		// read from serial
		n, err := sp.Read(buf)
		if ctx.Err() != nil || c.closed.Load() {
			return
		}
		if err != nil {
			c.log("isp2: " + err.Error())
			c.closePort()
			if !c.reconnect(ctx) {
				return
			}
			continue
		}
		if n == 0 {
			continue
		}

		// Process the received bytes
		c.processBytes(buf[:n])
	}
}

func (c *ISP2Client) processBytes(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Append new data to the sync buffer
	c.syncBuffer = append(c.syncBuffer, data...)

	// Try to find the header and process data
	for len(c.syncBuffer) >= 2 {
		headerWord := binary.BigEndian.Uint16(c.syncBuffer)
		if headerWord&ISP2_HEADER_BITS == ISP2_HEADER_BITS {
			// Found a valid header
			if len(c.syncBuffer) < 6 {
				// Not enough data to process a complete message, wait for more
				return
			}

			c.wordLength = c.syncBuffer[0]&0x01<<7 | c.syncBuffer[1]&0x7F
			if c.wordLength > 10 {
				log.Println("Invalid word length:", c.wordLength)
				// Invalid word length, remove the first byte and continue searching
				c.syncBuffer = c.syncBuffer[1:]
				continue
			}
			// log.Println("Word length:", c.wordLength)
			totalLength := int(c.wordLength*2) + 2 // +2 for the header word

			if len(c.syncBuffer) < totalLength {
				// Not enough data for the complete message, wait for more
				return
			}

			// Process the complete message
			c.processMessage(c.syncBuffer[:totalLength])

			// Remove the processed message from the buffer
			c.syncBuffer = c.syncBuffer[totalLength:]
			c.wordIndex = 0
		} else {
			// Invalid header, remove the first byte and continue searching
			c.syncBuffer = c.syncBuffer[1:]
		}
	}
}

func (c *ISP2Client) processMessage(message []byte) {
	for i := 0; i < len(message); i += 2 {
		word := message[i : i+2]
		switch c.wordIndex {
		case ISP2_WORD_HEADER:
			// Header already verified, skip
		case ISP2_WORD_STATUS:
			c.status = word[0] >> 2 & 0x07
			c.afrMultiplier = getAFRMultiplier(word)
		case ISP2_WORD_LAMBDA:
			c.lambda = getLambda(word)
			if c.lambda < 0.5 {
				c.lambda = 0.5
			}
			if c.lambda > 1.5 {
				c.lambda = 1.5
			}
			c.afr = c.lambda * c.afrMultiplier
		case ISP2_BATTERY_VOLTAGE:
			// Process battery voltage if needed
		}
		c.wordIndex++
	}
}

func getLambda(data []byte) float64 {
	return float64(uint16(data[0])<<7+uint16(data[1])+ISP2_LAMBDA_OFFSET) / ISP2_LAMBDA_DIVISOR
}

func getAFRMultiplier(data []byte) float64 {
	high := data[0] & 0x01 << 7
	low := data[1] & 0x7F
	return float64(high|low) * 0.1
}

func lambdaStatus(input uint8) string {
	switch input {
	case ISP2_NORMAL:
		return "Lambda valid and Aux data valid, normal operation (000)"
	case ISP2_O2:
		return "Lambda value contains O2 level in 1/10% (001)"
	case ISP2_CALIBRATING:
		return "Free air Calib in progress, Lambda data not valid (010)"
	case ISP2_NEED_CALIBRATION:
		return "Need Free air Calibration Request, Lambda data not valid (011)"
	case ISP2_WARMING:
		return "Warming up, Lambda value is temp in 1/10% of operating temp (100)"
	case ISP2_HEATER_CALIBRATING:
		return "Heater Calibration,  Lambda value contains calibration countdown (101)"
	case ISP2_LAMBDA_ERROR_CODE:
		return "Error code in Lambda value (110)"
	case ISP2_RESERVED:
		return "reserved (111)"
	default:
		return "Unknown"
	}
}
