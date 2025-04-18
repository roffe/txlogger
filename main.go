package main

import (
	"bufio"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ipc"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/windows"
	sdialog "github.com/sqweek/dialog"

	"net/http"
	_ "net/http/pprof"
)

var (
	workDirectory string
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	flag.StringVar(&workDirectory, "d", "", "working directory")
	flag.Parse()
}

func startpprof() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
}

func signalHandler(tx fyne.App) {
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT)
	go func() {
		<-sig
		//rdebug.PrintStack()
	}()
}

func startCanGateway(errFunc func(error), readyChan chan<- struct{}) *os.Process {

	if wd, err := os.Getwd(); err == nil {
		command := filepath.Join(wd, "cangateway.exe")
		cmd := exec.Command(command)
		//cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		rc, err := cmd.StderrPipe()
		if err != nil {
			log.Fatal(err)
		}
		r := bufio.NewReader(rc)
		go func() {
			starting := true
			for {
				str, err := r.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						errFunc(errors.New("GoCAN Gateway exited"))
						return
					}
					errFunc(fmt.Errorf("GoCAN Gateway error: %s", err))
					return
				}
				fmt.Print(str)
				if strings.Contains(str, "server listening") && starting {
					close(readyChan)
					starting = false
				}
			}
		}()
		if err := cmd.Start(); err != nil {
			log.Fatal("Failed to start GoCAN Gateway")
		} else {
			// defer cmd.Process.Kill()
			log.Printf("Starting GoCAN Gateway pid: %d", cmd.Process.Pid)
		}
		return cmd.Process
	}
	return nil
}

func main() {
	isAndroid := runtime.GOOS == "android"
	if isAndroid {
		mainAndroid()
	} else {
		mainDesktop()
	}
}

func mainAndroid() {
	tx := app.NewWithID("com.roffe.txlogger")
	tx.Settings().SetTheme(&txTheme{})
	if err := presets.Load(tx); err != nil {
		log.Println(err)
	}
	meta := tx.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d tempDir: %s", meta.Version, meta.Build, os.TempDir()))
	log.Printf("starting txlogger v%s build %d tempDir: %s", meta.Version, meta.Build, os.TempDir())
	mw := windows.NewMainWindow(tx)
	mw.ShowAndRun()
}

type RTL_OSVERSIONINFOEXW struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformId        uint32
	CSDVersion        [128]uint16
	ServicePackMajor  uint16
	ServicePackMinor  uint16
	SuiteMask         uint16
	ProductType       byte
	Reserved          byte
}

func RtlGetVersion() RTL_OSVERSIONINFOEXW {
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	rtlGetVersion := ntdll.NewProc("RtlGetVersion")
	var info RTL_OSVERSIONINFOEXW
	info.OSVersionInfoSize = 5*4 + 128*2 + 3*2 + 2*1
	rtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	return info
}

