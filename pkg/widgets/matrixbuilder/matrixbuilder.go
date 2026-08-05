// Package matrixbuilder provides a widget that learns a 2D map (matrix) from
// one or more log files. The widget loads the logs itself and merges their
// series, then builds the matrix from three selected series: one drives the X
// axis, one the Y axis and one supplies the Z value written into the cell the
// X/Y pair lands on. Every sample that maps to a cell is accumulated and the
// cell's final value is the average of all its hits. The resulting matrix is
// shown with a mapviewer (colored grid + 3D meshgrid), and both the axis
// breakpoints and the series can be edited or typed by hand.
package matrixbuilder

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	xlayout "fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

const (
	minAxis     = 1
	maxAxis     = 40
	defaultCols = 8
	defaultRows = 8

	// Tolerance is expressed as a percentage of a cell's half-spacing (the
	// distance from a breakpoint to the midpoint between it and its neighbor).
	// At 100% the whole nearest-neighbor region counts as a hit (the original
	// behaviour); lower values reject samples that fall near the cell edges,
	// keeping only those close to the breakpoint.
	minTolerance     = 1
	defaultTolerance = 100
)

// Display views for the built matrix. viewMatrix shows the learned Z values;
// viewCoverage shows how many samples landed in each cell.
const (
	viewMatrix   = "Matrix"
	viewCoverage = "Coverage (hits)"
)

var _ fyne.Widget = (*MatrixBuilder)(nil)

type MatrixBuilder struct {
	widget.BaseWidget
	renderMode meshgrid.RenderBackend

	// values holds every series merged across all loaded log files. All series
	// share the same length (nrecords); gaps are padded with NaN so samples
	// from different files stay row-aligned.
	values      map[string][]float64
	order       []string
	loadedFiles []string
	nrecords    int

	xSeries, ySeries, zSeries string

	cols, rows int
	xAxis      []float64
	yAxis      []float64
	zData      []float64

	// counts holds the number of samples that landed in each cell during the
	// last analyze, parallel to zData. It feeds the Coverage view.
	counts []int

	// xTolerance/yTolerance gate how close (as a percentage of the cell's
	// half-spacing) a sample must be to its nearest breakpoint to count as a
	// Z-hit on that axis. A sample is mapped only if it passes on both axes.
	xTolerance, yTolerance float64

	// built becomes true after the first successful analysis; until then the
	// display area shows a placeholder instead of an all-zero grid.
	built bool

	// widgets
	colsLabel, rowsLabel   *widget.Label
	status                 *widget.Label
	logsLabel              *widget.Label
	xBox, yBox             *fyne.Container
	xEntries, yEntries     []*widget.Entry
	xSel, ySel, zSel       *widget.SelectEntry
	xTolSlider, yTolSlider *widget.Slider
	xTolLabel, yTolLabel   *widget.Label
	presetSelect           *widget.Select
	nameEntry              *widget.Entry
	viewSelect             *widget.Select
	display                *fyne.Container

	// Filter tree: a nested group/condition builder. rootGroup is the live editor
	// state (read into a FilterNode tree on demand); filterHolder is the stable
	// container the root group is swapped into when a preset is loaded.
	rootGroup    *filterGroup
	filterHolder *fyne.Container

	// Filter query: an optional text query in the matrix builder query language.
	// When non-empty it overrides the visual rules above (see currentFilter).
	queryEntry  *widget.Entry
	queryStatus *widget.Label

	// controls is the scrolling left/right column that holds the rule editor.
	// Adding or removing rule rows only re-lays-out the rules box itself, so we
	// refresh this ancestor to reposition everything below it (see
	// refreshControls).
	controls *fyne.Container

	content fyne.CanvasObject
}

// filterChild is one editable node in the visual filter tree: either a group or
// a leaf condition. node reads the widgets into a FilterNode (returning nil for
// an incomplete leaf); object is the canvas object the parent group lays out.
type filterChild interface {
	node() *FilterNode
	object() fyne.CanvasObject
}

// filterGroup is a group node in the editor: a combinator (ALL/ANY), an optional
// "not", and an ordered list of child conditions and sub-groups.
type filterGroup struct {
	mb         *MatrixBuilder
	parent     *filterGroup // nil for the root group
	combinator *widget.Select
	negate     *widget.Check
	children   []filterChild
	inner      *fyne.Container // holds the children plus the add-buttons footer
	footer     fyne.CanvasObject
	container  *fyne.Container
}

// filterCond is a leaf condition in the editor: "<series> <op> <value>", where
// <value> is a single number, or a comma-separated list when <op> is "in".
type filterCond struct {
	parent    *filterGroup
	seriesSel *widget.SelectEntry
	opSel     *widget.Select
	value     *widget.Entry
	container *fyne.Container
}

// Preset is the on-disk representation of a matrix builder configuration. It
// holds only settings (series, dimensions and axis breakpoints); the learned
// matrix values are never stored.
type Preset struct {
	XSeries string    `json:"x_series"`
	YSeries string    `json:"y_series"`
	ZSeries string    `json:"z_series"`
	Cols    int       `json:"cols"`
	Rows    int       `json:"rows"`
	XAxis   []float64 `json:"x_axis"`
	YAxis   []float64 `json:"y_axis"`
	// XTolerance/YTolerance are percentages (1..100). Omitted in older presets,
	// which decode to 0 and are treated as the default (no filtering) on load.
	XTolerance float64 `json:"x_tolerance,omitempty"`
	YTolerance float64 `json:"y_tolerance,omitempty"`
	// Filter is the root of the visual builder's group/condition tree. Omitted in
	// presets saved before the tree existed; those carry Rules instead.
	Filter *FilterNode `json:"filter,omitempty"`
	// Rules is the legacy flat filter list. Read-only now: still decoded for
	// backward compatibility (migrated into an implicit ALL-of group on load), but
	// no longer written. Older presets that predate filters decode to nil.
	Rules []Rule `json:"rules,omitempty"`
	// Query is an optional filter written in the query language. When non-empty
	// it overrides the builder (matching the live behaviour). Omitted in older
	// presets.
	Query string `json:"query,omitempty"`
}

// FilterNode is one node of the visual builder's filter tree, persisted in a
// preset. It is either a group (Series empty: combines Children with Combinator,
// optionally negated) or a leaf condition (Series set: "<Series> <Operator>
// <Value>", or "<Series> in <Values>" when Operator is "in").
type FilterNode struct {
	Combinator string        `json:"combinator,omitempty"` // group: "and" | "or"
	Negate     bool          `json:"negate,omitempty"`     // wrap the node in not(...)
	Children   []*FilterNode `json:"children,omitempty"`   // group members

	Series   string    `json:"series,omitempty"`   // leaf series name
	Operator string    `json:"operator,omitempty"` // leaf comparison operator
	Value    float64   `json:"value,omitempty"`    // leaf threshold (non-"in" ops)
	Values   []float64 `json:"values,omitempty"`   // leaf membership list ("in" op)
}

