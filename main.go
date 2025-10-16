package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/roffe/txlogger/pkg/cangw"
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

	if workDirectory != "" {
		debug.Log("changing working directory to" + workDirectory)
		if err := os.Chdir(workDirectory); err != nil {
			debug.Log(fmt.Sprintf("failed to change working directory to %s: %v", workDirectory, err))
		}
	}
}

// func startpprof() {
// 	go func() {
// 		log.Println(http.ListenAndServe("localhost:6060", nil))
// 	}()
// }

// Unfortunately Fyne installs its own signal handler that needs to be overridden
// to allow graceful shutdown on SIGINT/SIGTERM.
func signalHandler(mw *windows.MainWindow) {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	log.Println("installed signal handler")
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Println("caught:", s)
	fyne.DoAndWait(mw.Close)
	//fyne.CurrentApp().Driver().Quit()
}

func main() {
	//startpprof()
	defer debug.Close()
	defer debug.Log("txlogger exit")

	if ipc.IsRunning() {
		return
	}

	p, err := cangw.Start()
	if p != nil {
		defer killProcess(p)
	}
	if err != nil {
		debug.Log("cangateway is not ready: " + err.Error())
	}

	tx := app.NewWithID("com.roffe.txlogger")
	tx.Settings().SetTheme(&theme.TxTheme{})

	if err := presets.Load(tx); err != nil {
		debug.Log("failed to load presets: " + err.Error())
	}

	meta := tx.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d", meta.Version, meta.Build))

	mw := windows.NewMainWindow(tx)

	fyne.CurrentApp().Lifecycle().SetOnStarted(func() {
		go signalHandler(mw) // Install our own signal handler
	})

	sockServ, err := ipc.NewServer(ipc.CreateIPCRouter(mw))
	if err != nil {
		debug.Log("error: " + err.Error())
	} else {
		defer sockServ.Close()
	}

	handleArgs(mw, tx)

	//go updateCheck(a, mw)

	mw.ShowAndRun()
}

func killProcess(p *os.Process) {
	if p != nil {
		p.Kill()
		p.Wait()
	}
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

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}
