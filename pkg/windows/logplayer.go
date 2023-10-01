package windows

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/widgets"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type Op int

const (
	OpTogglePlayback Op = iota
	OpSeek
	OpPrev
	OpNext
	OpPlaybackSpeed
	OpExit
)

type controlMsg struct {
	Op   Op
	Pos  int
	Rate float64
}

/*
func V(width, height int32) (mjpeg.AviWriter, error) {
	filename := fmt.Sprintf("capture-%s.avi", time.Now().Format("2006-01-02-15-04-05"))
	return mjpeg.New(filename, width, height, 24)
}
*/

func NewLogPlayer(a fyne.App, filename string, mw *MainWindow) {
	w := a.NewWindow("LogPlayer " + filename)
	w.Resize(fyne.NewSize(900, 600))
	lines, err := readLog(filename)
	if err != nil {
		// dialog.ShowError(err, w)
		mw.Log(err.Error())
		return
	}

	controlChan := make(chan *controlMsg, 10)

	db := NewDashboard(mw, true, mw.logBtn)

	w.SetCloseIntercept(func() {
		controlChan <- &controlMsg{Op: OpExit}
		close(db.metricsChan)
		w.Close()
	})

	db.SetValue("CEL", 0)
	db.SetValue("CRUISE", 0)
	db.SetValue("LIMP", 0)

	slider := widget.NewSlider(0, float64(len(lines)))
	posWidget := widget.NewLabel("")
	currLine := binding.NewFloat()

	currLine.AddListener(binding.NewDataListener(func() {
		val, err := currLine.Get()
		if err != nil {
			// dialog.ShowError(err, w)
			mw.Log(err.Error())
			return
		}
		slider.Value = val
		slider.Refresh()
		currPct := val / float64(len(lines)) * 100
		posWidget.SetText(fmt.Sprintf("%.01f%%", currPct))
		//posWidget.SetText(fmt.Sprintf("%.0f%%", val))
	}))

	slider.OnChanged = func(pos float64) {
		controlChan <- &controlMsg{Op: OpSeek, Pos: int(pos)}
		//posWidget.SetText(fmt.Sprintf("%.0f%%", pos))
		currPct := pos / float64(len(lines)) * 100
		posWidget.SetText(fmt.Sprintf("%.01f%%", currPct))
	}

	go playLogs(currLine, lines, db, controlChan, w)

	playing := false
	toggleBtn := &widget.Button{
		Text: "",
		Icon: theme.MediaPlayIcon(),
	}
	toggleBtn.OnTapped = func() {
		controlChan <- &controlMsg{Op: OpTogglePlayback}
		if playing {
			toggleBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			toggleBtn.SetIcon(theme.MediaPauseIcon())
		}
		playing = !playing
	}

	prevBtn := widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		controlChan <- &controlMsg{Op: OpPrev}
	})

	nextBtn := widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		controlChan <- &controlMsg{Op: OpNext}
	})

	restartBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		controlChan <- &controlMsg{Op: OpSeek, Pos: 0}
	})

	sel := widget.NewSelect([]string{"0.1x", "0.2x", "0.5x", "1x", "2x", "4x", "8x", "16x"}, func(s string) {
		switch s {
		case "0.1x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 3}
		case "0.2x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 2.5}
		case "0.5x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 1.6}
		case "1x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 1}
		case "2x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.5}
		case "4x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.25}
		case "8x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.125}
		case "16x":
			controlChan <- &controlMsg{Op: OpPlaybackSpeed, Rate: 0.0625}
		}
	})

	sel.Selected = "1x"

	main := container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			container.NewGridWithColumns(4,
				prevBtn,
				toggleBtn,
				restartBtn,
				nextBtn,
			),
			widgets.FixedWidth(75, sel),
			container.NewBorder(nil, nil, nil, posWidget, slider),
		),
		nil,
		nil,
		nil,
		db.Content(),
	)

	w.Canvas().SetOnTypedKey(keyHandler(w, controlChan, slider, toggleBtn, sel))
	w.SetContent(main)
	w.Show()
}

func keyHandler(w fyne.Window, controlChan chan *controlMsg, slider *widget.Slider, tb *widget.Button, sel *widget.Select) func(ev *fyne.KeyEvent) {
	return func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyF12:
			capture.Screenshot(w.Canvas())
		case fyne.KeyPlus:
			sel.SetSelectedIndex(sel.SelectedIndex() + 1)

		case fyne.KeyMinus:
			sel.SetSelectedIndex(sel.SelectedIndex() - 1)
		case fyne.KeyEnter:
			sel.SetSelected("1x")
		case fyne.KeyReturn, fyne.KeyHome:
			controlChan <- &controlMsg{Op: OpSeek, Pos: 0}
		case fyne.KeyPageUp:
			pos := int(slider.Value) + 100
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyUp:
			pos := int(slider.Value) + 10
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyDown:
			pos := int(slider.Value) - 10
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyPageDown:
			pos := int(slider.Value) - 100
			if pos < 0 {
				pos = 0
			}
			controlChan <- &controlMsg{Op: OpSeek, Pos: pos}
		case fyne.KeyLeft:
			controlChan <- &controlMsg{Op: OpPrev}
		case fyne.KeyRight:
			controlChan <- &controlMsg{Op: OpNext}
		case fyne.KeySpace:
			//controlChan <- &controlMsg{Op: OpToggle}
			tb.Tapped(&fyne.PointEvent{
				Position:         fyne.NewPos(0, 0),
				AbsolutePosition: fyne.NewPos(0, 0),
			})
		}
	}
}

