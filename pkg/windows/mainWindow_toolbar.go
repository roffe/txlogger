package windows

import (
	"bytes"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/canflasher"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/txweb"
)

func (mw *MainWindow) newToolbar() *fyne.Container {
	toolbar := container.NewHBox(
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("ECU"),
			nil,
			mw.selects.ecuSelect,
		),
		widget.NewSeparator(),
		mw.buttons.symbolListBtn,
		mw.buttons.logBtn,

		//mw.buttons.logplayerBtn,
		mw.buttons.openLogBtn,
		mw.buttons.dashboardBtn,
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
				GetECU: func() string {
					return mw.selects.ecuSelect.Selected
				},
			}))
			inner.Icon = theme.UploadIcon()
			mw.wm.Add(inner)
			inner.Resize(fyne.NewSize(450, 250))

		}),
		)

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
				case ".t5l", ".t7l", ".t8l", ".csv":
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
