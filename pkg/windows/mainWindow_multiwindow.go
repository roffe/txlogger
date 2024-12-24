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
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

type innerWindow struct {
	*multiwindow.InnerWindow
}

func newInnerWindow(title string, content fyne.CanvasObject) *innerWindow {
	return &innerWindow{
		InnerWindow: multiwindow.NewInnerWindow(title, content),
	}
}

type windowHistory struct {
	Title            string
	Position         fyne.Position
	Size             fyne.Size
	Maximized        bool
	PreMaximizedPos  fyne.Position
	PreMaximizedSize fyne.Size
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
	wm.MultipleWindows.Add(w.InnerWindow, startPosition...)
	return true
}

func (wm *windowManager) Remove(w *innerWindow) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	title := w.Title()
	delete(wm.open, title)
	wm.MultipleWindows.Remove(w.InnerWindow)
	/*
		wm.history[title] = windowHistory{
			position:         w.Position(),
			size:             w.Size(),
			maximized:        w.Maximized(),
			preMaximizedPos:  w.PreMaximizedPos(),
			preMaximizedSize: w.PreMaximizedSize(),
		}
	*/
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
	//go func() {
	//	for i := 0; i < 30; i++ {
	//		log.Println("focus", i)
	//		time.Sleep(50 * time.Millisecond)
	//		if c == nil {
	//			continue
	//		}
	//		c.Focus(input)
	//		return
	//	}
	//}()
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

	for _, w := range wm.open {
		w.Close()
		wm.Remove(w)
	}

	wm.mw.selects.ecuSelect.SetSelected(layout.ECU)
	wm.mw.selects.presetSelect.SetSelected(layout.Preset)

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
		w.Move(h.Position)
		w.Resize(h.Size)
		w.SetMaximized(h.Maximized, h.PreMaximizedPos, h.PreMaximizedSize)
	}
	return nil
}

func (wm *windowManager) jsonLayout() ([]byte, error) {
	var history []windowHistory
	for title, w := range wm.open {
		history = append(history, windowHistory{
			Title:            title,
			Position:         w.Position(),
			Size:             w.Size(),
			Maximized:        w.Maximized(),
			PreMaximizedPos:  w.PreMaximizedPos(),
			PreMaximizedSize: w.PreMaximizedSize(),
		})
	}

	b, err := json.Marshal(map[string]interface{}{
		"ecu":     wm.mw.selects.ecuSelect.Selected,
		"preset":  wm.mw.selects.presetSelect.Selected,
		"version": 1,
		"windows": history,
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}
