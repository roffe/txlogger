package t8util

import (
	"crypto/md5"
)

var T8binSize = uint64(0x100000)

var t8parts = []uint32{
	0x000000, // Boot (0)
	0x004000, // NVDM
	0x006000, // NVDM
	0x008000, // HWIO
	0x020000, // APP
	0x040000, // APP
	0x060000, // APP
	0x080000, // APP
	0x0C0000, // APP
	0x100000, // End (9)

	0x000000, // Special cases for range-md5 instead of partitions
	0x004000,
	0x020000,
}

func GetPartitionMD5(filebytes []byte, device byte, partition int) []byte {
	start := uint32(0)
	e := 0
	end := uint32(0x40100)
	byteswapped := false

	switch device {
	case 6:
		switch {
		case partition == 0:
			end = t8parts[9]
		case partition > 0 && partition < 10:
			start = t8parts[partition-1]
			end = t8parts[partition]
		case partition > 9 && partition < 13:
			start = t8parts[partition]
			end = GetLasAddress(filebytes)
		}
	case 5:
		byteswapped = MCPSwapped(filebytes)
		if partition > 0 && partition < 10 {
			if partition == 9 {
				start = 0x40000
				end = 0x40100
			} else {
				end = uint32(partition) << 15
				start = end - 0x8000
			}
		}
	default:
		return make([]byte, 16)
	}

	buf := make([]byte, end-start)

	if !byteswapped {
		copy(buf, filebytes[start:end])
	} else {
		for i := start; i < end; i += 2 {
			buf[e] = filebytes[i+1]
			e++
			buf[e] = filebytes[i]
			e++
		}
	}

	md := md5.Sum(buf)
	return md[:]
}

func GetLasAddress(filebytes []byte) uint32 {
	// Add another 512 bytes to include header region (with margin)!!!
	// Note; Legion is hardcoded to also add 512 bytes. -Do not change!
	return uint32(int(filebytes[0x020141])<<16 | int(filebytes[0x020142])<<8 | int(filebytes[0x020143]) + 0x200)
}

func MCPSwapped(filebytes []byte) bool {
	if filebytes[0] == 0x08 && filebytes[1] == 0x00 &&
		filebytes[2] == 0x00 && filebytes[3] == 0x20 {
		return true
	}
	return false
}

func CodeByte(b byte, count int) byte {
	var rb byte
	switch count {
	case 0:
		rb = b ^ 0x39
	case 1:
		rb = b ^ 0x68
	case 2:
		rb = b ^ 0x77
	case 3:
		rb = b ^ 0x6D
	case 4:
		rb = b ^ 0x47
	case 5:
		rb = b ^ 0x39
	}
	return rb
}

func GetCurrentBlock(filebytes []byte, block int, byteswapped bool) []byte {
	_blockNumber := block
	address := _blockNumber * 0x80

	buffer := make([]byte, 0x80)
	array := make([]byte, 0x88)

	if byteswapped {
		for byteCount := 0; byteCount < int(0x80); byteCount += 2 {
			buffer[byteCount+1] = filebytes[address]
			buffer[byteCount] = filebytes[address+1]
			address += 2
		}
	} else {
		for byteCount := 0; byteCount < int(0x80); byteCount++ {
			buffer[byteCount] = filebytes[address]
			address++
		}
	}
	var cnt int = 0
	for byteCount := 0; byteCount < int(0x80); byteCount++ {
		array[byteCount] = CodeByte(buffer[byteCount], cnt)
		cnt++
		if cnt == 6 {
			cnt = 0
		}
	}
	return array
}

func FFblock(filebytes []byte, address int, size byte) bool {
	var count int = 0
	len := len(filebytes)
	for byteCount := 0; byteCount < int(size); byteCount++ {
		if address == len {
			break
		}
		if filebytes[address] == 0xFF {
			count++
		}
		address++
	}
	if count == int(size) {
		return true
	}
	return false
}

func Erasedregion(address int, device byte, formatMask uint64) bool {
	// Note: The format function count partitions as 1 to 9.
	// this on the other hand will count from 0 to 8
	var part int = 0

	// T8 main
	switch device {
	case 6:
		if address >= 0xC0000 {
			part = 8
		} else if address >= 0x80000 {
			part = 7
		} else if address >= 0x60000 {
			part = 6
		} else if address >= 0x40000 {
			part = 5
		} else if address >= 0x20000 {
			part = 4
		} else if address >= 0x8000 {
			part = 3
		} else if address >= 0x6000 {
			part = 2
		} else if address >= 0x4000 {
			part = 1
		}
	case 5: //MCP
		part = int(address>>15) & 0xF
	}

	// Read one bit from selected part of the format mask to figure out if this partition should be written or not.
	if ((formatMask >> part) & 1) > 0 {
		return true
	} else {
		return false
	}
}

func getUlong(arr []byte, startIndex int, count int) uint64 {
	var result uint64 = 0x00
	for i := startIndex + count - 1; i >= startIndex; i-- {
		result <<= 8
		result += uint64(arr[i])
	}
	return result
}

func GetFrameCmd(frameNo int, array []byte, startIndex int) uint64 {
	res := getUlong(array, startIndex, 7)
	res = (res << 8) | uint64(frameNo&0xFF)
	return res
}

func GetVirginNVDM() []byte {
	buff := make([]byte, T8binSize)
	for i := range buff {
		buff[i] = 0xFF
	}
	copy(buff[t8parts[1]:], nvdmBytes)

	return buff
}
