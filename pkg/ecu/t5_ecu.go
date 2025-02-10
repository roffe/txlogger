package ecu

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
)

func GetSymbolsT5(ctx context.Context, dev gocan.Adapter, cb func(string)) (symbol.SymbolCollection, error) {
	cl, err := gocan.NewClient(context.TODO(), dev)
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

func sendCommand(ctx context.Context, c *gocan.Client, cmd []byte) error {
	for _, b := range cmd {
		frame := gocan.NewFrame(0x05, []byte{0xC4, b}, gocan.ResponseRequired)
		resp, err := c.SendAndWait(ctx, frame, 100*time.Millisecond, 0xC)
		if err != nil {
			return err
		}
		if resp.Data()[0] != 0xC6 {
			return fmt.Errorf("invalid response")
		}
	}
	return nil
}

func ack(c *gocan.Client) error {
	frame := gocan.NewFrame(0x05, []byte{0xC6, 0x00}, gocan.Outgoing)
	return c.Send(frame)
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
		data := resp.Data()
		if data[0] != 0xC6 && data[1] != 0x00 {
			return nil, fmt.Errorf("invalid response")
		}
		buff.WriteByte(data[2])
		if bytes.HasSuffix(buff.Bytes(), pattern) {
			return bytes.TrimSuffix(buff.Bytes(), pattern), nil
		}
		dd++
	}
}
