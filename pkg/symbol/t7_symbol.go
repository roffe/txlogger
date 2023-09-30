package symbol

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
)

func ValidateTrionic7File(data []byte) error {
	if len(data) != 0x80000 {
		return ErrInvalidLength
	}
	if !bytes.HasPrefix(data, []byte{0xFF, 0xFF, 0xEF, 0xFC}) {
		return ErrInvalidTrionic7File
	}
	return nil
}

func NewFromT7Bytes(data []byte, symb_count int) *Symbol {
	var internall_address uint32
	for i := 0; i < 4; i++ {
		internall_address <<= 8
		internall_address |= uint32(data[i])
	}
	var symbol_length uint16
	if symb_count == 0 {
		symbol_length = 0x08
	} else {
		for i := 4; i <= 5; i++ {
			symbol_length <<= 8
			symbol_length |= uint16(data[i])
		}
	}

	var symbol_mask uint16
	for i := 6; i <= 7; i++ {
		symbol_mask <<= 8
		symbol_mask |= uint16(data[i])
	}

	symbol_type := data[8]

	//	log.Printf("%X %d %X %X", internall_address, symbol_length, symbol_mask, symbol_type)

	return &Symbol{
		Name:    "Symbol-" + strconv.Itoa(symb_count),
		Number:  symb_count,
		Address: internall_address,
		Length:  symbol_length,
		Mask:    symbol_mask,
		Type:    symbol_type,
	}

}

func LoadT7Symbols(data []byte, cb func(string)) ([]*Symbol, error) {

	//fstats, err := file.Stat()
	//if err != nil {
	//	return nil, err
	//}

	//	symbol_collection := make(map[string]Symbol)

	if !IsBinaryPackedVersion(data, 0x9B) {
		return nil, errors.New("non binary packed not implemented, send your bin to Roffe")
		//log.Println("Not a binary packed version")
		//cb("Not a binary packed symbol table")
		//if err := nonBinaryPacked(cb, file, fstats); err != nil {
		//	return nil, err
		//}
	} else {
		//log.Println("Binary packed version")
		cb("Found binary packed symbol table")
		return BinaryPacked(data, cb)

	}
	//return nil, errors.New("not implemented")
}

func nonBinaryPacked(cb func(string), file *os.File, fstats fs.FileInfo) error {
	symbolListOffset, err := GetSymbolListOffSet(file, int(fstats.Size()))
	if err != nil {
		return err
	}
	log.Printf("Symbol list offset: %X", symbolListOffset)
	return nil
}

