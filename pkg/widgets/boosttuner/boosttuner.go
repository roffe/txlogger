// Package boosttuner provides a widget that helps auto-tune the Trionic 7 boost
// (APC) controller from logged data and writes the result back into the loaded
// binary.
//
// The T7 boost controller is a feedforward + PID loop (see Boost.c in the EU03
// source):
//
//	PWMCalc = RegConValue            // feedforward: BoostCal.RegMap[SetValue, rpm]
//	        + Adaption               // learned offset (BoostAdap.Adaption)
//	        + PFac + IFac + DFac     // PID on LoadDiff = SetValue - m_AirInlet
//	        + env compensation       // temp / altitude / E85 / noise reduction
//
// PWMCalc is the wastegate solenoid duty cycle in 0.1% units, clamped 2..98%.
//
// RegMap is the feedforward map and can be genuinely learned: at samples where
// the loop is settled and on target, the duty the feedforward *should* have
// supplied equals RegConValue plus everything the loop was adding to correct it
// (PFac + IFac + DFac + Adaption). Folding that sum back into RegMap leaves the
// loop with less to correct. The PID maps cannot be cleanly learned this way, so
// they are handled separately (heuristic suggestions + a replay simulator).
//
// Units note: the controller internals (RegConValue, P/I/D, Adaption, PWMCalc)
// are logged in raw 0.1% units (correction factor 1, e.g. 450 == 45.0%), while
// the BoostCal.RegMap symbol stores % (correction factor 0.1, e.g. 45.0). We
// learn in raw units and divide by dutyRawPerPct to land in the % the map uses.
package boosttuner

import (
	"fmt"
	"io"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

// dutyRawPerPct converts the controller's raw 0.1% duty units into the % units
// the BoostCal.RegMap symbol stores (450 raw -> 45.0%).
const dutyRawPerPct = 10.0

var _ fyne.Widget = (*BoostTuner)(nil)

// Config wires the tuner to the loaded binary. Symbols is read for the current
// maps and their axes; Save persists an edited map back into the binary (and to
// disk). Both are supplied by the main window, which owns mw.fw and the filename.
type Config struct {
	// Symbols is the currently loaded binary (mw.fw). Used read-only here.
	Symbols symbol.SymbolCollection
	// Save writes data (in engineering units) into the named symbol and persists
	// the binary to disk, taking a one-time backup on first write. nil disables
	// the "Apply to binary" buttons.
	Save func(symbolName string, data []float64) error

	MeshRenderer meshgrid.RenderBackend
	Colorblind   colors.ColorBlindMode
}

// channel is a logical signal the tuner needs, with the candidate series names to
// look for in a log (first present wins) and a human description for the checklist.
type channel struct {
	key        string
	candidates []string
	desc       string
}

// requiredChannels lists the log signals the RegMap learner and simulator rely
// on. Boost.SetValue is the exact load value the ECU feeds into the RegMap X
// lookup; m_Request is the same quantity under its airmass-master name and is a
// fallback. m_AirInletBoost is the airmass the loop regulates against.
var requiredChannels = []channel{
	{"rpm", []string{"ActualIn.n_Engine"}, "Engine speed (RegMap Y axis)"},
	{"setValue", []string{"Boost.SetValue", "m_Request", "AirMassMast.m_Request"}, "Load set value (RegMap X axis)"},
	{"regCon", []string{"BoostProt.RegConValue"}, "Feedforward duty from RegMap"},
	{"pFac", []string{"BoostProt.PFac"}, "P part"},
	{"iFac", []string{"BoostProt.IFac"}, "I part"},
	{"dFac", []string{"BoostProt.DFac"}, "D part"},
	{"adaption", []string{"BoostAdap.Adaption"}, "Adaption offset"},
	{"loadDiff", []string{"BoostProt.LoadDiff"}, "Load error (SetValue - airmass)"},
	{"pwmCalc", []string{"BoostProt.PWMCalc"}, "Total calculated duty"},
	{"pwm2pct", []string{"BoostProt.ST_PWM2Perc"}, "Open-loop/2% flag"},
	{"airInlet", []string{"MAF.m_AirInletBoost", "MAF.m_AirInlet"}, "Actual airmass"},
}

type BoostTuner struct {
	widget.BaseWidget
	cfg Config

	// values holds every series merged across all loaded log files, row-aligned
	// with NaN padding (same scheme as the matrix builder).
	values      map[string][]float64
	order       []string
	loadedFiles []string
	nrecords    int

	// resolved maps each logical channel key to the actual series name found in
	// the loaded logs (empty when missing).
	resolved map[string]string

	// RegMap state (see regmap.go).
	rmAxisX, rmAxisY              []float64 // breakpoints read from the binary
	rmCols, rmRows                int
	rmCurrent, rmLearned, rmDelta []float64 // engineering units (%)
	rmCounts                      []int
	rmBuilt                       bool

	// RegMap tuning parameters.
	onTarget   float64 // accept samples with |LoadDiff| <= this (mg/c)
	rpmStab    float64 // reject when |rpm step| exceeds this (rpm)
	loadStab   float64 // reject when |SetValue step| exceeds this (mg/c)
	minSamples int     // cells with fewer hits keep their current value
	blend      float64 // fraction (0..1) of the learned change to apply

	// PID editors keyed by "P"/"I"/"D" (see pid.go).
	pidEditors map[string]*pidEditor

	// widgets
	logsLabel   *widget.Label
	channelList *fyne.Container
	rmStatus    *widget.Label
	rmView      *widget.Select
	rmDisplay   *fyne.Container

	tabs    *container.AppTabs
	content fyne.CanvasObject
}

// New builds an empty tuner bound to the loaded binary in cfg.
func New(cfg Config) *BoostTuner {
	bt := &BoostTuner{
		cfg:        cfg,
		values:     make(map[string][]float64),
		resolved:   make(map[string]string),
		onTarget:   30,
		rpmStab:    150,
		loadStab:   50,
		minSamples: 5,
		blend:      1.0,
	}
	bt.ExtendBaseWidget(bt)
	bt.buildUI()
	return bt
}

func (bt *BoostTuner) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(bt.content)
}

