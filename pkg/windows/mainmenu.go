package windows

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
)

type MenuItem struct {
	Name     string
	Children []MenuItem
	Func     func()
	Data     string
	// Region is an optional LambdaCal MaxLoad table whose per-rpm airmass limit
	// outlines the closed-loop region on the opened map. Only used for single-map
	// items (Data without "|").
	Region string
}

func (mw *MainWindow) GetMenu(name string) *fyne.MainMenu {
	var tree []MenuItem
	var typ symbol.ECUType

	switch name {
	case "T5":
		tree = mw.t5Menu()
		typ = symbol.ECU_T5
	case "T7":
		tree = mw.t7Menu()
		typ = symbol.ECU_T7
	case "T8":
		tree = mw.t8Menu()
		typ = symbol.ECU_T8
	}

	menus := append([]*fyne.Menu{}, mw.leadingMenus...)
	for _, category := range tree {
		menus = append(menus, fyne.NewMenu(category.Name, mw.buildItems(typ, category.Children)...))
	}
	menus = append(menus, mw.trailingMenus...)

	return fyne.NewMainMenu(menus...)
}

func (mw *MainWindow) buildItems(typ symbol.ECUType, items []MenuItem) []*fyne.MenuItem {
	var out []*fyne.MenuItem
	for _, item := range items {
		out = append(out, mw.buildItem(typ, item))
	}
	return out
}

func (mw *MainWindow) buildItem(typ symbol.ECUType, item MenuItem) *fyne.MenuItem {
	switch {
	case len(item.Children) > 0:
		itm := fyne.NewMenuItemWithIcon(item.Name, theme.FolderIcon(), nil)
		itm.ChildMenu = fyne.NewMenu(item.Name, mw.buildItems(typ, item.Children)...)
		return itm
	case item.Func != nil:
		return fyne.NewMenuItemWithIcon(item.Name, theme.ComputerIcon(), item.Func)
	case strings.Contains(item.Data, "|"):
		// title + multiple symbols joined by "|": open tightly arranged together
		return fyne.NewMenuItemWithIcon(item.Name, theme.GridIcon(), func() {
			mw.openMultiMap(typ, item.Name, item.Data)
		})
	case item.Data != "":
		// title + symbol: open as a map
		return fyne.NewMenuItemWithIcon(item.Name, theme.GridIcon(), func() {
			mw.openMap(typ, item.Name, item.Data, item.Region)
		})
	default:
		// Name is itself a symbol
		return fyne.NewMenuItemWithIcon(item.Name, theme.GridIcon(), func() {
			mw.openMap(typ, "", item.Name, "")
		})
	}
}