func BinaryPacked(data []byte, cb func(string)) ([]*Symbol, error) {
	compressed, addressTableOffset, symbolNameTableOffset, symbolTableLength, err := getOffsets(data, cb)
	if err != nil && !errors.Is(err, ErrSymbolTableNotFound) {
		return nil, err
	}
	//os.WriteFile("compressedSymbolNameTable.bin", data[symbolNameTableOffset:symbolNameTableOffset+symbolTableLength, 0644)
	if addressTableOffset == -1 {
		return nil, ErrAddressTableOffsetNotFound
	}

	pos := addressTableOffset
	var (
		symb_count int
		symbols    []*Symbol
	)

	for {
		buff := data[pos : pos+10]
		pos += 10
		if len(buff) != 10 {
			return nil, errors.New("binaryPacked: not enough bytes read")
		}

		if int32(buff[0]) != 0x53 && int32(buff[1]) != 0x43 { // SC
			symbols = append(symbols, NewFromT7Bytes(buff, symb_count))
			symb_count++
		} else {
			// file.Seek(0, io.SeekCurrent)
			// if pos, err := file.Seek(0, io.SeekCurrent); err == nil {
			// 	log.Printf("EOT: %X", pos-0xA)
			// }
			break
		}

	}
	//log.Println("Symbols found: ", symb_count)
	cb(fmt.Sprintf("Loaded %d symbols from binary", symb_count))

	if compressed {
		if bytes.HasPrefix(data[symbolNameTableOffset:symbolNameTableOffset+symbolTableLength], []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
			return nil, errors.New("compressed symbol table is not present")
		}
		symbolNames, err := ExpandCompressedSymbolNames(data[symbolNameTableOffset : symbolNameTableOffset+symbolTableLength])
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(symbolNames)-1; i++ {
			symbols[i].Name = strings.TrimSpace(symbolNames[i])
			symbols[i].Unit = GetUnit(symbols[i].Name)
			symbols[i].Correctionfactor = GetCorrectionfactor(symbols[i].Name)
		}
		if err := readAllT7SymbolsData(data, symbols); err != nil {
			return symbols, err
		}
		return symbols, nil
	}

	if symbolTableLength < 0x100 {
		ver, err := determineVersion(data)
		if err != nil {
			if errors.Is(err, ErrVersionNotFound) {
				cb("Could not determine binary version")
				cb("Load symbols from XML")
			} else {
				return nil, fmt.Errorf("could not determine version: %v", err)
			}
		}

		nameMap, err := xml2map(ver)
		if err != nil {
			return nil, err
		}
		for i, s := range symbols {
			if value, ok := nameMap[s.Number]; ok {
				symbols[i].Name = value
				symbols[i].Unit = GetUnit(s.Name)
				symbols[i].Correctionfactor = GetCorrectionfactor(symbols[i].Name)
			}
		}
	}

	if err := readAllT7SymbolsData(data, symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}

func readAllT7SymbolsData(fileBytes []byte, symbols []*Symbol) error {
	dataLocationOffset := bytePatternSearch(fileBytes, searchPattern, 0x30000) - 10
	dataOffsetValue := binary.BigEndian.Uint32(fileBytes[dataLocationOffset : dataLocationOffset+4])
	for _, sym := range symbols {
		if sym.Address < 0x0F00000 {
			var err error
			sym.data, err = readSymbolData(fileBytes, sym, 0)
			if err != nil {
				return err
			}
		} else {
			//dataLocationOffset := bytePatternSearch(fileBytes, searchPattern3, 0x30000) + len(searchPattern3)
			if sym.Address-dataOffsetValue+uint32(sym.Length) > uint32(len(fileBytes)) {
				// debug.Log(fmt.Sprintf("symbol address out of range: %s", sym.String()))
				// log.Printf("symbol address out of range:%X %s", sym.Address-dataOffsetValue, sym.String())
				continue
			}
			var err error
			sym.data, err = readSymbolData(fileBytes, sym, dataOffsetValue)
			if err != nil {
				return err
			}

		}
	}
	log.Printf("KUK %X", fileBytes[dataLocationOffset:dataLocationOffset+4])
	log.Printf("dataLocationOffset: %X", dataLocationOffset)
	log.Printf("dataOffsetValue: %X", dataOffsetValue)

	return nil
}

func readSymbolData(file []byte, s *Symbol, offset uint32) ([]byte, error) {
	//	log.Println("readSymbolData: ", s.String())
	defer func() {
		if err := recover(); err != nil {
			log.Printf("%s, error reading symbol data: %v", s.String(), err)
		}
	}()
	symData := make([]byte, s.Length)
	copy(symData, file[s.Address-offset:(s.Address-offset)+uint32(s.Length)])
	return symData, nil
}

func determineVersion(data []byte) (string, error) {
	switch {
	case bytes.Contains(data, []byte("EU0CF01O")):
		return "EU0CF01O", nil
	case bytes.Contains(data, []byte("EU0AF01C")), bytes.Contains(data, []byte("EU0BF01C")), bytes.Contains(data, []byte("EU0CF01C")):
		return "EU0AF01C", nil
	}
	return "", ErrVersionNotFound
}

var searchPattern = []byte{0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00}

// var searchPattern2 = []byte{0x00, 0x08, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
var searchPattern3 = []byte{0x73, 0x59, 0x4D, 0x42, 0x4F, 0x4C, 0x74, 0x41, 0x42, 0x4C, 0x45, 0x00} // 12

//var readNo int

func getOffsets(data []byte, cb func(string)) (bool, int, int, int, error) {
	addressTableOffset := bytePatternSearch(data, searchPattern, 0x30000) - 0x06
	//	log.Printf("Address table offset: %08X", addressTableOffset)
	cb(fmt.Sprintf("Address table offset: %08X", addressTableOffset))

	//sramTableOffset := getAddressFromOffset(file, addressTableOffset-0x04)
	//log.Printf("SRAM table offset: %08X", sramTableOffset)
	//cb(fmt.Sprintf("SRAM table offset: %08X", sramTableOffset))

	symbolNameTableOffset := getAddressFromOffset(data, addressTableOffset)
	//	log.Printf("Symbol table offset: %08X", symbolNameTableOffset)
	cb(fmt.Sprintf("Symbol table offset: %08X", symbolNameTableOffset))

	symbolTableLength := getLengthFromOffset(data, addressTableOffset+0x04)
	//	log.Printf("Symbol table length: %08X", symbolTableLength)
	cb(fmt.Sprintf("Symbol table length: %08X", symbolTableLength))

	if symbolTableLength > 0x1000 && symbolNameTableOffset > 0 && symbolNameTableOffset < 0x70000 {
		//compressedSymbolTable := data[symbolNameTableOffset : symbolNameTableOffset+symbolTableLength]
		return true, addressTableOffset, symbolNameTableOffset, symbolTableLength, nil
	}
	return false, addressTableOffset, symbolNameTableOffset, symbolTableLength, ErrSymbolTableNotFound
}

func getLengthFromOffset(data []byte, offset int) int {
	return int(binary.BigEndian.Uint16(data[offset : offset+2]))
}

func getAddressFromOffset(data []byte, offset int) int {
	return int(binary.BigEndian.Uint32(data[offset : offset+4]))
}

func GetSymbolListOffSet(file *os.File, length int) (int, error) {
	retval := 0
	zerocount := 0
	var pos int64
	var err error

	for pos < int64(length) && retval == 0 {
		// Get current file position
		pos, err = file.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}
		b := make([]byte, 1)
		n, err := file.Read(b)
		if err != nil {
			return 0, err
		}
		if n != 1 {
			return 0, errors.New("read error")
		}
		if b[0] == 0x00 {
			zerocount++
		} else {
			if zerocount < 15 {
				zerocount = 0
			} else {
				retval = int(pos)
			}
		}
	}

	return -1, errors.New("Symbol list not found")
}

func ReadMarkerAddressContent(data []byte, value byte) (length, retval, val int, err error) {
	fileoffset := len(data) - 0x90
	inb := data[len(data)-0x90:]

	if len(inb) != 0x90 {
		err = fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", len(inb), 0x90)
		return
	}
	for t := 0; t < 0x90; t++ {
		if inb[t] == value && inb[t+1] < 0x30 {
			// Marker found, read 6 bytes
			retval = fileoffset + t // 0x07FF70 + t
			length = int(inb[t+1])
			break
		}
	}
	pos := retval - length
	info := data[pos : pos+length]
	if len(info) != length {
		err = fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", len(info), length)
		return
	}
	for bc := 0; bc < length; bc++ {
		val <<= 8
		val |= int(info[bc])
	}
	return
}

func IsBinaryPackedVersion(data []byte, filelength int) bool {
	length, retval, _, err := ReadMarkerAddressContent(data, 0x9B)
	if err != nil {
		panic(err)
	}
	//log.Printf("Length: %d, Retval: %X, Val: %X", length, retval, val)
	if retval > 0 && length < filelength && length > 0 {
		return true
	}
	return false
}
