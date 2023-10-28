package symbol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/roffe/txlogger/pkg/blowfish"
	"github.com/roffe/txlogger/pkg/lzhuf"
)

type Number interface {
	uint8 | int8 | uint16 | int16 | uint32 | int32 | uint64 | int64 | float64
}

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

func GetValue[V Number](sym *Symbol) V {
	return sym.Decode().(V)
}

func (s *Symbol) Decode() interface{} {
	switch {
	case s.Length == 1:
		if len(s.data) != 1 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return s.Int8()
		}
		return s.Uint8()
	case s.Length == 2:
		if len(s.data) != 2 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return s.Int16()
		}
		return s.Uint16()
	case s.Length == 4:
		if len(s.data) != 4 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return s.Int32()
		}
		return s.Uint32()
	case s.Length == 8:
		if len(s.data) != 8 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return s.Int64()
		}
		return s.Uint64()
	default:
		return -1
	}
}

func (s *Symbol) Read(r io.Reader) error {
	symbolData := make([]byte, s.Length)
	n, err := r.Read(symbolData)
	if err != nil {
		return fmt.Errorf("Symbol failed to Read: %w", err)
	}
	if n != int(s.Length) {
		return fmt.Errorf("Symbol expected %d bytes, got %d", s.Length, n)
	}
	s.data = symbolData
	return nil
}

func (s *Symbol) Bytes() []byte {
	return s.data
}

func (s *Symbol) String() string {
	return fmt.Sprintf("%s #%d @%08X type: %02X len: %d", s.Name, s.Number, s.Address, s.Type, s.Length)
}

func (s *Symbol) StringValue() string {
	if s.Correctionfactor != 1 {
		var result float64
		switch t := s.Decode().(type) {
		case int8:
			result = float64(t) * s.Correctionfactor
		case uint8:
			result = float64(t) * s.Correctionfactor
		case int16:
			result = float64(t) * s.Correctionfactor
		case uint16:
			result = float64(t) * s.Correctionfactor
		case int32:
			result = float64(t) * s.Correctionfactor
		case uint32:
			result = float64(t) * s.Correctionfactor
		}

		//var format string
		var precission int
		switch {
		case s.Correctionfactor == 0.1:
			precission = 1
		//	format = "%.1f"
		case s.Correctionfactor == 0.01:
			//	format = "%.2f"
			precission = 2
		case s.Correctionfactor == 0.001:
			//	format = "%.3f"
			precission = 3
		}
		return strconv.FormatFloat(result, 'f', precission, 64)
		//return fmt.Sprintf(format, roundFloat(result, precission))

	}
	return fmt.Sprintf("%v", s.Decode())
}

func (s *Symbol) Bool() bool {
	return s.data[0] == 1
}

func (s *Symbol) Uint8() uint8 {
	return uint8(s.data[0])
}

func (s *Symbol) Int8() int8 {
	return int8(s.data[0])
}

func (s *Symbol) Uint16() uint16 {
	return binary.BigEndian.Uint16(s.data)
}

func (s *Symbol) Int16() int16 {
	return int16(binary.BigEndian.Uint16(s.data))
}

func (s *Symbol) Uint32() uint32 {
	return binary.BigEndian.Uint32(s.data)
}

func (s *Symbol) Int32() int32 {
	return int32(binary.BigEndian.Uint32(s.data))
}

func (s *Symbol) Uint64() uint64 {
	return binary.BigEndian.Uint64(s.data)
}

func (s *Symbol) Int64() int64 {
	return int64(binary.BigEndian.Uint64(s.data))
}

func (s *Symbol) Float64() float64 {
	switch {
	case s.Length == 1:
		if len(s.data) != 1 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return float64(s.Int8()) * s.Correctionfactor
		}
		return float64(s.Uint8()) * s.Correctionfactor
	case s.Length == 2:
		if len(s.data) != 2 {
			return -1
		}
		if s.Type&SIGNED != 0 {

			return float64(s.Int16()) * s.Correctionfactor
		}
		return float64(s.Uint16()) * s.Correctionfactor
	case s.Length == 4:
		if len(s.data) != 4 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return float64(s.Int32()) * s.Correctionfactor
		}
		return float64(s.Uint32()) * s.Correctionfactor
	case s.Length == 8:
		if len(s.data) != 8 {
			return -1
		}
		if s.Type&SIGNED != 0 {
			return float64(s.Int64()) * s.Correctionfactor
		}
		return float64(s.Uint64()) * s.Correctionfactor
	default:
		return 0.0
	}
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

	expandedFileSize := int(in[0]) | (int(in[1]) << 8) | (int(in[2]) << 16) | (int(in[3]) << 24)

	if expandedFileSize == -1 {
		return nil, errors.New("invalid expanded file size")
	}

	out := make([]byte, expandedFileSize)

	returnedSize := lzhuf.Decode(in, out)

	if returnedSize != expandedFileSize {
		return nil, fmt.Errorf("decoded data size missmatch: %d != %d", returnedSize, expandedFileSize)
	}

	return strings.Split(strings.TrimSuffix(string(out), "\r\n"), "\r\n"), nil
}

/*
func bytePatternSearch2(data []byte, search []byte, startOffset int64) int {
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
*/

// Knuth-Morris-Pratt (KMP) algorithm
func bytePatternSearch(data []byte, search []byte, startOffset int64) int {
	if startOffset < 0 || startOffset >= int64(len(data)) || len(search) == 0 {
		return -1
	}

	lps := computeLPSArray(search)

	i, j := startOffset, 0

	for i < int64(len(data)) {
		if search[j] == data[i] {
			i++
			j++
		}

		if j == len(search) {
			return int(i) - j
		} else if i < int64(len(data)) && search[j] != data[i] {
			if j != 0 {
				j = lps[j-1]
			} else {
				i++
			}
		}
	}

	return -1
}

func computeLPSArray(pattern []byte) []int {
	length := 0
	lps := make([]int, len(pattern))
	lps[0] = 0
	i := 1

	for i < len(pattern) {
		if pattern[i] == pattern[length] {
			length++
			lps[i] = length
			i++
		} else {
			if length != 0 {
				length = lps[length-1]
			} else {
				lps[i] = 0
				i++
			}
		}
	}

	return lps
}
