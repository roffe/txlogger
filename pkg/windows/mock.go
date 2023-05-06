package windows

import (
	"fmt"
	"math/rand"
	"time"

	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/t7logger/pkg/sink"
)

func (mw *MainWindow) newMockBTN() *widget.Button {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	mockStop := make(chan bool, 1)
	var mockBtn *widget.Button
	mockBtn = widget.NewButtonWithIcon("Start mocking", theme.DownloadIcon(), func() {
		if loggingRunning {
			mockStop <- true
			return
		}
		if !loggingRunning {
			mockBtn.SetText("Stop mocking")

			go func() {
				mw.progressBar.Start()
				t := time.NewTicker(time.Second / time.Duration(25))
				loggingRunning = true
			outer:
				for {
					select {
					case <-mockStop:
						for i := range mw.vars.Get() {
							mw.sinkManager.Push(&sink.Message{
								Data: []byte(fmt.Sprintf("%d:%v", i, r.Intn(8000))),
							})
						}
						break outer
					case <-t.C:
					}
				}
				mw.progressBar.Stop()
				loggingRunning = false
				mockBtn.SetText("Start mocking")
			}()
		}
	})
	return mockBtn
}