// isGroup reports whether n is a group rather than a leaf. A leaf always names a
// series; a group never does.
func (n *FilterNode) isGroup() bool { return n.Series == "" }

// toQuery renders n as a query-language fragment, returning ok=false when the
// node contributes nothing (an incomplete leaf, or a group with no usable
// children). top suppresses the redundant outer parentheses on the root group.
func (n *FilterNode) toQuery(top bool) (string, bool) {
	if n.isGroup() {
		parts := make([]string, 0, len(n.Children))
		for _, c := range n.Children {
			if s, ok := c.toQuery(false); ok {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", false
		}
		joiner := " and "
		if n.Combinator == "or" {
			joiner = " or "
		}
		s := strings.Join(parts, joiner)
		if (len(parts) > 1 && !top) || n.Negate {
			s = "(" + s + ")"
		}
		if n.Negate {
			s = "not " + s
		}
		return s, true
	}

	if strings.TrimSpace(n.Series) == "" {
		return "", false
	}
	var s string
	if n.Operator == "in" {
		if len(n.Values) == 0 {
			return "", false
		}
		members := make([]string, len(n.Values))
		for i, v := range n.Values {
			members[i] = formatFloat(v)
		}
		s = fmt.Sprintf("%s in [%s]", n.Series, strings.Join(members, ", "))
	} else {
		op := n.Operator
		if op == "" {
			op = condOperators[0]
		}
		s = fmt.Sprintf("%s %s %s", n.Series, op, formatFloat(n.Value))
	}
	if n.Negate {
		s = "not (" + s + ")"
	}
	return s, true
}

// rulesToNode migrates a legacy flat rule list into an implicit ALL-of group.
func rulesToNode(rules []Rule) *FilterNode {
	root := &FilterNode{Combinator: "and"}
	for _, r := range rules {
		root.Children = append(root.Children, &FilterNode{
			Series: r.Series, Operator: r.Operator, Value: r.Threshold,
		})
	}
	return root
}

// Rule is a single filter condition. A sample is only counted as a Z-hit when it
// satisfies every active rule: the named series' value at that sample, compared
// against Threshold with Operator, must hold.
type Rule struct {
	Series    string  `json:"series"`
	Operator  string  `json:"operator"` // one of ">", ">=", "<", "<=", "==", "!=", "~"
	Threshold float64 `json:"threshold"`
}

// condOperators lists the operators offered in a condition row, in display
// order. The first entry is the default for a new condition. "~" matches values
// approximately equal to the value (see ruleEpsilonFrac); "in" tests membership
// of a comma-separated list.
var condOperators = []string{">", ">=", "<", "<=", "==", "!=", "~", "in"}

// combinator labels for a group, mapping to the query language's and/or.
const (
	combinatorAll = "ALL of" // every child must hold -> "and"
	combinatorAny = "ANY of" // any child may hold   -> "or"
)

var combinatorOptions = []string{combinatorAll, combinatorAny}

const (
	// ruleEpsilonFrac sets the half-width of the "~" (approximately-equal)
	// operator as a fraction of the threshold's magnitude, so the window scales
	// with the value being matched (e.g. 1% of an RPM target vs. 1% of a lambda
	// target).
	ruleEpsilonFrac = 0.01
	// ruleEpsilonMin is a small absolute floor so "~" stays usable when the
	// threshold is at or near zero (where a relative window would collapse to 0).
	ruleEpsilonMin = 1e-6
)

// New creates an empty MatrixBuilder. Log files are loaded from within the
// widget via a native file dialog.
func New(renderMode meshgrid.RenderBackend) *MatrixBuilder {
	mb := &MatrixBuilder{
		values:     make(map[string][]float64),
		cols:       defaultCols,
		rows:       defaultRows,
		xTolerance: defaultTolerance,
		yTolerance: defaultTolerance,
		renderMode: renderMode,
	}
	mb.ExtendBaseWidget(mb)
	mb.xAxis = make([]float64, mb.cols)
	mb.yAxis = make([]float64, mb.rows)
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.buildUI()
	return mb
}

func (mb *MatrixBuilder) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(mb.content)
}

