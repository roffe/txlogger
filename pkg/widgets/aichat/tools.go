package aichat

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/ollama"
)

// Caps on tool output. The model has a finite context and a 27B at 4-bit fills
// it slowly, so every tool truncates and says so rather than dumping.
const (
	maxSymbolRows = 150
	maxLogRows    = 400
)

func (w *Widget) tools() []ollama.Tool {
	return []ollama.Tool{
		ollama.NewTool("list_symbols",
			"List symbols (maps and scalars) in the loaded binary whose name contains the filter, with the scale factor txlogger applies to each. Use this first to find the exact name of a map before reading it.",
			ollama.Object(map[string]any{
				"filter": map[string]any{
					"type":        "string",
					"description": "Case-insensitive substring of the symbol name, e.g. \"fuel\", \"IgnE\", \"BoostCal\". Empty lists everything (truncated).",
				},
			})),

		ollama.NewTool("read_symbol",
			"Read one symbol from the loaded binary, exactly as txlogger displays it. Maps are returned as a table with their X and Y axes labelled. The 'scale' line says what the raw ECU counts were multiplied by: a scale other than 1 means the values are already in engineering units, a scale of 1 means they are raw counts. Never re-scale the numbers yourself.",
			ollama.Object(map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Exact symbol name, e.g. \"BFuelCal.Map\".",
				},
			}, "name")),

		ollama.NewTool("log_info",
			"Summarise the loaded log file: duration, sample count, and the min/max/average of every logged channel. Logged values are already scaled to the units txlogger displays. Use this to see what was logged and to spot which channels are interesting before pulling samples.",
			ollama.Object(map[string]any{})),

		ollama.NewTool("log_samples",
			"Read a slice of samples from the loaded log file as CSV. Use stride to scan a long log coarsely, then a stride of 1 to zoom in on an area of interest.",
			ollama.Object(map[string]any{
				"channels": map[string]any{
					"type":        "string",
					"description": "Comma-separated channel names, e.g. \"ActualIn.n_Engine,Out.PWM_BoostCntrl\". Empty returns every channel.",
				},
				"start":  map[string]any{"type": "integer", "description": "Index of the first sample (0-based)."},
				"count":  map[string]any{"type": "integer", "description": "Number of samples to return, default 50."},
				"stride": map[string]any{"type": "integer", "description": "Return every Nth sample, default 1."},
			})),

		ollama.NewTool("show_map_diff",
			"Show the user your suggested change to a map as a picture: opens a window in txlogger with three tabs — Before (the current map), After (your values) and Diff (the per-cell change). Use this whenever you propose new map values instead of only writing numbers in the chat. Only send the cells you want to change; everything else keeps its current value.",
			ollama.Object(map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Exact symbol name of the map, e.g. \"BFuelCal.Map\".",
				},
				"changes": map[string]any{
					"type":        "string",
					"description": "Semicolon-separated cell edits, each \"row,col=value\" using the [row]/[col] indices read_symbol prints, with the new value in the same units and scale read_symbol printed for that map (it is quantised to the map's scale step when written). Example: \"12,5=1.10; 12,6=1.12; 13,5=1.08\". A single number before the = is a flat index into a list-shaped symbol.",
				},
			}, "name", "changes")),
	}
}

func (w *Widget) runTool(name string, args map[string]any) string {
	switch name {
	case "list_symbols":
		return w.listSymbols(argStr(args, "filter"))
	case "read_symbol":
		return w.readSymbol(argStr(args, "name"))
	case "log_info":
		return w.logInfo()
	case "log_samples":
		return w.logSamples(argStr(args, "channels"), argInt(args, "start", 0), argInt(args, "count", 50), argInt(args, "stride", 1))
	case "show_map_diff":
		return w.showMapDiff(argStr(args, "name"), argStr(args, "changes"))
	default:
		return "Error: unknown tool " + name
	}
}

// --- binary ----------------------------------------------------------------

