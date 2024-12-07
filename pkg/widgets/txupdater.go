package widgets

import (
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ota"
)

type TxUpdater struct {
	widget.BaseWidget

	port string
	//output      *widget.Label
	output      *widget.List
	outputData  binding.StringList
	progressbar *widget.ProgressBar
	updateBtn   *widget.Button

	container *fyne.Container
}

func NewTxUpdater(port string) *TxUpdater {
	t := &TxUpdater{
		port:        port,
		progressbar: widget.NewProgressBar(),
		outputData:  binding.NewStringList(),
	}
	t.ExtendBaseWidget(t)

	t.output = widget.NewListWithData(
		t.outputData,
		func() fyne.CanvasObject {
			return &widget.Label{
				Alignment:  fyne.TextAlignLeading,
				Truncation: fyne.TextTruncateEllipsis,
			}

		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				log.Println()
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)

	t.progressbar.Max = 100
	t.progressbar.Value = 0

	t.updateBtn = widget.NewButton("Update", func() {
		t.updateBtn.Disable()
		defer t.updateBtn.Enable()
		defer func() {
			go func() {
				time.Sleep(100 * time.Millisecond)
				t.output.ScrollToBottom()
			}()
		}()

		cfg := ota.Config{
			Port: t.port,
			Logfunc: func(v ...any) {
				defer func() {
					go func() {
						time.Sleep(100 * time.Millisecond)
						t.output.ScrollToBottom()
					}()
				}()
				t.outputData.Append(fmt.Sprint(v...))
			},
			ProgressFunc: t.progressbar.SetValue,
		}

		if err := ota.UpdateOTA(cfg); err != nil {
			t.outputData.Append(fmt.Sprint("Error: ", err))
		}

	})

	t.render()
	return t
}

func (tu *TxUpdater) render() *TxUpdater {
	tu.container = container.NewStack(
		container.NewBorder(
			widget.NewLabel("Press update to start"),
			container.NewVBox(
				tu.progressbar,
				tu.updateBtn,
			),
			nil,
			nil,
			tu.output,
		),
	)
	return tu
}

func (tu *TxUpdater) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(tu.container)
}
