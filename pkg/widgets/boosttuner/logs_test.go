package boosttuner

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The tuner renders as its four tabs, in workflow order.
func TestNew_RendersTabs(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})

	objs := test.WidgetRenderer(bt).Objects()
	if len(objs) != 1 || objs[0] != fyne.CanvasObject(bt.tabs) {
		t.Fatalf("renderer objects = %v, want just the tabs", objs)
	}
	var names []string
	for _, it := range bt.tabs.Items {
		names = append(names, it.Text)
	}
	if want := []string{"Logs", "RegMap", "PID maps", "Simulator"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tabs = %v, want %v", names, want)
	}
	if bt.onTarget != 30 || bt.rpmStab != 150 || bt.loadStab != 50 || bt.minSamples != 5 || bt.blend != 1 {
		t.Fatalf("default filter = %v/%v/%v/%v/%v", bt.onTarget, bt.rpmStab, bt.loadStab, bt.minSamples, bt.blend)
	}
}

// parseLog returns every column except Pgm_status, row-aligned with NaN where a
// row has no value, plus the record count.
func TestParseLog_CSV(t *testing.T) {
	data := csvLog([]string{"A", "Pgm_status", "B"},
		[]float64{1, 7, 10},
		[]float64{2, 7, math.NaN()},
		[]float64{3, 7, 30},
	)
	local, n, err := parseLog("run.csv", strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("n = %d, want 3", n)
	}
	if _, ok := local["Pgm_status"]; ok {
		t.Error("Pgm_status should be dropped")
	}
	if got := local["A"]; !reflect.DeepEqual(got, []float64{1, 2, 3}) {
		t.Errorf("A = %v", got)
	}
	b := local["B"]
	if len(b) != 3 || b[0] != 10 || !math.IsNaN(b[1]) || b[2] != 30 {
		t.Errorf("B = %v, want [10 NaN 30]", b)
	}
}

// parseLog fails on unsupported extensions, unreadable logs and logs without
// any records.
func TestParseLog_Errors(t *testing.T) {
	// A BPL header with one column and no records parses but has zero rows.
	var bpl bytes.Buffer
	bpl.WriteString("BPL")
	bpl.WriteByte(1)
	_ = binary.Write(&bpl, binary.LittleEndian, uint16(1))
	_ = binary.Write(&bpl, binary.LittleEndian, uint16(1))
	bpl.WriteString("A")

	cases := []struct {
		name, data, want string
	}{
		{"log.txt", "whatever", "Unsupported filetype"},
		{"header.csv", "Time,A\n", "no valid records"},
		{"empty.bpl", bpl.String(), "empty.bpl contains no records"},
	}
	for _, c := range cases {
		_, _, err := parseLog(c.name, strings.NewReader(c.data))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want containing %q", c.name, err, c.want)
		}
	}
}

// mergeLog keeps every series row-aligned across files: series missing from a
// file are NaN-padded for its rows, new series are NaN-padded for earlier rows.
func TestMergeLog_RowAlignment(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{})
	bt.mergeLog("a.csv", map[string][]float64{"a": {1, 2}, "b": {10, 20}}, 2)
	bt.mergeLog("b.csv", map[string][]float64{"b": {30, 40, 50}, "c": {7, 8, 9}}, 3)

	if bt.nrecords != 5 {
		t.Fatalf("nrecords = %d, want 5", bt.nrecords)
	}
	if !reflect.DeepEqual(bt.loadedFiles, []string{"a.csv", "b.csv"}) {
		t.Fatalf("loadedFiles = %v", bt.loadedFiles)
	}
	nan := math.NaN()
	want := map[string][]float64{
		"a": {1, 2, nan, nan, nan},
		"b": {10, 20, 30, 40, 50},
		"c": {nan, nan, 7, 8, 9},
	}
	for k, w := range want {
		got := bt.values[k]
		if len(got) != len(w) {
			t.Fatalf("%s = %v, want %v", k, got, w)
		}
		for i := range w {
			if math.IsNaN(w[i]) != math.IsNaN(got[i]) || (!math.IsNaN(w[i]) && w[i] != got[i]) {
				t.Fatalf("%s = %v, want %v", k, got, w)
			}
		}
	}
}

// rebuildOrder sorts series names case-insensitively.
func TestRebuildOrder_CaseInsensitive(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{})
	bt.values = map[string][]float64{"b": nil, "A": nil, "c": nil, "B2": nil}
	bt.rebuildOrder()
	if want := []string{"A", "b", "B2", "c"}; !reflect.DeepEqual(bt.order, want) {
		t.Fatalf("order = %v, want %v", bt.order, want)
	}
}

// resolveChannels picks the first candidate present for each channel and
// missingChannels reports, in checklist order, the ones with none present.
func TestResolveChannels(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{})
	bt.values = map[string][]float64{
		"ActualIn.n_Engine":     {1},
		"m_Request":             {2},
		"Boost.SetValue":        {3}, // preferred over m_Request
		"MAF.m_AirInlet":        {4}, // fallback airmass name
		"BoostProt.RegConValue": {5},
	}
	bt.resolveChannels()

	want := map[string]string{
		"rpm":      "ActualIn.n_Engine",
		"setValue": "Boost.SetValue",
		"airInlet": "MAF.m_AirInlet",
		"regCon":   "BoostProt.RegConValue",
	}
	if !reflect.DeepEqual(bt.resolved, want) {
		t.Fatalf("resolved = %v, want %v", bt.resolved, want)
	}
	if v, ok := bt.series("setValue"); !ok || v[0] != 3 {
		t.Fatalf("series(setValue) = %v, %v", v, ok)
	}
	if v, ok := bt.series("pFac"); ok || v != nil {
		t.Fatalf("series(pFac) = %v, %v; want nil, false", v, ok)
	}
	wantMissing := []string{"P part", "I part", "D part", "Adaption offset",
		"Load error (SetValue - airmass)", "Total calculated duty", "Open-loop/2% flag"}
	if got := bt.missingChannels(); !reflect.DeepEqual(got, wantMissing) {
		t.Fatalf("missing = %v, want %v", got, wantMissing)
	}

	// The airmass-master name is the last fallback for the set value.
	bt.values = map[string][]float64{"AirMassMast.m_Request": {1}}
	bt.resolveChannels()
	if got := bt.resolved["setValue"]; got != "AirMassMast.m_Request" {
		t.Fatalf("setValue resolved to %q", got)
	}
}

