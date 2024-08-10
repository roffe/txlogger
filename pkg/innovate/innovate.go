package innovate

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"go.bug.st/serial"
)

const (
	ProductString = "Innovate Serial Protocol v2"
)

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
	wordBytes []byte

	wordLength uint8

	skipOneByte bool

	mu sync.Mutex

	log func(string)
}

func NewISP2Client(port string, logFunc func(string)) (*ISP2Client, error) {
	return &ISP2Client{
		port:      port,
		wordBytes: make([]byte, 2),
		buff:      bytes.NewBuffer(nil),
		log:       logFunc,
	}, nil
}

func (c *ISP2Client) Start(ctx context.Context) error {
	c.log("Starting ISP2 client")
	mode := &serial.Mode{
		BaudRate: 19200,
	}
	sp, err := serial.Open(c.port, mode)
	if err != nil {
		return err
	}
	c.sp = sp

	c.sp.SetReadTimeout(20 * time.Millisecond)

	go c.run(ctx)

	return nil
}

func (c *ISP2Client) Stop() {
	c.log("Stopping ISP2 client")
	if err := c.sp.Close(); err != nil {
		c.log(err.Error())
	}
}

func (c *ISP2Client) GetAFR() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.afr
}

func (c *ISP2Client) GetLambda() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lambda < 0.5 {
		return 0.5
	}
	if c.lambda > 1.5 {
		return 1.5
	}
	return c.lambda
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
	return fmt.Sprintf("AFR stoich: %.01f, AFR: %.03f, λ: %.03f - %s", c.afrMultiplier, c.afr, c.lambda, lambdaStatus(c.status))
}

func lambdaStatus(input uint8) string {
	switch input {
	case 0:
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

func (c *ISP2Client) run(ctx context.Context) {
	for {
		// read from serial
		buf := make([]byte, 16)
		n, err := c.sp.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log("isp2: " + err.Error())
			return
		}
		if n == 0 {
			continue
		}

		n1, err := c.buff.Write(buf[:n])
		if err != nil {
			c.log("isp2: " + err.Error())
			return
		}

		if n1 != n {
			c.log("isp2: n1 != n")
			return
		}

		if c.buff.Len() >= 2 {
			if err := c.process(); err != nil {
				c.log("isp2: " + err.Error())
			}
		}
	}
}

func (c *ISP2Client) process() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var headerWord uint16

	_, err := c.buff.Read(c.wordBytes)
	if err != nil {
		return err
	}
	switch c.wordIndex {
	case ISP2_WORD_HEADER:
		log.Println("check header")
		headerWord = binary.BigEndian.Uint16(c.wordBytes)
		if headerWord&ISP2_HEADER_BITS != ISP2_HEADER_BITS {
			//c.buff.ReadByte() // take one byte off the buffer if we managed to start reading in the middle of a word
			return nil
		}
		c.wordLength = c.wordBytes[0]&0x01<<7 | c.wordBytes[1]&0x7F
	case ISP2_WORD_STATUS:
		log.Println("status")
		c.status = c.wordBytes[0] >> 2 & 0x07 // bits 2-4 in first byte 0 indexed
		c.afrMultiplier = getAFRMultiplier(c.wordBytes)
	case ISP2_WORD_LAMBDA:
		log.Println("lambda")
		c.lambda = getLambda(c.wordBytes)
		c.afr = c.lambda * c.afrMultiplier
	case ISP2_BATTERY_VOLTAGE:
		//bv := c.wordBytes[0]&0x07<<7 | c.wordBytes[1]&0x7F
		//mb := c.wordBytes[0] & 0x38 >> 3
		//voltage := float64(bv) * 5 * float64(mb) / 1023
		//log.Println("Battery voltage:", voltage)
	default:
		return fmt.Errorf("unknown word index")
	}

	if c.wordIndex == c.wordLength {
		c.wordIndex = 0
	} else {
		c.wordIndex++
	}

	return nil
}

func getLambda(data []byte) float64 {
	return float64(uint16(data[0])<<7+uint16(data[1])+ISP2_LAMBDA_OFFSET) / ISP2_LAMBDA_DIVISOR
}

func getAFRMultiplier(data []byte) float64 {
	high := data[0] & 0x01 << 7
	low := data[1] & 0x7F
	return float64(high|low) / 10
}
