package presets

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	symbol "github.com/roffe/ecusymbol"
)

// isolate gives the test an empty Map and restores the package's Map when the
// test ends, since Map is shared package state.
func isolate(t *testing.T) {
	t.Helper()
	saved := Map
	Map = map[string]string{}
	t.Cleanup(func() { Map = saved })
}

// Each of the four built-in presets is valid JSON that decodes, through Get,
// into a non-empty list of complete symbols (name, length and correction
// factor set) with no symbol listed twice.
func TestDefaultsDecode(t *testing.T) {
	isolate(t)
	maps.Copy(Map, defaults)

	want := []string{"T5 Dash", "T7 Dash", "T7 Dash+Boost", "T8 Dash"}
	if got := slices.Sorted(maps.Keys(defaults)); !slices.Equal(got, want) {
		t.Fatalf("defaults = %v, want %v", got, want)
	}
	for _, name := range want {
		syms, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if len(syms) == 0 {
			t.Fatalf("%s has no symbols", name)
		}
		seen := map[string]bool{}
		for _, s := range syms {
			if s.Name == "" || s.Length == 0 || s.Correctionfactor == 0 {
				t.Errorf("%s: incomplete symbol %+v", name, s)
			}
			if seen[s.Name] {
				t.Errorf("%s lists %s twice", name, s.Name)
			}
			seen[s.Name] = true
		}
	}
}

// Names lists every preset in Map in sorted order.
func TestNamesSorted(t *testing.T) {
	isolate(t)
	Map["b"], Map["A"], Map["a"] = "[]", "[]", "[]"
	if got := strings.Join(Names(), ","); got != "A,a,b" {
		t.Errorf("Names = %s, want A,a,b", got)
	}
}

// IsSystem is true only for the built-in presets, whether or not they are in
// Map at the moment.
func TestIsSystem(t *testing.T) {
	isolate(t)
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"T5 Dash", true},
		{"T7 Dash", true},
		{"T7 Dash+Boost", true},
		{"T8 Dash", true},
		{"t7 dash", false},
		{"My preset", false},
		{"", false},
	} {
		if got := IsSystem(tc.name); got != tc.want {
			t.Errorf("IsSystem(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Set stores the symbols as JSON under the name and Get decodes them back;
// a later Set replaces the earlier one.
func TestSetGetRoundTrip(t *testing.T) {
	isolate(t)
	in := []*symbol.Symbol{
		{Name: "In.v_Vehicle", Number: 3408, Address: 0xF0EA12, Length: 2, Type: 0x21, Correctionfactor: 0.1, Unit: "Km/h"},
		{Name: "ECMStat.ST_ActiveAirDem", Number: 3753, Address: 0xF0EF8A, Length: 1, Type: 0x24, Correctionfactor: 1},
	}
	if err := Set("mine", in); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !strings.Contains(Map["mine"], `"Name":"In.v_Vehicle"`) {
		t.Errorf("stored JSON = %s", Map["mine"])
	}
	out, err := Get("mine")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("Get returned %d symbols, want %d", len(out), len(in))
	}
	for i := range in {
		if !reflect.DeepEqual(out[i], in[i]) {
			t.Errorf("symbol %d = %+v, want %+v", i, *out[i], *in[i])
		}
	}

	if err := Set("mine", in[:1]); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	if out, _ := Get("mine"); len(out) != 1 {
		t.Errorf("after replace Get returned %d symbols, want 1", len(out))
	}
}

// Set and Delete refuse built-in presets and leave Map untouched.
func TestSystemPresetsAreReadOnly(t *testing.T) {
	isolate(t)
	maps.Copy(Map, defaults)
	before := Map["T7 Dash"]

	if err := Set("T7 Dash", nil); err == nil {
		t.Error("Set on a system preset succeeded")
	}
	if err := Delete("T7 Dash"); err == nil {
		t.Error("Delete on a system preset succeeded")
	}
	if Map["T7 Dash"] != before {
		t.Error("system preset was modified")
	}
}

// Delete removes a user preset, after which Get reports it as not found;
// deleting a name that does not exist is not an error.
func TestDelete(t *testing.T) {
	isolate(t)
	Map["mine"] = "[]"
	if err := Delete("mine"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := Map["mine"]; ok {
		t.Error("preset still in Map")
	}
	if _, err := Get("mine"); err == nil || err.Error() != "preset not found" {
		t.Errorf("Get after Delete = %v, want preset not found", err)
	}
	if err := Delete("never existed"); err != nil {
		t.Errorf("Delete of a missing preset = %v", err)
	}
}

// Get returns the JSON decode error for a preset whose stored data is corrupt.
func TestGetCorrupt(t *testing.T) {
	isolate(t)
	Map["broken"] = `[{"Name":`
	if syms, err := Get("broken"); err == nil {
		t.Errorf("Get = %v, nil; want a decode error", syms)
	}
}

// Save stores Map as JSON in the "presets" preference, and Load restores the
// user presets from it while putting back the built-in presets on top of any
// stored copy of them.
func TestSaveLoad(t *testing.T) {
	isolate(t)
	app := test.NewTempApp(t)

	Map["mine"] = `[{"Name":"Rpm"}]`
	Map["T5 Dash"] = `[]` // a stale or tampered copy of a system preset
	if err := Save(app); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var stored map[string]string
	if err := json.Unmarshal([]byte(app.Preferences().String("presets")), &stored); err != nil {
		t.Fatalf("stored preference is not JSON: %v", err)
	}
	if stored["mine"] != Map["mine"] {
		t.Errorf("stored mine = %q", stored["mine"])
	}

	Map = map[string]string{}
	if err := Load(app); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if Map["mine"] != `[{"Name":"Rpm"}]` {
		t.Errorf("user preset after Load = %q", Map["mine"])
	}
	for name, data := range defaults {
		if Map[name] != data {
			t.Errorf("system preset %q not restored by Load", name)
		}
	}
	if got := len(Names()); got != len(defaults)+1 {
		t.Errorf("Names has %d entries, want %d", got, len(defaults)+1)
	}
}

// Load with no stored preference installs just the built-in presets, and a
// corrupt preference is reported as an error.
func TestLoadEmptyAndCorrupt(t *testing.T) {
	isolate(t)
	app := test.NewTempApp(t)

	if err := Load(app); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !maps.Equal(Map, defaults) {
		t.Errorf("Map after Load = %v, want the defaults", slices.Sorted(maps.Keys(Map)))
	}

	app.Preferences().SetString("presets", "{not json")
	if err := Load(app); err == nil {
		t.Error("Load of a corrupt preference succeeded")
	}
}
