package theme

import (
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

var txBackground = color.NRGBA{R: 23, G: 23, B: 24, A: 255}

func nrgba(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

// theme.default.json is a dump of TxTheme: every color and size it lists is
// what TxTheme returns for that name.
func TestDefaultJSONMatchesTxTheme(t *testing.T) {
	test.NewTempApp(t)
	b, err := os.ReadFile("theme.default.json")
	if err != nil {
		t.Fatal(err)
	}
	var dump struct {
		Colors map[string]string
		Sizes  map[string]float32
	}
	if err := json.Unmarshal(b, &dump); err != nil {
		t.Fatal(err)
	}
	if len(dump.Colors) == 0 || len(dump.Sizes) == 0 {
		t.Fatalf("dump has %d colors and %d sizes", len(dump.Colors), len(dump.Sizes))
	}
	parsed, err := theme.FromJSON(string(b))
	if err != nil {
		t.Fatal(err)
	}
	th := TxTheme{}
	for name, hex := range dump.Colors {
		n := fyne.ThemeColorName(name)
		if got, want := nrgba(th.Color(n, theme.VariantDark)), nrgba(parsed.Color(n, theme.VariantDark)); got != want {
			t.Errorf("color %s: TxTheme %v, dump %s", name, got, hex)
		}
	}
	for name, want := range dump.Sizes {
		if got := th.Size(fyne.ThemeSizeName(name)); got != want {
			t.Errorf("size %s: TxTheme %v, dump %v", name, got, want)
		}
	}
}

// TxTheme is always the dark palette, whichever variant is requested, with
// its own background and primary-hover colors.
func TestTxThemeIgnoresVariant(t *testing.T) {
	test.NewTempApp(t)
	th := TxTheme{}
	if got := nrgba(th.Color(theme.ColorNameBackground, theme.VariantLight)); got != txBackground {
		t.Errorf("background (light requested) = %v, want %v", got, txBackground)
	}
	for _, name := range []fyne.ThemeColorName{theme.ColorNameForeground, theme.ColorNameButton, theme.ColorNameInputBackground, "primary-hover"} {
		light, dark := th.Color(name, theme.VariantLight), th.Color(name, theme.VariantDark)
		if nrgba(light) != nrgba(dark) {
			t.Errorf("%s: light %v != dark %v", name, light, dark)
		}
	}
	if got, want := nrgba(th.Color(theme.ColorNameForeground, theme.VariantLight)), nrgba(theme.DefaultTheme().Color(theme.ColorNameForeground, theme.VariantDark)); got != want {
		t.Errorf("foreground = %v, want the default dark %v", got, want)
	}
}

// TxTheme uses the default theme's icons and fonts unchanged.
func TestTxThemeIconsAndFontsAreDefault(t *testing.T) {
	test.NewTempApp(t)
	th, def := TxTheme{}, theme.DefaultTheme()
	for _, name := range []fyne.ThemeIconName{theme.IconNameHome, theme.IconNameSettings, theme.IconNameDocumentSave} {
		got, want := th.Icon(name), def.Icon(name)
		if got.Name() != want.Name() || !bytes.Equal(got.Content(), want.Content()) {
			t.Errorf("icon %s: got %s, want %s", name, got.Name(), want.Name())
		}
	}
	for _, style := range []fyne.TextStyle{{}, {Bold: true}, {Italic: true}, {Monospace: true}} {
		if got, want := th.Font(style), def.Font(style); got.Name() != want.Name() {
			t.Errorf("font %+v: got %s, want %s", style, got.Name(), want.Name())
		}
	}
}

// The export and import icons are the embedded SVGs, themed with the
// foreground color.
func TestExportImportIcons(t *testing.T) {
	test.NewTempApp(t)
	for _, tt := range []struct {
		res  fyne.Resource
		name string
	}{{ExportIcon(), "foreground_export.svg"}, {ImportIcon(), "foreground_import.svg"}} {
		if _, ok := tt.res.(*theme.ThemedResource); !ok {
			t.Errorf("%s: %T is not a themed resource", tt.name, tt.res)
		}
		if tt.res.Name() != tt.name {
			t.Errorf("name %q, want %q", tt.res.Name(), tt.name)
		}
		if !bytes.Contains(tt.res.Content(), []byte("<svg")) {
			t.Errorf("%s: content is not SVG", tt.name)
		}
	}
}

// Load returns plain TxTheme with no error when there is no theme.json next to
// the executable (or the AppImage).
func TestLoadWithoutThemeFile(t *testing.T) {
	test.NewTempApp(t)
	t.Setenv("APPIMAGE", filepath.Join(t.TempDir(), "txlogger.AppImage"))
	th, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := th.(*TxTheme); !ok {
		t.Fatalf("got %T, want *TxTheme", th)
	}

	// Without APPIMAGE the directory of the running executable is used; the
	// test binary's build directory has no theme.json.
	t.Setenv("APPIMAGE", "")
	th, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := th.(*TxTheme); !ok {
		t.Fatalf("without APPIMAGE: got %T, want *TxTheme", th)
	}
}

// Load reports an unreadable or invalid theme.json but still returns TxTheme,
// which main applies regardless of the error.
func TestLoadErrorsFallBackToTxTheme(t *testing.T) {
	test.NewTempApp(t)
	write := func(dir, content string) {
		if err := os.WriteFile(filepath.Join(dir, "theme.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := map[string]func(t *testing.T) string{ // returns the APPIMAGE path
		"malformed JSON": func(t *testing.T) string {
			dir := t.TempDir()
			write(dir, `{"Colors":`)
			return filepath.Join(dir, "x.AppImage")
		},
		"bad color": func(t *testing.T) string {
			dir := t.TempDir()
			write(dir, `{"Colors":{"background":"not-a-color"}}`)
			return filepath.Join(dir, "x.AppImage")
		},
		"directory is a file": func(t *testing.T) string {
			dir := t.TempDir()
			file := filepath.Join(dir, "file")
			if err := os.WriteFile(file, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(file, "x.AppImage")
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("APPIMAGE", setup(t))
			th, err := Load()
			if err == nil {
				t.Fatal("expected an error")
			}
			if th == nil {
				t.Fatal("nil theme returned with the error")
			}
			if got := nrgba(th.Color(theme.ColorNameBackground, theme.VariantDark)); got != txBackground {
				t.Errorf("background = %v, want TxTheme's %v", got, txBackground)
			}
			if got := th.Size(theme.SizeNameText); got != 14 {
				t.Errorf("text size = %v, want TxTheme's 14", got)
			}
		})
	}
}
