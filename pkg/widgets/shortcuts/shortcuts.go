// Package shortcuts stores the user's keyboard shortcut bindings in app
// preferences and provides the widget to edit them.
package shortcuts

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/layout"
)

const prefsKey = "keyboardShortcuts"

// Actions a binding can trigger. Target holds a map/symbol name for ActionMap,
// a layout name for ActionLayout and is unused for ActionSettings.
const (
	ActionSettings   = "Open settings"
	ActionSymbolList = "Open symbol list"
	ActionMap        = "Open map"
	ActionLayout     = "Load layout"
)

var Actions = []string{ActionSettings, ActionSymbolList, ActionMap, ActionLayout}

// ECUAll makes a binding apply whichever ECU is selected.
const ECUAll = "All"

var ECUs = append([]string{ECUAll}, common.EcuList...)

// Binding is one user-configured shortcut.
type Binding struct {
	Modifier fyne.KeyModifier
	Key      fyne.KeyName
	Action   string
	Target   string
	ECU      string
}

// AppliesTo reports whether the binding is active for the selected ECU.
func (b Binding) AppliesTo(ecu string) bool {
	return b.ECU == "" || b.ECU == ECUAll || b.ECU == ecu
}

// Fireable reports whether the combo can reach us from the keyboard. Fyne only
// builds a shortcut when Ctrl, Alt or Super is held, so a bare F1 - or a
// Shift-only combo - runs from the menu entry but never from the key press.
func (b Binding) Fireable() bool {
	return b.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierAlt|fyne.KeyModifierSuper) != 0
}

// Label is the menu entry text for this binding.
func (b Binding) Label() string {
	label := b.Action
	if b.Target != "" {
		label += ": " + b.Target
	}
	return label
}

func (b Binding) encode() string {
	return strconv.Itoa(int(b.Modifier)) + "|" + string(b.Key) + "|" + b.Action + "|" + b.Target + "|" + b.ECU
}

func decode(s string) (Binding, bool) {
	// Entries saved before per-ECU bindings have no ECU field and mean "All".
	parts := strings.SplitN(s, "|", 5)
	if len(parts) < 4 {
		return Binding{}, false
	}
	mod, err := strconv.Atoi(parts[0])
	if err != nil || parts[1] == "" {
		return Binding{}, false
	}
	ecu := ECUAll
	if len(parts) == 5 && parts[4] != "" {
		ecu = parts[4]
	}
	return Binding{
		Modifier: fyne.KeyModifier(mod),
		Key:      fyne.KeyName(parts[1]),
		Action:   parts[2],
		Target:   parts[3],
		ECU:      ecu,
	}, true
}

// Load reads the saved bindings. ponytail: last binding wins if two share a
// combo, since they end up in the same canvas shortcut slot.
func Load() []Binding {
	var out []Binding
	for _, s := range fyne.CurrentApp().Preferences().StringList(prefsKey) {
		if b, ok := decode(s); ok {
			out = append(out, b)
		}
	}
	return out
}

func Save(bindings []Binding) {
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.encode())
	}
	fyne.CurrentApp().Preferences().SetStringList(prefsKey, out)
}

// keyNames are the keys offered in the picker. Picking from a list beats
// capturing a keypress here: the combo being bound would otherwise fire the
// shortcut it is already assigned to while we listen for it.
var keyNames = func() []string {
	var keys []string
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, string(c))
	}
	for c := '0'; c <= '9'; c++ {
		keys = append(keys, string(c))
	}
	for i := 1; i <= 12; i++ {
		keys = append(keys, fmt.Sprintf("F%d", i))
	}
	return append(keys, string(fyne.KeySpace), string(fyne.KeyReturn), string(fyne.KeyTab),
		string(fyne.KeyInsert), string(fyne.KeyDelete), string(fyne.KeyHome), string(fyne.KeyEnd),
		string(fyne.KeyPageUp), string(fyne.KeyPageDown),
		string(fyne.KeyUp), string(fyne.KeyDown), string(fyne.KeyLeft), string(fyne.KeyRight))
}()

type Widget struct {
	widget.BaseWidget

	bindings  []Binding
	layouts   func() []string
	onChange  func([]Binding)
	list      *fyne.Container
	container *fyne.Container
}

