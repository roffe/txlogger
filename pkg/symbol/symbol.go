package symbol

//#include "lzh.cfile"
import "C"
import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

type Symbol struct {
	data             []byte
	Name             string
	Number           int
	Type             uint8
	Address          uint32
	Length           uint16
	Correctionfactor string
	Unit             string
}

func NewFromData(data []byte, symb_count int) *Symbol {
	var internall_address uint32
	for i := 0; i < 4; i++ {
		internall_address <<= 8
		internall_address |= uint32(data[i])
	}
	var symbol_length uint16
	if symb_count == 0 {
		symbol_length = 0x08
	} else {
		for i := 4; i < 6; i++ {
			symbol_length <<= 8
			symbol_length |= uint16(data[i])
		}
	}
	//			log.Printf("Internal address: %X", internall_address)
	//			log.Printf("Symbol length: %X", symbollength)

	symbol_type := data[8]
	return &Symbol{
		Number:  symb_count,
		Type:    symbol_type,
		Name:    "Symbol " + strconv.Itoa(symb_count),
		Address: internall_address,
		Length:  symbol_length,
	}
}

func (s *Symbol) String() string {
	return fmt.Sprintf("%s #%d t:%X @%X len: %d", s.Name, s.Number, s.Type, s.Address, s.Length)
}

func LoadSymbols(filename string) ([]*Symbol, error) {
	return ExtractFile(filename, 0, "")
}

func ExtractFile(filename string, languageID int, m_current_softwareversion string) ([]*Symbol, error) {
	if filename == "" {
		return nil, errors.New("no filename given")
	}

	file, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fstats, err := file.Stat()
	if err != nil {
		return nil, err
	}

	//	symbol_collection := make(map[string]Symbol)

	if !IsBinaryPackedVersion(file, 0x9B) {
		log.Println("Not a binary packed version")
		if err := nonBinaryPacked(file, fstats); err != nil {
			return nil, err
		}
	} else {
		log.Println("Binary packed version")
		return binaryPacked(file)

	}
	return nil, errors.New("not implemented")
}

func nonBinaryPacked(file *os.File, fstats fs.FileInfo) error {
	symbolListOffset, err := GetSymbolListOffSet(file, int(fstats.Size()))
	if err != nil {
		return err
	}
	log.Printf("Symbol list offset: %X", symbolListOffset)
	return nil
}

func binaryPacked(file *os.File) ([]*Symbol, error) {
	compr_created, addressTableOffset, compressedSymbolTable, err := extractCompressedSymbolTable(file)
	if err != nil {
		return nil, err
	}
	if addressTableOffset == -1 {
		return nil, errors.New("could not find addressTableOffset table")
	}

	file.Seek(int64(addressTableOffset), io.SeekStart)
	symb_count := 0

	var symbols []*Symbol

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
			symbols = append(symbols, NewFromData(buff, symb_count))
			symb_count++
		} else {
			if pos, err := file.Seek(0, io.SeekCurrent); err == nil {
				log.Printf("EOT: %X", pos)
			}
			break
		}

	}
	log.Println("Symbol count: ", symb_count)

	if compr_created {
		log.Println("Decoding packed symbol table")
		symbolNames, err := expandCompressedSymbolNames(compressedSymbolTable)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(symbolNames)-1; i++ {
			symbols[i].Name = strings.TrimSpace(symbolNames[i])
			symbols[i].Unit = GetUnit(symbols[i].Name)
			symbols[i].Correctionfactor = GetCorrectionfactor(symbols[i].Name)
		}
	}
	return symbols, nil
}

var searchPattern = []byte{
	0x00, 0x00, 0x04, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x20, 0x00,
}

func expandCompressedSymbolNames(in []byte) ([]string, error) {
	var expandedFileSize int
	for i := 0; i < 4; i++ {
		expandedFileSize |= int(in[i]) << uint(i*8)
	}
	out := make([]byte, expandedFileSize)
	r1, err := C.Decode((*C.uchar)(unsafe.Pointer(&in[0])), (*C.uchar)(unsafe.Pointer(&out[0])))
	if err != nil {
		return nil, fmt.Errorf("error decoding compressed symbol table: %w", err)
	}
	//outB := C.GoBytes(ptr, (C.int)(r1))
	if int(r1) != expandedFileSize {
		return nil, fmt.Errorf("decoded data size missmatch: %d != %d", r1, expandedFileSize)
	}

	return strings.Split(string(out), "\r\n"), nil
}

func extractCompressedSymbolTable(file *os.File) (bool, int, []byte, error) {
	addressTableOffset := bytePatternSearch(file, searchPattern, 0x30000) - 0x06
	log.Printf("Address table offset: %08X", addressTableOffset)

	sramTableOffset := getAddressFromOffset(file, addressTableOffset-0x06)
	log.Printf("SRAM table offset: %08X", sramTableOffset)

	symbolTableOffset := getAddressFromOffset(file, addressTableOffset)
	log.Printf("Symbol table offset: %08X", symbolTableOffset)

	symbolTableLength := getLengthFromOffset(file, addressTableOffset+0x04)
	log.Printf("Symbol table length: %08X", symbolTableLength)
	if symbolTableLength > 0x1000 && symbolTableOffset > 0 && symbolTableOffset < 0x70000 {
		file.Seek(int64(symbolTableOffset), io.SeekStart)
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
	return false, -1, nil, errors.New("ecst: symbol table not found")
}

func getLengthFromOffset(file *os.File, offset int) int {
	file.Seek(int64(offset), io.SeekStart)
	var val uint16
	if err := binary.Read(file, binary.BigEndian, &val); err != nil {
		panic(err)
	}
	return int(val)
}

func getAddressFromOffset(file *os.File, offset int) int {
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
	return
}

func IsBinaryPackedVersion(file *os.File, filelength int) bool {
	length, retval, val, err := ReadMarkerAddressContent(file, 0x9B, filelength)
	if err != nil {
		panic(err)
	}
	log.Printf("Length: %d, Retval: %X, Val: %X", length, retval, val)
	if retval > 0 && length < filelength && length > 0 {
		return true
	}
	return false
}
