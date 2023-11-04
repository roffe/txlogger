package symbol

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/roffe/txlogger/pkg/debug"
)

const (
	SIGNED   = 0x01 /* signed flag in type */
	KONST    = 0x02 /* konstant flag in type */
	CHAR     = 0x04 /* character flag in type */
	LONG     = 0x08 /* long flag in type */
	BITFIELD = 0x10 /* bitfield flag in type */
	STRUCT   = 0x20 /* struct flag in type */
)

var T7SymbolsTuningOrder = []string{
	"Calibration",
	"Injectors",
	"Limiters",
	"Fuel",
	"Boost",
	"Ignition",
	"Adaption",
	"Myrtilos",
}

var T7SymbolsTuning = map[string][]string{
	"Calibration": {
		"AirCompCal.PressMap",
		"MAFCal.m_RedundantAirMap",
		"TCompCal.EnrFacE85Tab",
		"TCompCal.EnrFacTab",
		"VIOSMAFCal.FreqSP",
		"VIOSMAFCal.Q_AirInletTab2",
	},
	"Injectors": {
		"InjCorrCal.BattCorrTab",
		"InjCorrCal.BattCorrSP",
		"InjCorrCal.InjectorConst",
	},
	"Limiters": {
		"BstKnkCal.MaxAirmass",
		"TorqueCal.M_ManGearLim",
	},
	"Fuel": {
		"BFuelCal.Map",
		"BFuelCal.StartMap",
		"StartCal.EnrFacE85Tab",
		"StartCal.EnrFacTab",
	},
	"Boost": {
		"...|BoostCal.RegMap|BoostCal.PMap|BoostCal.IMap|BoostCal.DMap",
		"BoostCal.RegMap",
		"BoostCal.PMap",
		"BoostCal.IMap",
		"BoostCal.DMap",
	},
	"Ignition": {
		"IgnE85Cal.fi_AbsMap",
		"IgnIdleCal.fi_IdleMap",
		"IgnNormCal.Map",
		"IgnStartCal.fi_StartMap",
	},
	"Adaption": {
		"AdpFuelCal.T_AdaptLim",
		"FCutCal.ST_Enable",
		"LambdaCal.ST_Enable",
		"PurgeCal.ST_PurgeEnable",
	},
	"Myrtilos": {
		"MyrtilosCal.Launch_DisableSpeed",
		"MyrtilosCal.Launch_Ign_fi_Min",
		"MyrtilosCal.Launch_RPM",
		"MyrtilosCal.Launch_InjFac_at_rpm",
		"MyrtilosCal.Launch_PWM_max_at_stand",
	},
}

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
	extractUint32 := func(data []byte, start int) uint32 {
		return uint32(data[start])<<24 | uint32(data[start+1])<<16 | uint32(data[start+2])<<8 | uint32(data[start+3])
	}

	extractUint16 := func(data []byte, start int) uint16 {
		return uint16(data[start])<<8 | uint16(data[start+1])
	}

	internall_address := extractUint32(data, 0)

	symbol_length := uint16(0x08)
	if symb_count != 0 {
		symbol_length = extractUint16(data, 4)
	}

	symbol_mask := extractUint16(data, 6)

	symbol_type := data[8]

	return &Symbol{
		Name:    "Symbol-" + strconv.Itoa(symb_count),
		Number:  symb_count,
		Address: internall_address,
		Length:  symbol_length,
		Mask:    symbol_mask,
		Type:    symbol_type,
	}
}

func LoadT7Symbols(data []byte, cb func(string)) (SymbolCollection, error) {
	if err := ValidateTrionic7File(data); err != nil {
		return nil, err
	}

	for _, h := range GetAllT7HeaderFields(data) {
		switch h.ID {
		case 0x91, 0x94, 0x95, 0x97:
			cb(h.String())
		}
	}

	if !IsBinaryPackedVersion(data, 0x9B) {
		//return nil, errors.New("non binarypacked not implemented, send your bin to Roffe")
		//log.Println("Not a binarypacked version")
		cb("Not a binarypacked symbol table")
		return nonBinaryPacked(data, cb)

	} else {
		//log.Println("Binary packed version")
		cb("Found binary packed symbol table")
		return binaryPacked(data, cb)

	}
	//return nil, errors.New("not implemented")
}

