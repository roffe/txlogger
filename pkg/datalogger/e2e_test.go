package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// End-to-end tests: the real loggers run against the fake ECUs in pkg/ecusim
// through a real gocan bus. Each test ramps a value on the ECU while logging
// and checks the logged column follows it, plus a RAM write/read round trip
// through the logger's request channels and a clean shutdown.

type e2e struct {
	t        *testing.T
	ctx      context.Context
	cancel   context.CancelFunc
	cl       IClient
	filename string
	captures chan int
	done     chan error
	errs     int
}

func startE2E(t *testing.T, ecu string, dev gocan.Adapter, syms []*symbol.Symbol, fast bool) *e2e {
	t.Helper()
	e := &e2e{t: t, captures: make(chan int, 64), done: make(chan error, 1)}
	e.ctx, e.cancel = context.WithTimeout(context.Background(), 20*time.Second)
	cfg := Config{
		FilenamePrefix: "e2e",
		ECU:            ecu,
		Device:         dev,
		Symbols:        syms,
		Rate:           50,
		OnMessage:      func(s string) { t.Log(s) },
		CaptureCounter: func(n int) {
			select {
			case e.captures <- n:
			default:
			}
		},
		ErrorCounter:              func(n int) { e.errs = n },
		FpsCounter:                func(int) {},
		LogFormat:                 "CSV",
		LogPath:                   t.TempDir(),
		WidebandConfig:            WidebandConfig{Name: "None"},
		ExperimentalT5FastLogging: fast,
	}
	var err error
	e.cl, e.filename, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { e.done <- e.cl.Start(e.ctx) }()
	return e
}

// waitCaptures blocks until the logger has captured at least n frames.
func (e *e2e) waitCaptures(n int) {
	e.t.Helper()
	for {
		select {
		case c := <-e.captures:
			if c >= n {
				return
			}
		case err := <-e.done:
			e.t.Fatalf("logger stopped early: %v", err)
		case <-e.ctx.Done():
			e.t.Fatal("timed out waiting for captures")
		}
	}
}

// ramp calls set with 1, 2, 3... every 5ms until the run is stopped.
func (e *e2e) ramp(set func(i uint16)) {
	go func() {
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		for i := uint16(1); ; i++ {
			select {
			case <-e.ctx.Done():
				return
			case <-t.C:
				set(i)
			}
		}
	}()
}

// roundTrip writes data to addr through the logger and reads it back both
// from the ECU side and through the logger.
func (e *e2e) roundTrip(addr uint32, data []byte, peek func(uint32, int) []byte) {
	e.t.Helper()
	if err := e.cl.SetRAM(addr, data); err != nil {
		e.t.Fatal(err)
	}
	if got := peek(addr, len(data)); !bytes.Equal(got, data) {
		e.t.Fatalf("SetRAM landed as % X", got)
	}
	got, err := e.cl.GetRAM(addr, uint32(len(data)))
	if err != nil {
		e.t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		e.t.Fatalf("GetRAM returned % X", got)
	}
}

// stop closes the logger, checks it shut down cleanly with no errors and
// returns the CSV rows, header first.
func (e *e2e) stop() [][]string {
	e.t.Helper()
	e.cl.Close()
	select {
	case err := <-e.done:
		if err != nil {
			e.t.Fatalf("Start returned %v", err)
		}
	case <-time.After(5 * time.Second):
		e.t.Fatal("Start did not return after Close")
	}
	e.cancel()
	if e.errs != 0 {
		e.t.Fatalf("logger reported %d errors", e.errs)
	}
	f, err := os.Open(e.filename)
	if err != nil {
		e.t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		e.t.Fatal(err)
	}
	if len(rows) < 30 {
		e.t.Fatalf("only %d rows logged", len(rows))
	}
	return rows
}

// column returns the values of a named column over the data rows.
func column(t *testing.T, rows [][]string, name string) []string {
	t.Helper()
	i := slices.Index(rows[0], name)
	if i < 0 {
		t.Fatalf("column %q missing from header %v", name, rows[0])
	}
	out := make([]string, 0, len(rows)-1)
	for _, r := range rows[1:] {
		out = append(out, r[i])
	}
	return out
}

