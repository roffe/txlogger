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
						break outer
					case <-t.C:
						//metrics := make(map[int]interface{})
						for _, va := range mw.vars.Get() {
							mw.sinkManager.Push(&sink.Message{
								Data: []byte(fmt.Sprintf("%d:%v", va.Value, r.Intn(8000))),
							})
							//metrics[va.Value] = r.Intn(8000)
						}

						//b, err := json.Marshal(metrics)
						//if err != nil {
						//	log.Println(err)
						//} else {
						//	mw.sinkManager.Push(&sink.Message{
						//		Data: b,
						//	})
						//}
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
