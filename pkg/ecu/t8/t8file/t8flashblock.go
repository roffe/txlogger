package t8file

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
)

type FlashBlock struct {
	BlockType    int
	BlockAddress int
	BlockNumber  int
	BlockData    []byte
}

func (fb *FlashBlock) DecodeBlock(th *T8Header) {
	data := make([]byte, 0x130)
	copy(data, fb.BlockData)
	dataLen := len(data)
	for t := range fb.BlockData {
		data[t] += 0x53
		data[t] ^= 0xA4
	}
	if dataLen >= 0xAF+0x10 {
		th.ecuDescription = string(data[0xAF : 0xAF+0x10])
	}

	if dataLen >= 0xE0+0x11 {
		th.vin = string(data[0xE0 : 0xE0+0x11])
	}

	if dataLen >= 0x101+0x0B {
		var builder strings.Builder
		for _, b := range data[0x101 : 0x101+0x0B] {
			if b != 0xFF {
				builder.WriteByte(b)
			}
		}
		th.interfaceDevice = builder.String()
	}

	if dataLen >= 0x40+0x04 {
		th.pin = string(data[0x40 : 0x40+0x04])
	}

	if dataLen >= 0x55+0x06 {
		th.psk = fmt.Sprintf("%02X", data[0x55:0x55+0x06])
	}

	if dataLen >= 0x61+0x06 {
		th.isk = fmt.Sprintf("%02X", data[0x61:0x61+0x06])
	}
}

func (fb *FlashBlock) EncodeBlock(th *T8Header) []byte {
	data := make([]byte, len(fb.BlockData))
	copy(data, fb.BlockData)
	for t := range data {
		data[t] += 0x53
		data[t] ^= 0xA4
	}

	copy(data[0xAF:0xAF+10], th.ecuDescription)

	copy(data[0xE0:0xE0+17], th.vin)

	for i := 0x101; i < 0x101+11; i++ {
		data[i] = 0xFF
	}
	copy(data[0x101:0x101+11], th.interfaceDevice)
	copy(data[0x40:0x40+4], th.pin)

	if pskBytes, err := hex.DecodeString(th.psk); err == nil && len(pskBytes) <= 6 {
		copy(data[0x55:0x55+6], pskBytes)
	}
	if iskBytes, err := hex.DecodeString(th.isk); err == nil && len(iskBytes) <= 6 {
		copy(data[0x61:0x61+6], iskBytes)
	}

	for t := range data {
		data[t] ^= 0xA4
		data[t] -= 0x53
	}
	return data
}

func (fb *FlashBlock) isValid() bool {
	if len(fb.BlockData) < 8 {
		return false
	}

	invalidPattern := []byte{0xed, 0xed, 0xed, 0xed, 0xed, 0xed, 0xed, 0xed}
	return !bytes.Equal(fb.BlockData[:8], invalidPattern)
}