// assertMoving checks a column follows the ramp: never decreasing and with
// plenty of distinct samples, so a stuck or stale value cannot pass.
func assertMoving(t *testing.T, name string, vals []string) {
	t.Helper()
	distinct := 0
	prev := -1.0
	for i, s := range vals {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("%s row %d: %q is not a number", name, i, s)
		}
		if v < prev {
			t.Fatalf("%s row %d: %v after %v, value went backwards", name, i, v, prev)
		}
		if v != prev {
			distinct++
		}
		prev = v
	}
	if distinct < 10 {
		t.Fatalf("%s only took %d distinct values over %d rows: %v", name, distinct, len(vals), vals)
	}
}

func assertStatic(t *testing.T, vals []string, want string) {
	t.Helper()
	for i, v := range vals {
		if v != want {
			t.Fatalf("row %d: %q, want %q", i, v, want)
		}
	}
}

func be16(i uint16) []byte { return binary.BigEndian.AppendUint16(nil, i) }

func TestT7EndToEnd(t *testing.T) {
	sim := &ecusim.T7{Broadcast: 20 * time.Millisecond, Latency: time.Millisecond}
	sim.SetSymbol(100, []byte{0x12, 0x34})
	sim.SetSymbol(200, []byte{0, 0})
	syms := []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: 1, Length: 2, Correctionfactor: 1}, // served by broadcast, not polled
		{Name: "MAF.m_AirInlet", Number: 100, Length: 2, Correctionfactor: 1},
		{Name: "Out.PWM_BoostCntrl", Number: 200, Length: 2, Correctionfactor: 1},
	}

	e := startE2E(t, "T7", sim, syms, false)
	e.ramp(func(i uint16) { sim.SetSymbol(200, be16(i)) })
	e.waitCaptures(15)
	e.roundTrip(0xF00100, []byte{1, 2, 3, 4, 5}, sim.Peek)
	e.waitCaptures(45)
	rows := e.stop()

	assertMoving(t, "Out.PWM_BoostCntrl", column(t, rows, "Out.PWM_BoostCntrl"))
	assertStatic(t, column(t, rows, "MAF.m_AirInlet"), "4660")
	assertStatic(t, column(t, rows, "ActualIn.n_Engine"), strconv.Itoa(ecusim.BroadcastRPM))
	assertStatic(t, column(t, rows, "In.v_Vehicle"), strconv.Itoa(ecusim.BroadcastSpeed/10))
	assertStatic(t, column(t, rows, "Out.X_ActualGear"), strconv.Itoa(ecusim.BroadcastGear))
}

func TestT8EndToEnd(t *testing.T) {
	sim := &ecusim.T8{Latency: time.Millisecond}
	sim.SetSymbol(100, []byte{0, 0})
	sim.SetSymbol(200, []byte{0x12, 0x34})
	syms := []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: 100, Length: 2, Correctionfactor: 1},
		{Name: "MAF.m_AirInlet", Number: 200, Length: 2, Correctionfactor: 1},
	}

	e := startE2E(t, "T8", sim, syms, false)
	e.ramp(func(i uint16) { sim.SetSymbol(100, be16(i)) })
	e.waitCaptures(15)
	e.roundTrip(0x100100, []byte{1, 2, 3, 4, 5}, sim.Peek) // T8 GetRAM only accepts SRAM addresses
	e.waitCaptures(45)
	rows := e.stop()

	assertMoving(t, "ActualIn.n_Engine", column(t, rows, "ActualIn.n_Engine"))
	assertStatic(t, column(t, rows, "MAF.m_AirInlet"), "4660")
}

func TestT5EndToEnd(t *testing.T) {
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "classic", true: "gather"}[fast], func(t *testing.T) {
			sim := &ecusim.T5{Latency: time.Millisecond}
			sim.Poke(0x1010, []byte{123}) // Batt_volt, 0.1 V units
			syms := []*symbol.Symbol{
				{Name: "Rpm", SramOffset: 0x1000, Length: 2},
				{Name: "Batt_volt", SramOffset: 0x1010, Length: 1},
			}

			e := startE2E(t, "T5", sim, syms, fast)
			e.ramp(func(i uint16) { sim.Poke(0x1000, be16(i)) })
			e.waitCaptures(15)
			e.roundTrip(0x2000, []byte{1, 2, 3, 4, 5}, sim.Peek)
			e.waitCaptures(45)
			rows := e.stop()

			assertMoving(t, "Rpm", column(t, rows, "Rpm"))
			assertStatic(t, column(t, rows, "Batt_volt"), "12.30")
		})
	}
}