func (w *Widget) listSymbols(filter string) string {
	fw := w.cfg.FW()
	if fw == nil {
		return "Error: no binary is loaded in txlogger. Ask the user to open one."
	}

	filter = strings.ToLower(filter)
	var b strings.Builder
	b.WriteString("name,bytes,unit,scale\n")
	var matched, shown int
	for _, sym := range fw.Symbols() {
		if filter != "" && !strings.Contains(strings.ToLower(sym.Name), filter) {
			continue
		}
		matched++
		if shown >= maxSymbolRows {
			continue
		}
		shown++
		fmt.Fprintf(&b, "%s,%d,%s,%s\n", sym.Name, sym.Length, sym.Unit, fmtScale(sym.Correctionfactor))
	}

	if matched == 0 {
		return fmt.Sprintf("No symbol name contains %q (binary has %d symbols). Try a shorter filter.", filter, fw.Count())
	}
	if matched > shown {
		fmt.Fprintf(&b, "\n(%d matches, showing first %d — narrow the filter)\n", matched, shown)
	}
	return b.String()
}

func (w *Widget) readSymbol(name string) string {
	fw := w.cfg.FW()
	if fw == nil {
		return "Error: no binary is loaded in txlogger. Ask the user to open one."
	}
	name = strings.TrimSpace(name)
	sym := fw.GetByName(name)
	if sym == nil {
		return notFound(fw, name)
	}

	vals := sym.Float64s()
	prec := symbol.GetPrecision(sym.Correctionfactor)
	ecu := symbol.ECUTypeFromString(w.cfg.ECU())
	ax := symbol.GetInfo(ecu, sym.Name)

	var b strings.Builder
	fmt.Fprintf(&b, "%s", sym.Name)
	if unit := firstNonEmpty(sym.Unit, ax.ZDescription); unit != "" {
		fmt.Fprintf(&b, " [%s]", unit)
	}
	fmt.Fprintf(&b, " (%d values)\n", len(vals))
	// Whether these numbers are engineering units or raw ECU counts is decided
	// per symbol by its correction factor, so say which it is — a model that
	// guesses reads 175 as 175 degrees.
	if sym.Correctionfactor == 1 {
		b.WriteString("scale = 1 (raw ECU counts, unscaled)\n")
	} else {
		s := fmtScale(sym.Correctionfactor)
		fmt.Fprintf(&b, "scale = %s (values below are already scaled, smallest step %s)\n", s, s)
	}

	if len(vals) == 1 {
		fmt.Fprintf(&b, "%s\n", fmtVal(vals[0], prec))
		return b.String()
	}

	// A map with known axes prints as a labelled grid; that's what makes the
	// numbers interpretable ("rich at 4000rpm / high load" rather than "cell 47").
	if x, y, ok := axesFor(fw, ecu, sym.Name, len(vals)); ok {
		fmt.Fprintf(&b, "columns = %s [%s]\n", ax.X, ax.XDescription)
		if ax.Y != "" {
			fmt.Fprintf(&b, "rows = %s [%s]\n", ax.Y, ax.YDescription)
		}
		// Cells are labelled [row]/[col] because that is how show_map_diff
		// addresses them.
		b.WriteString("\ny\\x")
		for col, xv := range x {
			fmt.Fprintf(&b, ",[%d] %s", col, fmtVal(xv, precOf(fw, ax.X)))
		}
		b.WriteByte('\n')
		for row, yv := range y {
			fmt.Fprintf(&b, "[%d] %s", row, fmtVal(yv, precOf(fw, ax.Y)))
			for col := range x {
				fmt.Fprintf(&b, ",%s", fmtVal(vals[row*len(x)+col], prec))
			}
			b.WriteByte('\n')
		}
		return b.String()
	}

	for i, v := range vals {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmtVal(v, prec))
	}
	b.WriteByte('\n')
	return b.String()
}

// axesFor returns the decoded X and Y axis values for a map, but only when
// their dimensions actually multiply out to the map size — a mismatch means the
// axis table doesn't describe this binary and a grid would be a lie.
func axesFor(fw symbol.SymbolCollection, ecu symbol.ECUType, name string, zLen int) ([]float64, []float64, bool) {
	ax := symbol.GetInfo(ecu, name)
	if ax.X == "" {
		return nil, nil, false
	}
	symX := fw.GetByName(ax.X)
	if symX == nil {
		return nil, nil, false
	}
	x := symX.Float64s()
	y := []float64{0}
	if ax.Y != "" {
		symY := fw.GetByName(ax.Y)
		if symY == nil {
			return nil, nil, false
		}
		y = symY.Float64s()
	}
	if len(x) == 0 || len(y) == 0 || len(x)*len(y) != zLen {
		return nil, nil, false
	}
	return x, y, true
}

