package common

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// SanitizeFilename keeps only the base name and collapses every run of
// characters other than ASCII letters, digits, '_', '-' and '.' into one '_'.
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"log.t7l", "log.t7l"},
		{"dir/sub/log-2026_10.t7l", "log-2026_10.t7l"},
		{"../../etc/passwd", "passwd"},
		{"dir/", "dir"},
		{"my log (1).csv", "my_log_1_.csv"},
		{"a\tb", "a_b"},
		{"åäö.bin", "_.bin"},
		{"/", "_"},
		{"name:with*bad?chars", "name_with_bad_chars"},
	}
	for _, tt := range tests {
		if got := SanitizeFilename(tt.in); got != tt.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Names that reduce to nothing or to a directory reference become
// "file_<unix seconds>".
func TestSanitizeFilenameFallback(t *testing.T) {
	for _, in := range []string{"", ".", "..", "foo/.."} {
		before := time.Now().Unix()
		got := SanitizeFilename(in)
		after := time.Now().Unix()

		ts, ok := strings.CutPrefix(got, "file_")
		if !ok {
			t.Fatalf("SanitizeFilename(%q) = %q, want file_<unix>", in, got)
		}
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || n < before || n > after {
			t.Fatalf("SanitizeFilename(%q) = %q, timestamp not in [%d, %d]", in, got, before, after)
		}
	}
}

// ListFilesInPathByExtension returns the names of regular entries whose
// extension matches case-insensitively, in directory order, skipping
// directories.
func TestListFilesInPathByExtension(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.json", "B.JSON", "c.txt", "noext", "x.json.bak"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ListFilesInPathByExtension(dir, ".json")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"B.JSON", "a.json"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}

	// The extension must include the dot.
	got, err = ListFilesInPathByExtension(dir, "json")
	if err != nil || len(got) != 0 {
		t.Fatalf("ext without dot: got %q, %v; want no matches", got, err)
	}
}

// A missing directory is reported as an error with no result.
func TestListFilesInPathByExtensionMissingDir(t *testing.T) {
	got, err := ListFilesInPathByExtension(filepath.Join(t.TempDir(), "nope"), ".json")
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("err = %v, want not-exist error", err)
	}
	if got != nil {
		t.Fatalf("got %q, want nil", got)
	}
}

// setHome points the user home directory at a fresh temp dir.
func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

var pathGetters = []struct {
	name string
	fn   func() (string, error)
	sub  string
}{
	{"GetLogPath", GetLogPath, "logs"},
	{"GetADScannerPath", GetADScannerPath, "adscanner"},
	{"GetLayoutPath", GetLayoutPath, "layouts"},
	{"GetDashboardPath", GetDashboardPath, "dashboards"},
	{"GetMatrixBuilderPath", GetMatrixBuilderPath, "matrixbuilder"},
	{"GetScriptsPath", GetScriptsPath, "scripts"},
	{"GetBinPath", GetBinPath, "bins"},
}

// GetComponentPath places every component under <base>/txlogger/.
func TestGetComponentPath(t *testing.T) {
	if got, want := GetComponentPath("base", "logs"), filepath.Join("base", "txlogger", "logs"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Each path getter returns <home>/txlogger/<component> and creates it; calling
// again is harmless.
func TestPathGettersCreateDirs(t *testing.T) {
	home := setHome(t)
	if got, err := GetUserHomeDir(); err != nil || got != home {
		t.Fatalf("GetUserHomeDir = %q, %v; want %q", got, err, home)
	}
	for _, g := range pathGetters {
		t.Run(g.name, func(t *testing.T) {
			want := filepath.Join(home, "txlogger", g.sub)
			for range 2 {
				got, err := g.fn()
				if err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
				if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
					t.Fatalf("%s not created as a directory: %v", got, err)
				}
			}
		})
	}
}

// skipIfHomeHasFallback skips on platforms where os.UserHomeDir does not fail
// when $HOME is empty.
func skipIfHomeHasFallback(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "windows", "plan9", "android", "ios":
		t.Skipf("os.UserHomeDir does not depend only on $HOME on %s", runtime.GOOS)
	}
}