func nonBinaryPacked(data []byte, cb func(string)) (SymbolCollection, error) {
	symbolListOffset, err := getSymbolListOffSet(data) // 0x15FA in 5168646.BIN
	if err != nil {
		return nil, err
	}
	cb(fmt.Sprintf("Symbol list offset: %X", symbolListOffset))
	var symbolName strings.Builder
	var symbolCount int
	var symbolNames []string
	var symbolInternalPositions []int

outer:
	for pos := symbolListOffset; pos < len(data); pos++ {
		switch data[pos] {
		case 0xFF: // 0xFF used to keep the start of each string 'word' aligned
			continue
		case 0x02:
			break outer
		case 0x00: // 0x00 end of Symbol name string
			symbolNames = append(symbolNames, symbolName.String())
			symbolInternalPositions = append(symbolInternalPositions, pos-len(symbolName.String()))
			symbolName.Reset()
			symbolCount++
		default:
			symbolName.WriteByte(data[pos])
		}
	}

	cb(fmt.Sprintln("Symbols found: ", symbolCount))

	searchPattern := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0}
	searchPattern[12] = byte(symbolInternalPositions[0] >> 8)
	searchPattern[13] = byte(symbolInternalPositions[0])

	// log.Printf("Search pattern: %X", searchPattern)

	addressTableOffset := BytePatternSearch(data, searchPattern, 0)
	cb(fmt.Sprintf("Address table offset: %X", addressTableOffset))

	if addressTableOffset == -1 {
		return nil, ErrAddressTableOffsetNotFound
	}

	var symb_count int

	symCol := NewCollection()

	for pos := addressTableOffset; pos < len(data); pos += 14 {
		if symb_count >= symbolCount {
			break
		}
		buff := data[pos : pos+14]
		sram_address := binary.BigEndian.Uint32(buff[0:4])
		symbol_length := binary.BigEndian.Uint16(buff[4:6])
		internal_address := binary.BigEndian.Uint32(buff[10:14])
		sym_type := buff[8]

		var real_rom_address uint32
		if sram_address > 0xF00000 {
			real_rom_address = sram_address - 0xEF02F0
		} else {
			real_rom_address = sram_address
		}

		sym := &Symbol{
			Name:             strings.TrimSpace(symbolNames[symb_count]),
			Number:           symb_count,
			Address:          real_rom_address,
			Length:           symbol_length,
			Mask:             binary.BigEndian.Uint16(buff[6:8]),
			Type:             sym_type,
			Correctionfactor: GetCorrectionfactor(strings.TrimSpace(symbolNames[symb_count])),
			Unit:             GetUnit(strings.TrimSpace(symbolNames[symb_count])),
		}
		if sym.Address < 0x0F00000 {
			data, err := readSymbolData(data, sym, 0)
			if err == nil {
				sym.data = data
			} else {
				log.Println(err)
			}
		}

		if sym.Name == "BFuelCal.E85Map" {
			log.Println(sym.String())
		}

		if sym.Name == "BFuelCal.Map" {
			log.Printf("all %X", buff)
			log.Printf("sram address: %X", sram_address)
			log.Printf("symbol length: %X", symbol_length)
			log.Printf("internal address: %X", internal_address)
			log.Printf("rest %X", buff[6:10])
			log.Printf("real rom address: %X", real_rom_address)
			log.Println(sym.String())
		}

		symCol.Add(sym)
		symb_count++
	}
	/*
		for _, sym := range symCol.Symbols() {
			if sym.Address < 0x0F00000 {
				sym.data, err = readSymbolData(data, sym, 0)
				if err != nil {
					return nil, err
				}
			}
		}
	*/
	//log.Println("Symbols found: ", symb_count)
	cb(fmt.Sprintf("Loaded %d symbols from binary", symb_count))

	return symCol, nil
}

