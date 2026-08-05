package colors

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
)

// User-picked plot colors, persisted in the app preferences as a string list
// of "name=#RRGGBBAA" entries. They override colorMap/hash in GetColor.
const customColorsKey = "customPlotColors"

var (
	customMu sync.Mutex
	custom   map[string]color.RGBA
)

// loadCustom parses the preference entries once. Caller must hold customMu.
func loadCustom() map[string]color.RGBA {
	if custom != nil {
		return custom
	}
	custom = make(map[string]color.RGBA)
	app := fyne.CurrentApp()
	if app == nil { // tests
		return custom
	}
	for _, entry := range app.Preferences().StringList(customColorsKey) {
		name, hex, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		var r, g, b, a uint8
		if _, err := fmt.Sscanf(hex, "#%02X%02X%02X%02X", &r, &g, &b, &a); err != nil {
			continue
		}
		custom[name] = color.RGBA{r, g, b, a}
	}
	return custom
}

// saveCustom writes the map back to preferences. Caller must hold customMu.
func saveCustom() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	entries := make([]string, 0, len(custom))
	for name, c := range custom {
		entries = append(entries, fmt.Sprintf("%s=#%02X%02X%02X%02X", name, c.R, c.G, c.B, c.A))
	}
	sort.Strings(entries)
	app.Preferences().SetStringList(customColorsKey, entries)
}

// onChange listeners are notified after every SetCustom/DeleteCustom, outside
// the lock so they may call back into this package.
var onChange []func()

// OnChange registers a listener called whenever a custom color is set or
// removed. Listeners cannot be unregistered.
func OnChange(f func()) {
	customMu.Lock()
	defer customMu.Unlock()
	onChange = append(onChange, f)
}

func notify() {
	customMu.Lock()
	listeners := onChange
	customMu.Unlock()
	for _, f := range listeners {
		f()
	}
}

// SetCustom remembers a user-picked color for a series name.
func SetCustom(name string, c color.RGBA) {
	customMu.Lock()
	loadCustom()[name] = c
	saveCustom()
	customMu.Unlock()
	notify()
}

// DeleteCustom forgets the user-picked color for a series name.
func DeleteCustom(name string) {
	customMu.Lock()
	delete(loadCustom(), name)
	saveCustom()
	customMu.Unlock()
	notify()
}

// CustomNames returns the series names with a custom color, sorted.
func CustomNames() []string {
	customMu.Lock()
	defer customMu.Unlock()
	m := loadCustom()
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func getCustom(name string) (color.RGBA, bool) {
	customMu.Lock()
	defer customMu.Unlock()
	c, ok := loadCustom()[name]
	return c, ok
}
