package cansettings

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/layout"
)

const (
	prefsAdapter = "adapter"
	prefsPort    = "port"
	prefsSpeed   = "speed"
	prefsDebug   = "debug"

	MinimumtxbridgeVersion = "1.0.8"
)

var portSpeeds = []string{"9600", "19200", "38400", "57600", "115200", "230400", "460800", "921600", "1mbit", "2mbit", "3mbit"}

type Widget struct {
	widget.BaseWidget
	app             fyne.App
	adapterSelector *widget.Select
	debugCheckbox   *widget.Check
	portSelector    *widget.Select
	speedSelector   *widget.Select
	refreshBtn      *widget.Button
	adapters        map[string]*gocan.AdapterInfo

	mu sync.Mutex
}

func NewCANSettingsWidget() *Widget {
	csw := &Widget{
		app:      fyne.CurrentApp(),
		adapters: make(map[string]*gocan.AdapterInfo),
	}
	csw.ExtendBaseWidget(csw)

	csw.adapterSelector = widget.NewSelect([]string{}, func(s string) {
		if info, found := csw.adapters[s]; found {
			csw.app.Preferences().SetString(prefsAdapter, s)
			if info.RequiresSerialPort {
				csw.portSelector.Enable()
				csw.speedSelector.Enable()
				return
			}
			csw.portSelector.Disable()
			csw.speedSelector.Disable()
		}
	})

	csw.portSelector = widget.NewSelect(csw.ListPorts(), func(s string) {
		csw.app.Preferences().SetString(prefsPort, s)
	})
	csw.speedSelector = widget.NewSelect(portSpeeds, func(s string) {
		csw.app.Preferences().SetString(prefsSpeed, s)
	})

	csw.debugCheckbox = widget.NewCheck("Debug", func(b bool) {
		csw.app.Preferences().SetBool(prefsDebug, b)
	})

	csw.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		csw.portSelector.Options = csw.ListPorts()
		csw.portSelector.Refresh()
	})

	for _, adapter := range gocan.ListAdapters() {
		csw.adapters[adapter.Name] = &adapter
	}

	csw.loadPrefs()
	return csw
}

func (c *Widget) AddAdapters(adapters []*proto.AdapterInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range adapters {
		adapter := &gocan.AdapterInfo{
			Name:        a.GetName(),
			Description: a.GetDescription(),
			Capabilities: gocan.AdapterCapabilities{
				HSCAN: a.GetCapabilities().GetHSCAN(),
				SWCAN: a.GetCapabilities().GetSWCAN(),
				KLine: a.GetCapabilities().GetKLine(),
			},
			RequiresSerialPort: a.GetRequireSerialPort(),
		}

		if _, found := c.adapters[adapter.Name]; found {
			continue
		}
		c.adapters[adapter.Name] = adapter
	}
	names := make([]string, 0, len(c.adapters))
	for name := range c.adapters {
		names = append(names, name)
	}
	slices.SortFunc(names, func(i, j string) int {
		return strings.Compare(strings.ToLower(i), strings.ToLower(j))
	})
	c.adapterSelector.Options = names
	c.adapterSelector.Refresh()
	if ad := c.app.Preferences().String(prefsAdapter); ad != "" {
		c.adapterSelector.SetSelected(ad)
	}
}

func (c *Widget) Disable() {
	c.adapterSelector.Disable()
	c.portSelector.Disable()
	c.speedSelector.Disable()
	c.debugCheckbox.Disable()
	c.refreshBtn.Disable()
}

func (c *Widget) Enable() {
	c.adapterSelector.Enable()
	c.portSelector.Enable()
	c.speedSelector.Enable()
	c.debugCheckbox.Enable()
	c.refreshBtn.Enable()

	if info, found := c.adapters[c.adapterSelector.Selected]; found {
		if info.RequiresSerialPort {
			c.portSelector.Enable()
			c.speedSelector.Enable()
		} else {
			c.portSelector.Disable()
			c.speedSelector.Disable()
		}
	}
}

func (cs *Widget) GetAdapterName() string {
	return cs.adapterSelector.Selected
}

func (cs *Widget) GetSerialPort() string {
	return cs.portSelector.Selected
}

