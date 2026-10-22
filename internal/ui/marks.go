package ui

import (
	"image/color"

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"

	"github.com/holdemlab/sshdeck/internal/config"
)

// Color marks: sessions and folders can carry a text color and a fill
// (config: color / fill), inherited by everything inside a folder. They show
// in the tree and on the tabs, so environments are told apart at a glance.

// termDark is the terminal background, for the terminal theme (palette
// names then use Mocha on dark terminals and Latte on light ones).
var termDark = true

func activePalette() palette {
	if curTheme != nil {
		return curTheme.p
	}
	if termDark {
		return palettes["mocha"]
	}
	return palettes["latte"]
}

func (p palette) named(name string) string {
	switch name {
	case "rosewater":
		return p.Rosewater
	case "flamingo":
		return p.Flamingo
	case "pink":
		return p.Pink
	case "mauve":
		return p.Mauve
	case "red":
		return p.Red
	case "maroon":
		return p.Maroon
	case "peach":
		return p.Peach
	case "yellow":
		return p.Yellow
	case "green":
		return p.Green
	case "teal":
		return p.Teal
	case "sky":
		return p.Sky
	case "sapphire":
		return p.Sapphire
	case "blue":
		return p.Blue
	case "lavender":
		return p.Lavender
	case "text":
		return p.Text
	case "subtext":
		return p.Subtext0
	case "overlay":
		return p.Overlay1
	case "surface":
		return p.Surface1
	}
	return ""
}

// markColor turns a color value into a color; nil for "", "none" or junk.
func markColor(v string) color.Color {
	v, ok := config.NormalizeColor(v)
	if !ok || v == "" || v == "none" {
		return nil
	}
	if v[0] == '#' {
		return lipgloss.Color(v)
	}
	if h := activePalette().named(v); h != "" {
		return lipgloss.Color(h)
	}
	return nil
}

// onFill is readable text on a fill: dark text on light fills and the
// other way round.
func onFill(fill color.Color) color.Color {
	r, g, b, _ := fill.RGBA()
	lum := (299*float64(r>>8) + 587*float64(g>>8) + 114*float64(b>>8)) / 1000
	dark, light := palettes["mocha"].Base, palettes["latte"].Base
	if p := activePalette(); curTheme != nil {
		dark, light = p.Base, p.Text
		if !curTheme.dark {
			dark, light = p.Text, p.Base
		}
	}
	if lum > 140 {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

// mark is the style for something marked with color / fill; ok is false
// when it has no mark.
func mark(colorV, fillV string) (lipgloss.Style, bool) {
	fg, bg := markColor(colorV), markColor(fillV)
	if fg == nil && bg == nil {
		return lipgloss.NewStyle(), false
	}
	st := lipgloss.NewStyle()
	if bg != nil {
		st = st.Background(bg)
		if fg == nil {
			fg = onFill(bg)
		}
	}
	return st.Foreground(fg), true
}

// nodeMark is the effective mark of a tree row (inherited from folders).
func (m Model) nodeMark(n node) (st lipgloss.Style, marked, filled bool) {
	var d config.Defaults
	switch {
	case n.kind == localNode:
		d.Color, d.Fill = m.opts.Settings.LocalColor, m.opts.Settings.LocalFill
	case n.kind == folderNode:
		d = m.cfg.FolderDefaults(n.path)
	case n.session != nil:
		r, err := m.cfg.Resolve(n.path, n.session)
		if err != nil {
			return lipgloss.NewStyle(), false, false
		}
		d = r.Defaults
	}
	st, marked = mark(d.Color, d.Fill)
	return st, marked, markColor(d.Fill) != nil
}

// chip draws a name with its mark; a fill gets a space of padding on each
// side so it reads as a label.
func chip(st lipgloss.Style, name string, filled bool) string {
	if filled {
		return st.Render(" " + name + " ")
	}
	return st.Render(name)
}

// tabLabel draws a tab in the tab bar, in its session's colors when it has
// a mark: the fill (or the text color) becomes the tab's background, and
// the active tab is bold and underlined.
func tabLabel(t *Tab, active bool, label, dot string) string {
	fg, bg := markColor(t.session.Color), markColor(t.session.Fill)
	if fg == nil && bg == nil {
		if active {
			return activeTabStyle.Render(label + " " + ansi.Strip(dot))
		}
		return tabStyle.Render(label + " " + dot)
	}
	st := lipgloss.NewStyle().Padding(0, 1)
	switch {
	case bg != nil && fg != nil:
		st = st.Background(bg).Foreground(fg)
	case bg != nil:
		st = st.Background(bg).Foreground(onFill(bg))
	case active: // color only: the active tab is filled with it
		st = st.Background(fg).Foreground(onFill(fg))
	default:
		st = st.Background(barBg).Foreground(fg)
	}
	if active {
		return st.Bold(true).Underline(true).Render(label + " " + ansi.Strip(dot))
	}
	return st.Render(label + " " + ansi.Strip(dot))
}
