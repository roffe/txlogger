package datalogger

import (
	"math"
	"strconv"

	symbol "github.com/roffe/ecusymbol"
)

// Channel is one ordered column in a log. It pairs a name with a way to read
// its current value and a way to format that value as text.
//
// Whether the value comes from a polled ECU symbol, an asynchronous broadcast
// frame, the wideband controller or anywhere else is captured in the read
// closure and decided once when logging starts. Writers therefore only ever
// see a flat, ordered list of named channels and never need to know about
// sysvars vs symbols or sync vs async values.
type Channel struct {
	Name string
	read func() float64
	// appendFmt formats the value into dst and returns the extended slice,
	// letting writers format straight into a reused buffer with no per-sample
	// string garbage.
	appendFmt func(dst []byte, v float64) []byte
}

// Value returns the current value of the channel.
func (c *Channel) Value() float64 { return c.read() }

// Append formats the current value as text into dst and returns the extended
// slice. Allocation-free when dst has spare capacity.
func (c *Channel) Append(dst []byte) []byte { return c.appendFmt(dst, c.read()) }

// String returns the current value formatted as text.
func (c *Channel) String() string { return string(c.appendFmt(nil, c.read())) }

// newSysvarChannel reads the latest value of a named entry in the shared sysvars
// map. Used for asynchronously updated values such as T7 broadcast frames, the
// wideband and AD scanner lambda and other derived values.
func newSysvarChannel(sysvars *ThreadSafeMap, name string) Channel {
	return Channel{
		Name:      name,
		read:      func() float64 { return sysvars.Get(name) },
		appendFmt: sysvarFormat(name),
	}
}

// newSymbolChannel reads the value decoded into the symbol on the most recent
// payload Read.
func newSymbolChannel(sym *symbol.Symbol) Channel {
	return Channel{
		Name:      sym.Name,
		read:      sym.Float64,
		appendFmt: symbolFormat(sym.Correctionfactor),
	}
}

func newFunctionChannel(name string, read func() float64) Channel {
	return Channel{
		Name:      name,
		read:      read,
		appendFmt: func(dst []byte, v float64) []byte { return strconv.AppendFloat(dst, v, 'f', 2, 64) },
	}
}

// sysvarFormat mirrors the precision rules the text writers used for sysvars:
// whole numbers print without decimals, the external wideband lambda prints
// with three decimals and everything else with two.
func sysvarFormat(name string) func(dst []byte, v float64) []byte {
	return func(dst []byte, v float64) []byte {
		prec := 2
		switch {
		case v == math.Trunc(v):
			prec = 0
		case name == EXTERNALWBLSYM:
			prec = 3
		}
		return strconv.AppendFloat(dst, v, 'f', prec, 64)
	}
}

// symbolFormat mirrors symbol.StringValue: the number of decimals is derived
// from the symbol correction factor.
func symbolFormat(correctionfactor float64) func(dst []byte, v float64) []byte {
	prec := 0
	switch correctionfactor {
	case 0.1:
		prec = 1
	case 0.01, 0.0078125, 0.0009765625, 0.00390625, 0.004:
		prec = 2
	case 0.001:
		prec = 3
	}
	return func(dst []byte, v float64) []byte {
		return strconv.AppendFloat(dst, v, 'f', prec, 64)
	}
}
