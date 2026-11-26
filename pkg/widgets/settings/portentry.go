package settings

import (
	"cmp"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"go.bug.st/serial/enumerator"
)

// portInfo is a serial port and what is plugged into it.
type portInfo struct {
	// path is what gets opened: on Linux the /dev/serial/by-id link when there
	// is one, it stays with the device across plugs and boots.
	path   string
	name   string // ttyUSB0, cu.usbserial-A50285BI, COM5
	title  string
	detail string
}

// listPorts returns the serial ports that are there now, in name order.
func listPorts() []portInfo {
	found, _ := enumerator.GetDetailedPortsList()
	byID := serialByID()
	out := make([]portInfo, 0, len(found))
	for _, p := range found {
		name := filepath.Base(p.Name)
		if runtime.GOOS == "darwin" && strings.HasPrefix(name, "tty.") {
			continue // the same device as its cu. twin
		}
		path := p.Name
		if link, ok := byID[p.Name]; ok {
			path = link
		}
		out = append(out, newPortInfo(path, name, p))
	}
	slices.SortFunc(out, func(a, b portInfo) int { return comparePortNames(a.name, b.name) })
	return out
}

func newPortInfo(path, name string, p *enumerator.PortDetails) portInfo {
	// Windows gives the friendly name, "USB Serial Port (COM5)"
	product := strings.TrimSuffix(p.Product, " ("+name+")")
	title := cmp.Or(product, p.Manufacturer, "Serial port")
	detail := []string{name}
	if p.Manufacturer != "" && p.Manufacturer != title {
		detail = append(detail, p.Manufacturer)
	}
	if p.SerialNumber != "" {
		detail = append(detail, "S/N "+p.SerialNumber)
	}
	if p.VID != "" {
		detail = append(detail, strings.ToLower(p.VID+":"+p.PID))
	}
	return portInfo{path: path, name: name, title: title, detail: strings.Join(detail, " · ")}
}

// serialByID maps each device node to its /dev/serial/by-id link.
func serialByID() map[string]string {
	links, _ := filepath.Glob("/dev/serial/by-id/*")
	m := make(map[string]string, len(links))
	for _, link := range links {
		if node, err := filepath.EvalSymlinks(link); err == nil {
			m[node] = link
		}
	}
	return m
}

// comparePortNames sorts ttyUSB2 before ttyUSB10 and COM3 before COM12.
func comparePortNames(a, b string) int {
	stemA, numA := splitPortNumber(a)
	stemB, numB := splitPortNumber(b)
	return cmp.Or(strings.Compare(stemA, stemB), cmp.Compare(numA, numB))
}

func splitPortNumber(name string) (string, int) {
	stem := strings.TrimRight(name, "0123456789")
	n, _ := strconv.Atoi(name[len(stem):])
	return stem, n
}

// findPort returns the port typed names: by its path, on Unix also by any
// other link to its device node, so /dev/ttyUSB0 finds the by-id entry.
func findPort(ports []portInfo, typed string) int {
	typed = strings.TrimSpace(typed)
	node, _ := filepath.EvalSymlinks(typed)
	return slices.IndexFunc(ports, func(p portInfo) bool {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(p.path, typed)
		}
		return p.path == typed || node == "/dev/"+p.name
	})
}

// portEntry is a text field for a serial port with a drop down of the ports
// that are there, each with what is plugged into it.
type portEntry struct {
	widget.Entry
	ports []portInfo
	popUp *widget.PopUp
}

func newPortEntry() *portEntry {
	e := &portEntry{ports: listPorts()}
	e.ExtendBaseWidget(e)
	dropDown := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), e.showList)
	dropDown.Importance = widget.LowImportance
	e.ActionItem = dropDown
	return e
}

func (e *portEntry) Enable() {
	e.ActionItem.(fyne.Disableable).Enable()
	e.Entry.Enable()
}

func (e *portEntry) Disable() {
	e.ActionItem.(fyne.Disableable).Disable()
	e.Entry.Disable()
}

// relist scans the ports again.
func (e *portEntry) relist() {
	e.ports = listPorts()
}

// status says what is plugged into the typed port, or that nothing is.
func (e *portEntry) status() (string, widget.Importance) {
	typed := strings.TrimSpace(e.Text)
	if typed == "" || strings.Contains(typed, "://") {
		return "", widget.LowImportance
	}
	if i := findPort(e.ports, typed); i >= 0 {
		return e.ports[i].title + " · " + e.ports[i].detail, widget.SuccessImportance
	}
	// on Windows the list is every port there is
	if _, err := os.Stat(typed); runtime.GOOS == "windows" || err != nil {
		return "not plugged in", widget.DangerImportance
	}
	return "", widget.LowImportance
}

func (e *portEntry) showList() {
	c := fyne.CurrentApp().Driver().CanvasForObject(e)
	if c == nil {
		return
	}
	e.relist()
	var content fyne.CanvasObject
	height := float32(0)
	if len(e.ports) == 0 {
		content = widget.NewLabel("No serial ports found. Plug the cable in and open the list again.")
		height = content.MinSize().Height
	} else {
		content = e.portList()
		n := float32(len(e.ports))
		height = min(n*newPortRow().MinSize().Height+(n-1)*theme.Padding(), 300)
	}
	e.popUp = widget.NewPopUp(content, c)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(e)
	e.popUp.ShowAtPosition(pos.AddXY(0, e.Size().Height))
	padding := e.popUp.MinSize().Subtract(content.MinSize())
	e.popUp.Resize(fyne.NewSize(e.Size().Width, height+padding.Height))
}

func (e *portEntry) portList() *widget.List {
	current := findPort(e.ports, e.Text)
	list := widget.NewList(
		func() int { return len(e.ports) },
		newPortRow,
		func(id widget.ListItemID, o fyne.CanvasObject) {
			o.(*portRow).set(e.ports[id], id == current)
		},
	)
	list.HideSeparators = true
	list.OnSelected = func(id widget.ListItemID) {
		e.popUp.Hide()
		e.SetText(e.ports[id].path)
	}
	return list
}

// portRow is one port in the drop down: what it is, then its name, maker,
// serial number and USB id in small text.
type portRow struct {
	widget.BaseWidget
	bg            *canvas.Rectangle
	title, detail *canvas.Text
}

func newPortRow() fyne.CanvasObject {
	r := &portRow{
		bg:     canvas.NewRectangle(color.Transparent),
		title:  canvas.NewText("", theme.Color(theme.ColorNameForeground)),
		detail: canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder)),
	}
	r.detail.TextSize = theme.CaptionTextSize()
	r.ExtendBaseWidget(r)
	return r
}

func (r *portRow) CreateRenderer() fyne.WidgetRenderer {
	text := container.New(layout.NewCustomPaddedVBoxLayout(0), r.title, r.detail)
	return widget.NewSimpleRenderer(container.NewStack(r.bg, container.NewPadded(text)))
}

func (r *portRow) set(p portInfo, current bool) {
	r.title.Text = p.title
	r.detail.Text = p.detail
	r.bg.FillColor = color.Transparent
	if current {
		r.bg.FillColor = theme.Color(theme.ColorNameSelection)
	}
	r.title.Refresh()
	r.detail.Refresh()
	r.bg.Refresh()
}
