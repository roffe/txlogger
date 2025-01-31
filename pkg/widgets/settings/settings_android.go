package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func selectFolder() (string, error) {
	cb := func(d fyne.ListableURI, err error) {

	}
	dialog.ShowFolderOpen(cb, fyne.CurrentApp().Driver().AllWindows()[0])
}