func (mb *MatrixBuilder) buildUI() {
	// SelectEntry lets the user pick a loaded series from the dropdown or type a
	// name manually.
	mb.xSel = widget.NewSelectEntry(mb.order)
	mb.xSel.OnChanged = func(s string) { mb.xSeries = s }
	mb.ySel = widget.NewSelectEntry(mb.order)
	mb.ySel.OnChanged = func(s string) { mb.ySeries = s }
	mb.zSel = widget.NewSelectEntry(mb.order)
	mb.zSel.OnChanged = func(s string) { mb.zSeries = s }
	mb.xSel.PlaceHolder = "X series"
	mb.ySel.PlaceHolder = "Y series"
	mb.zSel.PlaceHolder = "Z series"

	mb.colsLabel = widget.NewLabel(strconv.Itoa(mb.cols))
	mb.rowsLabel = widget.NewLabel(strconv.Itoa(mb.rows))
	mb.status = widget.NewLabel("")
	mb.status.Wrapping = fyne.TextWrapWord

	colsRow := container.NewBorder(nil, nil,
		widget.NewLabel("Columns (X)"),
		container.NewHBox(
			widget.NewButton("-", func() { mb.setCols(mb.cols - 1) }),
			mb.colsLabel,
			widget.NewButton("+", func() { mb.setCols(mb.cols + 1) }),
			widget.NewButton("Auto", func() { mb.autoFill(true) }),
		),
	)
	rowsRow := container.NewBorder(nil, nil,
		widget.NewLabel("Rows (Y)"),
		container.NewHBox(
			widget.NewButton("-", func() { mb.setRows(mb.rows - 1) }),
			mb.rowsLabel,
			widget.NewButton("+", func() { mb.setRows(mb.rows + 1) }),
			widget.NewButton("Auto", func() { mb.autoFill(false) }),
		),
	)

	buildBtn := widget.NewButtonWithIcon("Build matrix", theme.GridIcon(), func() {
		if err := mb.analyze(); err != nil {
			mb.status.SetText(err.Error())
			return
		}
		mb.rebuildDisplay()
	})
	buildBtn.Importance = widget.HighImportance

	// View toggle: switch the display between the learned matrix and a coverage
	// heatmap of how many samples landed in each cell.
	mb.viewSelect = widget.NewSelect([]string{viewMatrix, viewCoverage}, func(string) {
		mb.rebuildDisplay()
	})
	// Set the field directly rather than SetSelected: the latter fires OnChanged,
	// which calls rebuildDisplay before mb.display exists (panic during buildUI).
	mb.viewSelect.Selected = viewMatrix
	bottomBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(widget.NewLabel("View"), mb.viewSelect),
		buildBtn,
	)

	mb.xBox = container.NewHBox()
	mb.yBox = container.NewVBox()
	mb.rebuildAxisEntries()

	mb.controls = container.NewVBox(
		// widget.NewLabelWithStyle("Series", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		colsRow,
		rowsRow,
		widget.NewSeparator(),
		labeled("X", mb.xSel),
		labeled("Y", mb.ySel),
		labeled("Z", mb.zSel),
		widget.NewSeparator(),
		mb.buildToleranceSection(),
		widget.NewSeparator(),
		mb.buildFilterSection(),
		xlayout.NewSpacer(),
		mb.status,
	)

	controlScroll := container.NewVScroll(mb.controls)
	controlScroll.SetMinSize(fyne.NewSize(240, 0))

	logList := container.NewBorder(
		nil,
		container.NewVBox(
			mb.buildPresetSection(),
		),
		nil,
		nil,
		container.NewVScroll(
			mb.buildLogSection(),
		),
	)

	mb.display = container.NewStack(mb.placeholder())

	// The X scale runs along the horizontal axis of the map; its editor sits as a
	// horizontal strip above the display.
	xPanel := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("X axis values", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		container.NewHScroll(mb.xBox),
	)

	mainSplit := container.NewHSplit(
		container.NewBorder(
			xPanel,
			bottomBar,
			container.NewVBox(
				widget.NewLabelWithStyle("Y axis values", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				mb.yBox,
			),
			nil,
			mb.display,
		),
		logList,
	)
	mainSplit.Offset = 0.9

	/*
		mb.content = container.NewBorder(
			nil,
			nil,
			controlScroll,
			nil,
			mainSplit,
		)
	*/
	split := container.NewHSplit(controlScroll, mainSplit)
	split.Offset = 0.25
	mb.content = split
}

func (mb *MatrixBuilder) placeholder() fyne.CanvasObject {
	return container.NewCenter(widget.NewLabel("Select X, Y and Z series, then click \"Build matrix\""))
}

// buildToleranceSection builds the per-axis Z-hit tolerance sliders. Each
// slider sets the maximum distance (as a percentage of the cell's half-spacing)
// a sample may sit from its nearest breakpoint and still count as a hit.
func (mb *MatrixBuilder) buildToleranceSection() fyne.CanvasObject {
	mb.xTolLabel = widget.NewLabel(tolText(mb.xTolerance))
	mb.yTolLabel = widget.NewLabel(tolText(mb.yTolerance))
	mb.xTolSlider = mb.newToleranceSlider(true)
	mb.yTolSlider = mb.newToleranceSlider(false)

	return container.NewVBox(
		widget.NewLabelWithStyle("Z-hit tolerance", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, widget.NewLabel("X"), layout.NewFixedWidth(44, mb.xTolLabel), mb.xTolSlider),
		container.NewBorder(nil, nil, widget.NewLabel("Y"), layout.NewFixedWidth(44, mb.yTolLabel), mb.yTolSlider),
	)
}

// newToleranceSlider builds a 1..100% slider bound to the X or Y tolerance.
// While dragging it only updates the value label; on release it re-runs the
// analysis (if a matrix is already built) so the user sees the effect live.
func (mb *MatrixBuilder) newToleranceSlider(isX bool) *widget.Slider {
	cur := mb.yTolerance
	if isX {
		cur = mb.xTolerance
	}
	s := widget.NewSlider(minTolerance, 100)
	s.Step = 1
	s.SetValue(cur)
	s.OnChanged = func(f float64) {
		if isX {
			mb.xTolerance = f
			mb.xTolLabel.SetText(tolText(f))
		} else {
			mb.yTolerance = f
			mb.yTolLabel.SetText(tolText(f))
		}
	}
	s.OnChangeEnded = func(float64) {
		if !mb.built {
			return
		}
		if err := mb.analyze(); err != nil {
			mb.status.SetText(err.Error())
			return
		}
		mb.rebuildDisplay()
	}
	return s
}

// --- filter tree ---

