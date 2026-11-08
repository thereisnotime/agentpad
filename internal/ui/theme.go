package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// appTheme is agentpad's dark theme: deep neutral background, raised card
// surfaces, one violet accent and readable secondary text.
type appTheme struct{}

var _ fyne.Theme = appTheme{}

func (appTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colBackground
	case theme.ColorNameButton:
		return colSurface3
	case theme.ColorNameDisabledButton:
		return colSurface2
	case theme.ColorNameDisabled:
		return colText2 // also used by LowImportance labels: keep it readable
	case theme.ColorNameError:
		return colorError
	case theme.ColorNameFocus:
		return withAlpha(colAccent, 0x99)
	case theme.ColorNameForeground:
		return colText
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnError:
		return color.White
	case theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning:
		return rgb(0x10120f)
	case theme.ColorNameHeaderBackground:
		return colSurface
	case theme.ColorNameHover:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x12}
	case theme.ColorNameHyperlink:
		return rgb(0xa892ff)
	case theme.ColorNameInputBackground:
		return rgb(0x15161c)
	case theme.ColorNameInputBorder:
		return colBorder
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return colSurface2
	case theme.ColorNamePlaceHolder:
		return colText3
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x22}
	case theme.ColorNamePrimary:
		return colAccent
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x40}
	case theme.ColorNameScrollBarBackground:
		return color.Transparent
	case theme.ColorNameSelection:
		return colAccentSoft
	case theme.ColorNameSeparator:
		return colBorder
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x80}
	case theme.ColorNameSuccess:
		return colorSuccess
	case theme.ColorNameWarning:
		return colorAttention
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (appTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }

func (appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameText:
		return 14
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameHeadingText:
		return 22
	case theme.SizeNameSubHeadingText:
		return 17
	case theme.SizeNameInputRadius, theme.SizeNameButtonRadius:
		return 10
	case theme.SizeNameSelectionRadius, theme.SizeNameMenuRadius:
		return 8
	case theme.SizeNameCardRadius, theme.SizeNameDialogRadius, theme.SizeNamePopupRadius:
		return 14
	case theme.SizeNameScrollBarRadius:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(n)
}
