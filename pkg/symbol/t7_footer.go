package symbol

import (
	"encoding/binary"
	"log"
)

func (t7 *T7File) clearPiArea() {
	const startPosition = 0x07FE00
	if len(t7.data) <= startPosition {
		return
	}
	for i := startPosition; i < len(t7.data); i++ {
		(t7.data)[i] = 0xFF
	}
	log.Println("Footer cleared")
}

func (t7 *T7File) createPiArea() {
	log.Println("Creating new footer")
	pos := len(t7.data) - 1

	pos = t7.writeFooterString(pos, 0x91, t7.vehicleIDNr)
	pos = t7.writeFooterString(pos, 0x94, t7.partNumber)
	pos = t7.writeFooterString(pos, 0x95, t7.softwareVersion)
	pos = t7.writeFooterString(pos, 0x97, t7.carDescription)
	pos = t7.writeFooterString(pos, 0x9A, t7.dateModified)

	if t7.symbolTableChecksumDetected {
		pos = t7.writeFooterInt(pos, 0x9C, t7.sramOffset)
	}

	if t7.symbolTableMarkerDetected {
		pos = t7.writeFooterInt(pos, 0x9B, t7.symbolTableAddress)
	}

	if t7.f2ChecksumDetected {
		pos = t7.writeFooterIntx(pos, 0xF2, t7.checksumF2)
	}

	pos = t7.writeFooterIntx(pos, 0xFB, t7.checksumFB)
	pos = t7.writeFooterInt(pos, 0xFC, t7.bottomOfFlash)
	pos = t7.writeFooterInt(pos, 0xFD, t7.romChecksumType)
	pos = t7.writeFooterIntx(pos, 0xFE, t7.fwLength)
	pos = t7.writeFooterBytes(pos, 0xFA, t7.lastModifiedBy)
	pos = t7.writeFooterString(pos, 0x92, t7.immobilizerID)
	pos = t7.writeFooterString(pos, 0x93, t7.ecuHardwareNr)
	pos = t7.writeFooterInt16(pos, 0xF8, t7.valueF8)
	pos = t7.writeFooterInt16(pos, 0xF7, t7.valueF7)
	pos = t7.writeFooterInt16(pos, 0xF6, t7.valueF6)
	pos = t7.writeFooterInt16(pos, 0xF5, t7.valueF5)
	pos = t7.writeFooterString(pos, 0x90, t7.chassisID)
	pos = t7.writeFooterString(pos, 0x99, t7.testserialnr)
	pos = t7.writeFooterString(pos, 0x98, t7.engineType)
	t7.writeFooterBytes(pos, 0xF9, []byte{t7.romChecksumError})

	//	log.Printf("pos: %d, %X", pos, t7.data[0x07FE00:])
}

func (t7 *T7File) writeFooter(pos int, h T7HeaderField) int {
	t7.data[pos] = h.Length
	pos--
	t7.data[pos] = h.ID
	for i := 0; i < int(h.Length); i++ {
		t7.data[pos-int(h.Length)+i] = h.Data[int(h.Length-1)-i]
	}
	pos -= int(h.Length + 1)
	return pos
}

func (t7 *T7File) writeFooterBytes(pos int, id byte, value []byte) int {
	h := T7HeaderField{
		ID:     id,
		Length: byte(len(value)),
		Data:   value,
	}
	return t7.writeFooter(pos, h)
}

func (t7 *T7File) writeFooterString(pos int, id byte, value string) int {
	h := T7HeaderField{
		ID:     id,
		Length: byte(len(value)),
		Data:   []byte(value),
	}
	return t7.writeFooter(pos, h)
}

func (t7 *T7File) writeFooterInt(pos int, id byte, value int) int {
	h := T7HeaderField{
		ID:     id,
		Length: 4,
		Data:   make([]byte, 4),
	}
	binary.BigEndian.PutUint32(h.Data, uint32(value))

	t7.data[pos] = h.Length
	pos--
	t7.data[pos] = h.ID
	pos--

	t7.data[pos] = h.Data[3]
	pos--
	t7.data[pos] = h.Data[2]
	pos--
	t7.data[pos] = h.Data[1]
	pos--
	t7.data[pos] = h.Data[0]
	pos--

	return pos
}

func (t7 *T7File) writeFooterIntx(pos int, id byte, value int) int {
	h := T7HeaderField{
		ID:     id,
		Length: 4,
		Data:   make([]byte, 4),
	}
	binary.BigEndian.PutUint32(h.Data, uint32(value))

	t7.data[pos] = h.Length
	pos--
	t7.data[pos] = h.ID
	pos--

	t7.data[pos] = h.Data[0]
	pos--
	t7.data[pos] = h.Data[1]
	pos--
	t7.data[pos] = h.Data[2]
	pos--
	t7.data[pos] = h.Data[3]
	pos--

	return pos
}

func (t7 *T7File) writeFooterInt16(pos int, id byte, value int) int {
	h := T7HeaderField{
		ID:     id,
		Length: 2,
		Data:   make([]byte, 2),
	}
	binary.LittleEndian.PutUint16(h.Data, uint16(value))
	return t7.writeFooter(pos, h)
}
