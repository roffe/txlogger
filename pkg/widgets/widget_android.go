package widgets

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
)

func SelectFolder(cbc func(str string)) {
	cb := func(d fyne.ListableURI, err error) {
		if err != nil {
			log.Println(err)
			return
		}
		if d == nil {
			log.Println("d is nil")
			return
		}
		cbc(d.Path())
	}
	w := fyne.CurrentApp().Driver().AllWindows()[0]
	dialog.ShowFolderOpen(cb, w)
}

func SelectFile(cbc func(r fyne.URIReadCloser), desc string, exts ...string) {
	cb := func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			log.Println(err)
			return
		}
		if reader == nil {
			log.Println("reader is nil")
			return
		}
		cbc(reader)
	}

	w := fyne.CurrentApp().Driver().AllWindows()[0]
	//dialog.ShowFileOpen(cb, w)
	d := dialog.NewFileOpen(cb, w)

	log.Println("SelectFile", desc, exts)

	newExts := make([]string, len(exts))
	for i, ext := range exts {
		newExts[i] = "." + ext
	}

	d.SetTitleText(desc)
	d.SetFilter(storage.NewExtensionFileFilter(newExts))
	d.Resize(w.Canvas().Size())
	d.Show()
}

func SaveFile(cbc func(str string), desc, ext string) {
	cb := func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			log.Println(err)
			return
		}
		defer writer.Close()
		if writer == nil {
			log.Println("reader is nil")
			return
		}
		cbc(writer.URI().Path())
	}
	w := fyne.CurrentApp().Driver().AllWindows()[0]
	dialog.ShowFileSave(cb, w)
}
