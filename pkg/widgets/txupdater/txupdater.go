package txupdater

import (
	"fmt"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ota"
)

type TxUpdater struct {
	port        string
	output      *widget.List
	outputData  binding.StringList
	progressbar *widget.ProgressBar
	updateBtn   *widget.Button
	container   *fyne.Container
	listener    binding.DataListener

	widget.BaseWidget
}

func New(port string) *TxUpdater {
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
		go func() {
			//defer fyne.Do(t.updateBtn.Enable)
			defer t.updateBtn.Enable()
			if err := ota.UpdateOTA(ota.Config{
				Port: t.port,
				Logfunc: func(v ...any) {
					//fyne.Do(func() {
					t.outputData.Append(fmt.Sprint(v...))
					//})
				},
				ProgressFunc: func(progress float64) {
					//fyne.Do(func() {
					t.progressbar.SetValue(progress)
					//})
				},
			}); err != nil {
				//fyne.Do(func() {
				t.outputData.Append(fmt.Sprint("Error: ", err))
				//})
			}
		}()
	})

	t.listener = binding.NewDataListener(func() {
		t.output.ScrollToBottom()
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
	tu.outputData.AddListener(tu.listener)
	return widget.NewSimpleRenderer(tu.container)
}

type TxUpdaterRenderer struct {
	t *TxUpdater
}

func (tr *TxUpdaterRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
}

func (tr *TxUpdaterRenderer) MinSize() fyne.Size {
	return tr.t.container.MinSize()
}

func (tr *TxUpdaterRenderer) Refresh() {

}

func (tr *TxUpdaterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *TxUpdaterRenderer) Destroy() {
	tr.t.outputData.RemoveListener(tr.t.listener)
}
