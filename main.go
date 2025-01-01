package main

import (
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ipc"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/windows"
)

var (
	workDirectory string
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	flag.StringVar(&workDirectory, "d", "", "working directory")
	flag.Parse()
}

func main() {
	socketFile := filepath.Join(os.TempDir(), "txlogger.sock")
	if fileExists(socketFile) {
		if !ping(socketFile) {
			log.Println("txlogger is not running, removing stale socket file")
			if err := os.Remove(socketFile); err != nil {
				log.Println(err)
			}
		} else {
			log.Println("txlogger is running, sending show request over socket")
			sendShow(socketFile)
			return
		}
	}

	if workDirectory != "" {
		if err := os.Chdir(workDirectory); err != nil {
			log.Println(err)
		}
	}

	s, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Printf("%T: %w", err, err)
	}
	defer s.Close() // cleanup

	a := app.NewWithID("com.roffe.txlogger")
	a.Settings().SetTheme(&txTheme{})

	if err := presets.Load(a); err != nil {
		log.Println(err)
	}

	meta := a.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d", meta.Version, meta.Build))
	log.Printf("starting txlogger v%s build %d", meta.Version, meta.Build)
	log.Printf("tempDir: %s", os.TempDir())

	var mw *windows.MainWindow

	go func() {
		for {
			conn, err := s.Accept()
			if err != nil {
				log.Println(err)
				return
			}
			go func() {
				defer conn.Close()
				gb := gob.NewDecoder(conn)
				ge := gob.NewEncoder(conn)
				var msg ipc.Message
				err := gb.Decode(&msg)
				if err != nil {
					if err == io.EOF {
						return
					}
					log.Println(err)
					return
				}
				log.Println(msg)
				switch msg.Type {
				case "ping":
					err = ge.Encode(ipc.Message{Type: "pong", Data: ""})
					if err != nil {
						log.Println(err)
					}
				case "open":
					mw.RequestFocus() // show window
					if strings.HasSuffix(msg.Data, ".bin") {
						mw.LoadSymbolsFromFile(msg.Data)
					}
					if strings.HasSuffix(msg.Data, ".t5l") || strings.HasSuffix(msg.Data, ".t7l") || strings.HasSuffix(msg.Data, ".t8l") || strings.HasSuffix(msg.Data, ".csv") {
						mw.LoadLogfile(msg.Data)
					}
				}
			}()
		}
	}()

	if mw == nil {
		mw = windows.NewMainWindow(a, "")
	}

	if filename := flag.Arg(0); filename != "" {
		switch strings.ToLower(path.Ext(filename)) {
		case ".bin":
			mw = windows.NewMainWindow(a, filename)
		case ".t5l", ".t7l", ".t8l", ".csv":
			mw.LoadLogfile(filename)
		}
	}

	//go updateCheck(a, mw)

	mw.ShowAndRun()
}

func sendShow(socketFile string) {
	c, err := net.Dial("unix", socketFile)
	if err != nil {
		var nErr *net.OpError
		if errors.As(err, &nErr) {
			if nErr.Op == "dial" {
				log.Println("txlogger is not running")
				return
			}
		}
		log.Println(err)
		return
	}
	defer c.Close()
	gb := gob.NewEncoder(c)
	if filename := flag.Arg(0); filename != "" {
		err = gb.Encode(ipc.Message{Type: "open", Data: flag.Arg(0)})
	} else {
		err = gb.Encode(ipc.Message{Type: "open", Data: ""})
	}
	if err != nil {
		log.Println(err)
	}
}

func ping(socketFile string) bool {
	c, err := net.Dial("unix", socketFile)
	if err != nil {
		var nErr *net.OpError
		if errors.As(err, &nErr) {
			if nErr.Op == "dial" {
				return false
			}
		}
		log.Println(err)
		return false
	}
	defer c.Close()

	gdec := gob.NewDecoder(c)
	gb := gob.NewEncoder(c)

	err = gb.Encode(ipc.Message{Type: "ping", Data: ""})
	if err != nil {
		log.Println(err)
	}

	var msg ipc.Message
	err = gdec.Decode(&msg)
	if err != nil {
		log.Println(err)
		return false
	}

	if msg.Type == "pong" {
		log.Println("txlogger is running")
		return true
	}

	return false
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
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