// Without a home directory every getter fails with an empty path, and so does
// CreatetxloggerDirs.
func TestPathGettersNoHome(t *testing.T) {
	skipIfHomeHasFallback(t)
	t.Setenv("HOME", "")

	if _, err := GetUserHomeDir(); err == nil || !strings.Contains(err.Error(), "could not determine user home directory") {
		t.Fatalf("GetUserHomeDir err = %v", err)
	}
	for _, g := range pathGetters {
		got, err := g.fn()
		if err == nil || got != "" {
			t.Errorf("%s = %q, %v; want empty path and an error", g.name, got, err)
		}
	}
	if err := CreatetxloggerDirs([]string{"T7"}); err == nil {
		t.Error("CreatetxloggerDirs succeeded without a home directory")
	}
}

// When a directory cannot be created (a file is in the way) the getters and
// CreatetxloggerDirs report it.
func TestPathGettersMkdirFailure(t *testing.T) {
	home := setHome(t)
	if err := os.WriteFile(filepath.Join(home, "txlogger"), []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, g := range pathGetters {
		if _, err := g.fn(); err == nil || !strings.Contains(err.Error(), "could not create directory") {
			t.Errorf("%s err = %v, want could not create directory", g.name, err)
		}
	}
	if err := CreatetxloggerDirs(nil); err == nil || !strings.Contains(err.Error(), "could not create directory") {
		t.Errorf("CreatetxloggerDirs err = %v, want could not create directory", err)
	}
}

// CreatetxloggerDirs builds the startup tree: logs, layouts, bins, adscanner,
// dashboards, and one scripts/<ECU> directory per ECU.
func TestCreatetxloggerDirs(t *testing.T) {
	home := setHome(t)
	if err := CreatetxloggerDirs([]string{"T5", "T7", "T8"}); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"logs", "layouts", "bins", "adscanner", "dashboards", "scripts/T5", "scripts/T7", "scripts/T8"} {
		p := filepath.Join(home, "txlogger", filepath.FromSlash(sub))
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Errorf("%s missing: %v", sub, err)
		}
	}
	// Running it again over an existing tree succeeds.
	if err := CreatetxloggerDirs([]string{"T7"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

// ParseFixedPrec recognises exactly "%.<digits>f" and returns the precision,
// or -1 for any other format.
func TestParseFixedPrec(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"%.0f", 0},
		{"%.2f", 2},
		{"%.10f", 10},
		{"%.123f", 123},
		{"%.f", -1},
		{"%f", -1},
		{"%5.2f", -1},
		{"%.2d", -1},
		{"%.2g", -1},
		{"%.2af", -1},
		{"%.-1f", -1},
		{"x%.2f", -1},
		{"%.2f%%", -1},
		{"", -1},
		{"%d", -1},
	}
	for _, tt := range tests {
		if got := ParseFixedPrec(tt.in); got != tt.want {
			t.Errorf("ParseFixedPrec(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// AppendFormatFloat appends exactly what fmt would produce for the format,
// both on its strconv fast path and for other formats, keeping dst's prefix.
func TestAppendFormatFloat(t *testing.T) {
	formats := []string{"%.0f", "%.1f", "%.2f", "%.3f", "%.10f", "%f", "%g", "%6.2f", "%e", "%.2f%%", "%.f"}
	values := []float64{0, math.Copysign(0, -1), 1, -1, 0.5, 1.5, 2.5, 1.005, 2.675, -2.5,
		1234.5678, 1e-7, 1e21, -99999.999, math.MaxFloat64, math.SmallestNonzeroFloat64,
		math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, f := range formats {
		for _, v := range values {
			want := "v=" + fmt.Sprintf(f, v)
			if got := string(AppendFormatFloat([]byte("v="), f, v)); got != want {
				t.Errorf("AppendFormatFloat(%q, %v) = %q, want %q", f, v, got, want)
			}
		}
	}
	if got := string(AppendFormatFloat(nil, "%.2f", 3.14159)); got != "3.14" {
		t.Fatalf("got %q, want 3.14", got)
	}
}

// SameTextBytes compares a string and a byte slice by content without
// allocating.
func TestSameTextBytes(t *testing.T) {
	tests := []struct {
		s    string
		b    []byte
		want bool
	}{
		{"", nil, true},
		{"", []byte{}, true},
		{"abc", []byte("abc"), true},
		{"abc", []byte("abd"), false},
		{"abc", []byte("ab"), false},
		{"ab", []byte("abc"), false},
		{"åäö", []byte("åäö"), true},
		{"12.50", []byte("12.5"), false},
	}
	for _, tt := range tests {
		if got := SameTextBytes(tt.s, tt.b); got != tt.want {
			t.Errorf("SameTextBytes(%q, %q) = %v, want %v", tt.s, tt.b, got, tt.want)
		}
	}
	b := []byte("1234.56")
	if n := testing.AllocsPerRun(100, func() { SameTextBytes("1234.56", b) }); n != 0 {
		t.Fatalf("SameTextBytes allocated %v times per run", n)
	}
}

// Abs returns the magnitude of an int.
func TestAbs(t *testing.T) {
	for in, want := range map[int]int{0: 0, 5: 5, -5: 5, -1: 1, math.MaxInt: math.MaxInt, -math.MaxInt: math.MaxInt} {
		if got := Abs(in); got != want {
			t.Errorf("Abs(%d) = %d, want %d", in, got, want)
		}
	}
}

type celsius int

// FindMinMax returns the smallest and largest element for any numeric type,
// including named types.
func TestFindMinMax(t *testing.T) {
	if mn, mx := FindMinMax([]int{3, -7, 12, 0}); mn != -7 || mx != 12 {
		t.Errorf("int: got %d, %d", mn, mx)
	}
	if mn, mx := FindMinMax([]uint8{200, 3, 255, 4}); mn != 3 || mx != 255 {
		t.Errorf("uint8: got %d, %d", mn, mx)
	}
	if mn, mx := FindMinMax([]float32{-1.5, 2.25, -3.75}); mn != -3.75 || mx != 2.25 {
		t.Errorf("float32: got %v, %v", mn, mx)
	}
	if mn, mx := FindMinMax([]celsius{21, -40, 105}); mn != -40 || mx != 105 {
		t.Errorf("named int: got %d, %d", mn, mx)
	}
	if mn, mx := FindMinMax([]int64{42}); mn != 42 || mx != 42 {
		t.Errorf("single: got %d, %d", mn, mx)
	}
}

// FindMinMax panics on an empty slice since there is no answer to return.
func TestFindMinMaxEmptyPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != "empty slice" {
			t.Fatalf("recover() = %v, want \"empty slice\"", r)
		}
	}()
	FindMinMax([]int{})
}

// Clamp limits a value to [min, max] for any ordered type.
func TestClamp(t *testing.T) {
	ints := []struct{ v, want int }{{-5, 0}, {0, 0}, {50, 50}, {100, 100}, {101, 100}}
	for _, tt := range ints {
		if got := Clamp(tt.v, 0, 100); got != tt.want {
			t.Errorf("Clamp(%d, 0, 100) = %d, want %d", tt.v, got, tt.want)
		}
	}
	if got := Clamp(1.25, 0.5, 1.0); got != 1.0 {
		t.Errorf("float above: got %v", got)
	}
	if got := Clamp(math.Inf(-1), -1.0, 1.0); got != -1.0 {
		t.Errorf("float -Inf: got %v", got)
	}
	if got := Clamp("m", "b", "y"); got != "m" {
		t.Errorf("string inside: got %q", got)
	}
	if got := Clamp("z", "b", "y"); got != "y" {
		t.Errorf("string above: got %q", got)
	}
}
