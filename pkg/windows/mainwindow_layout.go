package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/gauge"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

type LayoutFile struct {
	ECU     string
	Preset  string
	Windows []WindowProperties
}

func (mw *MainWindow) SaveLayout() error {
	input := widget.NewEntry()
	input.OnSubmitted = func(s string) {
		log.Println("submitted", s)
	}

	items := []*widget.FormItem{
		widget.NewFormItem("Name", layout.NewFixedWidth(200, input)),
	}

	callback := func(b bool) {
		if !b {
			return
		}
		layoutBytes, err := mw.jsonLayout()
		if err != nil {
			mw.Error(fmt.Errorf("failed to save layout: %w", err))
			return
		}
		if err := writeLayout(input.Text, layoutBytes); err != nil {
			mw.Error(fmt.Errorf("failed to save layout: %w", err))
			return
		}
		mw.buttons.layoutRefreshBtn.OnTapped()
	}

	in := dialog.NewForm("Save Window Layout", "Save", "Cancel", items, callback, fyne.CurrentApp().Driver().AllWindows()[0])
	in.Show()
	mw.Window.Canvas().Focus(input)

	return nil
}

func writeLayout(name string, data []byte) error {
	layoutPath, err := common.GetLayoutPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(layoutPath, name+".json"), data, 0o644); err != nil {
		return err
	}
	return nil
}

type WindowProperties struct {
	multiwindow.WindowProperties
	GaugeConfig *widgets.GaugeConfig `json:",omitempty"`
}

func (mw *MainWindow) jsonLayout() ([]byte, error) {
	var history []WindowProperties
	viewportSize := mw.wm.Size()

	for _, w := range mw.wm.Windows() {
		if w.IgnoreSave {
			continue
		}
		pos := w.Position()
		size := w.Size()
		preMaxPos := w.PreMaximizedPos()
		preMaxSize := w.PreMaximizedSize()

		var entry WindowProperties

		entry.Title = w.Title()
		entry.Ratios = multiwindow.WindowRatio{
			X: float64(pos.X) / float64(viewportSize.Width),
			Y: float64(pos.Y) / float64(viewportSize.Height),
			W: float64(size.Width) / float64(viewportSize.Width),
			H: float64(size.Height) / float64(viewportSize.Height),
		}
		entry.Maximized = w.Maximized()
		entry.PreMaximizedPos = multiwindow.WindowRatio{
			X: float64(preMaxPos.X) / float64(viewportSize.Width),
			Y: float64(preMaxPos.Y) / float64(viewportSize.Height),
		}
		entry.PreMaximizedSize = multiwindow.WindowRatio{
			W: float64(preMaxSize.Width) / float64(viewportSize.Width),
			H: float64(preMaxSize.Height) / float64(viewportSize.Height),
		}

		if tt, ok := w.Content().(widgets.Gauge); ok {
			entry.GaugeConfig = tt.GetConfig()
		}
		history = append(history, entry)
	}

	b, err := json.Marshal(&LayoutFile{
		ECU:     mw.selects.ecuSelect.Selected,
		Preset:  mw.selects.presetSelect.Selected,
		Windows: history,
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (mw *MainWindow) LoadLayout(name string) error {
	layoutPath, err := common.GetLayoutPath()
	if err != nil {
		return err
	}
	fname := filepath.Join(layoutPath, name+".json")
	debug.Log("LoadLayout: " + fname)
	b, err := os.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("LoadLayout failed to read file: %w", err)
	}

	var layout LayoutFile
	if err := json.Unmarshal(b, &layout); err != nil {
		return fmt.Errorf("LoadLayout failed to decode window layout: %w", err)
	}

	mw.wm.CloseAll()

	if mw.dlc == nil {
		// mw.selects.ecuSelect.SetSelected(layout.ECU)
		mw.selects.ecuSelect.Selected = layout.ECU
		mw.selects.presetSelect.SetSelected(layout.Preset)
	}

	for _, h := range layout.Windows {
		var openMap bool
		switch h.Title {
		case "Settings":
			mw.openSettings()
			continue
		case "Dashboard":
			mw.buttons.dashboardBtn.OnTapped()
			continue
		default:
			openMap = true
		}

		if h.GaugeConfig != nil {
			// Style is a global setting, not part of the saved layout
			h.GaugeConfig.Classic = mw.settings.GetClassicGauges()
			gauge, cancelFn, err := gauge.New(h.GaugeConfig)
			if err != nil {
				mw.Error(fmt.Errorf("failed to create gauge: %w", err))
				continue
			}
			iw := multiwindow.NewInnerWindow(h.Title, gauge)
			iw.OnClose = func() {
				cancelFn()
			}
			mw.wm.Add(iw)
			continue
		}

		parts := strings.Split(h.Title, " ")
		if len(parts) < 1 {
			continue
		}

		if openMap {
			mw.openMap(symbol.ECUTypeFromString(layout.ECU), h.Title, parts[0], "")
		}

	}

	layouts := make([]multiwindow.WindowProperties, len(layout.Windows))
	for i, h := range layout.Windows {
		layouts[i].Title = h.Title
		layouts[i].Ratios = h.Ratios
		layouts[i].Maximized = h.Maximized
		layouts[i].PreMaximizedPos = h.PreMaximizedPos
		layouts[i].PreMaximizedSize = h.PreMaximizedSize
	}

	return mw.wm.LoadLayout(layouts)
}
