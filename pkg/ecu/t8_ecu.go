package ecu

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
)

func CalculateT8AccessKey(seed []byte, level byte) (byte, byte) {
	val := int(seed[0])<<8 | int(seed[1])

	key := func(seed int) int {
		key := seed>>5 | seed<<11
		return (key + 0xB988) & 0xFFFF
	}(val)

	switch level {
	case 0xFB:
		key ^= 0x8749
		key += 0x06D3
		key ^= 0xCFDF
	case 0xFD:
		key /= 3
		key ^= 0x8749
		key += 0x0ACF
		key ^= 0x81BF
	}

	return (byte)((key >> 8) & 0xFF), (byte)(key & 0xFF)
}

func GetSymbolsT8(ctx context.Context, dev gocan.Adapter, cb func(string)) (symbol.SymbolCollection, error) {
	cl, err := gocan.New(context.TODO(), dev)
	if err != nil {
		return nil, err
	}
	defer cl.Close()

	//var symbols []*symbol.Symbol

	gm := gmlan.New(cl, 0x7e0, 0x7e8)

	if err := gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
		return nil, err
	}

	cb("Connected to ECU")

	defer gm.ReturnToNormalMode(ctx)

	gm.TesterPresentNoResponseAllowed()

	time.Sleep(25 * time.Millisecond)

	if err := gm.RequestSecurityAccess(ctx, 0xFD, 0, CalculateT8AccessKey); err != nil {
		return nil, err
	}

	noSymbols, crc, name, compressed, err := ReadSymbolChecksum(ctx, gm)
	if err != nil {
		return nil, err
	}

	cb(fmt.Sprintf("Number of symbols: %d", noSymbols))
	cb(fmt.Sprintf("CRC: %04X", crc))
	cb(fmt.Sprintf("Filename: %s", removeNullTerminators(name)))
	cb(fmt.Sprintf("Compressed: %v", compressed))

	cb("Downloading symbol table (this will take a while)")
	start := time.Now()
	symbols, err := ReadSymbolTable(ctx, gm)
	if err != nil {
		return nil, err
	}
	cb(fmt.Sprintf("Downloaded %d symbols in %s", len(symbols), time.Since(start).String()))

	start = time.Now()

	frame := gocan.NewFrame(0x7e0, []byte{0x05, gmlan.READ_DATA_BY_IDENTIFIER, 0x14, 0x19, 0x00, 0x00}, gocan.ResponseRequired)
	if _, err := gm.ReadDataByIdentifierFrame(ctx, frame); err != nil {
		return nil, err
	}

	gm.TesterPresentNoResponseAllowed()

	ll := 0
	cb("Downloading symbol names (this will take a while)")
	buff := bytes.NewBuffer(nil)
	for {
		//fmt.Print(".")
		data, err := gm.ReadDataByIdentifier(ctx, 0x13)
		if err != nil {
			if err.Error() == "Unknown - Unknown errror" {
				break
			}
			return nil, err
		}
		if len(data) == 3 {
			break
		}
		buff.Write(data[3:])
		ll++
		if ll == 15 {
			gm.TesterPresentNoResponseAllowed()
			ll = 0
		}
	}
	//fmt.Println()
	//cb(fmt.Sprintf("Downloaded: %d bytes in %s", buff.Len(), time.Since(start).String()))

	// os.WriteFile("t8_names.bin", buff.Bytes(), 0644)

	symbolNames, err := symbol.ExpandCompressedSymbolNames(buff.Bytes())
	if err != nil {
		return nil, err
	}

	for i, sym := range symbols {
		sym.Name = symbolNames[i]
		sym.Unit = symbol.GetUnit(symbols[i].Name)
		sym.Correctionfactor = symbol.GetCorrectionfactor(symbols[i].Name)
	}

	cb(fmt.Sprintf("Downloaded %d symbol names in %s", len(symbolNames), time.Since(start).Round(time.Millisecond).String()))

	return symbol.NewCollection(symbols...), nil
}

func removeNullTerminators(str string) string {
	return strings.ReplaceAll(str, "\x00", "") // Replace null terminators with empty string
}

func ReadSymbolTable(ctx context.Context, gm *gmlan.Client) ([]*symbol.Symbol, error) {
	var err error
	_, err = gm.ReadDataByIdentifier(ctx, 0x12)
	if err != nil {
		return nil, err
	}

	buff := bytes.NewBuffer(nil)
	v := 0
	for {
		//fmt.Print(".")
		data2, err := gm.ReadDataByIdentifier(ctx, 0x13)
		if err != nil {
			return nil, err
		}
		if len(data2) == 3 {
			break
		}
		buff.Write(data2[3:])
		v++
		if v > 15 {
			if err := gm.TesterPresentResponseRequired(ctx); err != nil {
				return nil, err
			}
			v = 0
		}
	}
	//fmt.Println()

	//os.WriteFile("t8_symbols.bin", buff.Bytes(), 0644)

	var symbols []*symbol.Symbol

	symbol_count := 0

	for buff.Len() > 0 {
		var address uint32
		if err := binary.Read(buff, binary.BigEndian, &address); err != nil {
			return nil, err
		}
		var length uint16
		if err := binary.Read(buff, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		stype, err := buff.ReadByte()
		if err != nil {
			return nil, err
		}
		_, err = buff.ReadByte()
		if err != nil {
			return nil, err
		}

		sym := &symbol.Symbol{
			Name:    fmt.Sprintf("Symbol-%d", symbol_count),
			Number:  symbol_count,
			Address: address,
			Length:  length,
			Type:    stype,
		}

		//log.Println(sym.String())

		symbols = append(symbols, sym)
		symbol_count++
	}

	//	log.Printf("Symbol count: %d", symbol_count)

	return symbols, nil
}

func ReadSymbolChecksum(ctx context.Context, gm *gmlan.Client) (uint16, uint32, string, bool, error) {
	ddd, err := gm.ReadDataByIdentifier(ctx, 0x11)
	if err != nil {
		return 0, 0, "", false, err
	}

	r := bytes.NewReader(ddd)

	var dataLen uint8

	if err := binary.Read(r, binary.BigEndian, &dataLen); err != nil {
		return 0, 0, "", false, err
	}

	//log.Printf("Data length: %d", dataLen)

	var noSymbols uint16

	if err := binary.Read(r, binary.BigEndian, &noSymbols); err != nil {
		return 0, 0, "", false, err
	}
	//	log.Printf("Number of symbols: %d", noSymbols)

	var crc uint32
	if err := binary.Read(r, binary.BigEndian, &crc); err != nil {
		return 0, 0, "", false, err
	}
	//	log.Printf("CRC: %X", crc)

	filename := make([]byte, 30)
	n, err := r.Read(filename)
	if err != nil {
		return 0, 0, "", false, err
	}
	if n != 30 {
		return 0, 0, "", false, fmt.Errorf("expected 30 bytes, got %d", n)
	}
	//	log.Printf("Filename: %s", filename)

	r.ReadByte() // skip terminator

	compressed, err := r.ReadByte()
	if err != nil {
		return 0, 0, "", false, err
	}
	//	log.Printf("Compressed: %v", compressed == 0x03)

	return noSymbols, crc, string(filename), compressed == 0x03, nil
}
