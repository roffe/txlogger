package windows

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
)

type MainMenu struct {
	w                 fyne.Window
	leading, trailing []*fyne.Menu
	openFunc          func(symbol.ECUType, string, string)
	//multiFunc         func(symbol.ECUType, ...string)
	funcMap map[string]func(string)
}

func NewMenu(w fyne.Window, leading, trailing []*fyne.Menu, openFunc func(symbol.ECUType, string, string), funcMap map[string]func(string)) *MainMenu {
	return &MainMenu{
		w:        w,
		openFunc: openFunc,
		leading:  leading,
		trailing: trailing,
		funcMap:  funcMap,
	}
}

func (mw *MainMenu) GetMenu(name string) *fyne.MainMenu {
	var order []string
	var ecuM map[string][]string
	var typ symbol.ECUType

	switch name {
	case "T5":
		order = T5SymbolsTuningOrder
		ecuM = T5SymbolsTuning
		typ = symbol.ECU_T5
	case "T7":
		order = T7SymbolsTuningOrder
		ecuM = T7SymbolsTuning
		typ = symbol.ECU_T7
	case "T8":
		order = T8SymbolsTuningOrder
		ecuM = T8SymbolsTuning
		typ = symbol.ECU_T8
	}

	menus := append([]*fyne.Menu{}, mw.leading...)

	for _, category := range order {
		var items []*fyne.MenuItem
		for _, mapName := range ecuM[category] {
			if f, ok := mw.funcMap[mapName]; ok {
				itm := fyne.NewMenuItemWithIcon(mapName, theme.ComputerIcon(), func() {
					f(mapName)
				})
				items = append(items, itm)
				continue
			}

			if strings.Contains(mapName, "|") {
				parts := strings.Split(mapName, "|")
				names := parts[1:]
				if len(parts) == 2 {
					itm := fyne.NewMenuItemWithIcon(parts[0], theme.GridIcon(), func() {
						mw.openFunc(typ, parts[0], names[0])
					})
					items = append(items, itm)
					continue
				}
				//itm := fyne.NewMenuItem(parts[0], func() {
				//	mw.multiFunc(typ, names...)
				//})
				//items = append(items, itm)
				continue
			}

			itm := fyne.NewMenuItemWithIcon(mapName, theme.GridIcon(), func() {
				mw.openFunc(typ, "", mapName)
			})
			items = append(items, itm)
		}
		menus = append(menus, fyne.NewMenu(category, items...))
	}

	menus = append(menus, mw.trailing...)

	return fyne.NewMainMenu(menus...)
}
