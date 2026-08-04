package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/common"
)

const (
	defaultGridCols = 32
	defaultGridRows = 18
	// Upper bound for a shared preset's grid: edit mode draws one line per
	// column and row, so an absurd grid would be a self-inflicted DoS.
	maxGridCols = 128
	maxGridRows = 128

	// Style values for the wblambda item.
	StyleBar   = "bar"
	StyleGauge = "gauge"

	prefDashboardLayout = "dashboardLayout"
)

// Item places one dashboard element on the snap grid.
type Item struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
	// Style selects the visualization for items that have more than one,
	// currently only "wblambda": StyleBar (default) or StyleGauge.
	Style string `json:"style,omitempty"`
}

// Layout is a full dashboard arrangement. Items render in slice order, so
// later items draw on top of earlier ones.
type Layout struct {
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	Items []Item `json:"items"`
}

// itemDef describes an element that can be placed on the dashboard.
type itemDef struct {
	id         string
	label      string
	addW, addH int // size used when added from the edit toolbar
}

// itemDefs is the roster of placeable elements, in Add-menu order.
var itemDefs = []itemDef{
	{id: "rpm", label: "RPM", addW: 5, addH: 5},
	{id: "speed", label: "Speed", addW: 5, addH: 5},
	{id: "iat", label: "Intake air temp", addW: 5, addH: 5},
	{id: "engineTemp", label: "Engine temp", addW: 5, addH: 5},
	{id: "airmass", label: "Airmass", addW: 5, addH: 5},
	{id: "pressure", label: "Pressure", addW: 5, addH: 5},
	{id: "throttle", label: "Throttle", addW: 2, addH: 8},
	{id: "pwm", label: "Boost PWM", addW: 2, addH: 8},
	{id: "nblambda", label: "NB lambda", addW: 8, addH: 2},
	{id: "wblambda", label: "WB lambda", addW: 8, addH: 2},
	{id: "ign", label: "Ignition text", addW: 4, addH: 1},
	{id: "ioff", label: "Ioff text", addW: 3, addH: 1},
	{id: "idc", label: "Injector duty text", addW: 3, addH: 1},
	{id: "amul", label: "Amul text", addW: 3, addH: 1},
	{id: "activeAirDem", label: "Air demand text", addW: 8, addH: 1},
	{id: "time", label: "Time (log player)", addW: 5, addH: 1},
	{id: "cruise", label: "Cruise light", addW: 4, addH: 2},
	{id: "checkEngine", label: "Check engine light", addW: 3, addH: 2},
	{id: "limpMode", label: "Limp mode light", addW: 3, addH: 2},
	{id: "knock", label: "Knock light", addW: 2, addH: 2},
	{id: "taz", label: "Taz", addW: 4, addH: 4},
}

func itemDefByID(id string) *itemDef {
	for i := range itemDefs {
		if itemDefs[i].id == id {
			return &itemDefs[i]
		}
	}
	return nil
}

// defaultLayout approximates the classic fixed dashboard arrangement.
func defaultLayout() *Layout {
	return &Layout{
		Cols: defaultGridCols,
		Rows: defaultGridRows,
		Items: []Item{
			{ID: "speed", X: 7, Y: 2, W: 18, H: 15},
			{ID: "rpm", X: 0, Y: 0, W: 5, H: 5},
			{ID: "airmass", X: 0, Y: 6, W: 5, H: 5},
			{ID: "pressure", X: 0, Y: 12, W: 5, H: 5},
			{ID: "iat", X: 27, Y: 3, W: 5, H: 5},
			{ID: "engineTemp", X: 27, Y: 10, W: 5, H: 5},
			{ID: "pwm", X: 5, Y: 1, W: 2, H: 16},
			{ID: "throttle", X: 25, Y: 1, W: 2, H: 16},
			{ID: "nblambda", X: 8, Y: 0, W: 16, H: 1},
			{ID: "wblambda", X: 8, Y: 17, W: 16, H: 1},
			{ID: "ign", X: 8, Y: 1, W: 4, H: 1},
			{ID: "ioff", X: 8, Y: 2, W: 3, H: 1},
			{ID: "idc", X: 21, Y: 1, W: 3, H: 1},
			{ID: "amul", X: 8, Y: 16, W: 3, H: 1},
			{ID: "activeAirDem", X: 12, Y: 3, W: 8, H: 1},
			{ID: "time", X: 13, Y: 4, W: 6, H: 1},
			{ID: "cruise", X: 8, Y: 14, W: 4, H: 2},
			{ID: "taz", X: 14, Y: 7, W: 4, H: 4},
			{ID: "limpMode", X: 14, Y: 5, W: 3, H: 2},
			{ID: "checkEngine", X: 21, Y: 15, W: 3, H: 2},
			{ID: "knock", X: 7, Y: 8, W: 2, H: 2},
		},
	}
}

