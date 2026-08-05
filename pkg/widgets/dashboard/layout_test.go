package dashboard

import (
	"encoding/json"
	"testing"
)

func TestDefaultLayoutSane(t *testing.T) {
	l := defaultLayout()
	before := len(l.Items)
	l.sanitize()
	if len(l.Items) != before {
		t.Fatalf("sanitize dropped %d default items", before-len(l.Items))
	}
	for _, it := range l.Items {
		if itemDefByID(it.ID) == nil {
			t.Errorf("default item %q has no itemDef", it.ID)
		}
		if it.X+it.W > l.Cols || it.Y+it.H > l.Rows {
			t.Errorf("default item %q out of grid: %+v", it.ID, it)
		}
	}
}

func TestSanitize(t *testing.T) {
	l := &Layout{
		Items: []Item{
			{ID: "rpm", X: -5, Y: 100, W: 900, H: 0},
			{ID: "rpm", X: 1, Y: 1, W: 2, H: 2},     // duplicate
			{ID: "bogus", X: 1, Y: 1, W: 2, H: 2},   // unknown id
			{ID: "speed", X: 30, Y: 16, W: 4, H: 4}, // partially outside
		},
	}
	l.sanitize()
	if l.Cols != defaultGridCols || l.Rows != defaultGridRows {
		t.Fatalf("grid defaults not applied: %d x %d", l.Cols, l.Rows)
	}
	if len(l.Items) != 2 {
		t.Fatalf("want 2 items after sanitize, got %d: %+v", len(l.Items), l.Items)
	}
	for _, it := range l.Items {
		if it.W < 1 || it.H < 1 || it.X < 0 || it.Y < 0 || it.X+it.W > l.Cols || it.Y+it.H > l.Rows {
			t.Errorf("item %q not clamped into grid: %+v", it.ID, it)
		}
	}
}

func TestSanitizeRejectsAbsurdGrid(t *testing.T) {
	l := &Layout{Cols: 1000000, Rows: 1000000, Items: []Item{{ID: "rpm", W: 1, H: 1}}}
	l.sanitize()
	if l.Cols != defaultGridCols || l.Rows != defaultGridRows {
		t.Fatalf("absurd grid not reset: %d x %d", l.Cols, l.Rows)
	}
}

func TestPresetRoundTripWithAwkwardName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // common.GetDashboardPath resolves under $HOME

	l := defaultLayout()
	const name = "Erik's Dash (v2)"
	if err := l.savePreset(name); err != nil {
		t.Fatal(err)
	}
	names := listPresets()
	if len(names) != 1 {
		t.Fatalf("want 1 preset listed, got %v", names)
	}
	// The dropdown offers exactly what listPresets returned, so that string
	// must load even though savePreset sanitized the filename.
	got, err := loadPreset(names[0])
	if err != nil {
		t.Fatalf("loading listed preset %q: %v", names[0], err)
	}
	if len(got.Items) != len(l.Items) {
		t.Errorf("item count %d != %d", len(got.Items), len(l.Items))
	}
}

func TestLayoutJSONRoundTrip(t *testing.T) {
	l := defaultLayout()
	l.item("wblambda").Style = StyleGauge
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var got Layout
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != len(l.Items) {
		t.Fatalf("item count changed: %d != %d", len(got.Items), len(l.Items))
	}
	if got.item("wblambda").Style != StyleGauge {
		t.Error("wblambda style lost in round trip")
	}
}
