package symbol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/roffe/txlogger/pkg/blowfish"
	"github.com/roffe/txlogger/pkg/lzhuf"
)

type ECUType int

const (
	ECU_T7 ECUType = iota // T7
	ECU_T8                // T8
)

func (e ECUType) String() string {
	switch e {
	case ECU_T7:
		return "T7"
	case ECU_T8:
		return "T8"
	default:
		return "Unknown"
	}
}

type ECUBinary interface {
	Bytes() []byte
	Symbols() []*Symbol
}

type Symbol struct {
	Name             string
	Number           int
	Address          uint32
	Length           uint16
	Mask             uint16
	Type             uint8
	ExtendedType     uint8
	Correctionfactor float64
	Unit             string

	data []byte
}

func (s *Symbol) Bytes() []byte {
	return s.data
}

func (s *Symbol) String() string {
	return fmt.Sprintf("%s #%d @%08X type: %02X len: %d", s.Name, s.Number, s.Address, s.Type, s.Length)
}

func (s *Symbol) IntFromData() []int {
	signed := s.Type&SIGNED == 1
	konst := s.Type&KONST == KONST
	char := s.Type&CHAR == CHAR

	log.Printf("IntFromData %s signed: %t konst: %t chat: %t len: %d: type %X", s.Name, signed, konst, char, s.Length, s.Type)

	if konst && char {
		return s.DataToUint8()
	}

	if s.Name == "VIOSMAFCal.Q_AirInletTab2" {
		return s.DataToUint16()
	}

	if s.Name == "BstKnkCal.MaxAirmass" {
		return s.DataToInt16()
	}

	/*
		if yLen*xLen == int(s.Length) {
			if signed {
				return s.DataToInt8()
			}
			return s.DataToUint8()
		}

		if yLen*xLen*2 == int(s.Length/2) {
			if signed {
				return s.DataToInt16()
			}
			return s.DataToUint16()
		}
	*/
	if !signed && s.Length == 1 {
		return s.DataToUint8()
	}
	if signed && s.Length == 1 {
		return s.DataToInt8()
	}
	if !signed && s.Length == 2 {
		return s.DataToUint16()
	}
	if signed && s.Length == 2 {
		return s.DataToInt16()
	}
	if !signed && (s.Length == 22 || s.Length == 30 || s.Length == 36) {
		return s.DataToUint16()
	}
	if signed && (s.Length == 22 || s.Length == 30 || s.Length == 36) {
		return s.DataToInt16()
	}
	if !signed {
		return s.DataToUint16()
	}
	return s.DataToInt16()
}

func (s *Symbol) DataToInt8() []int {
	values := make([]int, len(s.data))
	for i, b := range s.data {
		values[i] = int(int8(b))
	}
	return values
}

func (s *Symbol) DataToUint8() []int {
	values := make([]int, len(s.data))
	for i, b := range s.data {
		values[i] = int(b)
	}
	return values
}

func (s *Symbol) DataToUint16() []int {
	if len(s.data)%2 != 0 {
		log.Panicf("data length is not even: %d", len(s.data))
	}

	count := len(s.data) / 2
	values := make([]int, count)

	for i := 0; i < count; i++ {
		value := binary.BigEndian.Uint16(s.data[i*2 : i*2+2])
		values[i] = int(value)
	}

	return values
}

func (s *Symbol) DataToInt16() []int {
	if len(s.data)%2 != 0 {
		log.Panicf("data length is not even: %d", len(s.data))
	}

	count := len(s.data) / 2
	values := make([]int, count)

	for i := 0; i < count; i++ {
		value := int16(binary.BigEndian.Uint16(s.data[i*2 : i*2+2]))
		values[i] = int(value)
	}

	return values
}

func LoadSymbols(filename string, cb func(string)) (ECUType, SymbolCollection, error) {
	// check so filename is under 2mb
	fi, err := os.Stat(filename)
	if err != nil {
		return -1, nil, err
	}
	if fi.Size() > 2*1024*1024 {
		return -1, nil, fmt.Errorf("file too large: %d", fi.Size())
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return -1, nil, err
	}

	cb(fmt.Sprintf("Loading %s", filepath.Base(filename)))

	if err := ValidateTrionic7File(data); err == nil {
		sym, err := LoadT7Symbols(data, cb)
		return ECU_T7, sym, err
	}

	if err := ValidateTrionic8File(data); err == nil {
		sym, err := LoadT8Symbols(data, cb)
		return ECU_T8, sym, err
	}

	return -1, nil, fmt.Errorf("unknown file format: %s", filename)
}

func ExpandCompressedSymbolNames(in []byte) ([]string, error) {
	if len(in) < 0x1000 {
		return nil, errors.New("invalid symbol table size")
	}
	//os.WriteFile("compressedSymbolTable.bin", in, 0644)
	if bytes.HasPrefix(in, []byte{0xF1, 0x1A, 0x06, 0x5B, 0xA2, 0x6B, 0xCC, 0x6F}) {
		return blowfish.DecryptSymbolNames(in)
	}
	var expandedFileSize int
	for i := 0; i < 4; i++ {
		expandedFileSize |= int(in[i]) << uint(i*8)
	}

	if expandedFileSize == -1 {
		return nil, errors.New("invalid expanded file size")
	}

	out := make([]byte, expandedFileSize)

	r0 := lzhuf.Decode(in, out)

	if int(r0) != expandedFileSize {
		return nil, fmt.Errorf("decoded data size missmatch: %d != %d", r0, expandedFileSize)
	}

	return strings.Split(strings.TrimSuffix(string(out), "\r\n"), "\r\n"), nil
}

func bytePatternSearch(data []byte, search []byte, startOffset int64) int {
	if startOffset < 0 || startOffset >= int64(len(data)) {
		return -1
	}
	ix := 0
	for i := startOffset; i < int64(len(data)); i++ {
		if search[ix] == data[i] {
			ix++
			if ix == len(search) {
				return int(i - int64(ix) + 1)
			}
		} else {
			ix = 0
		}
	}
	return -1
}
