package ecu

import (
	"bytes"
	"errors"
	"log"
	"slices"
	"strings"
	"testing"

	"github.com/roffe/gocan/v2"
)

// captureLog sends the standard logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})
	return &buf
}

// LoadConfig(nil) builds an "Unknown ECU" config whose callbacks log what
// they are given.
func TestLoadConfigNil(t *testing.T) {
	buf := captureLog(t)
	cfg := LoadConfig(nil)
	if cfg.Name != "Unknown ECU" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.SeedKey != nil {
		t.Errorf("SeedKey = %+v, want nil", cfg.SeedKey)
	}
	cfg.OnProgress(0.25)
	cfg.OnError(errors.New("boom"))
	cfg.OnMessage("hello")
	if got, want := buf.String(), "0.25\nboom\nhello\n"; got != want {
		t.Errorf("default callbacks logged %q, want %q", got, want)
	}
}

// LoadConfig fills in only the callbacks the caller left nil, keeps the rest
// of the config and returns the same pointer.
func TestLoadConfigKeepsCallerFields(t *testing.T) {
	buf := captureLog(t)
	var progress []float64
	sk := &SeedKey{XOR: 1, Sub: 2}
	in := &Config{Name: "Trionic 7", SeedKey: sk, OnProgress: func(f float64) { progress = append(progress, f) }}
	cfg := LoadConfig(in)
	if cfg != in {
		t.Fatal("LoadConfig returned a different config")
	}
	if cfg.Name != "Trionic 7" || cfg.SeedKey != sk {
		t.Errorf("fields changed: %+v", cfg)
	}
	cfg.OnProgress(1)
	if !slices.Equal(progress, []float64{1}) {
		t.Errorf("caller OnProgress not kept: %v", progress)
	}
	if buf.Len() != 0 {
		t.Errorf("caller OnProgress replaced by the logging default: %q", buf)
	}
	if cfg.OnError == nil || cfg.OnMessage == nil {
		t.Fatal("nil callbacks not filled in")
	}
	cfg.OnMessage("filled")
	if !strings.Contains(buf.String(), "filled") {
		t.Errorf("OnMessage default did not log: %q", buf)
	}
}

// fakeClient is a Client that only remembers how it was built.
type fakeClient struct {
	Client
	bus *gocan.Bus
	cfg *Config
}

// register adds e to the flasher registry for the rest of the test.
func register(t *testing.T, e *EcuInfo) {
	t.Helper()
	Register(e)
	t.Cleanup(func() { delete(ecuMap, e.Name) })
}

// The flasher registry: New builds the named client with the bus and config
// it was given, Info returns the registration (zero when unknown) and List
// returns every name sorted.
func TestRegistry(t *testing.T) {
	newFake := func(c *gocan.Bus, cfg *Config) Client { return &fakeClient{bus: c, cfg: cfg} }
	register(t, &EcuInfo{Name: "test ECU b", NewFunc: newFake, CANRate: 500, Filter: []uint32{0x7E8}, ManualReset: true})
	register(t, &EcuInfo{Name: "test ECU a", NewFunc: newFake, CANRate: 615.384})

	if got := List(); !slices.Equal(got, []string{"test ECU a", "test ECU b"}) {
		t.Errorf("List() = %v", got)
	}

	bus := &gocan.Bus{}
	cfg := &Config{Name: "test ECU b"}
	cl, err := New(bus, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if fc, ok := cl.(*fakeClient); !ok || fc.bus != bus || fc.cfg != cfg {
		t.Errorf("New built %#v", cl)
	}

	if _, err := New(bus, &Config{Name: "nope"}); err == nil || err.Error() != "unknown ECU" {
		t.Errorf("New(unknown) error = %v", err)
	}

	info := Info("test ECU b")
	if info.Name != "test ECU b" || info.CANRate != 500 || !slices.Equal(info.Filter, []uint32{0x7E8}) || !info.ManualReset {
		t.Errorf("Info = %+v", info)
	}
	if info := Info("nope"); info.Name != "" || info.NewFunc != nil || info.ManualReset {
		t.Errorf("Info(unknown) = %+v, want zero", info)
	}
}

// Registering a name twice panics instead of silently replacing the first
// flasher.
func TestRegisterDuplicatePanics(t *testing.T) {
	first := &EcuInfo{Name: "test ECU dup"}
	register(t, first)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("second Register did not panic")
		}
		if msg, _ := r.(string); msg != "ECU already registered: test ECU dup" {
			t.Errorf("panic = %v", r)
		}
		if ecuMap["test ECU dup"] != first {
			t.Error("first registration was replaced")
		}
	}()
	Register(&EcuInfo{Name: "test ECU dup"})
}
