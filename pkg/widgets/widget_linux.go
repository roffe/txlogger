package widgets

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	sdialog "github.com/sqweek/dialog"
)

func SelectFolder(cb func(str string)) {
	go func() {
		dir, err := sdialog.Directory().Title("Select log folder").Browse()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			log.Println(err)
			return
		}
		cb(dir)
	}()
}

func SelectFile(cb func(r fyne.URIReadCloser), desc string, exts ...string) {
	go func() {
		filename, err := sdialog.File().Filter(desc, exts...).Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			fyne.LogError("Error selecting file", err)
			return
		}
		uri := storage.NewFileURI(filename)
		r, err := storage.Reader(uri)
		if err != nil {
			log.Println("Error reading file:", err)
			return
		}
		fyne.Do(func() {
			cb(r)
		})
	}()
}

func SaveFile(cb func(str string), desc, ext string) {
	go func() {
		filename, err := sdialog.File().Filter(desc, ext).Save()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			fyne.LogError("Error selecting file", err)
			return
		}
		fyne.Do(func() {
			cb(filename)
		})
	}()
}