func (l *Layout) item(id string) *Item {
	for i := range l.Items {
		if l.Items[i].ID == id {
			return &l.Items[i]
		}
	}
	return nil
}

func (l *Layout) remove(id string) {
	for i := range l.Items {
		if l.Items[i].ID == id {
			l.Items = append(l.Items[:i], l.Items[i+1:]...)
			return
		}
	}
}

// sanitize clamps every item into the grid and drops unknown or duplicate
// ids, so hand-edited or shared preset files can't break the dashboard.
func (l *Layout) sanitize() {
	if l.Cols <= 0 || l.Cols > maxGridCols {
		l.Cols = defaultGridCols
	}
	if l.Rows <= 0 || l.Rows > maxGridRows {
		l.Rows = defaultGridRows
	}
	seen := make(map[string]bool, len(l.Items))
	items := l.Items[:0]
	for _, it := range l.Items {
		if itemDefByID(it.ID) == nil || seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		it.W = common.Clamp(it.W, 1, l.Cols)
		it.H = common.Clamp(it.H, 1, l.Rows)
		it.X = common.Clamp(it.X, 0, l.Cols-it.W)
		it.Y = common.Clamp(it.Y, 0, l.Rows-it.H)
		items = append(items, it)
	}
	l.Items = items
}

// loadActiveLayout returns the layout stored in the application preferences,
// or the default layout if none is stored or it fails to parse.
func loadActiveLayout() *Layout {
	data := fyne.CurrentApp().Preferences().String(prefDashboardLayout)
	if data != "" {
		var l Layout
		if err := json.Unmarshal([]byte(data), &l); err == nil {
			l.sanitize()
			if len(l.Items) > 0 {
				return &l
			}
		}
	}
	return defaultLayout()
}

// save persists the layout as the active one in the application preferences.
func (l *Layout) save() {
	data, err := json.Marshal(l)
	if err != nil {
		fyne.LogError("marshal dashboard layout", err)
		return
	}
	fyne.CurrentApp().Preferences().SetString(prefDashboardLayout, string(data))
}

// --- preset files: ~/txlogger/dashboards/<name>.json, shareable as-is -----

func listPresets() []string {
	path, err := common.GetDashboardPath()
	if err != nil {
		return nil
	}
	files, err := common.ListFilesInPathByExtension(path, ".json")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(f, filepath.Ext(f)))
	}
	return names
}

func (l *Layout) savePreset(name string) error {
	path, err := common.GetDashboardPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	filename := common.SanitizeFilename(name) + ".json"
	return os.WriteFile(filepath.Join(path, filename), data, 0o644)
}

func loadPreset(name string) (*Layout, error) {
	path, err := common.GetDashboardPath()
	if err != nil {
		return nil, err
	}
	// Resolve against what listPresets showed rather than re-sanitizing, so
	// a preset file shared by someone else opens under its own name.
	base := filepath.Base(name)
	filename := base + ".json"
	if files, err := common.ListFilesInPathByExtension(path, ".json"); err == nil {
		for _, f := range files {
			if strings.TrimSuffix(f, filepath.Ext(f)) == base {
				filename = f
				break
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(path, filename))
	if err != nil {
		return nil, err
	}
	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	l.sanitize()
	return &l, nil
}