// buildFilterSection builds the visual query builder: a nestable tree of groups
// (ALL-of / ANY-of, optionally negated) and leaf conditions, plus a free-text
// query box that overrides the tree when non-empty.
func (mb *MatrixBuilder) buildFilterSection() fyne.CanvasObject {
	mb.rootGroup = mb.newFilterGroup(nil, &FilterNode{Combinator: "and"})
	mb.filterHolder = container.NewVBox(mb.rootGroup.object())

	// The query box accepts the matrix builder query language: comparisons
	// (> >= < <= == != ~), the "in [...]" membership test, joined with and/or/not
	// and grouped with (). A leading "if" is optional. When non-empty it overrides
	// the visual builder above.
	mb.queryEntry = widget.NewMultiLineEntry()
	mb.queryEntry.SetMinRowsVisible(2)
	mb.queryEntry.Wrapping = fyne.TextWrapWord
	mb.queryEntry.SetPlaceHolder("e.g. (ActualIn.n_Engine > 3000 and Out.X_AccPedal > 50) or ECMStat.ST_ActiveAirDem in [10, 20]")
	mb.queryEntry.OnChanged = func(string) { mb.validateQuery() }

	mb.queryStatus = widget.NewLabel("")
	mb.queryStatus.Wrapping = fyne.TextWrapWord

	fromTreeBtn := widget.NewButton("Builder->Query", func() {
		if s, ok := mb.filterTree().toQuery(true); ok {
			mb.queryEntry.SetText(s)
		} else {
			mb.queryEntry.SetText("")
		}
	})
	fromTreeBtn.Importance = widget.LowImportance

	return container.NewVBox(
		widget.NewLabelWithStyle("Filters (count a hit when…)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		mb.filterHolder,
		widget.NewSeparator(),
		widget.NewLabel("…or a query (overrides the builder when not empty):"),
		mb.queryEntry,
		container.NewBorder(nil, nil, nil, fromTreeBtn, mb.queryStatus),
	)
}

// validateQuery parses the query box live and reports its status: a syntax
// error, any referenced series that aren't loaded, or "ok". An empty box defers
// to the visual builder.
func (mb *MatrixBuilder) validateQuery() {
	src := strings.TrimSpace(mb.queryEntry.Text)
	if src == "" {
		mb.queryStatus.SetText("Empty — using the builder above")
		return
	}
	q, err := compileQuery(src, mb.resolve)
	if err != nil {
		mb.queryStatus.SetText("⚠ " + err.Error())
		return
	}
	// Only warn about unknown series once logs are loaded; before that every
	// name is "unknown" and the noise isn't helpful.
	if len(mb.values) > 0 {
		var unknown []string
		for _, s := range q.Series() {
			if _, ok := mb.values[s]; !ok {
				unknown = append(unknown, s)
			}
		}
		if len(unknown) > 0 {
			mb.queryStatus.SetText("⚠ unknown series: " + strings.Join(unknown, ", "))
			return
		}
	}
	mb.queryStatus.SetText("✓ query active")
}

// filterTree reads the live editor into a FilterNode tree (the root group).
func (mb *MatrixBuilder) filterTree() *FilterNode { return mb.rootGroup.node() }

// setFilterTree replaces the editor with a fresh tree built from model, swapping
// the new root group into the stable holder. A nil or non-group model becomes an
// empty (or single-condition) ALL-of root so there is always a group to edit.
func (mb *MatrixBuilder) setFilterTree(model *FilterNode) {
	switch {
	case model == nil:
		model = &FilterNode{Combinator: "and"}
	case !model.isGroup():
		model = &FilterNode{Combinator: "and", Children: []*FilterNode{model}}
	}
	mb.rootGroup = mb.newFilterGroup(nil, model)
	mb.filterHolder.Objects = []fyne.CanvasObject{mb.rootGroup.object()}
	mb.filterHolder.Refresh()
	mb.refreshControls()
}

// refreshControls re-lays-out the scrolling controls column after the filter
// tree changes shape. A nested Add/Remove only re-runs the affected group's own
// layout, leaving its ancestors (and everything below the filter section) at
// their old positions until the column is refreshed.
func (mb *MatrixBuilder) refreshControls() {
	if mb.controls != nil {
		mb.controls.Refresh()
	}
}

// indented wraps obj with a left margin so nested groups read as nested.
func indented(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, layout.NewFixedWidth(14, xlayout.NewSpacer()), nil, obj)
}

// --- group node ---

// newFilterGroup builds a group widget seeded from model, recursively creating
// its child conditions and sub-groups. parent is nil for the root group.
func (mb *MatrixBuilder) newFilterGroup(parent *filterGroup, model *FilterNode) *filterGroup {
	g := &filterGroup{mb: mb, parent: parent}

	g.combinator = widget.NewSelect(combinatorOptions, func(string) {})
	if model != nil && model.Combinator == "or" {
		g.combinator.SetSelected(combinatorAny)
	} else {
		g.combinator.SetSelected(combinatorAll)
	}

	g.negate = widget.NewCheck("not", nil)
	if model != nil {
		g.negate.SetChecked(model.Negate)
	}

	addCond := widget.NewButton("+ condition", func() { g.addChild(mb.newFilterCond(g, nil)) })
	addCond.Importance = widget.LowImportance
	addGroup := widget.NewButton("+ group", func() { g.addChild(mb.newFilterGroup(g, &FilterNode{Combinator: "and"})) })
	addGroup.Importance = widget.LowImportance
	g.footer = container.NewHBox(addCond, addGroup)

	header := container.NewHBox(layout.NewFixedWidth(96, g.combinator), g.negate)
	var headerRow fyne.CanvasObject = header
	if parent != nil {
		remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { parent.removeChild(g) })
		remove.Importance = widget.LowImportance
		headerRow = container.NewBorder(nil, nil, nil, remove, header)
	}

	g.inner = container.NewVBox()
	g.container = container.NewVBox(headerRow, indented(g.inner))

	if model != nil {
		for _, ch := range model.Children {
			if ch.isGroup() {
				g.children = append(g.children, mb.newFilterGroup(g, ch))
			} else {
				g.children = append(g.children, mb.newFilterCond(g, ch))
			}
		}
	}
	g.rebuild()
	return g
}

func (g *filterGroup) object() fyne.CanvasObject { return g.container }

// node reads the group (and, recursively, its children) into a FilterNode,
// dropping incomplete leaves. A group always yields a node, even when empty, so
// the tree's shape survives a save/load round-trip.
func (g *filterGroup) node() *FilterNode {
	combinator := "and"
	if g.combinator.Selected == combinatorAny {
		combinator = "or"
	}
	n := &FilterNode{Combinator: combinator, Negate: g.negate.Checked}
	for _, c := range g.children {
		if cn := c.node(); cn != nil {
			n.Children = append(n.Children, cn)
		}
	}
	return n
}

// addChild appends a new condition or sub-group and re-lays-out the group.
func (g *filterGroup) addChild(c filterChild) {
	g.children = append(g.children, c)
	g.rebuild()
}

// removeChild drops c from the group and re-lays-out.
func (g *filterGroup) removeChild(c filterChild) {
	for i, child := range g.children {
		if child == c {
			g.children = append(g.children[:i], g.children[i+1:]...)
			break
		}
	}
	g.rebuild()
}

// rebuild repopulates the group's inner box with its children followed by the
// add-buttons footer, then refreshes the surrounding controls column.
func (g *filterGroup) rebuild() {
	g.inner.Objects = g.inner.Objects[:0]
	for _, c := range g.children {
		g.inner.Add(c.object())
	}
	g.inner.Add(g.footer)
	g.inner.Refresh()
	g.mb.refreshControls()
}

// eachCond visits every leaf condition in the group's subtree, in order.
func (g *filterGroup) eachCond(fn func(*filterCond)) {
	for _, c := range g.children {
		switch v := c.(type) {
		case *filterCond:
			fn(v)
		case *filterGroup:
			v.eachCond(fn)
		}
	}
}

// --- condition node ---

