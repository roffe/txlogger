package windows

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/roffe/txlogger/pkg/common"
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
		mw.Log("Closing datalogger client")
		mw.dlc.Close()
		time.Sleep(250 * time.Millisecond)
	}
	mw.Window.Close()
	time.Sleep(200 * time.Millisecond)
	os.Exit(0)
}

func (mw *MainWindow) onDropped(p fyne.Position, uris []fyne.URI) {
	log.Println("Dropped", p, uris)
	for _, u := range uris {
		filename := u.Path()
		switch strings.ToLower(path.Ext(filename)) {
		case ".as2":
			if err := mw.LoadAS2File(filename); err != nil {
				mw.Error(err)
			}
		case ".bin":
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Error(err)
			}
		case ".t5l", ".t7l", ".t8l", ".csv", ".bpl":
			f, err := os.Open(filename)
			if err != nil {
				mw.Error(err)
				return
			}
			defer f.Close()
			if p.X < 100 {
				p.X = 100
			}
			mw.LoadLogfile(filename, f, p)
		}
	}
}

// list .json files in the folder layouts
func listLayouts() []string {
	opts := []string{"Save Layout"}

	layoutPath, err := common.GetLayoutPath()
	if err != nil {
		fyne.LogError("Error getting layout path", err)
		return opts
	}

	if layoutPath == "" {
		layoutPath = "layouts"
	}

	files, err := os.ReadDir(layoutPath)
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
	// selecting the ECU also selects its last used preset via the
	// ecuSelect callback
	ecu := mw.app.Preferences().StringWithFallback(prefsSelectedECU, "T7")
	mw.selects.ecuSelect.SetSelected(ecu)

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
