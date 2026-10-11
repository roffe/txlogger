package t8file

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"slices"
)

// FlashBlock is one NVDM record: the ECU's non-volatile data (VIN, immobilizer
// keys, odometer and service counters, fault history) as written at a power
// down, 0x130 bytes. The ECU stores records scrambled byte by byte and marks
// them "Cryp"; it reads a record without the scrambled marker as is.
type FlashBlock struct {
	BlockType    int
	BlockAddress int
	BlockNumber  int
	BlockData    []byte
}

const (
	recordSize  = 0x130
	cryptOffset = 0x50
)

// recordField is one value the editor shows, at its place in a record.
type recordField struct {
	off, n int
	get    func(*T8Header) string
	set    func(*T8Header, []byte)
	encode func(v string, n int) ([]byte, bool) // false: leave the stored bytes
}

// recordFields are the editable values. The ECU description is the hardware
// name (10 bytes) and its level and index (5); the interface device is the
// 10 byte repair shop code or tester serial, FF padded; PSK and ISK are the
// first 6 bytes of the 12 byte remote control and transponder keys.
var recordFields = []recordField{
	{0xAF, 15, (*T8Header).EcuDescription, func(th *T8Header, b []byte) { th.ecuDescription = string(b) }, padded(' ')},
	{0xE0, 17, (*T8Header).VIN, func(th *T8Header, b []byte) { th.vin = string(b) }, padded(' ')},
	{0x101, 10, (*T8Header).InterfaceDevice, func(th *T8Header, b []byte) { th.interfaceDevice = string(bytes.TrimRight(b, "\xFF")) }, padded(0xFF)},
	{0x40, 4, (*T8Header).PIN, func(th *T8Header, b []byte) { th.pin = string(b) }, padded(' ')},
	{0x55, 6, (*T8Header).PSK, func(th *T8Header, b []byte) { th.psk = fmt.Sprintf("%02X", b) }, hexBytes},
	{0x61, 6, (*T8Header).ISK, func(th *T8Header, b []byte) { th.isk = fmt.Sprintf("%02X", b) }, hexBytes},
}

func padded(pad byte) func(string, int) ([]byte, bool) {
	return func(v string, n int) ([]byte, bool) {
		b := bytes.Repeat([]byte{pad}, n)
		copy(b, v)
		return b, true
	}
}

func hexBytes(v string, n int) ([]byte, bool) {
	b, err := hex.DecodeString(v)
	return b, err == nil && len(b) == n
}

// record returns the block's record in plain form and whether it is stored
// scrambled; ok is false when it carries no "Cryp" marker either way.
func (fb *FlashBlock) record() (data []byte, scrambled, ok bool) {
	if len(fb.BlockData) < recordSize {
		return nil, false, false
	}
	data = slices.Clone(fb.BlockData[:recordSize])
	if string(data[cryptOffset:cryptOffset+4]) == "Cryp" {
		return data, false, true
	}
	for i := range data {
		data[i] = (data[i] + 0x53) ^ 0xA4
	}
	if string(data[cryptOffset:cryptOffset+4]) == "Cryp" {
		return data, true, true
	}
	return nil, false, false
}

// DecodeBlock reads the record's editable values into th; a block that is
// not a record leaves th as it is.
func (fb *FlashBlock) DecodeBlock(th *T8Header) {
	data, _, ok := fb.record()
	if !ok {
		return
	}
	for _, f := range recordFields {
		f.set(th, data[f.off:f.off+f.n])
	}
}

// EncodeBlock returns the block with every editable value taken from th.
func (fb *FlashBlock) EncodeBlock(th *T8Header) []byte {
	return fb.encode(th, recordFields)
}

// encode returns the block with the given fields set from th, stored the way
// the block was: scrambled or plain. Every other byte is kept, and a block
// that is not a record comes back unchanged.
func (fb *FlashBlock) encode(th *T8Header, fields []recordField) []byte {
	data, scrambled, ok := fb.record()
	if !ok {
		return slices.Clone(fb.BlockData)
	}
	for _, f := range fields {
		if b, ok := f.encode(f.get(th), f.n); ok {
			copy(data[f.off:f.off+f.n], b)
		}
	}
	if scrambled {
		for i := range data {
			data[i] = (data[i] ^ 0xA4) - 0x53
		}
	}
	return data
}