func (bt *BoostTuner) buildUI() {
	bt.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Logs", theme.FolderOpenIcon(), bt.buildLogsTab()),
		container.NewTabItemWithIcon("RegMap", theme.GridIcon(), bt.buildRegMapTab()),
		container.NewTabItemWithIcon("PID maps", theme.GridIcon(), bt.buildPIDTab()),
		container.NewTabItemWithIcon("Simulator", theme.MediaPlayIcon(), bt.buildSimTab()),
	)
	bt.content = bt.tabs
}

// --- Logs tab ---

func (bt *BoostTuner) buildLogsTab() fyne.CanvasObject {
	bt.logsLabel = widget.NewLabel("No log files loaded")
	bt.logsLabel.Wrapping = fyne.TextWrapWord

	addBtn := widget.NewButtonWithIcon("Add log files", theme.FolderOpenIcon(), bt.openLogDialog)
	clearBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), bt.clearLogs)

	bt.channelList = container.NewVBox()
	bt.refreshChannelList()

	intro := widget.NewLabel(
		"Load logs from boost pulls (ideally full-throttle runs across the rev range), " +
			"then use the RegMap tab to learn the feedforward map. The channels below " +
			"must be present in the logs.")
	intro.Wrapping = fyne.TextWrapWord

	return container.NewBorder(
		container.NewVBox(
			intro,
			container.NewGridWithColumns(2, addBtn, clearBtn),
			bt.logsLabel,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Required channels", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		nil, nil, nil,
		container.NewVScroll(bt.channelList),
	)
}

// refreshChannelList redraws the per-channel present/missing checklist against the
// currently loaded logs.
func (bt *BoostTuner) refreshChannelList() {
	bt.channelList.Objects = bt.channelList.Objects[:0]
	for _, ch := range requiredChannels {
		name := bt.resolved[ch.key]
		var icon fyne.Resource
		var detail string
		if name != "" {
			icon = theme.ConfirmIcon()
			detail = name
		} else {
			icon = theme.CancelIcon()
			detail = "missing: " + strings.Join(ch.candidates, " / ")
		}
		row := container.NewBorder(nil, nil,
			widget.NewIcon(icon),
			widget.NewLabel(detail),
			widget.NewLabel(ch.desc),
		)
		bt.channelList.Add(row)
	}
	bt.channelList.Refresh()
}

// resolveChannels picks, for each logical channel, the first candidate series
// present in the loaded logs.
func (bt *BoostTuner) resolveChannels() {
	bt.resolved = make(map[string]string)
	for _, ch := range requiredChannels {
		for _, cand := range ch.candidates {
			if _, ok := bt.values[cand]; ok {
				bt.resolved[ch.key] = cand
				break
			}
		}
	}
}

// series returns the merged log data for a resolved channel key.
func (bt *BoostTuner) series(key string) ([]float64, bool) {
	name := bt.resolved[key]
	if name == "" {
		return nil, false
	}
	v, ok := bt.values[name]
	return v, ok
}

// missingChannels lists the descriptions of any required channels not found.
func (bt *BoostTuner) missingChannels() []string {
	var out []string
	for _, ch := range requiredChannels {
		if bt.resolved[ch.key] == "" {
			out = append(out, ch.desc)
		}
	}
	return out
}

// --- log loading (mirrors the matrix builder's pipeline) ---

