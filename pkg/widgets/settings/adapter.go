package settings

import (
	"errors"
	"strconv"
	"strings"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ota"
)

func (sw *Widget) GetAdapter(ecuType string) (gocan.Adapter, error) {
	return sw.GetAdapterWithExtraFilters(ecuType, nil, false)
}

func (sw *Widget) GetAdapterWithOverrideFilters(ecuType string, filters []uint32) (gocan.Adapter, error) {
	return sw.GetAdapterWithExtraFilters(ecuType, filters, true)
}

func (sw *Widget) GetAdapterWithExtraFilters(ecuType string, filters []uint32, overrideFilters bool) (gocan.Adapter, error) {
	baudrate, err := parseBaudrate(prefSpeed.getOr(""))
	if err != nil {
		return nil, err
	}

	adapterName := prefAdapter.get()
	if adapterName == "" {
		return nil, errors.New("Select CANbus adapter in settings") //lint:ignore ST1005 This is ok
	}

	port := prefPort.get()
	if ad, found := sw.adapters[adapterName]; found && ad.RequiresSerialPort && port == "" && !ad.SerialPortOptional {
		return nil, errors.New("Select port in setings") //lint:ignore ST1005 This is ok
	}

	canFilter, canRate := canFilterAndRate(ecuType, adapterName, filters)

	if overrideFilters && len(filters) > 0 {
		canFilter = filters
	}

	cfg := gocan.Config{
		Port:         port,
		PortBaudrate: baudrate,
		CANRate:      canRate,
		CANFilter:    canFilter,
		Debug:        prefDebug.get(),
	}

	if adapterName == "txbridge wifi" {
		cfg.Extra = map[string]string{
			"minversion": ota.MinimumtxbridgeVersion,
		}
	}

	return gocan.NewAdapter(adapterName, cfg)
}

// parseBaudrate converts a stored port speed (which may use the "Nmbit"
// shorthand or be empty) into a numeric baudrate.
func parseBaudrate(speed string) (int, error) {
	switch speed {
	case "1mbit":
		speed = "1000000"
	case "2mbit":
		speed = "2000000"
	case "3mbit":
		speed = "3000000"
	case "":
		speed = "1000000"
	}
	return strconv.Atoi(speed)
}

// canFilterAndRate returns the CAN acceptance filter and bus rate for the given
// ECU type and adapter. extraFilters are appended for the Trionic 8 family.
func canFilterAndRate(ecuType, adapterName string, extraFilters []uint32) ([]uint32, float64) {
	wblOnCAN := prefWblSource.get() == "CAN"

	switch ecuType {
	case "T5", "Trionic 5":
		return []uint32{0xC}, 615.384

	case "T7", "Trionic 7":
		var filter []uint32
		if isOBDAdapter(adapterName) || strings.HasSuffix(adapterName, "Wifi") {
			filter = []uint32{0x238, 0x258, 0x270}
		} else {
			filter = []uint32{0x1A0, 0x238, 0x258, 0x270, 0x280, 0x3A0, 0x664, 0x665}
		}
		if wblOnCAN {
			filter = append(filter, 0x180)
		}
		return filter, 500

	case "T8", "Trionic 8", "Trionic 8 MCP", "Z22SE", "Z22SE MCP":
		var filter []uint32
		if isOBDAdapter(adapterName) {
			filter = []uint32{0x5E8, 0x7E8}
		} else {
			filter = []uint32{0x5E8, 0x7E8, 0x664, 0x665}
		}
		if wblOnCAN {
			filter = append(filter, 0x180)
		}
		return append(filter, extraFilters...), 500
	}

	return nil, 0
}

func isOBDAdapter(name string) bool {
	return strings.Contains(name, "ELM327") ||
		strings.Contains(name, "STN") ||
		strings.Contains(name, "OBDLink")
}
