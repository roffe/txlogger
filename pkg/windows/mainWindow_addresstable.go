package windows

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/trionic7/t7fwinfo"
)

// importAddressTable replaces the loaded T7 binary's symbol address table with
// the one from a donor of the same software trace and saves the result as a
// new file. Tuners poison address-table entries to hide maps; code and data
// are untouched, so a clean table restores them. The loaded file is left as is.
func (mw *MainWindow) importAddressTable() {
	if mw.fw == nil {
		mw.Error(errors.New("no binary loaded"))
		return
	}
	t7, ok := mw.fw.(*symbol.T7File)
	if !ok {
		mw.Error(errors.New("import address table is only implemented for Trionic 7"))
		return
	}
	widgets.SelectFile(func(r fyne.URIReadCloser) {
		defer r.Close()
		donor, err := io.ReadAll(r)
		if err != nil {
			mw.Error(err)
			return
		}
		if err := t7.TransferSymbolTable(donor); err != nil {
			mw.Error(fmt.Errorf("import address table from %s: %w", filepath.Base(r.URI().Path()), err))
			return
		}
		mw.Log(fmt.Sprintf("imported address table from %s: %d symbols", filepath.Base(r.URI().Path()), len(t7.Symbols())))
		widgets.SaveFile(func(filename string) {
			if !strings.HasSuffix(strings.ToLower(filename), ".bin") {
				filename += ".bin"
			}
			if err := os.WriteFile(filename, t7.Bytes(), 0o644); err != nil {
				mw.Error(err)
				return
			}
			mw.Log("saved " + filename)
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Error(err)
			}
		}, "Binary file", "bin")
	}, "Donor binary (same software)", "bin")
}

// saveLoadedBinary writes the loaded binary back to its file, taking a
// one-time .bak of the original first.
func (mw *MainWindow) saveLoadedBinary() error {
	if mw.filename == "" {
		return errors.New("no filename for loaded binary")
	}
	if bak := mw.filename + ".bak"; !fileExists(bak) {
		if orig, err := os.ReadFile(mw.filename); err == nil {
			_ = os.WriteFile(bak, orig, 0o644)
		}
	}
	return mw.fw.Save(mw.filename)
}

// openFirmwareInfo opens (or raises) T7Suite's "Firmware information" dialog
// for the loaded Trionic 7 binary.
func (mw *MainWindow) openFirmwareInfo() {
	if mw.fw == nil {
		mw.Error(errors.New("no binary loaded"))
		return
	}
	t7, ok := mw.fw.(*symbol.T7File)
	if !ok {
		mw.Error(errors.New("firmware information is only implemented for Trionic 7"))
		return
	}
	if w := mw.wm.HasWindow("Firmware information"); w != nil {
		mw.wm.Raise(w)
		return
	}
	inner := multiwindow.NewInnerWindow("Firmware information", t7fwinfo.New(t7fwinfo.Config{FW: t7, Save: mw.saveLoadedBinary, Log: mw.Log}))
	inner.Icon = theme.InfoIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(900, 760))
}