func notFound(fw symbol.SymbolCollection, name string) string {
	var near []string
	lower := strings.ToLower(name)
	if i := strings.IndexByte(lower, '.'); i > 2 {
		lower = lower[:i]
	}
	for _, sym := range fw.Symbols() {
		if strings.Contains(strings.ToLower(sym.Name), lower) {
			near = append(near, sym.Name)
			if len(near) == 8 {
				break
			}
		}
	}
	if len(near) == 0 {
		return fmt.Sprintf("Error: no symbol named %q. Use list_symbols to find the right name.", name)
	}
	return fmt.Sprintf("Error: no symbol named %q. Did you mean: %s", name, strings.Join(near, ", "))
}

func precOf(fw symbol.SymbolCollection, name string) int {
	if sym := fw.GetByName(name); sym != nil {
		return symbol.GetPrecision(sym.Correctionfactor)
	}
	return 2
}

// showMapDiff applies the model's cell edits on top of the current map and
// hands the result to the host to render. Only the changed cells are sent:
// asking a local model to re-emit a 288-cell map verbatim fails far more often
// than it succeeds. Validation happens here so a bad proposal comes back as a
// tool error the model can fix, not a popup at the user.
func (w *Widget) showMapDiff(name, changes string) string {
	fw := w.cfg.FW()
	if fw == nil {
		return "Error: no binary is loaded in txlogger. Ask the user to open one."
	}
	if w.cfg.ShowMapDiff == nil {
		return "Error: this build cannot display map diffs."
	}
	name = strings.TrimSpace(name)
	sym := fw.GetByName(name)
	if sym == nil {
		return notFound(fw, name)
	}

	proposed := sym.Float64s()
	cols := 1
	if x, _, ok := axesFor(fw, symbol.ECUTypeFromString(w.cfg.ECU()), sym.Name, len(proposed)); ok {
		cols = len(x)
	}
	rows := len(proposed) / cols

	edits := 0
	for _, tok := range strings.FieldsFunc(changes, func(r rune) bool { return r == ';' || r == '\n' }) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		cell, val, ok := strings.Cut(tok, "=")
		if !ok {
			return fmt.Sprintf("Error: %q is not a cell edit. Use \"row,col=value\", separated by semicolons.", tok)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return fmt.Sprintf("Error: %q in %q is not a number.", strings.TrimSpace(val), tok)
		}
		idx, errMsg := cellIndex(cell, rows, cols)
		if errMsg != "" {
			return errMsg
		}
		proposed[idx] = v
		edits++
	}
	if edits == 0 {
		return fmt.Sprintf("Error: no cell edits given for %s (it is %d rows x %d columns). Send at least one \"row,col=value\".", name, rows, cols)
	}

	if err := w.cfg.ShowMapDiff(name, proposed); err != nil {
		return "Error: " + err.Error()
	}
	return fmt.Sprintf("Opened a Before/After/Diff view of %s for the user, with %d cells changed. Now summarise what you changed and why.", name, edits)
}

// cellIndex resolves "row,col" (or a bare flat index) into an offset in the
// row-major value slice.
func cellIndex(cell string, rows, cols int) (int, string) {
	parts := splitList(cell)
	nums := make([]int, 0, 2)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Sprintf("Error: %q is not a cell index. Use \"row,col\" with the numbers read_symbol prints in brackets.", cell)
		}
		nums = append(nums, n)
	}
	var idx int
	switch len(nums) {
	case 1:
		idx = nums[0]
	case 2:
		if nums[0] < 0 || nums[0] >= rows || nums[1] < 0 || nums[1] >= cols {
			return 0, fmt.Sprintf("Error: cell %q is outside the map (rows 0-%d, columns 0-%d).", cell, rows-1, cols-1)
		}
		idx = nums[0]*cols + nums[1]
	default:
		return 0, fmt.Sprintf("Error: %q is not a cell index. Use \"row,col\".", cell)
	}
	if idx < 0 || idx >= rows*cols {
		return 0, fmt.Sprintf("Error: cell %q is outside the map (%d values).", cell, rows*cols)
	}
	return idx, ""
}