// The checklist shows one row per required channel: a confirm icon and the
// series found, or a cancel icon and the candidate names.
func TestRefreshChannelList(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{})
	loadLog(t, bt, "rpm.csv", csvLog([]string{"ActualIn.n_Engine"}, []float64{900}))

	if got := len(bt.channelList.Objects); got != len(requiredChannels) {
		t.Fatalf("rows = %d, want %d", got, len(requiredChannels))
	}
	for i, ch := range requiredChannels {
		row := bt.channelList.Objects[i]
		icons := findAll[*widget.Icon](row, nil)
		texts := labelTexts(row)
		if len(icons) != 1 {
			t.Fatalf("row %d has %d icons", i, len(icons))
		}
		wantIcon, wantDetail := theme.CancelIcon().Name(), "missing: "+strings.Join(ch.candidates, " / ")
		if ch.key == "rpm" {
			wantIcon, wantDetail = theme.ConfirmIcon().Name(), "ActualIn.n_Engine"
		}
		if icons[0].Resource.Name() != wantIcon {
			t.Errorf("row %s icon = %s, want %s", ch.key, icons[0].Resource.Name(), wantIcon)
		}
		if !reflect.DeepEqual(texts, []string{ch.desc, wantDetail}) {
			t.Errorf("row %s labels = %q, want [%q %q]", ch.key, texts, ch.desc, wantDetail)
		}
	}
}

// The log list names each loaded file by base name with the total record count,
// logStatus appends a message, and Clear forgets every log.
func TestLogList_AndClear(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{})
	if bt.logsLabel.Text != "No log files loaded" {
		t.Fatalf("initial label = %q", bt.logsLabel.Text)
	}
	loadLog(t, bt, "/logs/run1.csv", boostLog(repeat(sample{rpm: 3000}, 2)...))
	loadLog(t, bt, "run2.csv", boostLog(repeat(sample{rpm: 3000}, 3)...))

	want := "2 file(s), 5 records:\nrun1.csv\nrun2.csv"
	if bt.logsLabel.Text != want {
		t.Fatalf("label = %q, want %q", bt.logsLabel.Text, want)
	}
	if len(bt.missingChannels()) != 0 {
		t.Fatalf("missing = %v, want none", bt.missingChannels())
	}

	bt.logStatus("Loaded 2 file(s), 1 failed")
	if bt.logsLabel.Text != want+"\nLoaded 2 file(s), 1 failed" {
		t.Fatalf("label after status = %q", bt.logsLabel.Text)
	}

	test.Tap(findButton(t, tab(t, bt, "Logs"), "Clear"))
	if len(bt.values) != 0 || bt.order != nil || bt.loadedFiles != nil || bt.nrecords != 0 {
		t.Fatalf("state not cleared: %d values, order %v, files %v, n %d", len(bt.values), bt.order, bt.loadedFiles, bt.nrecords)
	}
	if bt.logsLabel.Text != "No log files loaded" {
		t.Fatalf("label after clear = %q", bt.logsLabel.Text)
	}
	if got := len(bt.missingChannels()); got != len(requiredChannels) {
		t.Fatalf("missing after clear = %d, want %d", got, len(requiredChannels))
	}
	if got := labelTexts(bt.channelList.Objects[0]); got[1] != "missing: ActualIn.n_Engine" {
		t.Fatalf("rpm row after clear = %q", got)
	}
}

// nanSlice returns n NaNs; formatFloat prints the shortest exact decimal.
func TestNanSliceAndFormatFloat(t *testing.T) {
	if s := nanSlice(0); len(s) != 0 {
		t.Fatalf("nanSlice(0) = %v", s)
	}
	s := nanSlice(3)
	if len(s) != 3 {
		t.Fatalf("nanSlice(3) = %v", s)
	}
	for _, v := range s {
		if !math.IsNaN(v) {
			t.Fatalf("nanSlice(3) = %v", s)
		}
	}
	for v, want := range map[float64]string{5: "5", 0.5: "0.5", 150: "150", -2.25: "-2.25", 1e-7: "0.0000001"} {
		if got := formatFloat(v); got != want {
			t.Errorf("formatFloat(%v) = %q, want %q", v, got, want)
		}
	}
}

// nearestIndex picks the closest breakpoint, the earlier one on a tie.
func TestNearestIndex_Ties(t *testing.T) {
	axis := []float64{1000, 2000, 3000}
	for v, want := range map[float64]int{-5: 0, 1499: 0, 1500: 0, 1501: 1, 2600: 2, 9000: 2} {
		if got := nearestIndex(axis, v); got != want {
			t.Errorf("nearestIndex(%v) = %d, want %d", v, got, want)
		}
	}
}
