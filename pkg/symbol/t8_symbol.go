package symbol

import (
	"fmt"
)

func LoadT8Symbols(fileBytes []byte, cb func(string)) ([]*Symbol, error) {
	if err := ValidateTrionic8File(fileBytes); err != nil {
		return nil, err
	}

	//addressTableOffset, err := GetAddrTableOffsetBySymbolTable(fileBytes)
	//if err != nil {
	//	return nil, err
	//}

	addrtaboffset, err := GetEndOfSymbolTable(fileBytes)
	if err != nil {
		return nil, err
	}

	NqNqNqOffset, err := GetFirstNqStringFromOffset(fileBytes, addrtaboffset)
	if err != nil {
		return nil, err
	}

	addressTableOffset := NqNqNqOffset + 21 + 7

	symbtaboffset, err := GetAddressFromOffset(fileBytes, NqNqNqOffset)
	if err != nil {
		return nil, err
	}

	symbtablength, err := GetLengthFromOffset(fileBytes, NqNqNqOffset+4)
	if err != nil {
		return nil, err
	}

	names, err := ExpandCompressedSymbolNames(fileBytes[symbtaboffset : symbtaboffset+symbtablength])
	if err != nil {
		return nil, err
	}

	if err := FindAddressTableOffset(fileBytes); err != nil {
		return nil, err
	}

	symbols, err := ReadAddressTable(fileBytes, addressTableOffset)
	if err != nil {
		return nil, err
	}

	for i, sym := range symbols {
		sym.Name = names[i+1]
		sym.Unit = GetUnit(sym.Name)
		sym.Correctionfactor = GetCorrectionfactor(sym.Name)
	}

	cb(fmt.Sprintf("End Of Symbol Table: 0x%X\n", addrtaboffset))
	cb(fmt.Sprintf("NqNqNq Offset: 0x%X\n", NqNqNqOffset))
	cb(fmt.Sprintf("Symbol Table Offset: 0x%X\n", symbtaboffset))
	cb(fmt.Sprintf("Symbol Table Length: 0x%X\n", symbtablength))
	cb(fmt.Sprintf("Real Address Table Offset: 0x%X\n", addressTableOffset))

	//log.Println("Symbols found: ", symb_count)
	cb(fmt.Sprintf("Loaded %d symbols from binary", len(symbols)))

	return symbols, nil
}

func ReadAddressTable(data []byte, offset int) ([]*Symbol, error) {
	pos := offset - 17
	symbols := make([]*Symbol, 0)
	symb_count := 0
	for {
		symb_count++
		symboldata := data[pos : pos+10]
		pos += 10
		if pos > len(data) {
			break
		}
		if symboldata[9] != 0x00 {
			//log.Printf("End of table found at 0x%X\n", pos)
			break
		} else {

			sym := &Symbol{
				Name:         fmt.Sprintf("Symbol-%d", symb_count),
				Number:       symb_count,
				Address:      uint32(symboldata[2]) | uint32(symboldata[1])<<8 | uint32(symboldata[0])<<16,
				Length:       uint16(symboldata[4]) | uint16(symboldata[3])<<8,
				Mask:         uint16(symboldata[6]) | uint16(symboldata[5])<<8,
				Type:         symboldata[7],
				ExtendedType: symboldata[8],
			}
			symbols = append(symbols, sym)
		}
	}
	return symbols, nil
}

func ValidateTrionic8File(data []byte) error {
	if len(data) != 0x100000 {
		return ErrInvalidLength
	}
	if data[0] == 0x00 && (data[1] == 0x10 || data[1] == 0x00) && data[2] == 0x0C && data[3] == 0x00 {
		return nil
	}
	return ErrInvalidTrionic8File
}

func GetAddressFromOffset(data []byte, offset int) (int, error) {
	if offset < 0 || offset > len(data)-4 {
		return 0, ErrOffsetOutOfRange
	}
	retval := 0
	retval = int(data[offset]) << 24
	retval += int(data[offset+1]) << 16
	retval += int(data[offset+2]) << 8
	retval += int(data[offset+3])
	return retval, nil
}

func GetLengthFromOffset(data []byte, offset int) (int, error) {
	if offset < 0 || offset > len(data)-2 {
		return 0, ErrOffsetOutOfRange
	}
	retval := 0
	retval += int(data[offset]) << 8
	retval += int(data[offset+1])
	return retval, nil
}

func CountNq(data []byte, offset int) int {
	cnt := 0
	if data != nil && len(data) > offset {
		state := 0
		for i := offset; i < len(data) && i > (offset-8) && cnt < 3; i++ {
			switch state {
			case 0:
				if data[i] != 0x4E {
					return cnt
				}
				state++
			case 1:
				if data[i] != 0x71 {
					return cnt
				}
				state = 0
				cnt++
				i -= 4
			}
		}
	}
	return cnt
}

func GetEndOfSymbolTable(data []byte) (int, error) {
	pattern := []byte{0x73, 0x59, 0x4D, 0x42, 0x4F, 0x4C, 0x74, 0x41, 0x42, 0x4C, 0x45}
	pos := bytePatternSearch(data, pattern, 0)
	if pos == -1 {
		return -1, ErrEndOfSymbolTableNotFound
	}
	return pos + len(pattern) - 1, nil
}

