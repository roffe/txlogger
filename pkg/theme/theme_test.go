package theme

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestLoadOverridesAndFallsBack(t *testing.T) {
	test.NewApp()
	dir := t.TempDir()
	t.Setenv("APPIMAGE", filepath.Join(dir, "txlogger.AppImage"))
	os.WriteFile(filepath.Join(dir, "theme.json"), []byte(`{"Colors":{"background":"#ff0000ff"},"Sizes":{"text":20}}`), 0o644)

	th, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := th.Color(theme.ColorNameBackground, theme.VariantDark).RGBA()
	if r>>8 != 0xff || g != 0 || b != 0 {
		t.Errorf("background not overridden: %v", th.Color(theme.ColorNameBackground, theme.VariantDark))
	}
	if th.Size(theme.SizeNameText) != 20 {
		t.Errorf("text size not overridden: %v", th.Size(theme.SizeNameText))
	}
	if got, want := th.Color("primary-hover", theme.VariantDark), (color.RGBA{R: 0x21, G: 0x99, B: 0xF3, A: 255}); got != want {
		t.Errorf("primary-hover should fall back to TxTheme: got %v want %v", got, want)
	}
}

func TestDefaultJSONParses(t *testing.T) {
	b, err := os.ReadFile("theme.default.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := theme.FromJSON(string(b)); err != nil {
		t.Fatal(err)
	}
}
