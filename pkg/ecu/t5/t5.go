package t5

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Trionic 5",
		NewFunc: New,
		CANRate: 615.384,
		Filter:  []uint32{0x00, 0x05, 0x06, 0x0C},
	})
}

// Timeouts matching TrionicCANLib (GeneralTimeoutmS, EraseTimeoutmS, ChecksumTimeoutmS)
const (
	generalTimeout  = 1 * time.Second
	eraseTimeout    = 60 * time.Second
	checksumTimeout = 10 * time.Second
)

type ECUType int

const (
	T52ECU ECUType = iota
	T55ECU16MHZAMDIntel
	T55ECU16MHZCatalyst
	T55ECU20MHZ
	Autodetect
	UnknownECU
	T55ECU
	T55AST52
)

const (
	Partnumber byte = 0x01
	SoftwareID byte = 0x02
	Dataname   byte = 0x03 // SW Version
	EngineType byte = 0x04
	ImmoCode   byte = 0x05
	Unknown    byte = 0x06
	ROMend     byte = 0xFC // Always 07FFFF
	ROMoffset  byte = 0xFD // T5.5 = 040000, T5.2 = 020000
	CodeEnd    byte = 0xFE
)

type Client struct {
	c          *gocan.Bus
	bootloaded bool
	chipTypes  []byte
	footer     []byte
	cfg        *ecu.Config
}

func (t *Client) MarryECU(context.Context, string) error {
	return errors.New("not supported")
}

// RecoverECU flashes without relying on the (possibly erased) flash footer:
// the box type is determined from the FLASH chip id which is readable even
// when the flash is blank. Requires the ECU to have stayed powered since the
// failed flash so the SRAM bootloader is still reachable.
func (t *Client) RecoverECU(ctx context.Context, bin []byte) error {
	return t.FlashECU(ctx, bin)
}

func New(c *gocan.Bus, cfg *ecu.Config) ecu.Client {
	t := &Client{
		c:   c,
		cfg: ecu.LoadConfig(cfg),
	}
	return t
}

// command sends a single 8-byte bootloader command and validates the reply:
// full 8 byte DLC and the first byte echoing back what we sent. Status byte
// (Data[1]) semantics differ per command and are left to the caller.
func (t *Client) command(ctx context.Context, payload []byte, timeout time.Duration) ([]byte, error) {
	frame := gocan.NewFrame(0x5, payload)
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := t.c.Request(rctx, frame, 0xC)
	if err != nil {
		return nil, err
	}
	if resp.Length != 8 {
		return nil, fmt.Errorf("short response to command %02X: %X", payload[0], resp.Data)
	}
	if resp.Data[0] != payload[0] {
		return nil, fmt.Errorf("response echo mismatch for command %02X: %X", payload[0], resp.Data)
	}
	return resp.Data[:], nil
}

func (t *Client) GetChipTypes(ctx context.Context) ([]byte, error) {
	if len(t.chipTypes) > 0 {
		return t.chipTypes, nil
	}
	data, err := t.command(ctx, []byte{0xC9, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, generalTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get chip types: %w", err)
	}
	if data[1] != 0x00 {
		return nil, fmt.Errorf("invalid GetChipTypes response: %X", data)
	}
	t.chipTypes = data[2:]
	return t.chipTypes, nil
}

// ReadMemoryByAddress reads the 6 bytes at [address-5 .. address].
// NOTE per MyBooty: reading outside the valid FLASH address range makes the
// ECU restart, so callers must keep address within [start+5, 0x7FFFF].
func (t *Client) ReadMemoryByAddress(ctx context.Context, address uint32) ([]byte, error) {
	p := []byte{0xC7, byte(address >> 24), byte(address >> 16), byte(address >> 8), byte(address), 0x00, 0x00, 0x00}
	data, err := t.command(ctx, p, generalTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory by address: %w", err)
	}
	if data[1] != 0x00 {
		return nil, fmt.Errorf("read memory by address failed: %X", data)
	}
	out := make([]byte, 6)
	copy(out, data[2:8])
	reverse(out)
	return out, nil
}

func reverse(s []byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (t *Client) readMemoryRetry(ctx context.Context, address uint32) ([]byte, error) {
	return retry.DoWithData(
		func() ([]byte, error) {
			return t.ReadMemoryByAddress(ctx, address)
		},
		retry.Context(ctx),
		retry.Attempts(3),
		retry.LastErrorOnly(true),
	)
}

// readFlash reads length (>= 6) bytes of flash starting at address with C7
// reads. The final read is aligned to the end of the range so no read ever
// goes past address+length-1.
func (t *Client) readFlash(ctx context.Context, address uint32, length int) ([]byte, error) {
	out := make([]byte, length)
	for i := 0; i < length; i += 6 {
		offset := i
		if offset+6 > length {
			offset = length - 6
		}
		b, err := t.readMemoryRetry(ctx, address+uint32(offset)+5)
		if err != nil {
			return nil, err
		}
		copy(out[offset:], b)
	}
	return out, nil
}
