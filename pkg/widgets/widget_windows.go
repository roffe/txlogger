package widgets

import (
	"errors"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"github.com/roffe/txlogger/pkg/native"
)

/*
func selectFolder(cbc func(str string)) {
	cb := func(d fyne.ListableURI, err error) {
		if err != nil {
			log.Println(err)
			return
		}
		if d == nil {
			log.Println("d is nil")
			return
		}
	}
	dialog.ShowFolderOpen(cb, fyne.CurrentApp().Driver().AllWindows()[0])
}
*/

/*
func SelectFolder2(cb func(str string)) {
	go func() {
		dir, err := sdialog.Directory().Title("Select log folder").Browse()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			log.Println(err)
			return
		}
		fyne.Do(func() {
			cb(dir)
		})
	}()
}
*/

func SelectFolder(cb func(str string)) {
	go func() {
		dir, err := native.OpenFolderDialog("Select log folder")
		if err != nil {
			if errors.Is(err, native.ErrCancelled) {
				return
			}
			log.Println("Error selecting folder:", err)
			return
		}
		fyne.Do(func() {
			cb(dir)
		})
	}()
}

func SelectFile(cb func(r fyne.URIReadCloser), desc string, exts ...string) {
	go func() {
		filter := native.FileFilter{
			Description: desc,
			Extensions:  exts,
		}
		filename, err := native.OpenFileDialog("Open file", filter)
		if err != nil {
			if errors.Is(err, native.ErrCancelled) {
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
		fyne.Do(func() { cb(r) })
	}()
}

/*
func SelectFile2(cb func(r fyne.URIReadCloser), desc string, exts ...string) {
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
			fyne.LogError("Error reading file", err)
			return
		}
		fyne.Do(func() { cb(r) })
	}()
}
*/

func SaveFile(cbc func(str string), desc string, ext string) {
	go func() {
		filename, err := native.SaveFileDialog("Save "+desc, ext, native.FileFilter{
			Description: desc,
			Extensions:  []string{ext},
		})
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			fyne.LogError("Error selecting file", err)
			return
		}
		fyne.Do(func() {
			cbc(filename)
		})
	}()
}

/*
func SaveFile2(cbc func(str string), desc string, ext string) {
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
			cbc(filename)
		})
	}()
}
*/
