package ecu

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
)

func GetSymbolsT5(ctx context.Context, dev gocan.Adapter, cb func(string)) (*symbol.Collection, error) {
	cl, err := gocan.OpenAdapter(ctx, dev)
	if err != nil {
		return nil, err
	}
	defer cl.Close()

	var symbols []*symbol.Symbol

	start := time.Now()
	if err := sendCommand(ctx, cl, []byte{'S', 0x0D}); err != nil {
		return nil, err
	}
	cb("Connected to ECU")
	cb("Downloading symbol table")

	data, err := recvDataEND(ctx, cl)
	if err != nil {
		return nil, err
	}
	sym_count := 0
	var swVersion string
	for n, line := range bytes.Split(bytes.TrimSuffix(data, []byte{0x0D, 0x0A}), []byte{0x0D, 0x0A}) {
		if n == 0 {
			swVersion = string(bytes.TrimPrefix(line, []byte("100100")))
			continue
		}
		addr, err := hex.DecodeString(string(line[0:4]))
		if err != nil {
			return nil, err
		}
		length, err := hex.DecodeString(string(line[4:8]))
		if err != nil {
			return nil, err
		}
		name := symbol.CString(line[8:])
		symbols = append(symbols, &symbol.Symbol{
			Number:           sym_count,
			SramOffset:       uint32(binary.BigEndian.Uint16(addr)),
			Name:             name,
			Length:           binary.BigEndian.Uint16(length),
			Correctionfactor: symbol.GetCorrectionfactor(name),
		})

		sym_count++
	}

	cb("SW: " + swVersion)

	cb(fmt.Sprintf("Loaded %d symbols from ECU in %s", sym_count, time.Since(start).Round(time.Millisecond).String()))

	return symbol.NewCollection(symbols...), nil
}

func sendCommand(ctx context.Context, c *gocan.Bus, cmd []byte) error {
	for _, b := range cmd {
		rctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		resp, err := c.Request(rctx, gocan.NewFrame(0x05, []byte{0xC4, b}), 0xC)
		cancel()
		if err != nil {
			return fmt.Errorf("send command failed: %w", err)
		}
		if resp.Data[0] != 0xC6 {
			return fmt.Errorf("invalid response")
		}
	}
	return nil
}

func ack(ctx context.Context, c *gocan.Bus) error {
	return c.Send(ctx, gocan.NewFrame(0x05, []byte{0xC6, 0x00}))
}

func recvDataEND(ctx context.Context, c *gocan.Bus) ([]byte, error) {
	pattern := []byte{'E', 'N', 'D', 0x0D, 0x0A}
	buff := bytes.NewBuffer(nil)
	defer fmt.Println()
	dd := 0
	for {
		if dd == 1024 {
			fmt.Print(".")
			dd = 0
		}
		if err := ack(ctx, c); err != nil {
			return nil, err
		}

		rctx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		resp, err := c.Recv(rctx, 0xC)
		cancel()
		if err != nil {
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