func (bt *BoostTuner) openLogDialog() {
	widgets.SelectFiles(func(readers []fyne.URIReadCloser) {
		c := fyne.CurrentApp().Driver().CanvasForObject(bt)
		if c == nil {
			if wins := fyne.CurrentApp().Driver().AllWindows(); len(wins) > 0 {
				c = wins[0].Canvas()
			}
		}
		var pm *progressmodal.ProgressModal
		if c != nil {
			pm = progressmodal.New(c, fmt.Sprintf("Parsing %d log file(s)...", len(readers)))
			pm.Show()
		}

		go func() {
			type parsed struct {
				name  string
				local map[string][]float64
				n     int
			}
			var ok []parsed
			var failed int
			for _, r := range readers {
				name := r.URI().Name()
				local, n, err := parseLog(name, r)
				r.Close()
				if err != nil {
					log.Println("boosttuner:", err)
					failed++
					continue
				}
				ok = append(ok, parsed{name, local, n})
			}

			fyne.Do(func() {
				if pm != nil {
					pm.Hide()
				}
				for _, p := range ok {
					bt.mergeLog(p.name, p.local, p.n)
				}
				bt.rebuildOrder()
				bt.resolveChannels()
				bt.refreshChannelList()
				bt.refreshLogList()
				if failed > 0 {
					bt.logStatus(fmt.Sprintf("Loaded %d file(s), %d failed", len(ok), failed))
				}
			})
		}()
	}, "logfile", "t5l", "t7l", "t8l", "csv", "bpl")
}

// parseLog reads a single log into a row-aligned series map, padding gaps with
// NaN. It touches no shared state, so it is safe off the UI goroutine.
func parseLog(name string, r io.Reader) (map[string][]float64, int, error) {
	lf, err := logfile.Open(name, r)
	if err != nil {
		return nil, 0, err
	}
	defer lf.Close()

	local := make(map[string][]float64)
	n := 0
	for {
		rec := lf.Next()
		if rec.EOF {
			break
		}
		for k, v := range rec.Values {
			if k == "Pgm_status" {
				continue
			}
			arr := local[k]
			for len(arr) < n { // back-fill records before this key first appeared
				arr = append(arr, math.NaN())
			}
			local[k] = append(arr, v)
		}
		n++
		for k, arr := range local { // forward-fill keys missing from this record
			for len(arr) < n {
				arr = append(arr, math.NaN())
			}
			local[k] = arr
		}
	}
	if n == 0 {
		return nil, 0, fmt.Errorf("%s contains no records", name)
	}
	return local, n, nil
}

// mergeLog appends a parsed log to the merged series set, keeping every series
// row-aligned. Must run on the UI goroutine.
func (bt *BoostTuner) mergeLog(name string, local map[string][]float64, n int) {
	base := bt.nrecords
	for k, arr := range local {
		cur, ok := bt.values[k]
		if !ok {
			cur = nanSlice(base)
		}
		bt.values[k] = append(cur, arr...)
	}
	for k, cur := range bt.values {
		if _, ok := local[k]; !ok {
			bt.values[k] = append(cur, nanSlice(n)...)
		}
	}
	bt.nrecords = base + n
	bt.loadedFiles = append(bt.loadedFiles, name)
}

func (bt *BoostTuner) clearLogs() {
	bt.values = make(map[string][]float64)
	bt.order = nil
	bt.loadedFiles = nil
	bt.nrecords = 0
	bt.resolveChannels()
	bt.refreshChannelList()
	bt.refreshLogList()
}

func (bt *BoostTuner) rebuildOrder() {
	bt.order = make([]string, 0, len(bt.values))
	for k := range bt.values {
		bt.order = append(bt.order, k)
	}
	sort.Slice(bt.order, func(i, j int) bool {
		return strings.ToLower(bt.order[i]) < strings.ToLower(bt.order[j])
	})
}

func (bt *BoostTuner) refreshLogList() {
	if len(bt.loadedFiles) == 0 {
		bt.logsLabel.SetText("No log files loaded")
		return
	}
	var b strings.Builder
	for i, f := range bt.loadedFiles {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(filepath.Base(f))
	}
	bt.logsLabel.SetText(fmt.Sprintf("%d file(s), %d records:\n%s",
		len(bt.loadedFiles), bt.nrecords, b.String()))
}

// logStatus appends a one-off message to the log list label.
func (bt *BoostTuner) logStatus(msg string) {
	bt.refreshLogList()
	bt.logsLabel.SetText(bt.logsLabel.Text + "\n" + msg)
}

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// --- shared helpers ---

// nearestIndex returns the index of the axis breakpoint closest to v.
func nearestIndex(axis []float64, v float64) int {
	best := 0
	bestDist := math.Abs(axis[0] - v)
	for i := 1; i < len(axis); i++ {
		if d := math.Abs(axis[i] - v); d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
