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
	"syscall"
	"unsafe"

	"github.com/roffe/txlogger/pkg/blowfish"
)

type ECUBinary interface {
	Bytes() []byte
	Symbols() []*Symbol
}

type Symbol struct {
	Name string

	Number int

	Address uint32
	Length  uint16
	Mask    uint16

	Type         uint8
	ExtendedType uint8

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
	if !signed && (s.Length == 22 || s.Length == 30) {
		return s.DataToUint16()
	}
	if signed && (s.Length == 22 || s.Length == 30) {
		return s.DataToInt16()
	}

	return s.DataToUint8()
}

func (s *Symbol) DataToUint16() []int {
	r := bytes.NewReader(s.data)
	var values []int
	for r.Len() > 0 {
		var v uint16
		err := binary.Read(r, binary.BigEndian, &v)
		if err != nil {
			log.Fatalf("error reading symbol data: %v", err)
		}
		values = append(values, int(v))
	}
	return values
}

func (s *Symbol) DataToInt8() []int {
	r := bytes.NewReader(s.data)
	var values []int
	for r.Len() > 0 {
		var v int8
		err := binary.Read(r, binary.BigEndian, &v)
		if err != nil {
			log.Panicf("error reading symbol data: %v", err)
		}
		values = append(values, int(v))
	}
	return values
}

func (s *Symbol) DataToUint8() []int {
	r := bytes.NewReader(s.data)
	var values []int
	for r.Len() > 0 {
		var v uint8
		err := binary.Read(r, binary.BigEndian, &v)
		if err != nil {
			log.Panicf("error reading symbol data: %v", err)
		}
		values = append(values, int(v))
	}
	return values
}

func (s *Symbol) DataToInt16() []int {
	r := bytes.NewReader(s.data)
	var values []int
	for r.Len() > 0 {
		var v int16
		err := binary.Read(r, binary.BigEndian, &v)
		if err != nil {
			log.Panicf("error reading symbol data: %v", err)
		}
		values = append(values, int(float64(v)*s.Correctionfactor))
	}
	return values
}

func (s *Symbol) DataToByte() []byte {
	r := bytes.NewReader(s.data)
	var values []byte
	for r.Len() > 0 {
		var v byte
		err := binary.Read(r, binary.BigEndian, &v)
		if err != nil {
			log.Fatalf("error reading symbol data: %v", err)
		}
		values = append(values, v)
	}
	return values
}

func LoadSymbols(filename string, ecu string, cb func(string)) (SymbolCollection, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	cb(fmt.Sprintf("Loading %s", filepath.Base(filename)))

	switch ecu {
	case "T7":
		return LoadT7Symbols(data, cb)
	case "T8":
		return LoadT8Symbols(data, cb)
	default:
		return nil, fmt.Errorf("unknown ECU type: %s", ecu)
	}

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

	path, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dll, err := syscall.LoadDLL(path + `\lzhuf.dll`)
	if err != nil {
		return nil, err
	}
	defer dll.Release()

	decode, err := dll.FindProc("Decode")
	if err != nil {
		log.Println(err)
		return nil, fmt.Errorf("error finding Decode in lzhuf.dll: %w", err)
	}

	r0, r1, err := decode.Call(uintptr(unsafe.Pointer(&in[0])), uintptr(unsafe.Pointer(&out[0])))
	if r1 == 0 {
		if err != nil {
			return nil, fmt.Errorf("error decoding compressed symbol table: %w", err)
		}
	}

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