func GetFirstNqStringFromOffset(data []byte, offset int) (int, error) {
	var retval, Nq1, Nq2, Nq3 int
	state := 0
outer:
	for i := offset; i < len(data) && i < offset+0x100; i++ {
		switch state {
		case 0:
			if data[i] == 0x4E {
				state++
			}
		case 1:
			if data[i] == 0x71 {
				state++
			} else {
				state = 0
			}
		case 2:
			Nq1 = i
			if data[i] == 0x4E {
				state++
			} else {
				state = 0
			}
		case 3:
			if data[i] == 0x71 {
				state++
			} else {
				state = 0
			}
		case 4:
			Nq2 = i
			if data[i] == 0x4E {
				state++
			} else {
				state = 0
			}
		case 5:
			if data[i] == 0x71 {
				state++
			} else {
				state = 0
			}
		case 6:
			Nq3 = i
			retval = i
			break outer
		}
	}

	if Nq3 == 0 {
		retval = Nq2
	}

	if retval == 0 {
		retval = Nq1
	}

	return retval, nil
}

func FindAddressTableOffset(data []byte) error {
	pos := bytePatternSearch(data, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20}, 0x3000)
	if pos == -1 {
		return ErrAddressTableOffsetNotFound
	}
	return nil
}

/*
func ExtractSymbolTable(data []byte) ([]string, error) {
	if bytes.HasPrefix(data, []byte{0xF1, 0x1A, 0x06, 0x5B, 0xA2, 0x6B, 0xCC, 0x6F}) {
		log.Println("Blowfish encrypted symbol table")
	} else {
		//return nil, ErrInvalidSymbolTableHeader
		unpackedLength := int(data[0]) | int(data[1])<<8 | int(data[2])<<16 | int(data[3])<<24
		log.Printf("Unpacked length: 0x%X\n", unpackedLength)
		if unpackedLength <= 0x00FFFFFF {
			log.Println("Decoding packed symbol table")
			return symbol.ExpandCompressedSymbolNames(data)
		}
	}
	return nil, ErrEndOfSymbolTableNotFound
}

func GetAddrTableOffsetBySymbolTable(data []byte) (int, error) {
	addrtaboffset, err := GetEndOfSymbolTable(data)
	if err != nil {
		return -1, err
	}
	log.Printf("End Of Symbol Table: 0x%X\n", addrtaboffset)

	NqNqNqOffset, err := GetFirstNqStringFromOffset(data, addrtaboffset)
	if err != nil {
		return -1, err
	}
	log.Printf("NqNqNq Offset: 0x%X\n", NqNqNqOffset)

	symbtaboffset, err := GetAddressFromOffset(data, NqNqNqOffset)
	if err != nil {
		return -1, err
	}
	log.Printf("Symbol Table Offset: 0x%X\n", symbtaboffset)

	nqCount := CountNq(data, NqNqNqOffset-2)
	log.Printf("Nq count: 0x%X\n", nqCount)

	m_addressoffset, err := GetAddressFromOffset(data, NqNqNqOffset-((nqCount*2)+6))
	if err != nil {
		return -1, err
	}
	log.Printf("Address Offset: 0x%X\n", m_addressoffset)

	symbtablength, err := GetLengthFromOffset(data, NqNqNqOffset+4)
	if err != nil {
		return -1, err
	}
	log.Printf("Symbol Table Length: 0x%X\n", symbtablength)

	if symbtablength < 0x1000 {
		return -1, ErrSymbolTableNotFound
	}

	if symbtaboffset > 0 && symbtaboffset < 0xF0000 {
		return NqNqNqOffset + 21 + 7, nil
	}

	return -1, ErrSymbolTableNotFound
}

func GetStartOfAddressTableOffset(data []byte) (int, error) {
	searchSequence := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20}
	symbolTableOffset := 0x30000
	addressTableOffset := 0
	adrState := 0
outer:
	for i := symbolTableOffset; i < len(data) && addressTableOffset == 0; i++ {
		adrb := data[i]
		switch adrState {
		case 0:
			if adrb == searchSequence[0] {
				adrState++
			}
		case 1:
			if adrb == searchSequence[1] {
				adrState++
			} else {
				adrState = 0
				i--
			}
		case 2:
			if adrb == searchSequence[2] {
				adrState++
			} else {
				adrState = 0
				i -= 2
			}
		case 3:
			if adrb == searchSequence[3] {
				adrState++
			} else {
				adrState = 0
				i -= 3
			}
		case 4:
			if adrb == searchSequence[4] {
				adrState++
			} else {
				adrState = 0
				i -= 4
			}
		case 5:
			if adrb == searchSequence[5] {
				adrState++
			} else {
				adrState = 0
				i -= 5
			}
		case 6:
			if adrb == searchSequence[6] {
				adrState++
			} else {
				adrState = 0
				i -= 6
			}
		case 7:
			if adrb == searchSequence[7] {
				adrState++
			} else {
				adrState = 0
				i -= 7
			}
		case 8:
			if adrb == searchSequence[8] {
				addressTableOffset = i - 1
				break outer
			} else {
				adrState = 0
				i -= 8
			}
		}
	}

	if addressTableOffset == 0 {
		return -1, ErrAddressTableOffsetNotFound
	}

	return addressTableOffset, nil
}
*/
