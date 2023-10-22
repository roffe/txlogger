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
	/*
		defer func() {
			if r := recover(); r != nil {
				os.WriteFile("panic.txt", []byte(fmt.Sprintf("Recovered from panic: %v", r)), 0644)
				fmt.Println("Recovered from panic:", r)
			}
		}()

		// Check if this is the parent or child process.
		if len(os.Args) < 2 || os.Args[1] != "child" {
			launchChild()
			return
		}
	*/

	args := os.Args
	if len(args) >= 2 && args[1] == "child" {
		log.Println("Child process started")
		args = append(args[:1], args[2:]...)
	}
	mainz(args)
}

/*
// launchChild starts the same program but with a "child" argument.
func launchChild() {
	args := append([]string{"child"}, os.Args[1:]...)
	log.Println(args)

	f, err := os.OpenFile("child.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	cmd := exec.Command(os.Args[0], args...)

	cmd.Stdout = io.MultiWriter(os.Stdout, f)
	cmd.Stderr = io.MultiWriter(os.Stderr, f)

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
*/

func mainz(args []string) {
	a := app.NewWithID("com.roffe.trl")
	a.Settings().SetTheme(&myTheme{})
	vars := kwp2000.NewVarDefinitionList()
	if len(args) == 2 {
		filename := args[1]
		if strings.HasSuffix(filename, ".t7l") || strings.HasSuffix(filename, ".t8l") {
			windows.NewLogPlayer(a, filename, nil, nil).ShowAndRun()
			return
		}

	}
	mw := windows.NewMainWindow(a, vars)
	if len(args) == 2 {
		filename := args[1]
		if strings.HasSuffix(filename, ".bin") {
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				log.Println(err)
				mw.Log(err.Error())
			}
		}
	}
	/*
		mw.SetOnDropped(func(pos fyne.Position, uri []fyne.URI) {
			for _, u := range uri {
				if strings.HasSuffix(u.Path(), ".bin") {
					log.Println("Loading symbols from", u.Path())
					if err := mw.LoadSymbolsFromFile(u.Path()); err != nil {
						log.Println(err)
						mw.Log(err.Error())
						return
					}
				}
				if strings.HasSuffix(u.Path(), ".t7l") || strings.HasSuffix(u.Path(), ".t8l") {
					log.Println("Loading log file", u.Path())
					windows.NewLogPlayer(a, u.Path(), nil, nil).ShowAndRun()
					return
				}
			}

		})
	*/

	mw.SetMaster()
	mw.Resize(fyne.NewSize(1024, 768))
	mw.SetContent(mw.Layout())

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
	if name == theme.SizeNamePadding {
		return 3
	}
	return theme.DefaultTheme().Size(name)
}