func readLog(filename string) ([]string, error) {
	readFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer readFile.Close()

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)

	var output []string
	for fileScanner.Scan() {
		output = append(output, fileScanner.Text())
	}
	return output, nil
}

const TIME_FORMAT = "02-01-2006 15:04:05.999"

func playLogs(currentLine binding.Float, lines []string, db *Dashboard, control <-chan *controlMsg, ww fyne.Window) {
	/*
		aw, err := V(int32(db.canvas.Size().Width), int32(db.canvas.Size().Height))
		if err != nil {
			log.Println(err)
		}

		imgChan := make(chan image.Image, 10)
		go func() {
			for img := range imgChan {
				buf := bytes.NewBuffer(nil)
				if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 55}); err != nil {
					log.Println(err)
					continue
				}
				if err := aw.AddFrame(buf.Bytes()); err != nil {
					log.Println(err)
					continue
				}
			}
			if err := aw.Close(); err != nil {
				log.Println(err)
			}
		}()
	*/

	pos := 0
	totalLines := len(lines)
	play := false
	playonce := false
	speedMultiplier := 1.0
	var nextFrame int64
	var lastSeek int64
	var currentMillis int64
	for {
		currentMillis = time.Now().UnixMilli()
		select {
		case op := <-control:
			switch op.Op {
			case OpPlaybackSpeed:
				speedMultiplier = op.Rate
			case OpTogglePlayback:
				play = !play
			case OpSeek:
				//if time.Since(lastSeek) > 24*time.Millisecond {
				if currentMillis-lastSeek > 24 {
					pos = op.Pos
					playonce = true
					lastSeek = currentMillis
				}
			case OpPrev:
				pos -= 2
				if pos < 0 {
					pos = 0
				}
				playonce = true
			case OpNext:
				playonce = true
			case OpExit:
				//close(imgChan)
				return
			}
		//case <-t.C:
		default:
			if (!play && !playonce) || pos >= totalLines || nextFrame-currentMillis > 10 {
				time.Sleep(time.Duration(nextFrame-currentMillis-2) * time.Millisecond)
				continue
			}
			if currentMillis < nextFrame {
				continue
			}

			touples := strings.Split(strings.TrimSuffix(lines[pos], "|"), "|")

			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Println("Recovered in f", r)
					}
				}()
				db.SetTimeText(strings.Split(touples[0], " ")[1])
			}()

			parsedTime, err := time.Parse(TIME_FORMAT, touples[0])
			if err != nil {
				log.Println(err)
				continue
			}

			if pos+1 < len(lines) {
				touples2 := strings.Split(strings.TrimSuffix(lines[pos+1], "|"), "|")
				parsedTime2, err := time.Parse(TIME_FORMAT, touples2[0])
				if err != nil {
					log.Println(err)
					continue
				}
				nextFrame = currentMillis + int64(float64(parsedTime2.Sub(parsedTime).Milliseconds())*speedMultiplier)
			}

			for _, kv := range touples[1:] {
				parts := strings.Split(kv, "=")
				if parts[0] == "IMPORTANTLINE" {
					continue
				}
				val, err := strconv.ParseFloat(strings.Replace(parts[1], ",", ".", 1), 64)
				if err != nil {
					log.Println(err)
					pos++
					currentLine.Set(float64(pos))
					continue
				}
				db.SetValue(parts[0], val)
			}
			if playonce {
				playonce = false
			}
			/*
				if pos < totalLines {
					touples2 := strings.Split(strings.TrimSuffix(lines[pos], "|"), "|")
					t1, err := time.Parse(TIME_FORMAT, touples[0])
					if err != nil {
						log.Fatal(err)
					}
					t2, err := time.Parse(TIME_FORMAT, touples2[0])
					if err != nil {
						log.Fatal(err)
						}

						tt := t2.Sub(t1) - time.Since(lastFrame)
						log.Println(tt.Milliseconds())
						t.Reset(tt)
					}
			*/
			currentLine.Set(float64(pos))
			pos++
			//time.Sleep(slp - 500*time.Microsecond)
			//imgChan <- ww.Canvas().Capture()
		}
	}
}
