package windows

/*

func (mw *MainWindow) newMockBtn() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	mockStop := make(chan bool, 1)
	mw.mockBtn = widget.NewButtonWithIcon("Start mocking", theme.DownloadIcon(), func() {
		if mw.mockRunning {
			mockStop <- true
			return
		}
		if !mw.mockRunning {
			mw.mockBtn.SetText("Stop mocking")
			go func() {
				mw.logBtn.Disable()
				defer mw.logBtn.Enable()

				//mw.progressBar.Start()

				t := time.NewTicker(time.Second / time.Duration(mw.freqSlider.Value))
				mw.mockRunning = true
			outer:
				for {
					select {
					case <-mockStop:
						break outer
					case <-t.C:
						//metrics := make(map[int]interface{})
						var ms []string
						for _, va := range mw.vars.Get() {
							ms = append(ms, fmt.Sprintf("%d:%d", va.Value, r.Intn(8000)))
							//metrics[va.Value] = r.Intn(8000)
						}
						mw.sinkManager.Push(&sink.Message{
							Data: []byte(time.Now().Format(datalogger.ISO8601) + "|" + strings.Join(ms, ",")),
						})
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
				//mw.progressBar.Stop()
				mw.mockRunning = false
				mw.mockBtn.SetText("Start mocking")
			}()
		}
	})
}
*/
