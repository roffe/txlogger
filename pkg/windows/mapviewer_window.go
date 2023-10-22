package windows

import (
	"fyne.io/fyne/v2"
)

type MapViewerWindow struct {
	fyne.Window
	mv MapViewerWindowWidget
}

type MapViewerWindowWidget interface {
	SetValue(name string, value float64)
	Close()
}

/*
func (mw *MainWindow) NewMapViewerWindow(mapname string) (*MapViewerWindow, error) {
	w := mw.app.NewWindow("Map Viewer - " + mapname)
	//w.SetFixedSize(true)
	if mw.symbols == nil {
		return nil, errors.New("no binary loaded")
	}

	return &MapViewerWindow{w, mw.app}, nil
}
*/
