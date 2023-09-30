package trionic

import (
	"bytes"
	"errors"

	"github.com/roffe/txlogger/pkg/symbol"
)

func ValidateTrionic7File(data []byte) error {
	if len(data) != 0x80000 {
		return symbol.ErrInvalidLength
	}
	if !bytes.HasPrefix(data, []byte{0xFF, 0xFF, 0xEF, 0xFC}) {
		return symbol.ErrInvalidTrionic7File
	}
	return nil
}

func NewT7(fileBytes []byte) (symbol.ECUBinary, error) {
	bin := &T7Binary{
		data:    fileBytes,
		symbols: make([]*symbol.Symbol, 0),
	}

	if err := bin.parse(); err != nil {
		return nil, err
	}

	return bin, nil
}

type T7Binary struct {
	data    []byte
	symbols []*symbol.Symbol
}

func (t7 *T7Binary) Bytes() []byte {
	return t7.data
}

func (t7 *T7Binary) Symbols() []*symbol.Symbol {
	return t7.symbols
}

// --- T7Binary private methods ---

func (t7 *T7Binary) parse() error {
	packed, err := t7.isBinaryPacked()
	if err != nil {
		return err
	}
	if !packed {
		return errors.New("non binarypacked not implemented")
	}

	//return BinaryPacked(t7.data)
	return nil
}

func (t7 *T7Binary) isBinaryPacked() (bool, error) {
	length, retval, _, err := t7.readMarkerAddressContent(0x9B)
	if err != nil {
		return false, err
	}
	if retval > 0 && length < len(t7.data) && length > 0 {
		return true, nil
	}
	return false, nil
}

func (t7 *T7Binary) readMarkerAddressContent(value byte) (int, int, int, error) {
	var retval int
	var length int
	var val int

	fileoffset := len(t7.data) - 0x201
	inb := t7.data[len(t7.data)-0x201:]
	//if len(inb) != 0x90 {
	//	return 0, 0, 0, fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", len(inb), 0x90)
	//}
	for t := 0; t < 0x90; t++ {
		if inb[t] == value && inb[t+1] < 0x30 {
			// Marker found, read 6 bytes
			retval = fileoffset + t // 0x07FF70 + t
			length = int(inb[t+1])
			break
		}
	}
	pos := retval - length
	info := t7.data[pos : pos+length]
	//if len(info) != length {
	//	return 0, 0, 0, fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", len(info), length)
	//}
	for bc := 0; bc < length; bc++ {
		val <<= 8
		val |= int(info[bc])
	}
	return length, retval, val, nil
}
