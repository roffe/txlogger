package canflasher

import (
	"fmt"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	// Import ecu packages

	_ "github.com/roffe/txlogger/pkg/ecu/t5"
	_ "github.com/roffe/txlogger/pkg/ecu/t5legion"
	_ "github.com/roffe/txlogger/pkg/ecu/t7"
	_ "github.com/roffe/txlogger/pkg/ecu/t8"
	_ "github.com/roffe/txlogger/pkg/ecu/t8mcp"
	_ "github.com/roffe/txlogger/pkg/ecu/t8mcpz22se"
	_ "github.com/roffe/txlogger/pkg/ecu/t8z22se"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

var _ fyne.Widget = (*CanFlasherWidget)(nil)

type CanFlasherWidget struct {
	widget.BaseWidget

	app fyne.App

	container *container.Split

	ecuSelect *widget.Select

	logList     *widget.List
	logValues   binding.StringList
	infoBTN     *widget.Button
	dumpBTN     *widget.Button
	flashBTN    *widget.Button
	progressBar *widget.ProgressBar

	l binding.DataListener

	cfg *Config
}

type Config struct {
	CSW *settings.Widget
}

func New(cfg *Config) *CanFlasherWidget {
	t := &CanFlasherWidget{
		app:       fyne.CurrentApp(),
		logValues: binding.NewStringList(),
		cfg:       cfg,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *CanFlasherWidget) Disable() {
	t.infoBTN.Disable()
	t.dumpBTN.Disable()
	t.flashBTN.Disable()
}

func (t *CanFlasherWidget) Enable() {
	t.infoBTN.Enable()
	t.dumpBTN.Enable()
	t.flashBTN.Enable()
}

func (t *CanFlasherWidget) log(s string) {
	var text string
	if s != "" {
		text = fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), s)
	}
	//logData = append(logData, text)
	t.logValues.Append(text)
}

func (t *CanFlasherWidget) progress(v float64) {
	if v < 0 {
		t.progressBar.Max = math.Abs(v)
		fyne.Do(func() {
			t.progressBar.SetValue(0)
		})
		return
	}
	fyne.Do(func() {
		t.progressBar.SetValue(v)
	})
}

func (t *CanFlasherWidget) CreateRenderer() fyne.WidgetRenderer {
	t.ecuSelect = widget.NewSelect([]string{"Trionic 5", "Trionic 7", "Trionic 8", "Trionic 8 z22se", "Trionic 8 MCP", "Trionic 8 MCP z22se"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString("canflasher_ecu", s)
	})

	t.ecuSelect.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback("canflasher_ecu", "Trionic 5"))

	t.logList = widget.NewListWithData(
		t.logValues,
		func() fyne.CanvasObject {
			w := widget.NewLabel("")
			w.TextStyle.Monospace = true
			w.Selectable = true
			return w
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				panic(err)
			}
			obj.(*widget.Label).SetText(txt)
		},
	)

	t.l = binding.NewDataListener(func() {
		t.logList.ScrollToBottom()
	})

	t.logValues.AddListener(t.l)

	t.progressBar = widget.NewProgressBar()

	// t.wizzardBTN = widget.NewButton("Wizzard", nil) //t.wizzard)
	t.infoBTN = widget.NewButton("Info", t.ecuInfo) //t.ecuInfo)
	//t.dtcBTN = widget.NewButton("Read DTC", nil)   //t.readDTC)
	t.dumpBTN = widget.NewButton("Dump", t.ecuDump)
	//t.sramBTN = widget.NewButton("Dump SRAM", nil) //t.dumpSRAM)
	t.flashBTN = widget.NewButton("Flash", nil) //t.ecuFlash)

	// t.ecuList.PlaceHolder = "Select ECU"
	// t.adapterList.PlaceHolder = "Select Adapter"
	// t.portList.PlaceHolder = "Select Port"
	// t.speedList.PlaceHolder = "Select Speed"

	left := container.New(layout.NewStackLayout(), t.logList)
	right := container.NewVBox(
		t.ecuSelect,
		t.infoBTN,
		//t.dtcBTN,
		t.dumpBTN,
		//t.sramBTN,
		t.flashBTN,
	)

	split := container.NewHSplit(left, right)
	split.Offset = 0.8

	t.container = container.NewVSplit(split, t.progressBar)
	t.container.Offset = 1

	//return widget.NewSimpleRenderer(t.container)
	return &CanFlasherWidgetRenderer{
		t: t,
	}
}

type CanFlasherWidgetRenderer struct {
	t *CanFlasherWidget
}

func (tr *CanFlasherWidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
}

func (tr *CanFlasherWidgetRenderer) MinSize() fyne.Size {
	return tr.t.container.MinSize()
}

func (tr *CanFlasherWidgetRenderer) Refresh() {

}

func (tr *CanFlasherWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *CanFlasherWidgetRenderer) Destroy() {
	tr.t.logValues.RemoveListener(tr.t.l)
}
