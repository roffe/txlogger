package t5can

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"time"

	"github.com/roffe/gocan"
)

const (
	canID           = 0x05
	replyID         = 0x0C
	cmdSetAddr byte = 0xA5
	maxBlock        = 133 // protocol cap per address window
	chunkSize       = 7   // bytes per data frame (payload[1:] = 7 bytes)
	respOK     byte = 0x00
)

type Client struct {
	c *gocan.Client

	defaultTimeout time.Duration
}

func NewClient(c *gocan.Client) *Client {
	return &Client{
		c:              c,
		defaultTimeout: 200 * time.Millisecond,
	}
}

func (c *Client) ReadRam(ctx context.Context, address, length uint32) ([]byte, error) {
	const chunk = 6
	const addrBias = 5

	buff := make([]byte, length)
	for offset := uint32(0); offset < length; offset += chunk {
		// Read up to 6 bytes at a time
		resp, err := c.sendReadCommand(ctx, address+addrBias+offset)
		if err != nil {
			return nil, err
		}
		n := min(int(length-offset), chunk)
		copy(buff[int(offset):int(offset)+n], resp[:n])
	}
	return buff, nil
}

func (c *Client) sendReadCommand(ctx context.Context, address uint32) ([]byte, error) {
	const cmdByte = 0xC7
	frame := gocan.NewFrame(0x05, []byte{cmdByte, 0x00, 0x00, byte(address >> 8), byte(address)}, gocan.ResponseRequired)
	resp, err := c.c.SendAndWait(ctx, frame, 200*time.Millisecond, 0x0C)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) < 8 { // need at least cmd + ? + 6 data bytes
		return nil, fmt.Errorf("short response: got %d bytes", len(resp.Data))
	}
	if resp.Data[0] != cmdByte {
		return nil, fmt.Errorf("invalid response: expected 0x%X, got 0x%X", cmdByte, resp.Data[0])
	}

	data := append([]byte(nil), resp.Data[2:8]...) // copy slice
	slices.Reverse(data)
	return data, nil
}

func (c *Client) WriteRam(ctx context.Context, address uint32, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Fast/legacy path: single byte via ASCII command supports only 16-bit addresses.
	if len(data) == 1 && address <= 0xFFFF {
		cmd := fmt.Sprintf("W%04X%02X\r", uint16(address), data[0])
		return sendCommand(ctx, c.c, []byte(cmd))
	}

	// Helper to send a single 0..maxBlock block starting at `addr`

	// Break the user buffer into ≤133-byte blocks; device auto-advances from the
	// base address we set for each block. We also advance `address` accordingly.
	left := len(data)
	start := 0
	for left > 0 {
		n := maxBlock
		if left < n {
			n = left
		}
		if err := c.sendBlock(ctx, address, data[start:start+n], maxBlock); err != nil {
			return err
		}
		start += n
		left -= n
		address += uint32(n)
	}

	return nil
}

func (c *Client) sendBlock(ctx context.Context, addr uint32, block []byte, maxBlock int) error {
	if len(block) == 0 || len(block) > maxBlock {
		return fmt.Errorf("invalid block size %d (max %d)", len(block), maxBlock)
	}

	// 1) Set address
	addrFrame := gocan.NewFrame(canID,
		[]byte{
			cmdSetAddr,
			byte(addr >> 24), byte(addr >> 16), byte(addr >> 8), byte(addr),
			byte(len(block)),
			0x00, 0x00,
		},
		gocan.ResponseRequired,
	)
	addrResp, err := c.c.SendAndWait(ctx, addrFrame, 200*time.Millisecond, replyID)
	if err != nil {
		return fmt.Errorf("set-address send failed: %w", err)
	}
	if len(addrResp.Data) < 2 {
		return fmt.Errorf("set-address short response: %d bytes", len(addrResp.Data))
	}
	if addrResp.Data[1] != respOK {
		return fmt.Errorf("set-address NACK: 0x%02X", addrResp.Data[1])
	}

	// 2) Stream data in 7-byte chunks
	bytesLeft := len(block)
	offset := 0

	// payload[0] = offset (device expects this)
	// payload[1:8] = up to 7 bytes of data
	var payload [8]byte

	for bytesLeft > 0 {
		n := chunkSize
		if bytesLeft < n {
			n = bytesLeft
		}

		// prepare payload
		payload[0] = byte(offset)
		// zero only the used area to be tidy (var payload zeros full array once anyway)
		for i := 1; i < 8; i++ {
			payload[i] = 0
		}
		copy(payload[1:], block[offset:offset+n])

		dataFrame := gocan.NewFrame(canID, payload[:], gocan.ResponseRequired)
		dataResp, err := c.c.SendAndWait(ctx, dataFrame, 200*time.Millisecond, replyID)
		if err != nil {
			return fmt.Errorf("data send failed at offset %d: %w", offset, err)
		}
		if len(dataResp.Data) < 2 {
			return fmt.Errorf("data short response at offset %d: %d bytes", offset, len(dataResp.Data))
		}
		if dataResp.Data[1] != respOK {
			return fmt.Errorf("data NACK at offset %d: 0x%02X", offset, dataResp.Data[1])
		}

		offset += n
		bytesLeft -= n
	}
	return nil
}

