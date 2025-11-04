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

var workDirectory string

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.StringVar(&workDirectory, "d", "", "working directory")
	flag.Parse()

}

// Unfortunately Fyne installs its own signal handler that needs to be overridden to allow graceful shutdown on SIGINT/SIGTERM.
func signalHandler(mw *windows.MainWindow) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	debug.Log("installed signal handler")
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	debug.Log("caught:" + s.String())
	fyne.DoAndWait(mw.Close)
	//fyne.CurrentApp().Driver().Quit()
}

func main() {
	InitConsole()
	// change working directory if requested
	if workDirectory != "" {
		debug.Log("changing working directory to " + workDirectory)
		if err := os.Chdir(workDirectory); err != nil {
			debug.Log(fmt.Sprintf("failed to change working directory to %s: %v", workDirectory, err))
		}
	}

	//startpprof()
	defer debug.Close()
	defer debug.Log("txlogger exit")

	// if another instance is running, just show its window and exit
	if ipc.IsRunning() {
		return
	}

	// start cangateway if not already running
	p, err := cangw.Start()
	if p != nil {
		defer killProcess(p)
	}
	if err != nil {
		debug.Log("cangateway is not ready: " + err.Error())
	}

	// create app
	tx := app.NewWithID("com.roffe.txlogger")
	tx.Settings().SetTheme(&theme.TxTheme{})

	// load presets
	if err := presets.Load(tx); err != nil {
		debug.Log("failed to load presets: " + err.Error())
	}

	// log version info
	metadata := tx.Metadata()
	debug.Log(fmt.Sprintf("starting txlogger v%s build %d", metadata.Version, metadata.Build))

	// create main window
	mw := windows.NewMainWindow(tx)

	// install our own signal handler
	fyne.CurrentApp().Lifecycle().SetOnStarted(func() {
		go signalHandler(mw)
	})

	// start IPC server
	sockServ, err := ipc.NewServer(ipc.CreateIPCRouter(mw))
	if err != nil {
		debug.Log("error: " + err.Error())
	} else {
		defer sockServ.Close()
	}

	// handle command line arguments
	handleArgs(mw, tx)

	// show main window
	mw.ShowAndRun()
}

/*
func startpprof() {
	go func() {
		debug.Log(http.ListenAndServe("localhost:6060", nil))
	}()
}
*/

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
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Error(err)
			}
		}
	}
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}