// newFilterCond builds a leaf-condition widget seeded from model (nil for a
// fresh, empty condition).
func (mb *MatrixBuilder) newFilterCond(parent *filterGroup, model *FilterNode) *filterCond {
	c := &filterCond{parent: parent}

	c.seriesSel = widget.NewSelectEntry(mb.order)
	c.seriesSel.PlaceHolder = "Series"

	c.value = widget.NewEntry()
	// The value field doubles as a list when "in" is selected, so its hint tracks
	// the operator.
	c.opSel = widget.NewSelect(condOperators, func(op string) {
		c.value.SetPlaceHolder(valuePlaceholder(op))
	})

	op := condOperators[0]
	if model != nil && model.Operator != "" {
		op = model.Operator
	}
	c.opSel.SetSelected(op)
	c.value.SetPlaceHolder(valuePlaceholder(op))

	if model != nil {
		c.seriesSel.SetText(model.Series)
		if model.Operator == "in" {
			c.value.SetText(formatList(model.Values))
		} else if model.Series != "" {
			c.value.SetText(formatFloat(model.Value))
		}
	}

	remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { parent.removeChild(c) })
	remove.Importance = widget.LowImportance

	// Series stretches to fill; operator/value/remove are pinned to the right at
	// fixed widths so the narrow panel stays readable. The operator box must fit
	// the two-char operators plus the dropdown arrow or the Select truncates to "…".
	c.container = container.NewBorder(nil, nil, nil,
		container.NewHBox(
			layout.NewFixedWidth(72, c.opSel),
			layout.NewFixedWidth(72, c.value),
			remove,
		),
		c.seriesSel,
	)
	return c
}

func (c *filterCond) object() fyne.CanvasObject { return c.container }

// node reads the condition into a FilterNode, returning nil when it is
// incomplete: no series, an empty/invalid value for a comparison, or an empty
// list for "in". Incomplete conditions are dropped rather than matched.
func (c *filterCond) node() *FilterNode {
	series := strings.TrimSpace(c.seriesSel.Text)
	if series == "" {
		return nil
	}
	op := c.opSel.Selected
	if op == "" {
		op = condOperators[0]
	}
	n := &FilterNode{Series: series, Operator: op}
	if op == "in" {
		n.Values = parseList(c.value.Text)
		if len(n.Values) == 0 {
			return nil
		}
		return n
	}
	v, ok := parseFloatLoose(c.value.Text)
	if !ok {
		return nil
	}
	n.Value = v
	return n
}

// valuePlaceholder hints the value field's content for the selected operator.
func valuePlaceholder(op string) string {
	if op == "in" {
		return "10, 20, …"
	}
	return "Value"
}

// formatList renders a membership list as comma-separated numbers.
func formatList(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = formatFloat(v)
	}
	return strings.Join(parts, ", ")
}

