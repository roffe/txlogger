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
	data          []byte           // the raw data
	Symbols       SymbolCollection // parsed symbols

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
}

func NewT7File(data []byte, autoFixFooter bool) (*T7File, error) {
	t7 := &T7File{
		autoFixFooter: autoFixFooter,
		data:          data,
		//Symbols:         symbols,
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
	var err error
	t7.Symbols, err = LoadT7Symbols(t7.data, func(s string) {
		log.Println(s)
	})
	if err != nil {
		return nil, err
	}
	return t7, nil
}
