package t5

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/roffe/txlogger/pkg/model"
)

func (t *Client) Info(ctx context.Context) ([]model.HeaderResult, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return nil, err
		}
	}

	footer, err := t.GetECUFooter(ctx)
	if err != nil {
		return nil, err
	}

	var out []model.HeaderResult
	for _, d := range T5Headers {
		h := GetIdentifierFromFooter(footer, d.ID)
		a := model.HeaderResult{Value: h}
		a.Desc = d.Desc
		a.ID = d.ID
		out = append(out, a)
	}

	if chip, err := t.GetChipTypes(ctx); err == nil {
		chips := model.HeaderResult{Value: chipDescription(chip[4], chip[5])}
		chips.Desc = "FLASH Chips"
		out = append(out, chips)
	}
	return out, nil
}

// Manufacturer and device ids as documented in MyBooty.asm.
var (
	chipMakes = map[byte]string{
		0x89: "Intel",
		0x01: "AMD",
		0x31: "CSI/Catalyst",
		0x1F: "Atmel",
		0xBF: "SST",
		0x20: "ST",
		0x37: "AMIC",
	}
	chipDevices = map[byte]string{
		0xB8: "28F512",
		0xB4: "28F010",
		0x25: "28F512",
		0xA7: "28F010",
		0x20: "29F010",
		0x5D: "29C512",
		0xD5: "29C010",
		0xB5: "39SF010A",
		0xA4: "A29010L",
	}
)

func chipDescription(manufacturer, device byte) string {
	maker, ok := chipMakes[manufacturer]
	if !ok {
		maker = fmt.Sprintf("unknown make 0x%02X", manufacturer)
	}
	dev, ok := chipDevices[device]
	if !ok {
		dev = fmt.Sprintf("unknown device 0x%02X", device)
	}
	if size := flashSizeFromChip(device); size > 0 {
		return fmt.Sprintf("%s %s (%d kB)", maker, dev, size)
	}
	return fmt.Sprintf("%s %s", maker, dev)
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	res, err := t.Info(ctx)
	if err != nil {
		return err
	}

	//	log.Println("----- ECU info ---------------")
	if err := t.printECUType(ctx); err != nil {
		return err
	}

	for _, r := range res {
		log.Println(r.Desc, r.Value)
	}
	//log.Println("------------------------------")
	return nil
}

func (t *Client) printECUType(ctx context.Context) error {
	typ, err := t.DetermineECU(ctx)
	if err != nil {
		return err
	}
	switch typ {
	case T52ECU:
		t.cfg.OnMessage("This is a Trionic 5.2 ECU with 128 kB of FLASH")
	case T55AST52:
		t.cfg.OnMessage("This is a Trionic 5.5 ECU with a T5.2 BIN")
	case T55ECU:
		t.cfg.OnMessage("This is a Trionic 5.5 ECU with 256 kB of FLASH")
	default:
		return errors.New("printECUType: unknown ECU")
	}
	return nil
}

// flashSizeFromChip maps the FLASH device id (last byte of the C9 reply) to
// the flash size in kB. Returns 0 for unknown chips. The ids match what
// MyBooty itself recognises for erase/programming.
func flashSizeFromChip(deviceID byte) uint16 {
	switch deviceID {
	case 0xB8, // Intel/CSI/OnSemi 28F512
		0x5D, // Atmel 29C512
		0x25: // AMD 28F512
		return 128
	case 0xD5, // Atmel 29C010
		0xB5, // SST 39F010
		0xB4, // Intel/CSI/OnSemi 28F010
		0xA7, // AMD 28F010
		0xA4, // AMIC 29F010
		0x20: // AMD/ST 29F010
		return 256
	}
	return 0
}

func (t *Client) DetermineECU(ctx context.Context) (ECUType, error) {
	footer, err := t.GetECUFooter(ctx)
	if err != nil {
		return UnknownECU, err
	}

	chip, err := t.GetChipTypes(ctx)
	if err != nil {
		return UnknownECU, err
	}

	romoffset := GetIdentifierFromFooter(footer, ROMoffset)

	flashsize := flashSizeFromChip(chip[5])

	switch flashsize {
	case 128:
		switch romoffset {
		case "060000":
			return T52ECU, nil
		default:
			return UnknownECU, errors.New("!!! ERROR !!! This is a Trionic 5.2 ECU running an unknown firmware")
		}
	case 256:
		switch romoffset {
		case "040000":
			return T55ECU, nil
		case "060000":
			return T55AST52, nil
		default:
			return UnknownECU, errors.New("!!! ERROR !!! This is a Trionic 5.5 ECU running an unknown firmware")
		}
	}

	return UnknownECU, errors.New("!!! ERROR !!! this is a unknown ECU")
}

var T5Headers = []model.Header{
	{Desc: "Part Number", ID: 0x01},
	{Desc: "Software ID", ID: 0x02},
	{Desc: "SW Version", ID: 0x03},
	{Desc: "Engine Type", ID: 0x04},
	{Desc: "IMMO Code", ID: 0x05},
	{Desc: "Other Info", ID: 0x06},
	{Desc: "ROM Start", ID: 0xFD},
	{Desc: "ROM End", ID: 0xFC},
	{Desc: "Code End", ID: 0xFE},
}