// parseFloatLoose parses a single value, accepting a comma as the decimal point
// (European keyboards). It reports ok=false for blank or unparseable input.
func parseFloatLoose(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseList parses a comma-separated membership list, skipping blank or
// unparseable members. Here a comma separates values, so it is not treated as a
// decimal point.
func parseList(s string) []float64 {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if v, err := strconv.ParseFloat(part, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// approxEqual reports whether v is approximately equal to threshold, using a
// window that scales with the threshold's magnitude (see ruleEpsilonFrac) with a
// small absolute floor (ruleEpsilonMin) so it stays usable near zero. Shared by
// the per-row "~" rule and the query language's "~" operator.
func approxEqual(v, threshold float64) bool {
	eps := math.Abs(threshold) * ruleEpsilonFrac
	if eps < ruleEpsilonMin {
		eps = ruleEpsilonMin
	}
	return math.Abs(v-threshold) <= eps
}

// resolve returns series' value at sample i, reporting ok=false when the series
// is absent or its reading is NaN. It is the bridge the compiled query uses to
// read log data.
func (mb *MatrixBuilder) resolve(series string, i int) (float64, bool) {
	data, ok := mb.values[series]
	if !ok || i < 0 || i >= len(data) {
		return 0, false
	}
	v := data[i]
	if math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// currentFilter returns the predicate that decides whether a sample counts as a
// hit. A non-empty query box takes precedence over the visual builder;
// otherwise the builder's tree is compiled to a query (an empty tree accepts
// every sample). A query that fails to compile is returned as an error so Build
// surfaces it instead of silently filtering nothing.
func (mb *MatrixBuilder) currentFilter() (func(i int) bool, error) {
	src := strings.TrimSpace(mb.queryEntry.Text)
	if src == "" {
		// The builder is just a structured way to author a query, so compile it
		// through the same path rather than maintaining a second evaluator.
		if s, ok := mb.filterTree().toQuery(true); ok {
			src = s
		}
	}
	if src == "" {
		return func(int) bool { return true }, nil
	}
	q, err := compileQuery(src, mb.resolve)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	return q.Eval, nil
}

// rebuildAxisEntries regenerates the editable entry fields for the current
// column/row counts, seeding them from the current axis values.
func (mb *MatrixBuilder) rebuildAxisEntries() {
	mb.xEntries = make([]*widget.Entry, mb.cols)
	mb.xBox.Objects = mb.xBox.Objects[:0]
	for i := 0; i < mb.cols; i++ {
		mb.xBox.Add(mb.makeAxisEntry(true, i))
	}
	mb.xBox.Refresh()

	mb.yEntries = make([]*widget.Entry, mb.rows)
	mb.yBox.Objects = mb.yBox.Objects[:0]
	// Y runs highest-at-top to match the mapviewer, so add breakpoints in
	// descending index order (the axis itself stays sorted ascending).
	for i := mb.rows - 1; i >= 0; i-- {
		mb.yBox.Add(mb.makeAxisEntry(false, i))
	}
	mb.yBox.Refresh()
}

func (mb *MatrixBuilder) makeAxisEntry(isX bool, idx int) fyne.CanvasObject {
	axis := mb.xAxis
	prefix := "X"
	if !isX {
		axis = mb.yAxis
		prefix = "Y"
	}
	e := widget.NewEntry()
	e.SetText(formatFloat(axis[idx]))
	e.OnChanged = func(s string) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return
		}
		if isX {
			mb.xAxis[idx] = v
		} else {
			mb.yAxis[idx] = v
		}
	}
	// Apply (relabel the displayed map) when the user commits a value.
	e.OnSubmitted = func(string) {
		if mb.built {
			mb.rebuildDisplay()
		}
	}
	if isX {
		mb.xEntries[idx] = e
		// X breakpoints run horizontally along the top: label above a
		// fixed-width entry so the strip stays compact.
		label := widget.NewLabelWithStyle(prefix+strconv.Itoa(idx), fyne.TextAlignLeading, fyne.TextStyle{})
		return container.NewVBox(label, layout.NewFixedWidth(64, e))
	}
	mb.yEntries[idx] = e
	// Y breakpoints stack vertically in the side panel: label beside entry.
	return container.NewBorder(nil, nil, widget.NewLabel(prefix+strconv.Itoa(idx)), nil, e)
}

func (mb *MatrixBuilder) setCols(n int) {
	n = clamp(n, minAxis, maxAxis)
	if n == mb.cols {
		return
	}
	mb.xAxis = resizeAxis(mb.xAxis, n)
	mb.cols = n
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.built = false
	mb.colsLabel.SetText(strconv.Itoa(n))
	mb.rebuildAxisEntries()
	mb.rebuildDisplay()
}

func (mb *MatrixBuilder) setRows(n int) {
	n = clamp(n, minAxis, maxAxis)
	if n == mb.rows {
		return
	}
	mb.yAxis = resizeAxis(mb.yAxis, n)
	mb.rows = n
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.built = false
	mb.rowsLabel.SetText(strconv.Itoa(n))
	mb.rebuildAxisEntries()
	mb.rebuildDisplay()
}

// autoFill spreads the selected series' min..max evenly across the axis
// breakpoints. isX selects the X axis, otherwise the Y axis.
func (mb *MatrixBuilder) autoFill(isX bool) {
	series := mb.ySeries
	axis := mb.yAxis
	if isX {
		series = mb.xSeries
		axis = mb.xAxis
	}

	data, ok := mb.values[series]
	if !ok || len(data) == 0 {
		return
	}

	lo, hi := common.FindMinMaxFloat64(data)
	n := len(axis)
	for i := 0; i < n; i++ {
		if n == 1 {
			axis[i] = lo
			continue
		}
		axis[i] = lo + (hi-lo)*float64(i)/float64(n-1)
	}
	mb.syncAxisEntries()
	if mb.built {
		mb.rebuildDisplay()
	}
}

// syncAxisEntries pushes the current axis values back into the entry widgets.
// The entry/axis lengths track each other via rebuildAxisEntries, but the
// guard keeps a transient desync from panicking the UI thread.
func (mb *MatrixBuilder) syncAxisEntries() {
	for i, e := range mb.xEntries {
		if i >= len(mb.xAxis) {
			break
		}
		e.SetText(formatFloat(mb.xAxis[i]))
	}
	for i, e := range mb.yEntries {
		if i >= len(mb.yAxis) {
			break
		}
		e.SetText(formatFloat(mb.yAxis[i]))
	}
}

// analyze walks the log and learns the matrix: each sample is assigned to the
// nearest cell and the cell's value becomes the average of its hits.
func (mb *MatrixBuilder) analyze() error {
	if len(mb.values) == 0 {
		return fmt.Errorf("load a log file first")
	}
	if mb.xSeries == "" || mb.ySeries == "" || mb.zSeries == "" {
		return fmt.Errorf("select X, Y and Z series first")
	}
	xv, okX := mb.values[mb.xSeries]
	yv, okY := mb.values[mb.ySeries]
	zv, okZ := mb.values[mb.zSeries]
	if !okX || !okY || !okZ {
		return fmt.Errorf("selected series not found in the loaded logs")
	}
	n := min(len(xv), min(len(yv), len(zv)))
	if n == 0 {
		return fmt.Errorf("selected series contain no samples")
	}

	// Sort the axes ascending so the learned map reads like a normal table.
	sort.Float64s(mb.xAxis)
	sort.Float64s(mb.yAxis)
	mb.syncAxisEntries()

	filter, err := mb.currentFilter()
	if err != nil {
		return err
	}

	size := mb.cols * mb.rows
	sum := make([]float64, size)
	cnt := make([]int, size)
	used := 0
	skipped := 0
	filtered := 0
	for i := 0; i < n; i++ {
		// Skip rows where any of the three series is missing (NaN padding from
		// merging logs with differing channel sets).
		if math.IsNaN(xv[i]) || math.IsNaN(yv[i]) || math.IsNaN(zv[i]) {
			continue
		}
		// Drop samples that fail any user-defined filter rule before they can
		// contribute to a cell.
		if !filter(i) {
			filtered++
			continue
		}
		c := nearestIndex(mb.xAxis, xv[i])
		r := nearestIndex(mb.yAxis, yv[i])
		// Reject samples sitting too far from their nearest breakpoint on
		// either axis, so only values close to a cell count as a Z-hit.
		if !withinTolerance(mb.xAxis, c, xv[i], mb.xTolerance) ||
			!withinTolerance(mb.yAxis, r, yv[i], mb.yTolerance) {
			skipped++
			continue
		}
		idx := r*mb.cols + c
		sum[idx] += zv[i]
		cnt[idx]++
		used++
	}

	mb.zData = make([]float64, size)
	mb.counts = cnt
	filled := 0
	for i := range sum {
		if cnt[i] > 0 {
			mb.zData[i] = sum[i] / float64(cnt[i])
			filled++
		}
	}
	mb.built = true
	msg := fmt.Sprintf("Mapped %d samples, %d/%d cells filled", used, filled, size)
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d skipped by tolerance)", skipped)
	}
	if filtered > 0 {
		msg += fmt.Sprintf(" (%d filtered)", filtered)
	}
	mb.status.SetText(msg)
	return nil
}

// rebuildDisplay swaps a freshly built mapviewer (grid + 3D mesh) into the
// display area, reflecting the current axes and learned Z data. Before the
// first build it shows the placeholder instead.
func (mb *MatrixBuilder) rebuildDisplay() {
	if !mb.built {
		mb.display.Objects = []fyne.CanvasObject{mb.placeholder()}
		mb.display.Refresh()
		return
	}

	// Default to the learned matrix; the Coverage view swaps in the per-cell hit
	// counts as a read-only heatmap.
	zData := mb.zData
	zLabel := mb.zSeries
	zPrec := precisionFor(mb.zData)
	editable := true
	if mb.viewSelect != nil && mb.viewSelect.Selected == viewCoverage {
		zData = countsToFloat(mb.counts)
		zLabel = "Hits"
		zPrec = 0
		editable = false
	}

	noop := func([]float64) {}
	mv, err := mapviewer.New(&mapviewer.Config{
		Name:           zLabel,
		XData:          mb.xAxis,
		YData:          mb.yAxis,
		ZData:          zData,
		XPrecision:     precisionFor(mb.xAxis),
		YPrecision:     precisionFor(mb.yAxis),
		ZPrecision:     zPrec,
		XLabel:         mb.xSeries,
		YLabel:         mb.ySeries,
		ZLabel:         zLabel,
		MeshView:       true,
		MeshRenderer:   mb.renderMode,
		Editable:       editable,
		ColorblindMode: colors.ModeNormal,
		// The matrix is in-memory only; editing cells just mutates zData.
		SaveECUFunc:  noop,
		OnUpdateCell: func(int, []float64) {},
	})
	if err != nil {
		mb.display.Objects = []fyne.CanvasObject{container.NewCenter(widget.NewLabel(err.Error()))}
		mb.display.Refresh()
		return
	}
	mb.display.Objects = []fyne.CanvasObject{mv}
	mb.display.Refresh()
}

