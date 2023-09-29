package symbol

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/roffe/txlogger/pkg/blowfish"
)

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

func LoadSymbols(filename string, ecu string, cb func(string)) ([]*Symbol, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	switch ecu {
	case "T7":
		if err := ValidateTrionic7File(data); err == nil {
			return LoadT7Symbols(data, cb)
		} else {
			return nil, err
		}
	case "T8":
		if err := ValidateTrionic8File(data); err == nil {
			return LoadT8Symbols(data, cb)
		} else {
			return nil, err
		}
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
