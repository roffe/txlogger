package cansettings

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/client"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/layout"
	"go.bug.st/serial/enumerator"
)

const (
	prefsAdapter = "adapter"
	prefsPort    = "port"
	prefsSpeed   = "speed"
	prefsDebug   = "debug"

	minimumtxbridgeVersion = "1.0.6"
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
	adapters        map[string]*adapter.AdapterInfo

	mu sync.Mutex
}

func NewCanSettingsWidget() *Widget {
	csw := &Widget{
		app:      fyne.CurrentApp(),
		adapters: make(map[string]*adapter.AdapterInfo),
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

	csw.loadPrefs()
	return csw
}

func (c *Widget) AddAdapters(adapters []*proto.AdapterInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range adapters {
		adapter := &adapter.AdapterInfo{
			Name:        a.GetName(),
			Description: a.GetDescription(),
			Capabilities: adapter.AdapterCapabilities{
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
	sort.Strings(names)
	c.adapterSelector.Options = names
	c.adapterSelector.Refresh()
	if ad := c.app.Preferences().String(prefsAdapter); ad != "" {
		c.adapterSelector.SetSelected(ad)
	}
}

func (c *Widget) AddAdapter(adapter *adapter.AdapterInfo) {

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

	if adapter.GetAdapterMap()[cs.adapterSelector.Selected].RequiresSerialPort {
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
		if strings.HasPrefix(cs.adapterSelector.Selected, "STN") || strings.HasPrefix(cs.adapterSelector.Selected, "OBDLink") || strings.HasSuffix(cs.adapterSelector.Selected, "Wifi") {
			canFilter = []uint32{0x238, 0x258, 0x270}
		} else {
			canFilter = []uint32{0x180, 0x1A0, 0x238, 0x258, 0x270, 0x280, 0x3A0, 0x664, 0x665}
		}

		canRate = 500
	case "T8":
		if strings.HasPrefix(cs.adapterSelector.Selected, "STN") || strings.HasPrefix(cs.adapterSelector.Selected, "OBDLink") {
			canFilter = []uint32{0x7e8}
		} else {
			canFilter = []uint32{0x180, 0x7e8, 0x664, 0x665}
		}
		canRate = 500
	}
	var minimumVersion string
	if cs.adapterSelector.Selected == "txbridge" {
		minimumVersion = minimumtxbridgeVersion
	}
	return client.New(
		cs.adapterSelector.Selected,
		&gocan.AdapterConfig{
			Port:         cs.portSelector.Selected,
			PortBaudrate: baudrate,
			CANRate:      canRate,
			CANFilter:    canFilter,
			OnMessage:    logger,
			Debug:        cs.debugCheckbox.Checked,
			OnError: func(err error) {
				logger(err.Error())
			},
			MinimumFirmwareVersion: minimumVersion,
		},
	)
}

func (cs *Widget) ListPorts() []string {
	var portsList []string
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		//m.output(err.Error())
		return []string{}
	}
	if len(ports) == 0 {
		//m.output("No serial ports found!")
		return []string{}
	}

	for _, port := range ports {
		//m.output(fmt.Sprintf("Found port: %s", port.Name))
		//if port.IsUSB {
		//m.output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
		//m.output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
		portsList = append(portsList, port.Name)
		//}
	}

	sort.Strings(portsList)

	return portsList
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
