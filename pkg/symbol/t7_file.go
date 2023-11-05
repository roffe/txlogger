package symbol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log"
)

type T7File struct {
	data    []byte
	Symbols SymbolCollection
}

func NewT7File(data []byte) (*T7File, error) {
	symbols, err := LoadT7Symbols(data, func(s string) {
		log.Println(s)
	})
	if err != nil {
		return nil, err
	}
	return &T7File{
		data:    data,
		Symbols: symbols,
	}, nil
}
func (t7 *T7File) Checksum() uint32 {
	x := findChecksumArea(t7.data)
	return uint32(x)
}

func findChecksumArea(data []byte) int {
	sequence := []byte{
		0x48, 0xE7, 0x00, 0x3C, 0x24, 0x7C, 0x00, 0xF0,
		0x00, 0x00, 0x26, 0x7C, 0x00, 0x00, 0x00, 0x00,
		0x28, 0x7C, 0x00, 0xF0, 0x00, 0x00, 0x2A, 0x7C,
	}
	seqMask := []byte{
		1, 1, 1, 1, 1, 1, 1, 1,
		0, 0, 1, 1, 1, 0, 0, 0,
		1, 1, 1, 1, 0, 0, 1, 1,
	}

	matchStart := -1 // Start index of the match
	i := 0           // Current index in the sequence

searchLoop:
	for pos, dataByte := range data {
		if dataByte == sequence[i] || seqMask[i] == 0 {
			if i == 0 {
				matchStart = pos
			}
			i++
			if i == len(sequence) {
				break searchLoop
			}
		} else {
			i = 0 // Reset sequence index if any byte does not match
		}
	}

	if i == len(sequence) {
		return matchStart
	}
	return -1
}

func calculateChecksum(data []byte, start int, length int) (int, error) {
	if int(start) > len(data) {
		return 0, errors.New("start position is beyond the length of the data slice")
	}

	dataReader := bytes.NewReader(data[start:])
	checksum := 0
	count := 0

	for count < (length>>2) && dataReader.Size() > 0 {
		var value uint32
		err := binary.Read(dataReader, binary.BigEndian, &value)
		if err != nil {
			return 0, err
		}
		checksum += int(value)
		count++
	}

	count <<= 2
	var checksum8 byte

	for count < length && dataReader.Size() > 0 {
		b, err := dataReader.ReadByte()
		if err != nil {
			return 0, err
		}
		checksum8 += b
		count++
	}

	checksum += int(checksum8)

	return checksum, nil
}

func calculateF2Checksum(data []byte, start int, length int) (uint32, error) {
	if start+length > len(data) {
		return 0, errors.New("the start and length range exceeds the data slice length")
	}

	xorTable := []uint32{
		0x81184224, 0x24421881, 0xc33c6666, 0x3cc3c3c3,
		0x11882244, 0x18241824, 0x84211248, 0x12345678,
	}

	var checksum uint32 = 0
	var xorCount uint8 = 1

	for count := 0; count < length && (start+count) < len(data)-3; count += 4 {
		temp := binary.BigEndian.Uint32(data[start+count : start+count+4])
		checksum += temp ^ xorTable[xorCount]
		xorCount++
		if xorCount > 7 {
			xorCount = 0
		}
	}

	checksum ^= 0x40314081
	checksum -= 0x7FEFDFD0

	return checksum, nil
}

func calculateFBChecksum(data []byte, start int, length int) (int, error) {
	if start+length > len(data) {
		return 0, errors.New("the start and length range exceeds the data slice length")
	}

	fbChecksum, err := calculateChecksum(data[start:], start, length)
	if err != nil {
		return 0, err
	}

	return fbChecksum, nil
}