// New creates the editor. layouts lists the saved window layouts for the
// "Load layout" target, onChange is called with the new set whenever it
// changes so the caller can re-register the shortcuts.
func New(layouts func() []string, onChange func([]Binding)) *Widget {
	w := &Widget{
		bindings: Load(),
		layouts:  layouts,
		onChange: onChange,
		list:     container.NewVBox(),
	}
	w.ExtendBaseWidget(w)

	add := widget.NewButtonWithIcon("Add shortcut", theme.ContentAddIcon(), func() {
		w.bindings = append(w.bindings, Binding{Modifier: fyne.KeyModifierControl, Key: fyne.KeyF1, Action: ActionSettings, ECU: ECUAll})
		w.persist()
		w.rebuild()
	})

	w.container = container.NewBorder(nil, add, nil, nil, container.NewVScroll(w.list))
	w.rebuild()
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.container)
}

func (w *Widget) persist() {
	Save(w.bindings)
	if w.onChange != nil {
		w.onChange(w.bindings)
	}
}

func (w *Widget) rebuild() {
	w.list.RemoveAll()
	if len(w.bindings) == 0 {
		w.list.Add(widget.NewLabel("No shortcuts configured yet."))
	}
	for i := range w.bindings {
		w.list.Add(w.row(i))
	}
	w.list.Refresh()
}

func (w *Widget) row(i int) fyne.CanvasObject {
	b := w.bindings[i]

	hint := widget.NewLabel("")
	hint.Importance = widget.WarningImportance
	updateHint := func() {
		if w.bindings[i].Fireable() {
			hint.SetText("")
			return
		}
		hint.SetText("menu only, add Ctrl or Alt")
	}
	updateHint()

	modCheck := func(label string, mod fyne.KeyModifier) *widget.Check {
		c := widget.NewCheck(label, func(on bool) {
			if on {
				w.bindings[i].Modifier |= mod
			} else {
				w.bindings[i].Modifier &^= mod
			}
			updateHint()
			w.persist()
		})
		c.Checked = b.Modifier&mod != 0
		return c
	}

	key := widget.NewSelect(keyNames, func(k string) {
		w.bindings[i].Key = fyne.KeyName(k)
		w.persist()
	})
	key.Selected = string(b.Key)

	target := widget.NewSelectEntry(nil)
	target.OnChanged = func(s string) {
		w.bindings[i].Target = s
		w.persist()
	}

	// Target means something different per action, so retune the entry
	// instead of rebuilding the whole row when the action changes.
	setAction := func(a string) {
		switch a {
		case ActionLayout:
			target.SetOptions(w.layouts())
			target.PlaceHolder = "layout name"
			target.Enable()
		case ActionMap:
			target.SetOptions(nil)
			target.PlaceHolder = "symbol name, e.g. BstKnkCal.MaxAirmass"
			target.Enable()
		default:
			target.SetOptions(nil)
			target.PlaceHolder = ""
			target.Disable()
		}
		target.Refresh()
	}

	action := widget.NewSelect(Actions, func(a string) {
		w.bindings[i].Action = a
		setAction(a)
		w.persist()
	})
	action.Selected = b.Action
	setAction(b.Action)
	target.SetText(b.Target)

	ecu := widget.NewSelect(ECUs, func(e string) {
		w.bindings[i].ECU = e
		w.persist()
	})
	ecu.Selected = b.ECU
	if ecu.Selected == "" {
		ecu.Selected = ECUAll
	}

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		w.bindings = append(w.bindings[:i:i], w.bindings[i+1:]...)
		w.persist()
		w.rebuild()
	})

	return container.NewBorder(nil, nil,
		container.NewHBox(
			modCheck("Ctrl", fyne.KeyModifierControl),
			modCheck("Shift", fyne.KeyModifierShift),
			modCheck("Alt", fyne.KeyModifierAlt),
			layout.NewFixedWidth(90, key),
			layout.NewFixedWidth(160, action),
			layout.NewFixedWidth(80, ecu),
		),
		container.NewHBox(hint, del),
		target,
	)
}
