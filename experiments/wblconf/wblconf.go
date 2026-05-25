package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

func main() {
	a := app.NewWithID("com.roffe.wblconfig")
	w := a.NewWindow("WBLConfig")

	yAxis := []int{102, 205, 358, 512, 665, 818, 921}
	zValues := []float64{0.58, 0.66, 0.78, 0.90, 1.02, 1.15, 1.23}

	wblConf := settings.NewWBLEditor(yAxis, zValues)

	w.SetContent(wblConf)
	w.Resize(fyne.NewSize(640, 440))
	w.ShowAndRun()
}
