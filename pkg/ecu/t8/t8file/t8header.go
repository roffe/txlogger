package t8file

import (
	"fmt"
	"strings"
)

type T8Header struct {
	lowaddress       [32]int
	lowtypes         [32]int
	highaddress      [32]int
	hightypes        [32]int
	fbc              []FlashBlock
	partNumber       string
	serialNumber     string
	programmerDevice string
	programmerName   string
	releaseDate      string
	softwareVersion  string
	hardwareID       string
	deviceType       string
	interfaceDevice  string
	ecuDescription   string
	vin              string
	pin              string
	psk              string
	isk              string
}

func (th *T8Header) DecodeInfo(fileData []byte) {
	if len(fileData) == 0 {
		return
	}

	checksumAreaOffset := int(GetChecksumAreaOffsetFromBytes(fileData))
	endOfPIArea := GetEmptySpaceStartFrom(fileData, checksumAreaOffset)
	length := endOfPIArea - checksumAreaOffset + 1
	piarea := readDataFromBytes(fileData, checksumAreaOffset, length)

	for t := range piarea {
		piarea[t] += 0xD6
		piarea[t] ^= 0x21
	}

	i := 0
	for i < len(piarea)-1 {
		lenVal := int(piarea[i])
		i++
		typeVal := int(piarea[i])
		i++

		if lenVal == 0xF7 && typeVal == 0xF7 {
			break
		}

		if i+lenVal > len(piarea) {
			break
		}

		dataStr := ""
		for range lenVal {
			b := piarea[i]
			i++

			if typeVal == 0x92 || typeVal == 0x97 || typeVal == 0x0C || typeVal == 0xC1 ||
				typeVal == 0x08 || typeVal == 0x1D || typeVal == 0x10 || typeVal == 0x0A ||
				typeVal == 0x0F || typeVal == 0x16 {
				dataStr += string(b)
			} else {
				dataStr += fmt.Sprintf("%02X", b)
			}
		}

		switch typeVal {
		case 0x10:
			th.programmerDevice = dataStr
		case 0x1D:
			th.programmerName = dataStr
		case 0x0A:
			th.releaseDate = dataStr
		case 0x08:
			th.softwareVersion = dataStr
		case 0xC1:
			th.partNumber = dataStr
		case 0x92:
			th.hardwareID = dataStr
		case 0x97:
			th.deviceType = dataStr
		}
	}
}

func (th *T8Header) DecodeExtraInfo(fullFileData []byte) {
	th.fbc = nil
	fileData := readDataFromBytes(fullFileData, 0x4000, 0x4000)
	if len(fileData) < 0x4000 {
		return
	}

	blockNumber := 0

	lowBlkCnt := 0
	lowAddressIdx := 0
	idx := 0x0E
	for range 32 {
		if fileData[idx] != 0x44 || fileData[idx+1] != 0x2A {
			if lowBlkCnt == 0 || lowBlkCnt >= 2 {
				th.lowaddress[lowAddressIdx] = int(fileData[idx])*256 + int(fileData[idx+1])
				th.lowtypes[lowAddressIdx] = int(fileData[idx+5])
				lowAddressIdx++
			}
			lowBlkCnt++
		}
		idx += 6
	}

	highBlkCnt := 0
	highAddressIdx := 0
	idx = 0x200E
	for range 32 {
		if fileData[idx] != 0x44 || fileData[idx+1] != 0x2A {
			if highBlkCnt == 0 || highBlkCnt >= 2 {
				th.highaddress[highAddressIdx] = int(fileData[idx])*256 + int(fileData[idx+1])
				th.hightypes[highAddressIdx] = int(fileData[idx+5])
				highAddressIdx++
			}
			highBlkCnt++
		}
		idx += 6
	}

	processBlocks := func(addresses [32]int, types [32]int) {
		for t := range 32 {
			addr := addresses[t]
			if addr != 0 && addr != 0xFFFF {
				dataOffset := addr - 0x4000
				if dataOffset < 0 || dataOffset+0x130 > len(fileData) {
					continue
				}

				switch types[t] {
				case 0x01, 0x03:
					fb := FlashBlock{
						BlockType:    types[t],
						BlockAddress: addr,
					}
					fb.BlockData = make([]byte, 0x130)
					copy(fb.BlockData, fileData[dataOffset:dataOffset+0x130])

					if fb.isValid() {
						fb.BlockNumber = blockNumber
						th.fbc = append(th.fbc, fb)
						blockNumber++
					}

				case 0xFF:
					if dataOffset+26 <= len(fileData) {
						th.partNumber = strings.TrimSpace(string(fileData[dataOffset : dataOffset+10]))
						th.serialNumber = strings.TrimSpace(string(fileData[dataOffset+10 : dataOffset+26]))
					}
				}
			}
		}
	}

	processBlocks(th.lowaddress, th.lowtypes)
	processBlocks(th.highaddress, th.hightypes)
	//update info based on last flash block
	th.fbc[len(th.fbc)-1].DecodeBlock(th)
}

