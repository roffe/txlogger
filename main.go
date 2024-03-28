package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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
	mainz(os.Args)
}

func mainz(args []string) {
	a := app.NewWithID("com.roffe.txlogger")
	a.Settings().SetTheme(&txTheme{})

	if err := presets.Load(a); err != nil {
		log.Println(err)
	}

	meta := a.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d", meta.Version, meta.Build))

	var mw *windows.MainWindow

	if len(args) == 2 {
		filename := args[1]
		if strings.HasSuffix(filename, ".bin") {
			mw = windows.NewMainWindow(a, filename)
		}
		if strings.HasSuffix(filename, ".t7l") || strings.HasSuffix(filename, ".t8l") || strings.HasSuffix(filename, ".csv") {
			windows.NewLogPlayer(a, filename, nil).ShowAndRun()
			return
		}
	}

	if mw == nil {
		mw = windows.NewMainWindow(a, "")
	}

	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quitChan
		mw.CloseIntercept()
		a.Quit()
	}()

	lastVersion := a.Preferences().String("lastVersion")
	if lastVersion != a.Metadata().Version {
		go func() {
			time.Sleep(1500 * time.Millisecond)
			ww := a.NewWindow("What's new")
			md := widget.NewRichTextFromMarkdown(whatsNew)
			md.Wrapping = fyne.TextWrapWord
			ww.SetContent(container.NewVScroll(md))
			ww.Resize(fyne.NewSize(700, 400))
			ww.Show()
			time.Sleep(200 * time.Millisecond)
			ww.RequestFocus()
		}()
	}
	a.Preferences().SetString("lastVersion", a.Metadata().Version)
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
		return 4
	}
	if name == theme.SizeNameScrollBar {
		return 8
	}
	if name == theme.SizeNamePadding {
		return 2
	}
	return theme.DefaultTheme().Size(name)
}
