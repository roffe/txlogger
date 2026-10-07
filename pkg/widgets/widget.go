package widgets

import (
	"errors"
	"fmt"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"github.com/roffe/browse"
)

const (
	opOpenFile  = "open_file"
	opOpenFiles = "open_files"
	opSaveFile  = "save_file"
	opFolder    = "select_folder"
)

// showDialog shows a native file dialog in this process.
func showDialog(op string, o browse.Options) ([]string, error) {
	var path string
	var err error
	switch op {
	case opOpenFiles:
		return browse.OpenFiles(o)
	case opOpenFile:
		path, err = browse.OpenFile(o)
	case opSaveFile:
		path, err = browse.SaveFile(o)
	case opFolder:
		path, err = browse.OpenFolder(o)
	default:
		return nil, fmt.Errorf("unknown file dialog operation %q", op)
	}
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

// pick shows a file dialog and reports whether the user chose anything.
func pick(op string, o browse.Options) ([]string, bool) {
	paths, err := dialog(op, o)
	if err != nil {
		if !errors.Is(err, browse.ErrCancelled) {
			log.Println("File dialog:", err)
		}
		return nil, false
	}
	return paths, true
}

func SelectFolder(callback func(str string)) {
	go func() {
		paths, ok := pick(opFolder, browse.Options{Title: "Select log folder"})
		if !ok {
			return
		}
		fyne.Do(func() {
			callback(paths[0])
		})
	}()
}

func SelectFile(callback func(r fyne.URIReadCloser), desc string, exts ...string) {
	go func() {
		paths, ok := pick(opOpenFile, browse.Options{Title: "Open " + desc, Filters: []browse.Filter{{Name: desc, Extensions: exts}}})
		if !ok {
			return
		}
		uri := storage.NewFileURI(paths[0])
		r, err := storage.Reader(uri)
		if err != nil {
			log.Println("Error reading file:", err)
			return
		}
		fyne.Do(func() { callback(r) })
	}()
}

func SelectFiles(callback func(rc []fyne.URIReadCloser), desc string, exts ...string) {
	go func() {
		filenames, ok := pick(opOpenFiles, browse.Options{Title: "Open " + desc, Filters: []browse.Filter{{Name: desc, Extensions: exts}}})
		if !ok {
			return
		}
		readers := make([]fyne.URIReadCloser, 0, len(filenames))
		for _, filename := range filenames {
			uri := storage.NewFileURI(filename)
			r, err := storage.Reader(uri)
			if err != nil {
				log.Println("Error reading file:", err)
				continue
			}
			readers = append(readers, r)
		}
		if len(readers) == 0 {
			return
		}
		fyne.Do(func() { callback(readers) })
	}()
}

// SaveFile asks for a file to save to, name is the suggested file name.
func SaveFile(callback func(str string), desc, ext, name string) {
	go func() {
		paths, ok := pick(opSaveFile, browse.Options{Title: "Save " + desc, Name: name, Filters: []browse.Filter{{Name: desc, Extensions: []string{ext}}}})
		if !ok {
			return
		}
		fyne.Do(func() {
			callback(paths[0])
		})
	}()
}
