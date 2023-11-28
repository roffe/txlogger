package symbol

import (
	"log"
)

const (
	T7Length      = 0x80000
	T7SRAMAddress = 0x0F00000
)

type T7File struct {
	autoFixFooter bool
	data          []byte // the raw data

	chassisIDDetected           bool
	immocodeDetected            bool
	symbolTableMarkerDetected   bool
	symbolTableChecksumDetected bool
	f2ChecksumDetected          bool
	chassisIDCounter            int

	symbolTableAddress int
	sramOffset         int
	checksumF2         int
	checksumFB         int

	bottomOfFlash   int
	romChecksumType int
	fwLength        int

	valueF5          int
	valueF6          int
	valueF7          int
	valueF8          int
	romChecksumError byte

	chassisID       string
	immobilizerID   string
	softwareVersion string
	carDescription  string
	partNumber      string
	engineType      string
	vehicleIDNr     string
	dateModified    string
	ecuHardwareNr   string
	testserialnr    string
	lastModifiedBy  []byte

	csumArea [16]T7ChecksumArea

	*Collection // the symbol collection
}

func NewT7File(data []byte, autoFixFooter bool) (*T7File, error) {
	t7 := &T7File{
		autoFixFooter:   autoFixFooter,
		data:            data,
		chassisID:       "00000000000000000",
		immobilizerID:   "000000000000000",
		engineType:      "0000000000000",
		vehicleIDNr:     "000000000",
		partNumber:      "0000000",
		softwareVersion: "000000000000",
		carDescription:  "00000000000000000000",
		dateModified:    "0000",
		ecuHardwareNr:   "0000000",
		lastModifiedBy:  []byte{0x42, 0xFB, 0xFA, 0xFF, 0xFF},
		testserialnr:    "050225",
	}
	return t7.init()
}

func (t7 *T7File) init() (*T7File, error) {
	t7.loadHeaders()
	symbols, err := LoadT7Symbols(t7.data, func(s string) {
		log.Println(s)
	})
	if err != nil {
		return nil, err
	}
	t7.Collection = symbols
	return t7, t7.VerifyChecksum()
}

func (t7 *T7File) UpdateSymbol(sym *Symbol) error {
	addr := sym.Address
	if sym.Address > 0x7FFFFF {
		if sym.Address-sym.SramOffset > uint32(len(t7.data)) {
			return ErrAddressOutOfRange
		}
		addr = sym.Address - sym.SramOffset
	}
	log.Printf("Updating Symbol %s at %X, %X", sym.Name, addr, sym.data)
	copy(t7.data[addr:addr+uint32(sym.Length)], sym.data[:sym.Length])
	return nil
}

func (t7 *T7File) Bytes() []byte {
	return t7.data
}