// --- log -------------------------------------------------------------------

func (w *Widget) logInfo() string {
	lf := w.cfg.Log()
	if lf == nil || lf.Len() == 0 {
		return "Error: no log file is loaded in txlogger. Ask the user to open one."
	}

	type stat struct {
		min, max, sum float64
		n             int
	}
	stats := make(map[string]*stat)
	n := lf.Len()
	for i := 0; i < n; i++ {
		for k, v := range lf.RecordAt(i).Values {
			s := stats[k]
			if s == nil {
				s = &stat{min: math.Inf(1), max: math.Inf(-1)}
				stats[k] = s
			}
			s.min = math.Min(s.min, v)
			s.max = math.Max(s.max, v)
			s.sum += v
			s.n++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "log: %s\nsamples: %d (indices 0-%d)\nduration: %s\n\n",
		w.cfg.LogName(), n, n-1, lf.End().Sub(lf.Start()).Round(1e6))
	b.WriteString("channel,min,max,average\n")
	for _, k := range sortedKeys(stats) {
		s := stats[k]
		fmt.Fprintf(&b, "%s,%s,%s,%s\n", k, fmtVal(s.min, 2), fmtVal(s.max, 2), fmtVal(s.sum/float64(s.n), 2))
	}
	return b.String()
}

func (w *Widget) logSamples(channels string, start, count, stride int) string {
	lf := w.cfg.Log()
	if lf == nil || lf.Len() == 0 {
		return "Error: no log file is loaded in txlogger. Ask the user to open one."
	}

	if stride < 1 {
		stride = 1
	}
	if count < 1 {
		count = 50
	}
	truncated := false
	if count > maxLogRows {
		count, truncated = maxLogRows, true
	}
	if start < 0 {
		start = 0
	}
	// Range-check before touching a record: column discovery below reads
	// RecordAt(start), and the Logfile interface doesn't promise clamping.
	if start >= lf.Len() {
		return fmt.Sprintf("Error: start index %d is past the end of the log (%d samples, indices 0-%d).", start, lf.Len(), lf.Len()-1)
	}

	cols := splitList(channels)
	if len(cols) == 0 {
		cols = sortedKeys(lf.RecordAt(start).Values)
	}

	var b strings.Builder
	b.WriteString("index,t_s")
	for _, c := range cols {
		b.WriteString("," + c)
	}
	b.WriteByte('\n')

	t0 := lf.Start()
	rows := 0
	for i := start; i < lf.Len() && rows < count; i, rows = i+stride, rows+1 {
		rec := lf.RecordAt(i)
		fmt.Fprintf(&b, "%d,%.2f", i, rec.Time.Sub(t0).Seconds())
		for _, c := range cols {
			if v, ok := rec.Values[c]; ok {
				fmt.Fprintf(&b, ",%s", fmtVal(v, 2))
			} else {
				b.WriteString(",")
			}
		}
		b.WriteByte('\n')
	}
	if truncated {
		fmt.Fprintf(&b, "\n(capped at %d rows — use stride to cover more ground)\n", maxLogRows)
	}
	return b.String()
}

// --- helpers ---------------------------------------------------------------

// fmtVal trims trailing zeros so a table of integers doesn't spend context on
// ".000000".
func fmtVal(v float64, prec int) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', prec, 64)
}

// fmtScale prints a correction factor the way a tuner writes it: 0.01, not
// 1e-02, and 0.0009765625 in full rather than rounded to something that no
// longer divides evenly.
func fmtScale(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// argStr and argInt tolerate the model sending numbers as strings and vice
// versa, which happens often enough with small local models to be worth it.
func argStr(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case string:
		return v
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(v))
		for _, e := range v {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(v)
	}
}

func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}
