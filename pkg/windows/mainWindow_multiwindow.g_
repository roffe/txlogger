package windows

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/gauge"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

type innerWindow struct {
	*multiwindow.InnerWindow
	Persist    bool // Persist through layout changes
	IgnoreSave bool // Ignore saving to layout
}

// newInnerWindow creates a new innerWindow that is not persisted and saved in layout
func newInnerWindow(title string, content fyne.CanvasObject) *innerWindow {
	return &innerWindow{
		InnerWindow: multiwindow.NewInnerWindow(title, content),
	}
}

// newSystemWindow creates a new innerWindow that is persisted and ignored in layout saving
func newSystemWindow(title string, content fyne.CanvasObject) *innerWindow {
	iw := &innerWindow{
		InnerWindow: multiwindow.NewInnerWindow(title, content),
	}
	iw.Persist = true
	iw.IgnoreSave = true
	return iw
}

type windowRatio struct {
	X, Y float64 // Position ratios
	W, H float64 // Size ratios
}

type windowHistory struct {
	Title            string
	Ratios           windowRatio
	Maximized        bool
	PreMaximizedPos  windowRatio
	PreMaximizedSize windowRatio
	GaugeConfig      widgets.GaugeConfig `json:",omitempty"`
}

type windowManager struct {
	mw      *MainWindow
	open    map[string]*innerWindow
	history map[string]windowHistory
	mu      sync.RWMutex
	*multiwindow.MultipleWindows
}

func newWindowManager(mw *MainWindow) *windowManager {
	wm := &windowManager{
		mw:              mw,
		open:            make(map[string]*innerWindow),
		history:         make(map[string]windowHistory),
		MultipleWindows: multiwindow.NewMultipleWindows(),
	}
	wm.MultipleWindows.LockViewport = true
	return wm
}

func (wm *windowManager) HasWindow(title string) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	w, ok := wm.open[title]
	if ok {
		wm.MultipleWindows.Raise(w.InnerWindow)
	}
	return ok
}

func (wm *windowManager) Add(w *innerWindow, startPosition ...fyne.Position) bool {
	if wm.HasWindow(w.Title()) {
		return false
	}
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.open[w.Title()] = w

	var foo func()
	if w.CloseIntercept != nil {
		foo = w.CloseIntercept
		w.CloseIntercept = func() {
			foo()
			wm.remove(w)
		}
	} else {
		w.CloseIntercept = func() {
			wm.remove(w)
		}
	}

	wm.MultipleWindows.Add(w.InnerWindow, startPosition...)
	return true
}

func (wm *windowManager) remove(w *innerWindow) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	delete(wm.open, w.Title())

}

func (wm *windowManager) CloseAll() {
	for _, w := range wm.open {
		if w.Persist {
			continue
		}
		w.Close()
	}
}

func (wm *windowManager) Size() fyne.Size {
	return wm.MultipleWindows.Size()
}

func (wm *windowManager) SaveLayout() error {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	input := widget.NewEntry()
	items := []*widget.FormItem{
		widget.NewFormItem("Name", layout.NewFixedWidth(200, input)),
	}

	callback := func(b bool) {
		if !b {
			return
		}
		bb, err := wm.jsonLayout()
		if err != nil {
			wm.mw.Error(fmt.Errorf("failed to save layout: %w", err))
			return
		}
		if err := writeLayout(input.Text, bb); err != nil {
			wm.mw.Error(fmt.Errorf("failed to save layout: %w", err))
		}
	}

	in := dialog.NewForm("Save Window Layout", "Save", "Cancel", items, callback, fyne.CurrentApp().Driver().AllWindows()[0])
	in.Show()
	fyne.CurrentApp().Driver().CanvasForObject(wm.MultipleWindows).Focus(input)
	return nil
}

func writeLayout(name string, data []byte) error {
	if f, err := os.Stat("layouts"); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir("layouts", 0777); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !f.IsDir() {
			return errors.New("layouts exists but is not a directory")
		}
		if err := os.WriteFile("layouts/"+name+".json", data, 0777); err != nil {
			return err
		}
	}
	return nil
}

type layoutFile struct {
	ECU     string
	Preset  string
	Version int
	Windows []windowHistory
}

