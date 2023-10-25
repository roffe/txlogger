package windows

import (
	"strings"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/symbol"
)

type MainMenu struct {
	w     fyne.Window
	menus []*fyne.Menu
	one   func(symbol.ECUType, string)
	mul   func(symbol.ECUType, ...string)
}

func NewMainMenu(w fyne.Window, menus []*fyne.Menu, one func(symbol.ECUType, string), mul func(symbol.ECUType, ...string)) *MainMenu {
	return &MainMenu{
		w:     w,
		one:   one,
		mul:   mul,
		menus: menus,
	}
}

func (mw *MainMenu) GetMenu(name string) *fyne.MainMenu {
	var order []string
	var ecuM map[string][]string
	var typ symbol.ECUType

	switch name {
	case "T7":
		order = symbol.T7SymbolsTuningOrder
		ecuM = symbol.T7SymbolsTuning
		typ = symbol.ECU_T7
	case "T8":
		order = symbol.T8SymbolsTuningOrder
		ecuM = symbol.T8SymbolsTuning
		typ = symbol.ECU_T8
	}

	menus := append([]*fyne.Menu{}, mw.menus...)

	for _, category := range order {
		var items []*fyne.MenuItem
		for _, mapName := range ecuM[category] {
			if strings.Contains(mapName, "|") {
				parts := strings.Split(mapName, "|")
				names := parts[1:]
				itm := fyne.NewMenuItem(parts[0], func() {
					mw.mul(typ, names...)
				})
				items = append(items, itm)
				continue
			}
			itm := fyne.NewMenuItem(mapName, func() {
				mw.one(typ, mapName)
			})
			items = append(items, itm)
		}
		menus = append(menus, fyne.NewMenu(category, items...))
	}
	return fyne.NewMainMenu(menus...)
}