// --- log files ---

func (mb *MatrixBuilder) buildLogSection() fyne.CanvasObject {
	mb.logsLabel = widget.NewLabel("No log files loaded")
	mb.logsLabel.Truncation = fyne.TextTruncateEllipsis

	addBtn := widget.NewButtonWithIcon("Add log files", theme.FolderOpenIcon(), mb.openLogDialog)
	clearBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), mb.clearLogs)

	return container.NewVBox(
		// widget.NewLabelWithStyle("Log files", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, addBtn, clearBtn),
		mb.logsLabel,
	)
}

// openLogDialog shows the native multi-file picker and loads the chosen logs.
// Parsing runs off the UI goroutine behind a progress modal, then the parsed
// series are merged on the UI goroutine.
func (mb *MatrixBuilder) openLogDialog() {
	widgets.SelectFiles(func(readers []fyne.URIReadCloser) {
		c := fyne.CurrentApp().Driver().CanvasForObject(mb)
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
					log.Println("matrixbuilder:", err)
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
					mb.mergeLog(p.name, p.local, p.n)
				}
				mb.rebuildOrder()
				mb.refreshSeriesOptions()
				mb.refreshLogList()
				if failed > 0 {
					mb.status.SetText(fmt.Sprintf("Loaded %d file(s), %d failed", len(ok), failed))
				} else {
					mb.status.SetText(fmt.Sprintf("Loaded %d file(s), %d records total", len(ok), mb.nrecords))
				}
			})
		}()
	}, "logfile", "t5l", "t7l", "t8l", "csv", "bpl")
}

// parseLog reads a single log file into a row-aligned series map, padding gaps
// with NaN. It touches no shared state, so it is safe to call off the UI
// goroutine.
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

// mergeLog merges a parsed log into the global series set, padding so all
// series stay row-aligned. Must run on the UI goroutine.
func (mb *MatrixBuilder) mergeLog(name string, local map[string][]float64, n int) {
	base := mb.nrecords
	for k, arr := range local {
		cur, ok := mb.values[k]
		if !ok {
			cur = nanSlice(base)
		}
		mb.values[k] = append(cur, arr...)
	}
	for k, cur := range mb.values {
		if _, ok := local[k]; !ok {
			mb.values[k] = append(cur, nanSlice(n)...)
		}
	}
	mb.nrecords = base + n
	mb.loadedFiles = append(mb.loadedFiles, name)
}

func (mb *MatrixBuilder) clearLogs() {
	mb.values = make(map[string][]float64)
	mb.order = nil
	mb.loadedFiles = nil
	mb.nrecords = 0
	mb.built = false
	mb.refreshSeriesOptions()
	mb.refreshLogList()
	mb.rebuildDisplay()
	mb.status.SetText("Cleared loaded logs")
}

// rebuildOrder refreshes the sorted list of available series names.
func (mb *MatrixBuilder) rebuildOrder() {
	mb.order = make([]string, 0, len(mb.values))
	for k := range mb.values {
		mb.order = append(mb.order, k)
	}
	sort.Slice(mb.order, func(i, j int) bool {
		return strings.ToLower(mb.order[i]) < strings.ToLower(mb.order[j])
	})
}

func (mb *MatrixBuilder) refreshSeriesOptions() {
	mb.xSel.SetOptions(mb.order)
	mb.ySel.SetOptions(mb.order)
	mb.zSel.SetOptions(mb.order)
	mb.rootGroup.eachCond(func(c *filterCond) {
		c.seriesSel.SetOptions(mb.order)
	})
	// Loaded series changed, so the query's unknown-series check may now differ.
	if mb.queryEntry != nil {
		mb.validateQuery()
	}
}

func (mb *MatrixBuilder) refreshLogList() {
	if len(mb.loadedFiles) == 0 {
		mb.logsLabel.SetText("No log files loaded")
		return
	}
	mb.logsLabel.SetText(fmt.Sprintf("%d file(s), %d records:\n%s",
		len(mb.loadedFiles), mb.nrecords, buildFilenamesList(mb.loadedFiles)))
}

func buildFilenamesList(files []string) string {
	if len(files) == 0 {
		return ""
	}
	var b strings.Builder
	for i, f := range files {
		if i > 0 {
			b.WriteString("\n")
		}
		base := filepath.Base(f)
		//if len(base) > 25 {
		//	base = base[:25] + "…"
		//}
		b.WriteString(base)
	}
	return b.String()
}

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// --- presets ---

func (mb *MatrixBuilder) buildPresetSection() fyne.CanvasObject {
	mb.presetSelect = widget.NewSelect(mb.listPresets(), func(name string) {
		if name == "" {
			return
		}
		if err := mb.loadPreset(name); err != nil {
			mb.status.SetText(err.Error())
		}
	})
	mb.presetSelect.PlaceHolder = "Load preset"

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), mb.refreshPresets)

	mb.nameEntry = widget.NewEntry()
	mb.nameEntry.SetPlaceHolder("Preset name")
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		name := strings.TrimSpace(mb.nameEntry.Text)
		if name == "" {
			mb.status.SetText("Enter a preset name to save")
			return
		}
		saved, err := mb.savePreset(name)
		if err != nil {
			mb.status.SetText(err.Error())
			return
		}
		mb.refreshPresets()
		// Reflect the saved name in the picker without re-triggering a load.
		mb.presetSelect.Selected = saved
		mb.presetSelect.Refresh()
		mb.status.SetText("Saved preset " + saved)
	})

	return container.NewVBox(
		// widget.NewLabelWithStyle("Presets", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, refreshBtn, mb.presetSelect),
		container.NewBorder(nil, nil, nil, saveBtn, mb.nameEntry),
	)
}

// listPresets returns the names (without extension) of the saved presets.
func (mb *MatrixBuilder) listPresets() []string {
	path, err := common.GetMatrixBuilderPath()
	if err != nil {
		return nil
	}
	files, err := common.ListFilesInPathByExtension(path, ".json")
	if err != nil {
		return nil
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = strings.TrimSuffix(f, ".json")
	}
	return names
}

func (mb *MatrixBuilder) refreshPresets() {
	mb.presetSelect.Options = mb.listPresets()
	mb.presetSelect.Refresh()
}