func (cs *Widget) loadPrefs() {
	if adapter := cs.app.Preferences().String(prefsAdapter); adapter != "" {
		cs.adapterSelector.SetSelected(adapter)
	}
	if port := cs.app.Preferences().String(prefsPort); port != "" {
		cs.portSelector.SetSelected(port)
	}
	if speed := cs.app.Preferences().String(prefsSpeed); speed != "" {
		cs.speedSelector.SetSelected(speed)
	}
	if debug := cs.app.Preferences().Bool(prefsDebug); debug {
		cs.debugCheckbox.SetChecked(debug)
	}
}

func (cs *Widget) GetAdapter(ecuType string, logger func(string)) (gocan.Adapter, error) {
	baudstring := cs.speedSelector.Selected
	switch baudstring {
	case "1mbit":
		baudstring = "1000000"
	case "2mbit":
		baudstring = "2000000"
	case "3mbit":
		baudstring = "3000000"
	}

	baudrate, err := strconv.Atoi(baudstring)

	if cs.adapterSelector.Selected == "" {
		return nil, errors.New("Select CANbus adapter in settings") //lint:ignore ST1005 This is ok
	}

	if cs.adapters[cs.adapterSelector.Selected].RequiresSerialPort {
		if cs.portSelector.Selected == "" {
			return nil, errors.New("Select port in setings") //lint:ignore ST1005 This is ok

		}
		if cs.speedSelector.Selected == "" {
			return nil, errors.New("Select port speed in settings") //lint:ignore ST1005 This is ok
		}
	}

	if err != nil {
		if cs.speedSelector.Selected != "" {
			return nil, err
		}
	}

	var canFilter []uint32
	var canRate float64

	switch ecuType {
	case "T5":
		canFilter = []uint32{0xC}
		canRate = 615.384
	case "T7":
		if strings.Contains(cs.adapterSelector.Selected, "STN") || strings.Contains(cs.adapterSelector.Selected, "OBDLink") || strings.HasSuffix(cs.adapterSelector.Selected, "Wifi") {
			canFilter = []uint32{0x238, 0x258, 0x270}
		} else {
			canFilter = []uint32{0x180, 0x1A0, 0x238, 0x258, 0x270, 0x280, 0x3A0, 0x664, 0x665}
		}

		canRate = 500
	case "T8":
		if strings.Contains(cs.adapterSelector.Selected, "STN") || strings.Contains(cs.adapterSelector.Selected, "OBDLink") {
			canFilter = []uint32{0x7e8}
		} else {
			canFilter = []uint32{0x180, 0x7e8, 0x664, 0x665}
		}
		canRate = 500
	}
	var minimumVersion string
	if strings.HasPrefix(cs.adapterSelector.Selected, "txbridge") {
		minimumVersion = MinimumtxbridgeVersion
	}

	gocan.ListAdapters()

	if strings.HasPrefix(cs.adapterSelector.Selected, "J2534") || strings.HasPrefix(cs.adapterSelector.Selected, "CANlib") { // || (strings.HasPrefix(cs.adapterSelector.Selected, "CANUSB ") && cs.adapterSelector.Selected != "CANUSB VCP") {
		return gocan.NewGWClient(
			cs.adapterSelector.Selected,
			&gocan.AdapterConfig{
				Port:                   cs.portSelector.Selected,
				PortBaudrate:           baudrate,
				CANRate:                canRate,
				CANFilter:              canFilter,
				OnMessage:              logger,
				Debug:                  cs.debugCheckbox.Checked,
				MinimumFirmwareVersion: minimumVersion,
				PrintVersion:           true,
			},
		)
	} else {
		return gocan.NewAdapter(
			cs.adapterSelector.Selected,
			&gocan.AdapterConfig{
				Port:                   cs.portSelector.Selected,
				PortBaudrate:           baudrate,
				CANRate:                canRate,
				CANFilter:              canFilter,
				OnMessage:              logger,
				Debug:                  cs.debugCheckbox.Checked,
				MinimumFirmwareVersion: minimumVersion,
				PrintVersion:           true,
			},
		)
	}
}

func (cs *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewVBox(
		container.NewBorder(
			nil,
			nil,
			layout.NewFixedWidth(70, widget.NewLabel("Adapter")),
			cs.debugCheckbox,
			cs.adapterSelector,
		),
		container.NewBorder(
			nil,
			nil,
			layout.NewFixedWidth(70, widget.NewLabel("Port")),
			cs.refreshBtn,
			cs.portSelector,
		),
		container.NewBorder(
			nil,
			nil,
			layout.NewFixedWidth(70, widget.NewLabel("Speed")),
			nil,
			cs.speedSelector,
		),
	))
}
