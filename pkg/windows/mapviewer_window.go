package windows

import (
	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/widgets"
)

type MapViewerWindow struct {
	fyne.Window
	mv *widgets.MapViewer
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
