package t5can

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
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
	buff := make([]byte, length)
	realAddress := address + 5
	num := length / 6
	if length%6 > 0 {
		num++
	}
	for i := 0; i < int(num); i++ {
		resp, err := c.sendReadCommand(ctx, realAddress)
		if err != nil {
			return nil, err
		}
		for j := 0; j < 6; j++ {
			if (i*6)+j < int(length) {
				buff[(i*6)+j] = resp[j]
			}
		}
		realAddress += 6
	}
	return buff, nil
}

func (c *Client) WriteRam(ctx context.Context, address uint32, data byte) error {
	command := fmt.Sprintf("W%04X%02X\r", uint16(address), data)
	if err := sendCommand(ctx, c.c, []byte(command)); err != nil {
		return err
	}
	return nil
}

func (c *Client) sendReadCommand(ctx context.Context, address uint32) ([]byte, error) {
	frame := gocan.NewFrame(0x05, []byte{0xC7, byte(address >> 24), byte(address >> 16), byte(address >> 8), byte(address)}, gocan.ResponseRequired)
	resp, err := c.c.SendAndWait(ctx, frame, 200*time.Millisecond, 0xC)
	if err != nil {
		return nil, err
	}
	if resp.Data[0] != 0xC7 {
		return nil, fmt.Errorf("invalid response")
	}
	respData := resp.Data[2:]
	for j := 0; j < 3; j++ {
		respData[j], respData[5-j] = respData[5-j], respData[j]
	}
	return respData, nil
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
