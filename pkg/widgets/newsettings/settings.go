package settings

import "fyne.io/fyne/v2"

type SettingsWidget interface {
}

type SettingsDefinition struct {
	Name        string
	Description string
	Type        string
}

type Settings struct {
	app fyne.App
	CAN fyne.Widget
}

func NewSettings(app fyne.App) *Settings {
	w := &Settings{
		app: app,
	}
	return w
}
