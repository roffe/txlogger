package colors

import (
	"image/color"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// freshCustom gives the test its own in-memory app preferences and an empty,
// not yet loaded custom color cache.
func freshCustom(t *testing.T) fyne.App {
	t.Helper()
	app := test.NewTempApp(t)
	resetCache := func() {
		customMu.Lock()
		custom = nil
		customMu.Unlock()
	}
	resetCache()
	t.Cleanup(resetCache)
	return app
}

// Well-known symbols get their fixed plot color, and the lambda scanner
// symbols of the different ECUs share one color.
func TestGetColorBuiltin(t *testing.T) {
	freshCustom(t)
	if got, want := GetColor("m_Request"), (color.RGBA{247, 10, 10, 255}); got != want {
		t.Fatalf("m_Request = %v, want %v", got, want)
	}
	lambda := GetColor("DisplProt.LambdaScanner")
	for _, name := range []string{"LambdaScan.LambdaScanner", "LambdaScan.LambdaScanner2", "Lambda.ADScanner"} {
		if got := GetColor(name); got != lambda {
			t.Errorf("%s = %v, want shared lambda color %v", name, got, lambda)
		}
	}
}

// Symbols without a fixed or custom color get an opaque color derived from the
// CRC32 of the name, so the same symbol keeps its color across sessions.
func TestGetColorHashFallback(t *testing.T) {
	freshCustom(t)
	tests := []struct {
		name string
		want color.RGBA
	}{
		{"hello", color.RGBA{166, 16, 134, 255}},
		{"RPM", color.RGBA{67, 175, 181, 255}},
		{"Out.X_AccPedal", color.RGBA{150, 29, 122, 255}},
	}
	for _, tt := range tests {
		if got := GetColor(tt.name); got != tt.want {
			t.Errorf("GetColor(%q) = %v, want %v", tt.name, got, tt.want)
		}
		if got := GetColor(tt.name); got != tt.want {
			t.Errorf("GetColor(%q) not stable: %v", tt.name, got)
		}
	}
}

// Custom colors are persisted as a sorted list of "name=#RRGGBBAA" entries in
// uppercase hex, and deleting one rewrites the list without it.
func TestCustomColorPreferenceFormat(t *testing.T) {
	app := freshCustom(t)

	SetCustom("b.sym", color.RGBA{0xab, 0xcd, 0xef, 0x80})
	SetCustom("a.sym", color.RGBA{1, 2, 3, 4})

	got := app.Preferences().StringList(customColorsKey)
	want := []string{"a.sym=#01020304", "b.sym=#ABCDEF80"}
	if !slices.Equal(got, want) {
		t.Fatalf("stored entries = %q, want %q", got, want)
	}
	if names := CustomNames(); !slices.Equal(names, []string{"a.sym", "b.sym"}) {
		t.Fatalf("CustomNames = %q", names)
	}

	DeleteCustom("a.sym")
	got = app.Preferences().StringList(customColorsKey)
	if want := []string{"b.sym=#ABCDEF80"}; !slices.Equal(got, want) {
		t.Fatalf("after delete, stored entries = %q, want %q", got, want)
	}
}

// Loading skips preference entries that lack "=" or carry an unparsable color,
// keeping the valid ones (hex is case-insensitive).
func TestCustomColorLoadSkipsMalformed(t *testing.T) {
	app := freshCustom(t)
	app.Preferences().SetStringList(customColorsKey, []string{
		"good=#0A0B0C0D",
		"lower=#a0b0c0d0",
		"noequals",
		"bad=#ZZ000000",
		"short=#0102",
		"nohash=01020304",
	})

	if names := CustomNames(); !slices.Equal(names, []string{"good", "lower"}) {
		t.Fatalf("CustomNames = %q, want [good lower]", names)
	}
	if got, want := GetColor("good"), (color.RGBA{10, 11, 12, 13}); got != want {
		t.Fatalf("good = %v, want %v", got, want)
	}
	if got, want := GetColor("lower"), (color.RGBA{0xa0, 0xb0, 0xc0, 0xd0}); got != want {
		t.Fatalf("lower = %v, want %v", got, want)
	}
	if got, want := GetColor("bad"), (color.RGBA{57, 43, 251, 255}); got != want {
		t.Fatalf("malformed entry should fall back to hash color: got %v want %v", got, want)
	}
}

// Without a running app, custom colors still work in memory for the session.
func TestCustomColorWithoutApp(t *testing.T) {
	freshCustom(t)
	fyne.SetCurrentApp(nil)

	want := color.RGBA{9, 8, 7, 6}
	SetCustom("RPM", want)
	if got := GetColor("RPM"); got != want {
		t.Fatalf("GetColor = %v, want %v", got, want)
	}
	DeleteCustom("RPM")
	if got, want := GetColor("RPM"), (color.RGBA{67, 175, 181, 255}); got != want {
		t.Fatalf("after delete GetColor = %v, want hash color %v", got, want)
	}
}

// OnChange listeners run after every SetCustom and DeleteCustom, outside the
// package lock, so they can read the new state back through the package.
func TestOnChangeListener(t *testing.T) {
	freshCustom(t)

	var active atomic.Bool
	active.Store(true)
	t.Cleanup(func() { active.Store(false) }) // listeners cannot be removed

	seen := make(chan []string, 4)
	OnChange(func() {
		if active.Load() {
			seen <- CustomNames()
		}
	})

	expect := func(op func(), want []string) {
		t.Helper()
		done := make(chan struct{})
		go func() { op(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("listener deadlocked calling back into the package")
		}
		select {
		case got := <-seen:
			if !slices.Equal(got, want) {
				t.Fatalf("listener saw %q, want %q", got, want)
			}
		default:
			t.Fatal("listener was not called")
		}
	}

	expect(func() { SetCustom("x", color.RGBA{1, 1, 1, 255}) }, []string{"x"})
	expect(func() { DeleteCustom("x") }, []string{})
}

// Custom color reads and writes are safe from several goroutines at once.
func TestCustomColorConcurrentAccess(t *testing.T) {
	freshCustom(t)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := range 50 {
				SetCustom("sym", color.RGBA{uint8(i), uint8(j), 0, 255})
			}
		}()
		go func() {
			defer wg.Done()
			for range 50 {
				_ = GetColor("sym")
				_ = CustomNames()
			}
		}()
	}
	wg.Wait()
	if names := CustomNames(); !slices.Equal(names, []string{"sym"}) {
		t.Fatalf("CustomNames = %q", names)
	}
}
