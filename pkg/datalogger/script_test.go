package datalogger

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
)

type nopWriter struct{}

func (nopWriter) Write(time.Time, []Channel) error { return nil }
func (nopWriter) Close() error                     { return nil }

const rocScript = `
inputs  = { "Rpm" }
outputs = { "RPM.RoC" }

local prev_rpm, prev_t
function update(v, t)
  local rpm = v["Rpm"]
  if prev_t and t > prev_t then
    out["RPM.RoC"] = (rpm - prev_rpm) * 1000 / (t - prev_t)
  end
  prev_rpm, prev_t = rpm, t
end
`

func TestScriptWriterRoC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roc.lua")
	if err := os.WriteFile(path, []byte(rocScript), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadScript(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	sysvars := NewThreadSafeMap()
	sw := &scriptWriter{lw: nopWriter{}, sysvars: sysvars, onMessage: func(m string) { t.Log(m) }, scripts: []*luaScript{s}}
	defer sw.Close()

	rpm := 2000.0
	channels := []Channel{newFunctionChannel("Rpm", func() float64 { return rpm })}

	t0 := time.UnixMilli(1_000_000)
	if err := sw.Write(t0, channels); err != nil {
		t.Fatal(err)
	}
	rpm = 2500
	if err := sw.Write(t0.Add(50*time.Millisecond), channels); err != nil {
		t.Fatal(err)
	}
	if s.disabled {
		t.Fatal("script was disabled")
	}
	// 500 rpm gained in 50 ms = 10000 rpm/s
	if got := sysvars.Get("RPM.RoC"); got != 10000 {
		t.Fatalf("RPM.RoC = %v, want 10000", got)
	}
}

func writeScript(t *testing.T, dir, name, src string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// loadScript rejects scripts that do not run, lack a non-empty outputs table
// or an update function, or declare an output named like a logged symbol.
// The inputs table is optional.
func TestLoadScriptValidation(t *testing.T) {
	syms := []*symbol.Symbol{{Name: "Rpm"}}
	tests := []struct {
		name, src, wantErr string
	}{
		{"syntax error", `outputs = {`, "syntax error"},
		{"missing outputs", `function update(v, t) end`, `missing "outputs" table`},
		{"empty outputs", "outputs = {}\nfunction update(v, t) end", `"outputs" table is empty`},
		{"output collides with symbol", "outputs = {\"Rpm\"}\nfunction update(v, t) end", `output "Rpm" collides with a logged symbol`},
		{"missing update", `outputs = {"X"}`, "missing function update(v, t)"},
		{"update not a function", "outputs = {\"X\"}\nupdate = 5", "missing function update(v, t)"},
		{"runtime error at load", `error("nope")`, "nope"},
		{"no inputs is fine", "outputs = {\"X\"}\nfunction update(v, t) out[\"X\"] = 1 end", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeScript(t, t.TempDir(), "s.lua", tt.src)
			s, err := loadScript(path, syms)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				defer s.L.Close()
				if s.name != "s.lua" || len(s.inputs) != 0 || !slices.Equal(s.outputs, []string{"X"}) {
					t.Fatalf("script %+v", s)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// newScriptWriter loads every .lua file in ~/txlogger/scripts/<ECU>, reports
// each load and failure, and returns nil when no script loaded.
func TestNewScriptWriterLoadsECUScripts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, "txlogger", "scripts", "T7")
	writeScript(t, dir, "roc.lua", rocScript)
	writeScript(t, dir, "broken.lua", `outputs = {`)
	writeScript(t, dir, "notes.txt", `not a script`)
	writeScript(t, filepath.Join(home, "txlogger", "scripts", "T8"), "broken.lua", `outputs = {`)

	msgs := newMsgLog(t)
	cfg := Config{ECU: "T7", OnMessage: msgs.add}
	sw := newScriptWriter(cfg, nopWriter{}, NewThreadSafeMap())
	if sw == nil {
		t.Fatal("no script writer")
	}
	defer sw.Close()
	if len(sw.scripts) != 1 || sw.scripts[0].name != "roc.lua" {
		t.Fatalf("scripts %+v", sw.scripts)
	}
	if !msgs.has("script roc.lua loaded: RPM.RoC") || !msgs.has("script broken.lua:") {
		t.Fatalf("messages %v", msgs.all())
	}
	if got := sw.outputNames(); !slices.Equal(got, []string{"RPM.RoC"}) {
		t.Fatalf("outputNames %v", got)
	}

	if sw := newScriptWriter(Config{ECU: "T8", OnMessage: msgs.add}, nopWriter{}, NewThreadSafeMap()); sw != nil {
		t.Fatal("writer returned although every script failed")
	}
	if sw := newScriptWriter(Config{ECU: "T5", OnMessage: msgs.add}, nopWriter{}, NewThreadSafeMap()); sw != nil {
		t.Fatal("writer returned for an ECU without scripts")
	}
}

// A logger whose ECU has scripts wraps its writer, and the script outputs
// become sysvar columns of the log.
func TestBaseLoggerAddsScriptColumns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeScript(t, filepath.Join(home, "txlogger", "scripts", "T8"), "roc.lua", rocScript)

	bl := NewBaseLogger(Config{ECU: "T8", OnMessage: func(string) {}, Symbols: []*symbol.Symbol{{Name: "Rpm", Number: 1, Length: 2}}}, nopWriter{})
	defer bl.secondTicker.Stop()
	sw, ok := bl.lw.(*scriptWriter)
	if !ok {
		t.Fatalf("writer %T is not wrapped", bl.lw)
	}
	defer sw.Close()
	var names []string
	for _, ch := range bl.buildChannels() {
		names = append(names, ch.Name)
	}
	if !slices.Equal(names, []string{"RPM.RoC", "Rpm"}) {
		t.Fatalf("channels %v", names)
	}
}

const twoOutputScript = `
inputs  = { "Rpm" }
outputs = { "Twice", "Label" }
function update(v, t)
  out["Twice"] = v["Rpm"] * 2
  out["Label"] = "not a number"
end
`

// Script outputs are computed before the row is written, so a sysvar column
// for an output carries the value computed from that same row; they are also
// published on the ebus. Non-numeric outputs are ignored.
func TestScriptWriterOutputsLandInSameRow(t *testing.T) {
	s, err := loadScript(writeScript(t, t.TempDir(), "two.lua", twoOutputScript), nil)
	if err != nil {
		t.Fatal(err)
	}
	sysvars := NewThreadSafeMap()
	rec := newRecWriter()
	sw := &scriptWriter{lw: rec, sysvars: sysvars, onMessage: func(string) {}, scripts: []*luaScript{s}}
	defer sw.Close()
	pub := collectEbus(t, "Twice")

	rpm := 1000.0
	channels := []Channel{
		newFunctionChannel("Rpm", func() float64 { return rpm }),
		newSysvarChannel(sysvars, "Twice"),
		newSysvarChannel(sysvars, "Label"),
	}
	for _, r := range []float64{1000, 1500} {
		rpm = r
		if err := sw.Write(time.UnixMilli(0), channels); err != nil {
			t.Fatal(err)
		}
	}
	rows := rec.snapshot()
	if len(rows) != 2 || rows[0].vals["Twice"] != 2000 || rows[1].vals["Twice"] != 3000 {
		t.Fatalf("rows %+v", rows)
	}
	if sysvars.Exists("Label") {
		t.Fatal("non-numeric output stored")
	}
	if got := pub.all(); !slices.Equal(got, []float64{2000, 3000}) {
		t.Fatalf("published %v", got)
	}
}

// A script whose input is not a log channel, or whose update raises an
// error, is disabled for the session with a message; the row is still
// written and other scripts keep running.
func TestScriptWriterDisablesFailingScripts(t *testing.T) {
	dir := t.TempDir()
	missing, err := loadScript(writeScript(t, dir, "missing.lua", "inputs = {\"Boost\"}\noutputs = {\"A\"}\nfunction update(v, t) out[\"A\"] = 1 end"), nil)
	if err != nil {
		t.Fatal(err)
	}
	failing, err := loadScript(writeScript(t, dir, "failing.lua", "outputs = {\"B\"}\nfunction update(v, t) error(\"kaboom\") end"), nil)
	if err != nil {
		t.Fatal(err)
	}
	good, err := loadScript(writeScript(t, dir, "good.lua", "inputs = {\"Rpm\"}\noutputs = {\"C\"}\nn = 0\nfunction update(v, t) n = n + 1; out[\"C\"] = n end"), nil)
	if err != nil {
		t.Fatal(err)
	}
	msgs := newMsgLog(t)
	sysvars := NewThreadSafeMap()
	rec := newRecWriter()
	sw := &scriptWriter{lw: rec, sysvars: sysvars, onMessage: msgs.add, scripts: []*luaScript{missing, failing, good}}

	channels := []Channel{newFunctionChannel("Rpm", func() float64 { return 900 })}
	for range 3 {
		if err := sw.Write(time.UnixMilli(0), channels); err != nil {
			t.Fatal(err)
		}
	}
	if !missing.disabled || !failing.disabled || good.disabled {
		t.Fatalf("disabled: missing %v failing %v good %v", missing.disabled, failing.disabled, good.disabled)
	}
	if !msgs.has(`script missing.lua disabled: input "Boost" is not logged`) || !msgs.has("script failing.lua disabled:") {
		t.Fatalf("messages %v", msgs.all())
	}
	if n := len(msgs.all()); n != 2 {
		t.Fatalf("%d messages, want one per disabled script: %v", n, msgs.all())
	}
	if sysvars.Exists("A") || sysvars.Exists("B") || sysvars.Get("C") != 3 {
		t.Fatalf("A %v B %v C %v", sysvars.Exists("A"), sysvars.Exists("B"), sysvars.Get("C"))
	}
	if len(rec.snapshot()) != 3 {
		t.Fatalf("%d rows written, want 3", len(rec.snapshot()))
	}
	if err := sw.Close(); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, rec.closed, time.Second, "inner writer close")
}

// Write passes on the wrapped writer's error.
func TestScriptWriterPassesWriteError(t *testing.T) {
	rec := newRecWriter()
	rec.err = errors.New("disk full")
	sw := &scriptWriter{lw: rec, sysvars: NewThreadSafeMap(), onMessage: func(string) {}}
	if err := sw.Write(time.Now(), nil); !errors.Is(err, rec.err) {
		t.Fatalf("Write = %v", err)
	}
}
