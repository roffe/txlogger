package widgets

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
)

// RunFileChild is only used on desktop Linux, where file dialogs are shown by a
// child process.
func RunFileChild() bool {
	return false
}

// SelectFile shows the Android document picker. The picked document is copied
// into the app's files directory and passed on as a plain file, so callers can
// keep using its path for reading, writeback and the recent list.
// ponytail: edits land in the copy, not the picked document; export through
// the save picker once SaveFile is supported.
func SelectFile(callback func(r fyne.URIReadCloser), desc string, exts ...string) {
	filter := make([]string, len(exts))
	for i, ext := range exts {
		filter[i] = "." + ext
	}
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			log.Println("File dialog:", err)
			return
		}
		if r == nil {
			return // cancelled
		}
		go func() {
			local, err := copyToLocal(r)
			if err != nil {
				log.Println("File dialog:", err)
				return
			}
			fyne.Do(func() { callback(local) })
		}()
	}, mainWindow())
	d.SetFilter(storage.NewExtensionFileFilter(filter))
	d.Show()
}

// SelectFiles picks a single file, the Android picker has no multi-select.
func SelectFiles(callback func(rc []fyne.URIReadCloser), desc string, exts ...string) {
	SelectFile(func(r fyne.URIReadCloser) { callback([]fyne.URIReadCloser{r}) }, desc, exts...)
}

// ponytail: callers write to the path after the callback returns, a content
// URI needs a writer; add a copy-back when saving on Android is needed.
func SaveFile(callback func(str string), desc, ext, name string) {
	dialog.ShowError(errors.New("saving files is not supported on Android yet"), mainWindow())
}

func SelectFolder(callback func(str string)) {
	dialog.ShowError(errors.New("selecting folders is not supported on Android yet"), mainWindow())
}

func mainWindow() fyne.Window {
	return fyne.CurrentApp().Driver().AllWindows()[0]
}

// copyToLocal copies the picked document into the app's files directory under
// its display name and opens the copy.
func copyToLocal(r fyne.URIReadCloser) (fyne.URIReadCloser, error) {
	defer r.Close()
	// Base: the name comes from the document provider.
	dst := filepath.Join(fyne.CurrentApp().Storage().RootURI().Path(), filepath.Base(r.URI().Name()))
	f, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return storage.Reader(storage.NewFileURI(dst))
}
