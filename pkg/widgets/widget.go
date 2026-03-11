package widgets

import (
	"errors"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"github.com/roffe/txlogger/pkg/native"
)

func SelectFolder(callback func(str string)) {
	go func() {
		//dir, err := native.OpenFolderDialog("Select log folder")
		dir, err := selectFolder()
		if err != nil {
			if errors.Is(err, native.ErrCancelled) {
				return
			}
			log.Println("Error selecting folder:", err)
			return
		}
		fyne.Do(func() {
			callback(dir)
		})
	}()
}

func SelectFile(callback func(r fyne.URIReadCloser), desc string, exts ...string) {
	go func() {
		//filter := native.FileFilter{Description: desc, Extensions: exts}
		//filename, err := native.OpenFileDialog("Open file", filter)
		filename, err := selectFile(desc, exts...)
		if err != nil {
			if errors.Is(err, native.ErrCancelled) || err.Error() == "Cancelled" {
				return
			}
			log.Println("Error selecting file:", err)
			return
		}
		uri := storage.NewFileURI(filename)
		r, err := storage.Reader(uri)
		if err != nil {
			log.Println("Error reading file:", err)
			return
		}
		fyne.Do(func() { callback(r) })
	}()
}

func SaveFile(callback func(str string), desc string, ext string) {
	go func() {
		//filter := native.FileFilter{Description: desc, Extensions: []string{ext}}
		//filename, err := native.SaveFileDialog("Save "+desc, ext, filter)
		filename, err := saveFile(desc, ext)
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			fyne.LogError("Error selecting file", err)
			return
		}
		fyne.Do(func() {
			callback(filename)
		})
	}()
}