func mainDesktop() {
	defer log.Println("txlogger exit")
	ver := RtlGetVersion()
	if ver.MajorVersion < 10 {
		sdialog.Message("txlogger requires Windows 10 or later").Title("Unsupported Windows version").Error()
		return
	}

	//startpprof()
	socketFile := filepath.Join(os.TempDir(), "txlogger.sock")

	if ipc.IsRunning(socketFile) {
		return
	}

	if workDirectory != "" {
		debug.Log("changing working directory to" + workDirectory)
		if err := os.Chdir(workDirectory); err != nil {
			debug.Log(fmt.Sprintf("failed to change working directory to %s: %v", workDirectory, err))
		}
	}

	readyChan := make(chan struct{})

	var errFunc = func(err error) {
		log.Print(err.Error())
	}

	if p := startCanGateway(errFunc, readyChan); p != nil {
		defer func(p *os.Process) {
			if p != nil {
				p.Kill()
				p.Wait()
			}
		}(p)
	}

	select {
	case <-readyChan:
		debug.Log("GoCAN Gateway is ready")
	case <-time.After(5 * time.Second):
		debug.Log("GoCAN Gateway was not ready after 5 seconds")
	}

	tx := app.NewWithID("com.roffe.txlogger")

	//signalHandler(tx)

	tx.Settings().SetTheme(&txTheme{})

	if err := presets.Load(tx); err != nil {
		log.Println(err)
	}

	meta := tx.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d tempDir: %s", meta.Version, meta.Build, os.TempDir()))
	//log.Printf("starting txlogger v%s build %d tempDir: %s", meta.Version, meta.Build, os.TempDir())

	mw := windows.NewMainWindow(tx)

	errFunc = mw.Error

	router := ipc.Router{
		"ping": func(data string) *ipc.Message {
			return &ipc.Message{Type: "pong", Data: ""}
		},
		"open": func(filename string) *ipc.Message {
			fyne.Do(mw.Window.RequestFocus) // show window
			if strings.HasSuffix(filename, ".bin") {
				f, err := os.Open(filename)
				if err != nil {
					mw.Error(err)
				}
				defer f.Close()
				mw.LoadSymbolsFromFile(filename, f)
			}
			if strings.HasSuffix(filename, ".t5l") || strings.HasSuffix(filename, ".t7l") || strings.HasSuffix(filename, ".t8l") || strings.HasSuffix(filename, ".csv") {
				f, err := os.Open(filename)
				if err != nil {
					mw.Error(err)
				}
				defer f.Close()

				mw.LoadLogfile(filename, f, fyne.Position{})
			}
			return nil
		},
	}

	sockServ, err := ipc.NewServer(router, socketFile)
	if err != nil {
		debug.Log("error: " + err.Error())
	}
	defer sockServ.Close()

	var loadedSymbols bool

	if filename := flag.Arg(0); filename != "" {
		switch strings.ToLower(path.Ext(filename)) {
		case ".bin":
			//mw = windows.NewMainWindow(a, filename)
			f, err := os.Open(filename)
			if err != nil {
				mw.Error(err)
			}
			defer f.Close()
			if err := mw.LoadSymbolsFromFile(filename, f); err != nil {
				mw.Error(err)
			} else {
				loadedSymbols = true
			}

		case ".t5l", ".t7l", ".t8l", ".csv":
			f, err := os.Open(filename)
			if err != nil {
				mw.Error(err)
			}
			defer f.Close()

			mw.LoadLogfile(filename, f, fyne.Position{})
		}
	}

	if filename := tx.Preferences().String("lastBinFile"); filename != "" && !loadedSymbols {
		if fileExists(filename) {
			f, err := os.Open(filename)
			if err != nil {
				mw.Error(err)
			} else {
				defer f.Close()
				mw.LoadSymbolsFromFile(filename, f)
			}
		}
	}

	//go updateCheck(a, mw)

	mw.ShowAndRun()
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}

/*
func socketServer(s net.Listener, mw *windows.MainWindow) {
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
					mw.Window.RequestFocus // show window
					if strings.HasSuffix(msg.Data, ".bin") {
						mw.LoadSymbolsFromFile(msg.Data)
					}
					if strings.HasSuffix(msg.Data, ".t5l") || strings.HasSuffix(msg.Data, ".t7l") || strings.HasSuffix(msg.Data, ".t8l") || strings.HasSuffix(msg.Data, ".csv") {
						mw.LoadLogfile(msg.Data, fyne.Position{}, true)
					}
				}
			}()
		}
	}()
}
*/

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
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 23, G: 23, B: 24, A: 255}
	case fyne.ThemeColorName("primary-hover"):
		return color.RGBA{R: 0x21, G: 0x99, B: 0xF3, A: 255}
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
		return 2
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
	case theme.SizeNameWindowTitleBarHeight:
		return 26
	case theme.SizeNameWindowButtonHeight:
		return 20
	case theme.SizeNameWindowButtonIcon:
		return 20
	case theme.SizeNameWindowButtonRadius:
		return 0
	default:
		return 0
	}
}
