package main

import (
	"image/color"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/windows"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

func main() {
	ready := make(chan struct{})
	a := app.NewWithID("com.roffe.trl")
	a.Settings().SetTheme(&myTheme{})
	vars := kwp2000.NewVarDefinitionList()
	//sm := sink.NewManager()
	mw := windows.NewMainWindow(a, vars)
	//go dashboard.Start(mw.Log, a.Metadata().Release, sm, vars, ready)
	mw.SetMaster()
	mw.Resize(fyne.NewSize(1024, 768))
	mw.SetContent(mw.Layout())
	close(ready)
	mw.ShowAndRun()
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
	return theme.DefaultTheme().Size(name)
}
