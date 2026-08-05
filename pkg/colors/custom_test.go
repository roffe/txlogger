package colors

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestCustomColorRoundTrip(t *testing.T) {
	app := test.NewApp()
	defer test.NewApp()

	want := color.RGBA{1, 2, 3, 255}
	SetCustom("MAF.m_AirInlet", want)
	if got := GetColor("MAF.m_AirInlet"); got != want {
		t.Fatalf("custom color not returned: got %v want %v", got, want)
	}
	if names := CustomNames(); len(names) != 1 || names[0] != "MAF.m_AirInlet" {
		t.Fatalf("CustomNames = %v", names)
	}

	// survives a reload from preferences
	custom = nil
	if got := GetColor("MAF.m_AirInlet"); got != want {
		t.Fatalf("color did not survive reload: got %v want %v", got, want)
	}
	_ = app

	DeleteCustom("MAF.m_AirInlet")
	if got := GetColor("MAF.m_AirInlet"); got != colorMap["MAF.m_AirInlet"] {
		t.Fatalf("delete did not restore default: got %v", got)
	}
}
