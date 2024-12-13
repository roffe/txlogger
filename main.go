package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"path"
	"strings"
	"time"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/windows"
)

//go:embed WHATSNEW.md
var whatsNew string

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

func main() {
	//if err := wn.TimeBeginPeriod(1); err != nil {
	//	log.Println(err)
	//}
	//defer func() {
	//	if err := wn.TimeEndPeriod(1); err != nil {
	//		log.Println(err)
	//	}
	//}()
	a := app.NewWithID("com.roffe.txlogger")
	a.Settings().SetTheme(&txTheme{})

	if err := presets.Load(a); err != nil {
		log.Println(err)
	}

	meta := a.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d", meta.Version, meta.Build))
	log.Printf("starting txlogger v%s build %d", meta.Version, meta.Build)

	var mw *windows.MainWindow
	if len(os.Args) == 2 {
		filename := os.Args[1]
		switch strings.ToLower(path.Ext(filename)) {
		case ".bin":
			mw = windows.NewMainWindow(a, filename)
		case ".t5l", ".t7l", ".t8l", ".csv":
			windows.NewLogPlayer(a, filename, nil).ShowAndRun()
			return
		}
	}

	if mw == nil {
		mw = windows.NewMainWindow(a, "")
	}

	//quitChan := make(chan os.Signal, 2)
	//signal.Notify(quitChan, os.Interrupt, syscall.SIGTERM)
	//go func() {
	//	<-quitChan
	//	//mw.CloseIntercept()
	//	a.Quit()
	//}()

	lastVersion := a.Preferences().String("lastVersion")
	if lastVersion != a.Metadata().Version {
		go func() {
			time.Sleep(1000 * time.Millisecond)
			ww := a.NewWindow("What's new")
			md := widget.NewRichTextFromMarkdown(whatsNew)
			md.Wrapping = fyne.TextWrapWord
			ww.SetContent(container.NewVScroll(md))
			ww.Resize(fyne.NewSize(700, 400))
			ww.CenterOnScreen()
			ww.Show()
			time.Sleep(100 * time.Millisecond)
			ww.RequestFocus()
		}()
	}
	a.Preferences().SetString("lastVersion", a.Metadata().Version)
	//go updateCheck(a, mw)

	//a.Lifecycle().SetOnEnteredForeground(func() {
	//	mw.Maximize()
	//})

	mw.ShowAndRun()
}

/*
func updateCheck(a fyne.App, mw fyne.Window) {
	doUpdateCheck := false
	nextUpdateCheck := a.Preferences().String("nextUpdateCheck")
	ignoreVersion := a.Preferences().String("ignoreVersion")
	if nextUpdateCheck != "" {
		if nextCheckTime, err := time.Parse(time.RFC3339, nextUpdateCheck); err == nil {
			if time.Now().After(nextCheckTime) {
				doUpdateCheck = true
			}
		}
	}
	if doUpdateCheck {
		if isLatest, latestVersion := update.IsLatest("v" + a.Metadata().Version); !isLatest {
			if ignoreVersion == latestVersion {
				return
			}
			u, err := url.Parse("https://txlogger.com")
			if err != nil {
				panic(err)
			}
			link := widget.NewHyperlink("txlogger.com", u)
			link.Alignment = fyne.TextAlignCenter
			link.TextStyle = fyne.TextStyle{Bold: true}
			dialog.ShowCustomConfirm(
				"Update available!",
				"Remind me", "Don't remind me",
				container.NewVBox(
					widget.NewLabel("There is a new version available"),
					link,
				),
				func(choice bool) {
					if !choice {
						a.Preferences().SetString("ignoreVersion", "v"+a.Metadata().Version)
					}
				},
				mw,
			)
		}
		if tt, err := time.Now().Add(96 * time.Hour).MarshalText(); err == nil {
			a.Preferences().SetString("nextUpdateCheck", string(tt))
		}
	}
}
*/

type txTheme struct{}

func (m txTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		return color.RGBA{R: 23, G: 23, B: 24, A: 255}
	}

	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

var dragcornerindicatorleftIconRes = &fyne.StaticResource{
	StaticName:    "drag-corner-indicator-left.svg",
	StaticContent: assets.LeftCornerBytes,
}

func (m txTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	switch name {
	case fyne.ThemeIconName("drag-corner-indicator-left"):
		return theme.NewThemedResource(dragcornerindicatorleftIconRes)
	default:
		return theme.DefaultTheme().Icon(name)
	}
}

func (m txTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m txTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameSeparatorThickness: // denna 0
		return 0
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameLineSpacing:
		return 4
	case theme.SizeNamePadding: // 2
		return 4
	case theme.SizeNameScrollBar: // 8
		return 16
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNameSubHeadingText:
		return 18
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInputRadius:
		return 5
	case theme.SizeNameSelectionRadius:
		return 3
	default:
		return 0
	}

}