func (wm *windowManager) LoadLayout(name string) error {
	b, err := os.ReadFile("layouts/" + name + ".json")
	if err != nil {
		return fmt.Errorf("LoadLayout failed to read file: %w", err)
	}
	var layout layoutFile

	if err := json.Unmarshal(b, &layout); err != nil {
		return fmt.Errorf("LoadLayout failed to decode window layout: %w", err)
	}

	wm.CloseAll()

	wm.mw.selects.ecuSelect.SetSelected(layout.ECU)
	wm.mw.selects.presetSelect.SetSelected(layout.Preset)

	viewportSize := wm.Size()
	var openMap bool

	for _, h := range layout.Windows {
		switch h.Title {
		case "Settings":
			wm.mw.openSettings()
			openMap = false
		case "Dashboard":
			wm.mw.buttons.dashboardBtn.Tapped(&fyne.PointEvent{})
			openMap = false
		default:
			openMap = true
		}

		if h.GaugeConfig.Type != "" {
			gauge, cancels, err := gauge.New(h.GaugeConfig)
			if err != nil {
				wm.mw.Error(fmt.Errorf("failed to create gauge: %w", err))
				continue
			}
			iw := newInnerWindow(h.Title, gauge)
			iw.CloseIntercept = func() {
				for _, cancel := range cancels {
					cancel()
				}
			}

			// Convert ratios to absolute positions
			position := fyne.NewPos(
				float32(h.Ratios.X*float64(viewportSize.Width)),
				float32(h.Ratios.Y*float64(viewportSize.Height)),
			)

			if !wm.Add(iw, position) {
				for _, cancel := range cancels {
					cancel()
				}
			}

			// Convert ratios to absolute size
			size := fyne.NewSize(
				float32(h.Ratios.W*float64(viewportSize.Width)),
				float32(h.Ratios.H*float64(viewportSize.Height)),
			)
			iw.Resize(size)
			continue
		}

		parts := strings.Split(h.Title, " ")
		if len(parts) < 1 {
			continue
		}
		if openMap {
			wm.mw.openMap(symbol.ECUTypeFromString(layout.ECU), parts[0])
		}

		w, found := wm.open[h.Title]
		if !found {
			continue
		}

		// Convert ratios to absolute positions and sizes
		position := fyne.NewPos(
			float32(h.Ratios.X*float64(viewportSize.Width)),
			float32(h.Ratios.Y*float64(viewportSize.Height)),
		)
		size := fyne.NewSize(
			float32(h.Ratios.W*float64(viewportSize.Width)),
			float32(h.Ratios.H*float64(viewportSize.Height)),
		)

		preMaxPos := fyne.NewPos(
			float32(h.PreMaximizedPos.X*float64(viewportSize.Width)),
			float32(h.PreMaximizedPos.Y*float64(viewportSize.Height)),
		)
		preMaxSize := fyne.NewSize(
			float32(h.PreMaximizedSize.W*float64(viewportSize.Width)),
			float32(h.PreMaximizedSize.H*float64(viewportSize.Height)),
		)

		w.Move(position)
		w.Resize(size)
		w.SetMaximized(h.Maximized, preMaxPos, preMaxSize)
	}
	return nil
}

func (wm *windowManager) jsonLayout() ([]byte, error) {
	var history []windowHistory
	viewportSize := wm.Size()

	for title, w := range wm.open {
		if w.IgnoreSave {
			continue
		}
		pos := w.Position()
		size := w.Size()
		preMaxPos := w.PreMaximizedPos()
		preMaxSize := w.PreMaximizedSize()

		entry := windowHistory{
			Title: title,
			Ratios: windowRatio{
				X: float64(pos.X) / float64(viewportSize.Width),
				Y: float64(pos.Y) / float64(viewportSize.Height),
				W: float64(size.Width) / float64(viewportSize.Width),
				H: float64(size.Height) / float64(viewportSize.Height),
			},
			Maximized: w.Maximized(),
			PreMaximizedPos: windowRatio{
				X: float64(preMaxPos.X) / float64(viewportSize.Width),
				Y: float64(preMaxPos.Y) / float64(viewportSize.Height),
			},
			PreMaximizedSize: windowRatio{
				W: float64(preMaxSize.Width) / float64(viewportSize.Width),
				H: float64(preMaxSize.Height) / float64(viewportSize.Height),
			},
		}

		if tt, ok := w.InnerWindow.Content().(widgets.Gauge); ok {
			entry.GaugeConfig = tt.GetConfig()
		}
		history = append(history, entry)
	}

	b, err := json.Marshal(map[string]interface{}{
		"ecu":     wm.mw.selects.ecuSelect.Selected,
		"preset":  wm.mw.selects.presetSelect.Selected,
		"version": 2, // Increment version to indicate ratio-based layout
		"windows": history,
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}
