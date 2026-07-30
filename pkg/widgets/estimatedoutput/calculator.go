// Package estimatedoutput implements the "Estimated output" tool: an
// offline estimator that reads maps from the currently loaded binary and
// plots estimated WOT power/torque curves. The math is ported from
// T5Suite's "Show dyno graph" and T7/T8Suite's "Airmass result viewer".
package estimatedoutput

import (
	"fmt"
	"math"
	"strings"

	symbol "github.com/roffe/ecusymbol"
)

type OptionKind int

const (
	OptionBool OptionKind = iota
	OptionChoice
	OptionNumber
)

// Option describes one user-configurable input to a Calculator. Values are
// carried as float64: bools as 0/1, choices as the index into Choices.
type Option struct {
	Key      string
	Label    string
	Kind     OptionKind
	Default  float64
	Choices  []string
	Disabled bool // feature not present in the loaded binary
}

// Curve is one plotted series over the shared RPM axis.
type Curve struct {
	Name   string
	Values []float64
	Hidden bool // start hidden, toggleable in the legend
	Peak   bool // report "peak <value> @ rpm" in the summary line
}

type Result struct {
	RPM      []float64
	Curves   []Curve
	Limiters []string // optional, per RPM point: active WOT limiter ("" = none)
	Table    *Table   // optional heat-table view of the full map
}

// Table is a heat-table over the full pedal/throttle x RPM range.
// Row 0 is the highest pedal/throttle position (WOT at the top, like the
// suites' table view).
type Table struct {
	Title     string
	Cols      []float64   // column axis (RPM)
	Rows      []float64   // row axis display values (pedal %, throttle)
	Cells     [][]float64 // [row][col]
	Limiters  [][]string  // optional, same shape: active limiter per cell
	Precision int
}

// Calculator estimates output curves for one ECU family from its binary.
type Calculator interface {
	Options(fw symbol.SymbolCollection) []Option
	Calculate(fw symbol.SymbolCollection, opts map[string]float64) (*Result, error)
}

// For returns the Calculator for the given ECU name, or nil when the ECU
// has no estimator. New ECUs are added here.
func For(ecu string) Calculator {
	switch ecu {
	case "T5":
		return t5{}
	case "T7":
		return t7{}
	case "T8":
		return t8{}
	}
	return nil
}

func has(fw symbol.SymbolCollection, name string) bool {
	return fw.GetByName(name) != nil
}

// nonZero reports whether the symbol exists and holds any non-zero byte.
func nonZero(fw symbol.SymbolCollection, name string) bool {
	s := fw.GetByName(name)
	if s == nil {
		return false
	}
	for _, b := range s.Bytes() {
		if b != 0 {
			return true
		}
	}
	return false
}

// signed16 applies the suites' manual sign correction for tables read as
// unsigned 16-bit; harmless when the collection already decoded them signed.
func signed16(v []int) []int {
	for i, val := range v {
		if val > 32000 {
			v[i] = -(65536 - val)
		}
	}
	return v
}

// unsigned16 maps values back to the raw unsigned 16-bit view the suites
// operate on, for tables ecusymbol decodes as signed but the original code
// reads unsigned.
func unsigned16(v []int) []int {
	for i, val := range v {
		if val < 0 {
			v[i] = val + 65536
		}
	}
	return v
}

// cint mirrors C# Convert.ToInt32(double) / Math.Round: round half to even.
func cint(v float64) int { return int(math.RoundToEven(v)) }

// symLoader collects missing-symbol errors so the user gets one message
// naming everything the binary lacks.
type symLoader struct {
	fw      symbol.SymbolCollection
	missing []string
}

func (l *symLoader) ints(name string) []int {
	s := l.fw.GetByName(name)
	if s == nil {
		l.missing = append(l.missing, name)
		return nil
	}
	return s.Ints()
}

func (l *symLoader) first(name string) int {
	if v := l.ints(name); len(v) > 0 {
		return v[0]
	}
	return 0
}

func (l *symLoader) err() error {
	if len(l.missing) > 0 {
		return fmt.Errorf("missing symbols: %s", strings.Join(l.missing, ", "))
	}
	return nil
}
