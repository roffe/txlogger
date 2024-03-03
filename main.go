package main

import (
	"image/color"
	"log"
	"os"
	"strings"

	//	_ "net/http/pprof"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"github.com/roffe/txlogger/pkg/windows"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	//go func() {
	//	if err := http.ListenAndServe(":8080", nil); err != nil {
	//		log.Println(err)
	//	}
	//}()
}

func main() {
	mainz(os.Args)
}

func mainz(args []string) {
	a := app.NewWithID("com.roffe.txlogger")
	a.Settings().SetTheme(&txTheme{})

	var mw *windows.MainWindow

	if len(args) == 2 {
		filename := args[1]
		if strings.HasSuffix(filename, ".bin") {
			mw = windows.NewMainWindow(a, filename)
		}
		if strings.HasSuffix(filename, ".t7l") || strings.HasSuffix(filename, ".t8l") {
			windows.NewLogPlayer(a, filename, nil).ShowAndRun()
			return
		}
	}

	if mw == nil {
		mw = windows.NewMainWindow(a, "")
	}

	//go updateCheck(a, mw)
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
		return color.RGBA{R: 23, G: 23, B: 24, A: 0xff}
	}

	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (m txTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m txTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m txTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameSeparatorThickness {
		return 0
	}
	if name == theme.SizeNameScrollBarSmall {
		return 5
	}
	if name == theme.SizeNameScrollBar {
		return 8
	}
	if name == theme.SizeNamePadding {
		return 3
	}
	return theme.DefaultTheme().Size(name)
}
