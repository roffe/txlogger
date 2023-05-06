package windows

import (
	"fmt"
	"math/rand"
	"time"

	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/t7logger/pkg/sink"
)

func (mw *MainWindow) newMockBtn() *widget.Button {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	mockStop := make(chan bool, 1)
	var mockBtn *widget.Button
	mockBtn = widget.NewButtonWithIcon("Start mocking", theme.DownloadIcon(), func() {
		if mw.mockRunning {
			mockStop <- true
			return
		}
		if !mw.mockRunning {
			mockBtn.SetText("Stop mocking")
			go func() {
				mw.logBtn.Disable()
				defer mw.logBtn.Enable()

				mw.progressBar.Start()

				t := time.NewTicker(time.Second / time.Duration(25))
				mw.mockRunning = true
			outer:
				for {
					select {
					case <-mockStop:
						for _, v := range mw.vars.Get() {
							mw.sinkManager.Push(&sink.Message{
								Data: []byte(fmt.Sprintf("%d:%v", v.Value, r.Intn(8000))),
							})
						}
						break outer
					case <-t.C:
					}
				}
				mw.progressBar.Stop()
				mw.mockRunning = false
				mockBtn.SetText("Start mocking")
			}()
		}
	})
	return mockBtn
}
