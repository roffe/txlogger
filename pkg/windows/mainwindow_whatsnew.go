package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
)

const (
	lastVersionKey = "lastVersion"
)

func (mw *MainWindow) whatsNew() {
	lastVersion := mw.app.Preferences().String(lastVersionKey)
	if lastVersion != mw.app.Metadata().Version {
		mw.showWhatsNew()
	}
	mw.app.Preferences().SetString(lastVersionKey, mw.app.Metadata().Version)
}

func (mw *MainWindow) showWhatsNew() {
	if w := mw.wm.HasWindow("What's new"); w != nil {
		mw.wm.Raise(w)
		return
	}
	md := widget.NewRichTextFromMarkdown(assets.WhatsNew)
	md.Wrapping = fyne.TextWrapWord
	iw := multiwindow.NewSystemWindow("What's new", container.NewVScroll(md))
	iw.Icon = theme.InfoIcon()
	iw.Resize(fyne.NewSize(700, 400))
	mw.wm.Add(iw)
}
