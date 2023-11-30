package symbol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type T7HeaderField struct {
	ID     byte
	Length byte
	Data   []byte
}

func (h *T7HeaderField) PrettyString() string {
	if h.Length == 4 {
		return fmt.Sprintf("0x%02x %d> 0x%08X  %q", h.ID, len(h.Data), binary.BigEndian.Uint32(h.Data), h.Data)
	} else if h.Length == 2 {
		return fmt.Sprintf("0x%02x %d> 0x%04X  %q", h.ID, len(h.Data), binary.BigEndian.Uint16(h.Data), h.Data)
	} else {
		return fmt.Sprintf("0x%02x> %s", h.ID, string(h.Data))
	}
}

func (h *T7HeaderField) String() string {
	nullIndex := bytes.IndexByte(h.Data, '\x00')
	if nullIndex != -1 {
		return string(h.Data[:nullIndex])
	} else {
		return string(h.Data)
	}
}

func (h *T7HeaderField) Int16() int {
	if len(h.Data) < 2 {
		panic("data should have at least 2 bytes")
	}
	return int(binary.LittleEndian.Uint16(h.Data))
}

func (h *T7HeaderField) Int32() int {
	if len(h.Data) < 4 {
		panic("data should have at least 4 bytes")
	}

	return int(binary.LittleEndian.Uint32(h.Data))
}

func (h *T7HeaderField) Uint32() int {
	if len(h.Data) < 4 {
		panic("data should have at least 4 bytes")
	}

	return int(binary.BigEndian.Uint32(h.Data))
}

func (t7 *T7File) loadHeaders() {
	for _, h := range t7.GetHeaders() {
		switch h.ID {
		case 0x90:
			t7.chassisID = h.String()
			t7.chassisIDDetected = true
			t7.chassisIDCounter++
		case 0x91:
			t7.vehicleIDNr = h.String()
		case 0x92:
			t7.immobilizerID = h.String()
			t7.immocodeDetected = true
		case 0x93:
			t7.ecuHardwareNr = h.String()
		case 0x94:
			t7.partNumber = h.String()
		case 0x95:
			t7.softwareVersion = h.String()
		case 0x97:
			t7.carDescription = h.String()
		case 0x98:
			t7.engineType = h.String()
		case 0x99:
			t7.testserialnr = h.String()
		case 0x9A:
			t7.dateModified = h.String()
		case 0x9B:
			t7.symbolTableAddress = h.Int32()
			t7.symbolTableMarkerDetected = true
		case 0x9C:
			t7.sramOffset = h.Int32()
			//log.Printf("sramOffset: %X", t7.sramOffset)
			t7.symbolTableChecksumDetected = true
		case 0xF2:
			t7.checksumF2 = h.Int32()
			t7.f2ChecksumDetected = true
		case 0xF5:
			t7.valueF5 = h.Int16()
		case 0xF6:
			t7.valueF6 = h.Int16()
		case 0xF7:
			t7.valueF7 = h.Int16()
		case 0xF8:
			t7.valueF8 = h.Int16()
		case 0xF9:
			t7.romChecksumError = h.Data[0]
		case 0xFA:
			t7.lastModifiedBy = h.Data
		case 0xFB:
			t7.checksumFB = h.Int32()
		case 0xFC:
			t7.bottomOfFlash = h.Int32()
		case 0xFD:
			t7.romChecksumType = h.Int32()
		case 0xFE:
			t7.fwLength = h.Uint32()
		}
	}
	if (t7.chassisIDCounter > 1 || !t7.immocodeDetected || !t7.chassisIDDetected) && t7.autoFixFooter {
		t7.clearPiArea()
		t7.createPiArea()
	}
}

func (t7 *T7File) GetHeaders() []*T7HeaderField {
	binLength := len(t7.data)
	addr := binLength - 1

	fields := make([]*T7HeaderField, 0)

	for addr > (binLength - 0x1FF) {
		//		log.Printf("addr: %X", addr)
		/* The first byte is the length of the data */
		fieldLength := t7.data[addr]
		//		log.Printf("fieldLength %X", fieldLength)
		//log.Printf("%3d, %x", lengthField, lengthField)
		if fieldLength == 0x00 || fieldLength == 0xFF {
			break
		}
		addr--

		fieldID := t7.data[addr]
		addr--

		fieldData := make([]byte, int(fieldLength))
		fieldData[fieldLength-1] = 0x00

		for i := 0; i < int(fieldLength); i++ {
			fieldData[i] = t7.data[addr]
			addr--
		}
		fields = append(fields, &T7HeaderField{
			ID:     fieldID,
			Length: fieldLength,
			Data:   fieldData,
		})
		//		log.Printf("0x%02x %d> %q len: %d", fieldID, len(fieldData), string(fieldData), fieldLength)
	}
	return fields
}