func (th *T8Header) EncodeExtraInfo() {
	for i, fb := range th.fbc {
		th.fbc[i].BlockData = fb.EncodeBlock(th)
	}
}

func (t *T8Header) FlashBlocks() []FlashBlock { return t.fbc }
func (t *T8Header) PartNumber() string        { return t.partNumber }
func (t *T8Header) SerialNumber() string      { return t.serialNumber }
func (t *T8Header) ProgrammerDevice() string  { return t.programmerDevice }
func (t *T8Header) ProgrammerName() string    { return t.programmerName }
func (t *T8Header) ReleaseDate() string       { return t.releaseDate }
func (t *T8Header) SoftwareVersion() string   { return t.softwareVersion }
func (t *T8Header) HardwareID() string        { return t.hardwareID }
func (t *T8Header) DeviceType() string        { return t.deviceType }
func (t *T8Header) InterfaceDevice() string   { return t.interfaceDevice }
func (t *T8Header) EcuDescription() string    { return t.ecuDescription }
func (t *T8Header) VIN() string               { return t.vin }
func (t *T8Header) PIN() string               { return t.pin }
func (t *T8Header) PSK() string               { return t.psk }
func (t *T8Header) ISK() string               { return t.isk }

func (t *T8Header) SetPartNumber(v string)       { t.partNumber = v }
func (t *T8Header) SetSerialNumber(v string)     { t.serialNumber = v }
func (t *T8Header) SetProgrammerDevice(v string) { t.programmerDevice = v }
func (t *T8Header) SetProgrammerName(v string)   { t.programmerName = v }
func (t *T8Header) SetReleaseDate(v string)      { t.releaseDate = v }
func (t *T8Header) SetSoftwareVersion(v string)  { t.softwareVersion = v }
func (t *T8Header) SetHardwareID(v string)       { t.hardwareID = v }
func (t *T8Header) SetDeviceType(v string)       { t.deviceType = v }
func (t *T8Header) SetInterfaceDevice(v string)  { t.interfaceDevice = v }
func (t *T8Header) SetEcuDescription(v string)   { t.ecuDescription = v }
func (t *T8Header) SetVIN(v string)              { t.vin = v }
func (t *T8Header) SetPIN(v string)              { t.pin = v }
func (t *T8Header) SetPSK(v string)              { t.psk = v }
func (t *T8Header) SetISK(v string)              { t.isk = v }

func (th *T8Header) GetFormattedLog() string {
	var sb strings.Builder

	sb.WriteString("\n=== DECODED ECU INFORMATION ===\n")
	sb.WriteString(fmt.Sprintf("ECU Description:  %s\n", th.ecuDescription))
	sb.WriteString(fmt.Sprintf("Part Number:      %s\n", th.partNumber))
	sb.WriteString(fmt.Sprintf("Serial Number:    %s\n", th.serialNumber))
	sb.WriteString(fmt.Sprintf("VIN:              %s\n", th.vin))
	sb.WriteString(fmt.Sprintf("PIN:              %s\n", th.pin))
	sb.WriteString(fmt.Sprintf("ISK:              %s\n", th.isk))
	sb.WriteString(fmt.Sprintf("PSK:              %s\n", th.psk))

	sb.WriteString("\n--- HARDWARE & SOFTWARE ---\n")
	sb.WriteString(fmt.Sprintf("Hardware ID:      %s\n", th.hardwareID))
	sb.WriteString(fmt.Sprintf("Device Type:      %s\n", th.deviceType))
	sb.WriteString(fmt.Sprintf("Software Ver:     %s\n", th.softwareVersion))
	sb.WriteString(fmt.Sprintf("Release Date:     %s\n", th.releaseDate))

	sb.WriteString("\n--- PROGRAMMER INFO ---\n")
	sb.WriteString(fmt.Sprintf("Programmer Name:  %s\n", th.programmerName))
	sb.WriteString(fmt.Sprintf("Programmer Dev:   %s\n", th.programmerDevice))
	sb.WriteString(fmt.Sprintf("Interface Dev:    %s\n", th.interfaceDevice))

	sb.WriteString("\n--- FLASH BLOCKS ---\n")
	sb.WriteString(fmt.Sprintf("Total Blocks:     %d\n", len(th.fbc)))
	for _, fb := range th.fbc {
		fb.DecodeBlock(th)
		sb.WriteString(fmt.Sprintf("[Block %02d] Addr: 0x%04X, Type: 0x%02X\n",
			fb.BlockNumber, fb.BlockAddress, fb.BlockType))
		sb.WriteString(fmt.Sprintf("\\__ VIN: %s PIN: %s\n", th.vin, th.pin))
		sb.WriteString(fmt.Sprintf(" \\_ ISK: %s PSK: %s\n", th.isk, th.psk))
	}
	sb.WriteString("===============================\n")

	return sb.String()
}
