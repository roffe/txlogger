package symbol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
)

type T7Checksum struct {
	Address int
	Value   int
}

type T7ChecksumArea struct {
	Address int
	Length  int
}

func (t7c *T7ChecksumArea) String() string {
	return fmt.Sprintf("Address: %X, Length: %X", t7c.Address, t7c.Length)
}

func (t7 *T7File) VerifyChecksum() error {

	c, err := t7.getFWChecksum()
	if err != nil {
		return err
	}
	calculatedChecksum, err := t7.calculateFWChecksum()
	if err != nil {
		return err
	}
	log.Printf("Stored checksum: %X", uint32(c.Value))
	log.Printf("Calculated checksum: %X", calculatedChecksum)
	if c.Value != int(calculatedChecksum) {
		return errors.New("checksum mismatch")
	}
	return nil
}

func (t7 *T7File) getFWChecksum() (T7Checksum, error) {
	checksumArea := t7.findChecksumArea()
	if checksumArea < 0 {
		return T7Checksum{}, errors.New("checksum area not found")
	}

	//log.Printf("Checksum area: %X", checksumArea)
	if checksumArea > T7Length {
		log.Printf("Checksum area sram: %X", checksumArea)
		checksumArea = checksumArea - t7.sramOffset
	}

	chksum := t7.findFWChecksum(checksumArea)
	return chksum, nil
}

func (t7 *T7File) calculateFWChecksum() (uint32, error) {
	_, err := t7.getFWChecksum()
	if err != nil {
		return 0, err
	}

	var checksum32 uint32

	for i := 0; i < 16; i++ {
		addr := t7.csumArea[i].Address
		checksum := t7.calculateChecksum(addr, t7.csumArea[i].Length)
		checksum32 += uint32(checksum)
		//		log.Println("Checksum area:", i, t7.csumArea[i].String())
		//		log.Printf("Checksum: %d %X", i, uint32(checksum))
	}

	return checksum32, nil
}

// findFWChecksum finds the firmware checksum in the T7File data.
func (t7 *T7File) findFWChecksum(areaStart int) T7Checksum {
	var areaNumber byte
	var baseAddr int
	var ltemp int
	var csumAddr int
	var csumLength int16

	rCheckSum := T7Checksum{}

	if areaStart > 0x7FFFF {
		rCheckSum.Address = -1
		rCheckSum.Value = -1
		return rCheckSum
	}

	pos := areaStart + 22

	for pos < 0x7FFFF {
		if t7.data[pos] == 0x48 {
			switch t7.data[pos+1] {
			case 0x6D:
				csumAddr = baseAddr + int(t7.data[pos+2])<<8 | int(t7.data[pos+3])
				t7.csumArea[areaNumber].Address = csumAddr
				areaNumber++
				pos += 4
			case 0x78:
				csumLength = int16(t7.data[pos+2])<<8 | int16(t7.data[pos+3])
				t7.csumArea[areaNumber].Length = int(csumLength)
				pos += 4
			case 0x79:
				csumAddr = int(t7.data[pos+2])<<24 | int(t7.data[pos+3])<<16 | int(t7.data[pos+4])<<8 | int(t7.data[pos+5])
				t7.csumArea[areaNumber].Address = csumAddr
				areaNumber++
				pos += 6
			default:
				pos += 2
			}
		} else if t7.data[pos] == 0x2A && t7.data[pos+1] == 0x7C {
			ltemp = int(t7.data[pos+2])<<24 | int(t7.data[pos+3])<<16 | int(t7.data[pos+4])<<8 | int(t7.data[pos+5])
			if ltemp < 0xF00000 {
				baseAddr = ltemp
			}
			pos += 6
		} else if t7.data[pos] == 0xB0 && t7.data[pos+1] == 0xB9 {
			csumAddr = int(t7.data[pos+2])<<24 | int(t7.data[pos+3])<<16 | int(t7.data[pos+4])<<8 | int(t7.data[pos+5])
			tpos := csumAddr - t7.sramOffset
			rCheckSum.Address = tpos
			rCheckSum.Value = int(t7.data[tpos])<<24 | int(t7.data[tpos+1])<<16 | int(t7.data[tpos+2])<<8 | int(t7.data[tpos+3])
			break
		} else {
			pos += 2
		}
	}
	if rCheckSum.Address > 0x7FFFF {
		log.Printf("Checksum address 1x: %X", rCheckSum.Address)
		rCheckSum.Address = rCheckSum.Address - t7.sramOffset
	}
	return rCheckSum
}

func (t7 *T7File) findChecksumArea() int {
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
	for pos, dataByte := range t7.data {
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

func (t7 *T7File) calculateChecksum(start, length int) int {
	var checksum uint32
	var checksum8 byte
	count := 0

	pos := start
	end := len(t7.data)
	if end > 0x7FFFF {
		end = 0x7FFFF
	}

	for count < (length>>2) && pos < end {
		checksum += uint32(t7.data[pos])<<24 | uint32(t7.data[pos+1])<<16 | uint32(t7.data[pos+2])<<8 | uint32(t7.data[pos+3])
		count++
		pos += 4
	}

	count <<= 2
	for count < length && pos < end {
		checksum8 += t7.data[pos]
		count++
		pos++
	}

	checksum += uint32(checksum8)
	return int(checksum)
}

func (t7 *T7File) calculateFBChecksum(data []byte, start int, length int) (uint32, error) {
	if start+length > len(data) {
		return 0, errors.New("the start and length range exceeds the data slice length")
	}

	fbChecksum := t7.calculateChecksum(start, length)

	return uint32(fbChecksum), nil
}

func (t7 *T7File) calculateF2Checksum(data []byte, start int, length int) (uint32, error) {
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
