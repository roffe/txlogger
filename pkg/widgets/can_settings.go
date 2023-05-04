package widgets

import (
	"errors"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"go.bug.st/serial/enumerator"
)

var portSpeeds = []string{"9600", "19200", "38400", "57600", "115200", "230400", "460800", "921600", "1000000", "2000000", "3000000"}

func NewCanSettingsWidget(app fyne.App) *CanSettingsWidget {
	csw := &CanSettingsWidget{
		app: app,
	}

	csw.adapterSelector = widget.NewSelect(adapter.List(), func(s string) {
		//		log.Println("Selected adapter: ", s)
		if info, found := adapter.GetAdapterMap()[s]; found {
			app.Preferences().SetString(prefsAdapter, s)
			if info.RequiresSerialPort {
				csw.portSelector.Enable()
				csw.speedSelector.Enable()
				return
			}
			csw.portSelector.Disable()
			csw.speedSelector.Disable()
		}
	})

	csw.portSelector = widget.NewSelect(csw.listPorts(), func(s string) {
		app.Preferences().SetString(prefsPort, s)
	})
	csw.speedSelector = widget.NewSelect(portSpeeds, func(s string) {
		app.Preferences().SetString(prefsSpeed, s)
	})

	csw.objects = []fyne.CanvasObject{
		container.NewVBox(
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Select adapter"),
				nil,
				csw.adapterSelector,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Select port"),
				widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
					csw.portSelector.Options = csw.listPorts()
					csw.portSelector.Refresh()
				}),
				csw.portSelector,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Select speed"),
				nil,
				csw.speedSelector,
			),
		),
	}
	csw.loadPrefs()
	return csw
}

type CanSettingsWidget struct {
	widget.BaseWidget
	app             fyne.App
	objects         []fyne.CanvasObject
	adapterSelector *widget.Select
	portSelector    *widget.Select
	speedSelector   *widget.Select
}

const (
	prefsAdapter = "adapter"
	prefsPort    = "port"
	prefsSpeed   = "speed"
)

func (cs *CanSettingsWidget) loadPrefs() {
	if adapter := cs.app.Preferences().String(prefsAdapter); adapter != "" {
		cs.adapterSelector.SetSelected(adapter)
	}
	if port := cs.app.Preferences().String(prefsPort); port != "" {
		cs.portSelector.SetSelected(port)
	}
	if speed := cs.app.Preferences().String(prefsSpeed); speed != "" {
		cs.speedSelector.SetSelected(speed)
	}
}

func (cs *CanSettingsWidget) GetAdapter(logger func(string)) (gocan.Adapter, error) {
	baudrate, err := strconv.Atoi(cs.speedSelector.Selected)

	if cs.adapterSelector.Selected == "" {
		return nil, errors.New("No adapter selected") //lint:ignore ST1005 This is ok
	}

	if adapter.GetAdapterMap()[cs.adapterSelector.Selected].RequiresSerialPort {
		if cs.portSelector.Selected == "" {
			return nil, errors.New("No port selected") //lint:ignore ST1005 This is ok

		}
		if cs.speedSelector.Selected == "" {
			return nil, errors.New("No speed selected") //lint:ignore ST1005 This is ok
		}
	}

	if err != nil {
		if cs.speedSelector.Selected != "" {
			return nil, err
		}
	}
	return adapter.New(
		cs.adapterSelector.Selected,
		&gocan.AdapterConfig{
			Port:         cs.portSelector.Selected,
			PortBaudrate: baudrate,
			CANRate:      500,
			CANFilter:    []uint32{0x238, 0x258, 0x270},
			OnMessage:    logger,
			OnError: func(err error) {
				logger(err.Error())
			},
		},
	)

}

func (cs *CanSettingsWidget) MinSize() fyne.Size {
	return cs.objects[0].MinSize()
}

func (cs *CanSettingsWidget) Resize(size fyne.Size) {
	cs.objects[0].Resize(size)
}

func (cs *CanSettingsWidget) listPorts() []string {
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
		if port.IsUSB {
			//m.output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
			//m.output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
			portsList = append(portsList, port.Name)
		}
	}
	return portsList
}

func (cs *CanSettingsWidget) CreateRenderer() fyne.WidgetRenderer {
	return &canSettingsWidgetRenderer{
		obj: cs,
	}
}

type canSettingsWidgetRenderer struct {
	obj *CanSettingsWidget
}

func (cs *canSettingsWidgetRenderer) Layout(size fyne.Size) {
	//log.Println(size)
}

func (cs *canSettingsWidgetRenderer) MinSize() fyne.Size {
	return cs.obj.MinSize()
}

func (cs *canSettingsWidgetRenderer) Refresh() {
}

func (cs *canSettingsWidgetRenderer) Destroy() {
}

func (cs *canSettingsWidgetRenderer) Objects() []fyne.CanvasObject {
	return cs.obj.objects
}