// savePreset writes the current configuration (and learned matrix, if any) to
// name.json in the matrix builder directory. It returns the stored preset name
// (without extension), which may differ from name after sanitization.
func (mb *MatrixBuilder) savePreset(name string) (string, error) {
	path, err := common.GetMatrixBuilderPath()
	if err != nil {
		return "", err
	}
	p := Preset{
		XSeries:    mb.xSeries,
		YSeries:    mb.ySeries,
		ZSeries:    mb.zSeries,
		Cols:       mb.cols,
		Rows:       mb.rows,
		XAxis:      mb.xAxis,
		YAxis:      mb.yAxis,
		XTolerance: mb.xTolerance,
		YTolerance: mb.yTolerance,
		Filter:     mb.filterTree(),
		Query:      strings.TrimSpace(mb.queryEntry.Text),
	}
	b, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return "", err
	}
	stored := common.SanitizeFilename(name + ".json")
	if err := os.WriteFile(filepath.Join(path, stored), b, 0o644); err != nil {
		return "", err
	}
	return strings.TrimSuffix(stored, ".json"), nil
}

// loadPreset reads name.json and applies it to the builder.
func (mb *MatrixBuilder) loadPreset(name string) error {
	path, err := common.GetMatrixBuilderPath()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(path, common.SanitizeFilename(name+".json")))
	if err != nil {
		return err
	}
	var p Preset
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("failed to decode preset: %w", err)
	}
	mb.applyPreset(&p)
	mb.status.SetText("Loaded preset " + name)
	return nil
}

// applyPreset replaces the current state with the preset's, rebuilding the
// editor and display.
func (mb *MatrixBuilder) applyPreset(p *Preset) {
	mb.cols = clamp(p.Cols, minAxis, maxAxis)
	mb.rows = clamp(p.Rows, minAxis, maxAxis)
	mb.xAxis = resizeAxis(p.XAxis, mb.cols)
	mb.yAxis = resizeAxis(p.YAxis, mb.rows)
	// A preset carries only settings, so the matrix must be rebuilt after load.
	// Clear built first: Slider.SetValue below fires OnChangeEnded, and we must
	// not let it re-run analyze() against the half-applied state.
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.built = false

	// Older presets predate the tolerance fields and decode to 0; fall back to
	// the default (no filtering) rather than rejecting every sample.
	mb.xTolerance = toleranceOrDefault(p.XTolerance)
	mb.yTolerance = toleranceOrDefault(p.YTolerance)
	mb.xTolSlider.SetValue(mb.xTolerance)
	mb.yTolSlider.SetValue(mb.yTolerance)
	mb.xTolLabel.SetText(tolText(mb.xTolerance))
	mb.yTolLabel.SetText(tolText(mb.yTolerance))

	// SetText fires OnChanged, which just records the series name (no auto-fill),
	// so this is safe and keeps mb.xSeries/etc. in sync.
	mb.xSel.SetText(p.XSeries)
	mb.ySel.SetText(p.YSeries)
	mb.zSel.SetText(p.ZSeries)

	// Replace the filter tree with the preset's, falling back to the legacy flat
	// rules for presets saved before the tree existed. Restoring the query box
	// fires OnChanged, which re-runs validateQuery.
	root := p.Filter
	if root == nil && len(p.Rules) > 0 {
		root = rulesToNode(p.Rules)
	}
	mb.setFilterTree(root)
	mb.queryEntry.SetText(p.Query)

	mb.colsLabel.SetText(strconv.Itoa(mb.cols))
	mb.rowsLabel.SetText(strconv.Itoa(mb.rows))
	mb.rebuildAxisEntries()
	mb.rebuildDisplay()
}

// --- helpers ---

func labeled(label string, obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, obj)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// resizeAxis grows or shrinks an axis to length n, preserving existing values
// and padding new slots with the previous value (or zero for an empty axis).
func resizeAxis(old []float64, n int) []float64 {
	out := make([]float64, n)
	copy(out, old)
	for i := len(old); i < n; i++ {
		if i > 0 {
			out[i] = out[i-1]
		}
	}
	return out
}

// withinTolerance reports whether v is close enough to breakpoint c on a sorted
// axis to count as a hit. tolPct is the allowed distance expressed as a
// percentage of the half-spacing to the neighbouring breakpoint on v's side, so
// 100% accepts the entire nearest-neighbor region and lower values reject
// samples sitting near the cell boundary. Edge cells use their inner spacing as
// the reference, so a sample far beyond the axis range is rejected too.
func withinTolerance(axis []float64, c int, v, tolPct float64) bool {
	if tolPct >= 100 || len(axis) < 2 {
		return true
	}
	bp := axis[c]
	var spacing float64
	if v >= bp { // neighbour on the high side, falling back to the low side
		switch {
		case c+1 < len(axis):
			spacing = axis[c+1] - bp
		case c-1 >= 0:
			spacing = bp - axis[c-1]
		}
	} else { // neighbour on the low side, falling back to the high side
		switch {
		case c-1 >= 0:
			spacing = bp - axis[c-1]
		case c+1 < len(axis):
			spacing = axis[c+1] - bp
		}
	}
	if spacing <= 0 { // duplicate/degenerate breakpoints: nothing to gate on
		return true
	}
	maxDist := tolPct / 100.0 * (spacing / 2.0)
	return math.Abs(v-bp) <= maxDist
}

// toleranceOrDefault clamps a stored tolerance into the valid range, mapping the
// zero value (older presets without the field) to the default.
func toleranceOrDefault(v float64) float64 {
	if v < minTolerance {
		return defaultTolerance
	}
	if v > 100 {
		return 100
	}
	return v
}

func tolText(v float64) string {
	return strconv.Itoa(int(v)) + "%"
}

// nearestIndex returns the index of the axis breakpoint closest to v.
func nearestIndex(axis []float64, v float64) int {
	best := 0
	bestDist := math.Abs(axis[0] - v)
	for i := 1; i < len(axis); i++ {
		d := math.Abs(axis[i] - v)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// countsToFloat converts per-cell hit counts to float64 Z data for the
// mapviewer's Coverage view.
func countsToFloat(counts []int) []float64 {
	out := make([]float64, len(counts))
	for i, c := range counts {
		out[i] = float64(c)
	}
	return out
}

// precisionFor picks a sensible decimal precision: 0 for all-integer data,
// otherwise more decimals for small-magnitude values.
func precisionFor(data []float64) int {
	allInt := true
	maxAbs := 0.0
	for _, v := range data {
		if v != math.Trunc(v) {
			allInt = false
		}
		if a := math.Abs(v); a > maxAbs {
			maxAbs = a
		}
	}
	if allInt {
		return 0
	}
	if maxAbs < 10 {
		return 3
	}
	return 2
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
