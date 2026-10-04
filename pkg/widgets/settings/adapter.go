package settings

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ota"
	"github.com/roffe/txlogger/pkg/wbl"
)

// GetAdapter returns the configured adapter set up for talking to the named
// ECU (see ecu.Profile).
func (sw *Widget) GetAdapter(ecuName string) (gocan.Adapter, error) {
	p := ecu.GetProfile(ecuName)
	if p.CANFilter == nil {
		return nil, fmt.Errorf("%s can't be reached over CAN", ecuName)
	}
	adapterName := prefAdapter.get()
	filter := p.CANFilter(adapterName)
	// STN/ELM dongles have no passive listen mode, so a CAN wideband can't
	// work on them; its ids would only widen their single ATCF/ATCM mask
	// until broadcasts (0x1A0, 0x3A0...) fill the STPX r: reply slots.
	if p.CANWideband && !ecu.IsOBDAdapter(adapterName) {
		filter = append(filter, wbl.CANIDs(prefWblSource.get(), prefWBLPort.get())...)
	}
	return sw.GetAdapterWith(p.CANRate, filter)
}

// GetAdapterWith returns the configured adapter set up with an explicit CAN
// rate (kbit/s) and acceptance filter.
func (sw *Widget) GetAdapterWith(canRate float64, canFilter []uint32) (gocan.Adapter, error) {
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
