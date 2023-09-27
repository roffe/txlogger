package symbol

import (
	"bufio"
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
	"syscall"
	"unsafe"

	"github.com/roffe/txlogger/pkg/blowfish"
)

type Symbol struct {
	Name string

	Number int

	Address uint32
	Length  uint16
	Mask    uint16
	Type    uint8

	Correctionfactor float64
	Unit             string

	data []byte
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

func (s *Symbol) Bytes() []byte {
	return s.data
}

func (s *Symbol) String() string {
	return fmt.Sprintf("%s #%d @%08X type: %02X len: %d", s.Name, s.Number, s.Address, s.Type, s.Length)
}

func LoadSymbols(filename string, cb func(string)) ([]*Symbol, error) {
	return ExtractFile(cb, filename)
}

func ExtractFile(cb func(string), filename string) ([]*Symbol, error) {
	if filename == "" {
		return nil, errors.New("no filename given")
	}

	file, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	//fstats, err := file.Stat()
	//if err != nil {
	//	return nil, err
	//}

	//	symbol_collection := make(map[string]Symbol)

	if !IsBinaryPackedVersion(file, 0x9B) {
		return nil, errors.New("non binary packed not implemented, send your bin to Roffe")
		//log.Println("Not a binary packed version")
		//cb("Not a binary packed symbol table")
		//if err := nonBinaryPacked(cb, file, fstats); err != nil {
		//	return nil, err
		//}
	} else {
		//log.Println("Binary packed version")
		cb("Found binary packed symbol table")
		return BinaryPacked(cb, file)

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

func BinaryPacked(cb func(string), file io.ReadSeeker) ([]*Symbol, error) {
	compr_created, addressTableOffset, compressedSymbolTable, err := extractCompressedSymbolTable(cb, file)
	if err != nil {
		if err.Error() != "symbol name table not found" {
			return nil, err
		}
	}

	//os.WriteFile("compressedSymbolNameTable.bin", compressedSymbolTable, 0644)
	if addressTableOffset == -1 {
		return nil, errors.New("could not find addressTableOffset table")
	}

	//ff, err := os.Create("compressedSymbolTable.bin")
	//if err != nil {
	//	return nil, err
	//}
	//defer ff.Close()
	file.Seek(int64(addressTableOffset), io.SeekStart)

	var (
		symb_count int
		symbols    []*Symbol
	)

	for {
		buff := make([]byte, 10)
		n, err := file.Read(buff)
		if err != nil {
			return nil, err
		}
		if n != 10 {
			return nil, errors.New("binaryPacked: not enough bytes read")
		}

		if int32(buff[0]) != 0x53 && int32(buff[1]) != 0x43 { // SC
			symbols = append(symbols, NewFromT7Bytes(buff, symb_count))
			symb_count++
		} else {
			file.Seek(0, io.SeekCurrent)
			// if pos, err := file.Seek(0, io.SeekCurrent); err == nil {
			// 	log.Printf("EOT: %X", pos-0xA)
			// }
			break
		}

	}
	//log.Println("Symbols found: ", symb_count)
	cb(fmt.Sprintf("Loaded %d symbols from binary", symb_count))

	if compr_created {
		if bytes.HasPrefix(compressedSymbolTable, []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
			return nil, errors.New("compressed symbol table is not present")
		}
		//log.Println("Decoding packed symbol table")
		//cb("Decoding packed symbol table")
		symbolNames, err := ExpandCompressedSymbolNames(compressedSymbolTable)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(symbolNames)-1; i++ {
			symbols[i].Name = strings.TrimSpace(symbolNames[i])
			symbols[i].Unit = GetUnit(symbols[i].Name)
			symbols[i].Correctionfactor = GetCorrectionfactor(symbols[i].Name)
		}
		if err := readAllSymbolsData(file, symbols); err != nil {
			return symbols, err
		}
		return symbols, nil
	}

	ver, err := determineVersion(file)
	if err != nil {
		if err.Error() == "not found" {
			cb("Could not determine binary version")
			cb("Lload symbols from XML")
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
	if err := readAllSymbolsData(file, symbols); err != nil {
		return nil, err
	}
	/*
		ff2, err := os.OpenFile("symbols.txt", os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		defer ff2.Close()

		for _, s := range symbols {
			ff2.WriteString(fmt.Sprintf("%s\n", s.String()))
		}
	*/

	return symbols, nil

}

func readAllSymbolsData(file io.ReadSeeker, symbols []*Symbol) error {
	file.Seek(0, io.SeekStart)
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	file.Seek(0, io.SeekStart)

	dataLocationOffset := bytePatternSearch(file, searchPattern3, 0x30000) + 12

	file.Seek(int64(dataLocationOffset), io.SeekStart)
	var dataOffsetValue uint32
	if err := binary.Read(file, binary.BigEndian, &dataOffsetValue); err != nil {
		return err
	}

	//log.Printf("atx %X OV: %X", dataLocationOffset, dataOffsetValue)

	for _, sym := range symbols {
		if sym.Address-dataOffsetValue > uint32(len(fileBytes)) {
			//log.Printf("symbol address out of range: %s", sym.String())
			continue
		}
		sym.data, err = readSymbolData(fileBytes, sym, dataOffsetValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func readSymbolData(file []byte, s *Symbol, offset uint32) ([]byte, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("%s, error reading symbol data: %v", s.String(), err)
		}
	}()
	symData := make([]byte, s.Length)
	copy(symData, file[s.Address-offset:(s.Address-offset)+uint32(s.Length)])
	return symData, nil
}

func determineVersion(file io.ReadSeeker) (string, error) {
	file.Seek(0, io.SeekStart)
	b, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	switch {
	case bytes.Contains(b, []byte("EU0CF01O")):
		return "EU0CF01O", nil
	case bytes.Contains(b, []byte("EU0AF01C")), bytes.Contains(b, []byte("EU0BF01C")), bytes.Contains(b, []byte("EU0CF01C")):
		return "EU0AF01C", nil
	}
	file.Seek(0, io.SeekStart)
	return "", errors.New("not found")
}

var searchPattern = []byte{0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00}

// var searchPattern2 = []byte{0x00, 0x08, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
var searchPattern3 = []byte{0x73, 0x59, 0x4D, 0x42, 0x4F, 0x4C, 0x74, 0x41, 0x42, 0x4C, 0x45, 0x00} // 12

//var readNo int

func ExpandCompressedSymbolNames(in []byte) ([]string, error) {
	//os.WriteFile("compressedSymbolTable.bin", in, 0644)
	if bytes.HasPrefix(in, []byte{0xF1, 0x1A, 0x06, 0x5B, 0xA2, 0x6B, 0xCC, 0x6F}) {
		return blowfish.DecryptSymbolNames(in)
	}
	var expandedFileSize int
	for i := 0; i < 4; i++ {
		expandedFileSize |= int(in[i]) << uint(i*8)
	}

	if expandedFileSize == -1 {
		return nil, errors.New("invalid expanded file size")
	}

	out := make([]byte, expandedFileSize)

	path, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dll, err := syscall.LoadDLL(path + `\lzhuf.dll`)
	if err != nil {
		return nil, err
	}
	defer dll.Release()

	decode, err := dll.FindProc("Decode")
	if err != nil {
		log.Println(err)
		return nil, fmt.Errorf("error finding Decode in lzhuf.dll: %w", err)
	}

	r0, r1, err := decode.Call(uintptr(unsafe.Pointer(&in[0])), uintptr(unsafe.Pointer(&out[0])))
	if r1 == 0 {
		if err != nil {
			return nil, fmt.Errorf("error decoding compressed symbol table: %w", err)
		}
	}

	if int(r0) != expandedFileSize {
		return nil, fmt.Errorf("decoded data size missmatch: %d != %d", r0, expandedFileSize)
	}

	return strings.Split(strings.TrimSuffix(string(out), "\r\n"), "\r\n"), nil
}

func extractCompressedSymbolTable(cb func(string), file io.ReadSeeker) (bool, int, []byte, error) {

	addressTableOffset := bytePatternSearch(file, searchPattern, 0x30000) - 0x06
	//	log.Printf("Address table offset: %08X", addressTableOffset)
	cb(fmt.Sprintf("Address table offset: %08X", addressTableOffset))

	//sramTableOffset := getAddressFromOffset(file, addressTableOffset-0x04)
	//log.Printf("SRAM table offset: %08X", sramTableOffset)
	//cb(fmt.Sprintf("SRAM table offset: %08X", sramTableOffset))

	symbolNameTableOffset := getAddressFromOffset(file, addressTableOffset)
	//	log.Printf("Symbol table offset: %08X", symbolNameTableOffset)
	cb(fmt.Sprintf("Symbol table offset: %08X", symbolNameTableOffset))

	symbolTableLength := getLengthFromOffset(file, addressTableOffset+0x04)
	//	log.Printf("Symbol table length: %08X", symbolTableLength)
	cb(fmt.Sprintf("Symbol table length: %08X", symbolTableLength))

	if symbolTableLength > 0x1000 && symbolNameTableOffset > 0 && symbolNameTableOffset < 0x70000 {
		file.Seek(int64(symbolNameTableOffset), io.SeekStart)
		compressedSymbolTable := make([]byte, symbolTableLength)
		n, err := file.Read(compressedSymbolTable)
		if err != nil {
			return false, -1, nil, err
		}
		if n != symbolTableLength {
			return false, -1, nil, errors.New("did not read enough bytes for symbol table")
		}
		return true, addressTableOffset, compressedSymbolTable, nil
	}
	return false, addressTableOffset, nil, errors.New("symbol name table not found")
}

func getLengthFromOffset(file io.ReadSeeker, offset int) int {
	file.Seek(int64(offset), io.SeekStart)
	var val uint16
	if err := binary.Read(file, binary.BigEndian, &val); err != nil {
		panic(err)
	}
	return int(val)
}

func getAddressFromOffset(file io.ReadSeeker, offset int) int {
	file.Seek(int64(offset), io.SeekStart)
	var val uint32
	if err := binary.Read(file, binary.BigEndian, &val); err != nil {
		panic(err)
	}
	return int(val)
}

func bytePatternSearch(f io.ReadSeeker, search []byte, startOffset int64) int {
	f.Seek(startOffset, io.SeekStart)
	ix := 0
	r := bufio.NewReader(f)
	for ix < len(search) {
		b, err := r.ReadByte()
		if err != nil {
			return -1
		}
		if search[ix] == b {
			ix++
		} else {
			ix = 0
		}
		startOffset++
	}
	f.Seek(0, io.SeekStart) // Seeks to the beginning
	return int(startOffset - int64(len(search)))
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

func ReadMarkerAddressContent(file *os.File, value byte, filelength int) (length, retval, val int, err error) {
	s, err := file.Stat()
	if err != nil {
		return
	}
	fileoffset := int(s.Size() - 0x90)

	file.Seek(int64(fileoffset), 0)
	inb := make([]byte, 0x90)
	n, err := file.Read(inb)
	if err != nil {
		return
	}
	if n != 0x90 {
		err = fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", n, 0x90)
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

	file.Seek(int64(retval-length), 0)
	info := make([]byte, length)
	n, err = file.Read(info)
	if err != nil {
		return
	}
	if n != length {
		err = fmt.Errorf("ReadMarkerAddressContent: read %d bytes, expected %d", n, length)
		return
	}
	for bc := 0; bc < length; bc++ {
		val <<= 8
		val |= int(info[bc])
	}
	//log.Printf("%X", val)
	return
}

func IsBinaryPackedVersion(file *os.File, filelength int) bool {
	length, retval, _, err := ReadMarkerAddressContent(file, 0x9B, filelength)
	if err != nil {
		panic(err)
	}
	//log.Printf("Length: %d, Retval: %X, Val: %X", length, retval, val)
	if retval > 0 && length < filelength && length > 0 {
		return true
	}
	return false
}
