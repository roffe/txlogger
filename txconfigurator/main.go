package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/roffe/txlogger/pkg/widgets/txconfigurator"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("txbridge configurator")

	p := func() string {
		return "192.168.4.1:1337"
	}
	cfg := txconfigurator.NewConfigurator(p)
	myWindow.SetContent(cfg)
	myWindow.Resize(fyne.NewSize(480, 200))
	myWindow.ShowAndRun()
}
