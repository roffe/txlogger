package main

import (
	"image/color"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/update"
	"github.com/roffe/txlogger/pkg/windows"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

func main() {
	ready := make(chan struct{})
	a := app.NewWithID("com.roffe.trl")
	a.Settings().SetTheme(&myTheme{})
	vars := kwp2000.NewVarDefinitionList()
	if len(os.Args) > 1 {
		filename := os.Args[1]
		if strings.HasSuffix(filename, ".t7l") || strings.HasSuffix(filename, ".t8l") {
			windows.NewLogPlayer(a, filename, nil, nil).ShowAndRun()
			return
		}
	}

	mw := windows.NewMainWindow(a, vars)
	mw.SetMaster()
	mw.Resize(fyne.NewSize(1024, 768))
	mw.SetContent(mw.Layout())
	close(ready)

	go updateCheck(a, mw)
	mw.ShowAndRun()
}

func updateCheck(a fyne.App, mw fyne.Window) {
	doUpdateCheck := true
	nextUpdateCheck := a.Preferences().String("nextUpdateCheck")
	ignoreVersion := a.Preferences().String("ignoreVersion")
	if nextUpdateCheck != "" {
		if nextCheckTime, err := time.Parse(time.RFC3339, nextUpdateCheck); err == nil {
			if time.Now().Before(nextCheckTime) {
				doUpdateCheck = false
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

type myTheme struct{}

func (m myTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		//log.Println(theme.DefaultTheme().Color(name, variant))
		return color.RGBA{R: 23, G: 23, B: 24, A: 0xff}
	}

	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (m myTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m myTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m myTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameSeparatorThickness {
		return 0
	}
	if name == theme.SizeNameScrollBarSmall {
		return 5
	}
	if name == theme.SizeNameScrollBar {
		return 8
	}
	return theme.DefaultTheme().Size(name)
}
