package theme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/roffe/txlogger/pkg/assets"
)

type TxTheme struct{}

func (m TxTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 23, G: 23, B: 24, A: 255}
	case fyne.ThemeColorName("primary-hover"):
		return color.RGBA{R: 0x21, G: 0x99, B: 0xF3, A: 255}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

var dragcornerindicatorleftIconRes = &fyne.StaticResource{
	StaticName:    "drag-corner-indicator-left.svg",
	StaticContent: assets.LeftCornerBytes,
}

func (m TxTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	switch name {
	case fyne.ThemeIconName("drag-corner-indicator-left"):
		return theme.NewThemedResource(dragcornerindicatorleftIconRes)
	default:
		return theme.DefaultTheme().Icon(name)
	}
}

func (m TxTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m TxTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameSeparatorThickness: // denna 0
		return 0
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameLineSpacing:
		return 4
	case theme.SizeNamePadding: // 2
		return 2
	case theme.SizeNameScrollBar: // 8
		return 16
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNameSubHeadingText:
		return 18
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInputRadius:
		return 5
	case theme.SizeNameSelectionRadius:
		return 3
	case theme.SizeNameWindowTitleBarHeight:
		return 26
	case theme.SizeNameWindowButtonHeight:
		return 20
	case theme.SizeNameWindowButtonIcon:
		return 20
	case theme.SizeNameWindowButtonRadius:
		return 0
	default:
		return 0
	}
}