func binaryPacked(data []byte, cb func(string)) (SymbolCollection, error) {
	compressed, addressTableOffset, symbolNameTableOffset, symbolTableLength, err := getOffsets(data, cb)
	if err != nil && !errors.Is(err, ErrSymbolTableNotFound) {
		return nil, err
	}
	//os.WriteFile("compressedSymbolNameTable.bin", data[symbolNameTableOffset:symbolNameTableOffset+symbolTableLength, 0644)

	if addressTableOffset == -1 {
		return nil, ErrAddressTableOffsetNotFound
	}

	var symb_count int
	var symbols []*Symbol

	os.WriteFile("adresstable.bin", data[addressTableOffset:], 0644)

	// parse addresstable and create symbols with generic names
	for pos := addressTableOffset; pos < len(data)+10; pos += 10 {
		if data[pos] == 0x53 && data[pos+1] == 0x43 { // SC
			break
		}
		symbols = append(symbols, NewFromT7Bytes(data[pos:pos+10], symb_count))
		symb_count++
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
			return NewCollection(symbols...), err
		}
		return NewCollection(symbols...), nil
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

	return NewCollection(symbols...), nil
}

func readAllT7SymbolsData(fileBytes []byte, symbols []*Symbol) error {
	dataLocationOffset := BytePatternSearch(fileBytes, searchPattern, 0x30000) - 10
	dataOffsetValue := binary.BigEndian.Uint32(fileBytes[dataLocationOffset : dataLocationOffset+4])

	//sram_offset, err := GetAddressFromOffset(fileBytes, dataLocationOffset+4)
	//if err != nil {
	//	return err
	//}
	/*
		log.Printf("sram_offset: %X", sram_offset)
		log.Printf("dataLocationOffsetRaw %X", fileBytes[dataLocationOffset:dataLocationOffset+4])
		log.Printf("dataLocationOffset: %X", dataLocationOffset)
		log.Printf("dataOffsetValue: %X", dataOffsetValue)
	*/

	c9value, err := GetT7HeaderField(fileBytes, 0x9C)
	if err != nil {
		return err
	}

	sramOffset := reverseInt(binary.BigEndian.Uint32(c9value))
	//	log.Printf("sramOffset: %X", sramOffset)

	for _, sym := range symbols {
		sym.SramOffset = sramOffset
		if sym.Address < 0x0F00000 {
			sym.data, err = readSymbolData(fileBytes, sym, 0)
			if err != nil {
				return err
			}
		} else {
			if sym.Address-dataOffsetValue < uint32(len(fileBytes)) {
				sym.data, err = readSymbolData(fileBytes, sym, dataOffsetValue)
				if err != nil {
					return err
				}
			} else if sym.Address-sramOffset < uint32(len(fileBytes)) {
				sym.data, err = readSymbolData(fileBytes, sym, sramOffset)
				if err != nil {
					return err
				}
			} else {
				log.Printf("symbol address out of range:%X %s", sym.Address-dataOffsetValue, sym.String())
			}

		}
	}

	return nil
}

func reverseInt(value uint32) uint32 {
	// input            0x34FCEF00
	// desired output   0x00EFFC34
	var retval uint32
	retval |= (value & 0xFF000000) >> 24
	retval |= ((value & 0x00FF0000) >> 16) << 8
	retval |= ((value & 0x0000FF00) >> 8) << 16
	retval |= (value & 0x000000FF) << 24

	return retval
}