func (c *Client) WriteRam2(ctx context.Context, address uint32, data []byte) error {
	if len(data) == 1 {
		return c.writeRamSingle(ctx, address, data[0])
	}

	left := len(data)
	for left > 0 {
		chunkSize := min(left, 133)
		if err := c.writeRamMulti(ctx, address, data[len(data)-left:len(data)-left+chunkSize]); err != nil {
			return err
		}
		left -= chunkSize
		address += uint32(chunkSize)
	}
	return nil
}

func (c *Client) writeRamSingle(ctx context.Context, address uint32, data byte) error {
	command := fmt.Sprintf("W%04X%02X\r", uint16(address), data)
	if err := sendCommand(ctx, c.c, []byte(command)); err != nil {
		return err
	}
	return nil
}

// we can only write up to 133 bytes at a time ( 0x7E)
func (c *Client) writeRamMulti(ctx context.Context, address uint32, data []byte) error {
	if len(data) > 133 {
		return fmt.Errorf("data too long")
	}
	addressFrame := gocan.NewFrame(0x05, []byte{0xA5, byte(address >> 24), byte(address >> 16), byte(address >> 8), byte(address), byte(len(data)), 0x00, 0x00}, gocan.ResponseRequired)
	addressResp, err := c.c.SendAndWait(ctx, addressFrame, 200*time.Millisecond, 0x0C)
	if err != nil {
		return fmt.Errorf("failed to send address command: %v", err)
	}
	if addressResp.Data[1] != 0x00 {
		return fmt.Errorf("failed to set address %02X", addressResp.Data[1])
	}
	bytesLeft := len(data)
	offset := 0
	payload := make([]byte, 8)
	for bytesLeft > 0 {
		chunkSize := min(bytesLeft, 7)
		payload[0] = byte(offset)
		payload[1] = 0x00
		payload[2] = 0x00
		payload[3] = 0x00
		payload[4] = 0x00
		payload[5] = 0x00
		payload[6] = 0x00
		payload[7] = 0x00
		copy(payload[1:], data[offset:offset+chunkSize])
		dataFrame := gocan.NewFrame(0x05, payload, gocan.ResponseRequired)
		dataResp, err := c.c.SendAndWait(ctx, dataFrame, 200*time.Millisecond, 0x0C)
		if err != nil {
			return fmt.Errorf("failed to send data command: %v", err)
		}
		if dataResp.Data[1] != 0x00 {
			return fmt.Errorf("failed to write data")
		}
		bytesLeft -= chunkSize
		offset += chunkSize
	}
	return nil
}

func sendCommand(ctx context.Context, c *gocan.Client, cmd []byte) error {
	for _, b := range cmd {
		frame := gocan.NewFrame(0x05, []byte{0xC4, b}, gocan.ResponseRequired)
		resp, err := c.SendAndWait(ctx, frame, 1*time.Second, 0xC)
		if err != nil {
			return err
		}
		if resp.Data[0] != 0xC6 {
			return fmt.Errorf("invalid response")
		}
	}
	return nil
}

func ack(c *gocan.Client) error {
	return c.Send(0x05, []byte{0xC6, 0x00}, gocan.Outgoing)
}

func recvDataEND(ctx context.Context, c *gocan.Client) ([]byte, error) {
	pattern := []byte{'E', 'N', 'D', 0x0D, 0x0A}
	buff := bytes.NewBuffer(nil)
	defer fmt.Println()
	dd := 0
	for {
		if dd == 1024 {
			fmt.Print(".")
			dd = 0
		}
		ack(c)
		resp, err := c.Wait(ctx, 40*time.Millisecond, 0xC)
		if err != nil {
			os.WriteFile("dump", buff.Bytes(), 0644)
			return nil, err
		}
		if resp.Data[0] != 0xC6 && resp.Data[1] != 0x00 {
			return nil, fmt.Errorf("invalid response")
		}
		buff.WriteByte(resp.Data[2])
		if bytes.HasSuffix(buff.Bytes(), pattern) {
			return bytes.TrimSuffix(buff.Bytes(), pattern), nil
		}
		dd++
	}
}

func recvData(ctx context.Context, c *gocan.Client) ([]byte, error) {
	var lastByte byte
	buff := bytes.NewBuffer(nil)
	for {
		ack(c)
		resp, err := c.Wait(ctx, 75*time.Millisecond, 0xC)
		if err != nil {
			log.Printf("%s", buff.Bytes())
			return nil, err
		}
		if resp.Data[0] != 0xC6 && resp.Data[1] != 0x00 {
			return nil, fmt.Errorf("invalid response")
		}
		if lastByte == 0x0D && resp.Data[2] == 0x0A {
			return bytes.TrimSuffix(buff.Bytes(), []byte{0x0D}), nil
		}
		buff.WriteByte(resp.Data[2])
		lastByte = resp.Data[2]
	}
}
