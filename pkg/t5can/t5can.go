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
	for off := uint32(0); off < length; off += chunk {
		// Read up to 6 bytes at a time
		resp, err := c.sendReadCommand(ctx, address+addrBias+off)
		if err != nil {
			return nil, err
		}
		n := min(int(length-off), chunk)
		copy(buff[int(off):int(off)+n], resp[:n])
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

func (c *Client) WriteRam(ctx context.Context, address uint32, data byte) error {
	command := fmt.Sprintf("W%04X%02X\r", uint16(address), data)
	if err := sendCommand(ctx, c.c, []byte(command)); err != nil {
		return err
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
