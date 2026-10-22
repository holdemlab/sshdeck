package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Styles are package-wide and rebuilt by buildStyles when the theme changes
// (see theme.go). With the terminal theme they use ANSI colors, so the
// terminal's own palette decides the look.
var (
	accent, muted, green, red, yellow, barBg color.Color

	sidebarStyle, titleStyle, folderStyle, selectedStyle, mutedStyle, labelStyle,
	errStyle, okStyle, warnStyle, barStyle, tabStyle, activeTabStyle, brandStyle,
	prefixStyle, overlayStyle, helpBox lipgloss.Style
)

func init() { buildStyles(nil) }

func buildStyles(t *theme) {
	onAccent := color.Color(lipgloss.Color("0"))
	brandBg, brandFg := color.Color(lipgloss.Color("4")), color.Color(lipgloss.Color("15"))
	selected := lipgloss.NewStyle().Reverse(true)
	if t == nil {
		accent, muted = lipgloss.Color("12"), lipgloss.Color("8")
		green, red, yellow = lipgloss.Color("10"), lipgloss.Color("9"), lipgloss.Color("11")
		barBg = lipgloss.Color("236")
	} else {
		c := lipgloss.Color
		accent, muted = c(t.p.Blue), c(t.p.Overlay1)
		green, red, yellow = c(t.p.Green), c(t.p.Red), c(t.p.Yellow)
		barBg = c(t.p.Mantle)
		onAccent = c(t.p.Base)
		brandBg, brandFg = c(t.p.Mauve), c(t.p.Base)
		selected = lipgloss.NewStyle().Background(c(t.p.Surface1)).Foreground(c(t.p.Text))
	}

	sidebarStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderForeground(muted).
		Padding(0, 1)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	folderStyle = lipgloss.NewStyle().Bold(true)
	selectedStyle = selected
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
	labelStyle = lipgloss.NewStyle().Foreground(muted).Width(10)
	errStyle = lipgloss.NewStyle().Foreground(red)
	okStyle = lipgloss.NewStyle().Foreground(green)
	warnStyle = lipgloss.NewStyle().Foreground(yellow).Bold(true)

	barStyle = lipgloss.NewStyle().Background(barBg)
	tabStyle = lipgloss.NewStyle().Background(barBg).Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().Background(accent).Foreground(onAccent).Bold(true).Padding(0, 1)
	brandStyle = lipgloss.NewStyle().Background(brandBg).Foreground(brandFg).Bold(true).Padding(0, 1)
	prefixStyle = lipgloss.NewStyle().Background(yellow).Foreground(onAccent).Bold(true).Padding(0, 1)

	overlayStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2)
	helpBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
}
