package main

import (
	"bufio"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ipc"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/theme"
	"github.com/roffe/txlogger/pkg/windows"
	// _ "net/http/pprof"
)

var (
	workDirectory string
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	flag.StringVar(&workDirectory, "d", "", "working directory")
	flag.Parse()
}

/*
	func startpprof() {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

*/

// Unfortunately Fyne installs its own signal handler that needs to be overridden
// to allow graceful shutdown on SIGINT/SIGTERM.
func signalHandler(mw *windows.MainWindow) {
	time.Sleep(1 * time.Second)
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	log.Println("installed signal handler")
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Println("Caught:", s)
	fyne.DoAndWait(mw.Close)
	//fyne.CurrentApp().Driver().Quit()
}

func startCanGateway() (*os.Process, error) {
	if wd, err := os.Getwd(); err == nil {
		readyChan := make(chan struct{})
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
						log.Println("goCAN Gateway exited")
						return
					}
					log.Printf("goCAN Gateway error: %s", err)
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
			log.Fatal("Failed to start goCAN Gateway")
		}

		log.Printf("Started goCAN Gateway pid: %d", cmd.Process.Pid)

		select {
		case <-readyChan:
			debug.Log("goCAN Gateway is ready")
			return cmd.Process, nil
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("goCAN Gateway was not ready after 5 seconds")
		}
	}
	return nil, fmt.Errorf("failed to get working directory")
}

/*
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
*/

func main() {
	//startpprof()
	defer debug.Close()
	defer debug.Log("txlogger exit")

	//ver := RtlGetVersion()
	//if ver.MajorVersion < 10 {
	//	sdialog.Message("txlogger requires Windows 10 or later").Title("Unsupported Windows version").Error()
	//	return
	//}

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

	p, err := startCanGateway()
	if p != nil {
		defer func(p *os.Process) {
			if p != nil {
				p.Kill()
				p.Wait()
			}
		}(p)
	}
	if err != nil {
		debug.Log("GoCAN Gateway was not ready after 5 seconds")
	}

	tx := app.NewWithID("com.roffe.txlogger")
	tx.Settings().SetTheme(&theme.TxTheme{})

	if err := presets.Load(tx); err != nil {
		debug.Log("failed to load presets: " + err.Error())
	}

	meta := tx.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d tempDir: %s", meta.Version, meta.Build, os.TempDir()))
	//log.Printf("starting txlogger v%s build %d tempDir: %s", meta.Version, meta.Build, os.TempDir())

	mw := windows.NewMainWindow(tx)

	sockServ, err := ipc.NewServer(
		createIPCRouter(mw),
		socketFile,
	)
	if err != nil {
		debug.Log("error: " + err.Error())
	} else {
		defer sockServ.Close()
	}

	handleArgs(mw, tx)

	//go updateCheck(a, mw)

	go signalHandler(mw)
	mw.ShowAndRun()
}

func handleArgs(mw *windows.MainWindow, tx fyne.App) {
	var loadedSymbols bool
	if filename := flag.Arg(0); filename != "" {
		switch strings.ToLower(path.Ext(filename)) {
		case ".bin":
			//mw = windows.NewMainWindow(a, filename)
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
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
			sz := mw.Canvas().Size()
			mw.LoadLogfile(filename, f, fyne.Position{X: sz.Width / 2, Y: sz.Height / 2})
		}
	}

	if filename := tx.Preferences().String("lastBinFile"); filename != "" && !loadedSymbols {
		if fileExists(filename) {
			mw.LoadSymbolsFromFile(filename)
		}
	}
}

func createIPCRouter(mw *windows.MainWindow) ipc.Router {
	return ipc.Router{
		"ping": func(data string) *ipc.Message {
			return &ipc.Message{Type: "pong", Data: ""}
		},
		"open": func(filename string) *ipc.Message {
			fyne.DoAndWait(mw.Window.RequestFocus)
			if strings.HasSuffix(filename, ".bin") {
				mw.LoadSymbolsFromFile(filename)
			}
			if isLogfile(filename) {
				f, err := os.Open(filename)
				if err != nil {
					mw.Error(err)
				}
				defer f.Close()
				sz := mw.Canvas().Size()
				mw.LoadLogfile(filename, f, fyne.Position{X: sz.Width / 2, Y: sz.Height / 2})
			}
			return nil
		},
	}
}

var logfileExtensions = [...]string{".t5l", ".t7l", ".t8l", ".csv"}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}

func isLogfile(name string) bool {
	filename := strings.ToLower(name)
	for _, ext := range logfileExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
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
