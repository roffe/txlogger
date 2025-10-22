package windows

import (
	"context"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets/ebusmonitor"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

/*
// Remember that you should **not** create more than one context
func newOtoContext() *oto.Context {
	opts := &oto.NewContextOptions{
		// Usually 44100 or 48000. Other values might cause distortions in Oto
		SampleRate: 44100,
		// Number of channels (aka locations) to play sounds from. Either 1 or 2.
		// 1 is mono sound, and 2 is stereo (most speakers are stereo).
		ChannelCount: 2,
		// Format of the source. go-mp3's format is signed 16bit integers.
		Format: oto.FormatSignedInt16LE,
	}
	otoCtx, readyChan, err := oto.NewContext(opts)
	if err != nil {
		panic("oto.NewContext failed: " + err.Error())
	}
	// It might take a bit for the hardware audio devices to be ready, so we wait on the channel.
	select {
	case <-readyChan:
		return otoCtx
	case <-time.After(5 * time.Second):
		fyne.LogError("oto", errors.New("timeout waiting for audio device"))
		return nil
	}
}
*/

func (mw *MainWindow) Close() {
	if mw.dlc != nil {
		log.Println("Closing datalogger client")
		mw.dlc.Close()
		time.Sleep(250 * time.Millisecond)
	}
	if mw.gwclient != nil {
		log.Println("sending quit to cangateway")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := mw.gwclient.SendCommand(ctx, &proto.Command{Data: []byte("quit")})
		if err != nil {
			log.Println("error sending quit to cangateway:", err)
		}
		log.Println("quit command sent")
	}
	mw.Window.Close()
	log.Println("mainwindow Closed")
}

func (mw *MainWindow) onDropped(p fyne.Position, uris []fyne.URI) {
	log.Println("Dropped", p, uris)
	for _, u := range uris {
		filename := u.Path()
		switch strings.ToLower(path.Ext(filename)) {
		case ".bin":
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Error(err)
			}
		case ".t5l", ".t7l", ".t8l", ".csv":
			// Check if we dropped it on the open log button
			// log.Println(mw.buttons.openLogBtn.Position(), mw.buttons.openLogBtn.Size())
			if p.X >= mw.buttons.openLogBtn.Position().X && p.X <= mw.buttons.openLogBtn.Position().X+mw.buttons.openLogBtn.Size().Width &&
				p.Y >= mw.buttons.openLogBtn.Position().Y+30 && p.Y <= mw.buttons.openLogBtn.Position().Y+30+mw.buttons.openLogBtn.Size().Height {
				f, err := os.Open(filename)
				if err != nil {
					mw.Error(err)
					return
				}
				defer f.Close()
				mw.LoadLogfileCombined(filename, f, p, false)
			} else {
				f, err := os.Open(filename)
				if err != nil {
					mw.Error(err)
					return
				}
				defer f.Close()
				mw.LoadLogfile(filename, f, p)
			}
		}
	}
}

// list .json files in the folder layouts
func listLayouts() []string {
	opts := []string{"Save Layout"}
	files, err := os.ReadDir("layouts")
	if err != nil {
		if os.IsNotExist(err) {
			return opts
		}
		fyne.LogError("Error reading layouts folder", err)
		return opts
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		opts = append(opts, strings.TrimSuffix(f.Name(), ".json"))
	}
	return opts
}

func (mw *MainWindow) openEBUSMonitor() {
	if w := mw.wm.HasWindow("EBUS Monitor"); w != nil {
		mw.wm.Raise(w)
		return
	}
	mon := ebusmonitor.New()
	eb := multiwindow.NewSystemWindow("EBUS Monitor", mon)
	eb.Icon = theme.ComputerIcon()
	ebus.SetOnMessage(mon.SetText)
	eb.OnClose = func() {
		ebus.SetOnMessage(nil)
	}
	mw.wm.Add(eb)
}

func (mw *MainWindow) openSettings() {
	if w := mw.wm.HasWindow("Settings"); w != nil {
		mw.wm.Raise(w)
		return
	}
	inner := multiwindow.NewInnerWindow("Settings", mw.settings)
	inner.Icon = theme.SettingsIcon()
	mw.wm.Add(inner)
}

func (mw *MainWindow) loadPrefs() {
	if ecu := mw.app.Preferences().StringWithFallback(prefsSelectedECU, "T7"); ecu != "" {
		mw.selects.ecuSelect.SetSelected(ecu)
	}

	if preset := mw.app.Preferences().String(prefsSelectedPreset); preset != "" {
		mw.selects.presetSelect.SetSelected(preset)
	}
	/*
		if filename == "" {
			if filename := mw.app.Preferences().String(prefsLastBinFile); filename != "" {
				if err := mw.LoadSymbolsFromFile(filename); err != nil {
					mw.Error(err)
					return
				}
				mw.filename = filename
				return
			}
		} else {
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Error(err)
				return
			}
			mw.filename = filename
		}
	*/

}

func (mw *MainWindow) newSymbolnameTypeahead() {
	mw.selects.symbolLookup = xwidget.NewCompletionEntry([]string{})
	mw.selects.symbolLookup.PlaceHolder = "Search for symbol"
	mw.selects.symbolLookup.OnChanged = func(s string) {
		if mw.fw == nil {
			return
		}
		// completion start for text length >= 3
		if len(s) < 3 {
			mw.selects.symbolLookup.HideCompletion()
			return
		}
		// Get the list of possible completion
		results := []string{"ADC1", "ADC2", "ADC3", "ADC4", "ADC5"}
		for _, sym := range mw.fw.Symbols() {
			if sym.Length > 8 {
				continue
			}

			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) {
				results = append(results, sym.Name)
			}
		}
		// no results
		if len(results) == 0 {
			mw.selects.symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		mw.selects.symbolLookup.SetOptions(results)
		mw.selects.symbolLookup.ShowCompletion()

	}

	mw.selects.symbolLookup.OnSubmitted = func(s string) {
		log.Println("Submitted", s)
	}
}
