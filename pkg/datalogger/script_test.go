package datalogger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