func readSymbolData(file []byte, s *Symbol, offset uint32) ([]byte, error) {
	//	log.Println("readSymbolData: ", s.String())
	defer func() {
		if err := recover(); err != nil {
			debug.Log(fmt.Sprintf("%s, error reading symbol data: %v", s.String(), err))
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
	case bytes.Contains(data, []byte("C10FA0UE")), bytes.Contains(data, []byte("EU0AF01C")), bytes.Contains(data, []byte("EU0BF01C")), bytes.Contains(data, []byte("EU0CF01C")):
		return "EU0AF01C", nil
	}
	return "", ErrVersionNotFound
}

var searchPattern = []byte{0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00}

// var searchPattern2 = []byte{0x00, 0x08, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
//var searchPattern3 = []byte{0x73, 0x59, 0x4D, 0x42, 0x4F, 0x4C, 0x74, 0x41, 0x42, 0x4C, 0x45, 0x00} // 12

func getOffsets(data []byte, cb func(string)) (bool, int, int, int, error) {
	addressTableOffset := BytePatternSearch(data, searchPattern, 0x30000) - 0x06
	cb(fmt.Sprintf("Address table offset: %08X", addressTableOffset))

	sramTableOffset := getAddressFromOffset(data, addressTableOffset-0x04)
	cb(fmt.Sprintf("SRAM table offset: %08X", sramTableOffset))

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

func getSymbolListOffSet(data []byte) (int, error) {
	zerocount := 0
	for pos := 0; pos < len(data); pos++ {
		if data[pos] == 0x00 {
			zerocount++
		} else {
			if zerocount < 15 {
				zerocount = 0
			} else {
				return pos, nil
			}
		}
	}
	return -1, errors.New("Symbol list not found")
}

func readMarkerAddressContent(data []byte, marker byte) (length, retval, val int, err error) {
	fileoffset := len(data) - 0x201
	inb := data[len(data)-0x201:]

	if len(inb) != 0x201 {
		err = fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", len(inb), 0x201)
		return
	}
	for t := 0; t < 0x201; t++ {
		if inb[t] == marker && inb[t+1] < 0x30 {
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
	length, retval, _, err := readMarkerAddressContent(data, 0x9B)
	if err != nil {
		panic(err)
	}
	//log.Printf("Length: %d, Retval: %X, Val: %X", length, retval, val)
	if retval > 0 && length < filelength && length > 0 {
		return true
	}
	return false
}

func GetT7HeaderField(bin []byte, id byte) ([]byte, error) {
	binLength := len(bin)
	var answer []byte
	addr := binLength - 1
	var found bool
	for addr > (binLength - 0x1FF) {
		/* The first byte is the length of the data */
		fieldLength := bin[addr]
		//log.Printf("%3d, %x", lengthField, lengthField)
		if fieldLength == 0x00 || fieldLength == 0xFF {
			break
		}
		addr--

		/* Second byte is an ID field */
		fieldID := bin[addr]
		addr--

		if fieldID == id {
			answer = make([]byte, int(fieldLength))
			answer[fieldLength-1] = 0x00
			//answer[fieldLength] = 0x00
			for i := 0; i < int(fieldLength); i++ {
				answer[i] = bin[addr]
				addr--
			}
			//			log.Printf("0x%02x %d> %q", fieldID, len(answer), string(answer))
			found = true
			//break
			// when this return is commented out, the function will
			// find the last field if there are several (mainly this
			// is for searching for the last VIN field)
			// return 1;
		}
		addr -= int(fieldLength)
	}
	if found {
		return answer, nil
	}
	return nil, fmt.Errorf("did not find header for id 0x%02x", id)
}

type T7HeaderField struct {
	ID     byte
	Length byte
	Data   []byte
}

func (h *T7HeaderField) String() string {
	if h.Length == 4 {
		return fmt.Sprintf("0x%02x %d> 0x%08X  %q", h.ID, len(h.Data), binary.BigEndian.Uint32(h.Data), h.Data)
	} else if h.Length == 2 {
		return fmt.Sprintf("0x%02x %d> 0x%04X  %q", h.ID, len(h.Data), binary.BigEndian.Uint16(h.Data), h.Data)
	} else {
		return fmt.Sprintf("0x%02x> %s", h.ID, string(h.Data))
	}
}

func GetAllT7HeaderFields(bin []byte) []*T7HeaderField {
	binLength := len(bin)
	addr := binLength - 1

	fields := make([]*T7HeaderField, 0)

	for addr > (binLength - 0x1FF) {
		//		log.Printf("addr: %X", addr)
		/* The first byte is the length of the data */
		fieldLength := bin[addr]
		//		log.Printf("fieldLength %X", fieldLength)
		//log.Printf("%3d, %x", lengthField, lengthField)
		if fieldLength == 0x00 || fieldLength == 0xFF {
			break
		}
		addr--

		fieldID := bin[addr]
		addr--

		fieldData := make([]byte, int(fieldLength))
		fieldData[fieldLength-1] = 0x00

		for i := 0; i < int(fieldLength); i++ {
			fieldData[i] = bin[addr]
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
