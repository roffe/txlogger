package windows

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/boosttuner"
	"github.com/roffe/txlogger/pkg/widgets/canflasher"
	"github.com/roffe/txlogger/pkg/widgets/matrixbuilder"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

// openBoostTuner opens (or raises) the T7 boost auto-tuner. It reads the current
// BoostCal maps from the loaded binary and writes tuned maps back through a save
// closure that takes a one-time .bak of the file before the first write.
func (mw *MainWindow) openBoostTuner() {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}
	if w := mw.wm.HasWindow("Boost Auto-Tuner"); w != nil {
		mw.wm.Raise(w)
		return
	}
	save := func(symbolName string, data []float64) error {
		sym := mw.fw.GetByName(symbolName)
		if sym == nil {
			return fmt.Errorf("symbol %s not found", symbolName)
		}
		if err := sym.SetData(sym.EncodeFloat64s(data)); err != nil {
			return err
		}
		if mw.filename != "" {
			if bak := mw.filename + ".bak"; !fileExists(bak) {
				if orig, err := os.ReadFile(mw.filename); err == nil {
					_ = os.WriteFile(bak, orig, 0o644)
				}
			}
		}
		return mw.fw.Save(mw.filename)
	}
	bt := boosttuner.New(boosttuner.Config{
		Symbols:      mw.fw,
		Save:         save,
		MeshRenderer: mw.settings.GetMeshRenderer(),
		Colorblind:   mw.settings.GetColorBlindMode(),
	})
	inner := multiwindow.NewInnerWindow("Boost Auto-Tuner", bt)
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(1100, 760))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// openMatrixBuilder opens (or raises) the matrix builder window. The builder
// loads its own log files, so it is independent of any open log player.
func (mw *MainWindow) openMatrixBuilder() {
	if w := mw.wm.HasWindow("Matrix builder"); w != nil {
		mw.wm.Raise(w)
		return
	}
	inner := multiwindow.NewInnerWindow("Matrix builder", matrixbuilder.New(mw.settings.GetMeshRenderer()))
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(1000, 720))
}

func (mw *MainWindow) newToolbar() *fyne.Container {
	toolbar := container.NewHBox(
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("ECU"),
			mw.selects.remoteSelect,
			mw.selects.ecuSelect,
		),
		widget.NewSeparator(),
		mw.buttons.symbolListBtn,
		mw.buttons.logBtn,
		mw.buttons.dashboardBtn,
		mw.buttons.livePlotBtn,
		widget.NewButtonWithIcon("Matrix", theme.GridIcon(), mw.openMatrixBuilder),
		widget.NewButtonWithIcon("", theme.GridIcon(), func() {
			mw.wm.Arrange(&multiwindow.GridArranger{})
		}),
		mw.buttons.addGaugeBtn,
		widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
			mw.wm.CloseAll()
		}),
	)
	if mw.previewFeatures {
		toolbar.Add(widget.NewButtonWithIcon("", theme.UploadIcon(), func() {
			if w := mw.wm.HasWindow("Canflasher"); w != nil {
				mw.wm.Raise(w)
				return
			}
			inner := multiwindow.NewInnerWindow("Canflasher", canflasher.New(&canflasher.Config{
				CSW: mw.settings,
			}))
			inner.Icon = theme.UploadIcon()
			mw.wm.Add(inner)
			inner.Resize(fyne.NewSize(450, 250))
		}),
		)

		toolbar.Add(widget.NewButtonWithIcon("Boost", theme.MediaFastForwardIcon(), mw.openBoostTuner))
		/*
			toolbar.Add(widget.NewButtonWithIcon("", theme.DocumentIcon(), func() {
				if w := mw.wm.HasWindow("txweb"); w != nil {
					mw.wm.Raise(w)
					return
				}
				txb := txweb.New()
				txb.LoadFileFunc = func(name string, data []byte) error {
					switch filepath.Ext(name) {
					case ".bin":
						if err := mw.LoadSymbolsFromBytes(name, data); err != nil {
							return err
						}
						return nil
					case ".csv": // ".t5l", ".t7l", ".t8l",
						mw.LoadLogfile(name, bytes.NewReader(data), fyne.NewPos(100, 100))
						return nil
					}
					return nil
				}
				inner := multiwindow.NewInnerWindow("txweb", txb)
				inner.Icon = theme.FileApplicationIcon()
				mw.wm.Add(inner)
				inner.Resize(fyne.NewSize(700, 500))
			}),
			)
		*/

		/*
			widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
				if w := mw.wm.HasWindow("Map"); w != nil {
					mw.wm.Raise(w)
					return
				}
				mapp := maps.NewMap()
				cnt := container.NewBorder(
					nil,
					widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
						mapp.SetCenter(59.644810, 17.058252)
					}),
					nil,
					nil,
					mapp,
				)

				inner := multiwindow.NewInnerWindow("Map", cnt)
				inner.Icon = theme.NavigateNextIcon()
				mw.wm.Add(inner)
			}),
		*/
	}
	return toolbar
}
