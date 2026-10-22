package ui

import (
	"encoding/json"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// Color themes. "terminal" keeps the terminal's own palette (the 16 ANSI
// colors and its background); the Catppuccin flavours paint everything —
// sshdeck's chrome and the remote programs in the tabs — with their palette.

// palette is one Catppuccin flavour (https://catppuccin.com/palette).
type palette struct {
	Rosewater, Flamingo, Pink, Mauve, Red, Maroon, Peach, Yellow, Green,
	Teal, Sky, Sapphire, Blue, Lavender, Text, Subtext1, Subtext0, Overlay2,
	Overlay1, Overlay0, Surface2, Surface1, Surface0, Base, Mantle, Crust string
}

type theme struct {
	name  string // setting value
	label string
	dark  bool
	p     palette
	ansi  [16]color.Color // what ANSI colors 0–15 become
	fg    color.Color     // default text
	bg    color.Color     // default background
}

var themeNames = []string{"terminal", "latte", "frappe", "macchiato", "mocha"}

var themeLabels = map[string]string{
	"terminal": "terminal", "latte": "Latte", "frappe": "Frappé", "macchiato": "Macchiato", "mocha": "Mocha",
}

var palettes = map[string]palette{
	"latte": {
		"#dc8a78", "#dd7878", "#ea76cb", "#8839ef", "#d20f39", "#e64553", "#fe640b", "#df8e1d", "#40a02b",
		"#179299", "#04a5e5", "#209fb5", "#1e66f5", "#7287fd", "#4c4f69", "#5c5f77", "#6c6f85", "#7c7f93",
		"#8c8fa1", "#9ca0b0", "#acb0be", "#bcc0cc", "#ccd0da", "#eff1f5", "#e6e9ef", "#dce0e8",
	},
	"frappe": {
		"#f2d5cf", "#eebebe", "#f4b8e4", "#ca9ee6", "#e78284", "#ea999c", "#ef9f76", "#e5c890", "#a6d189",
		"#81c8be", "#99d1db", "#85c1dc", "#8caaee", "#babbf1", "#c6d0f5", "#b5bfe2", "#a5adce", "#949cbb",
		"#838ba7", "#737994", "#626880", "#51576d", "#414559", "#303446", "#292c3c", "#232634",
	},
	"macchiato": {
		"#f4dbd6", "#f0c6c6", "#f5bde6", "#c6a0f6", "#ed8796", "#ee99a0", "#f5a97f", "#eed49f", "#a6da95",
		"#8bd5ca", "#91d7e3", "#7dc4e4", "#8aadf4", "#b7bdf8", "#cad3f5", "#b8c0e0", "#a5adcb", "#939ab7",
		"#8087a2", "#6e738d", "#5b6078", "#494d64", "#363a4f", "#24273a", "#1e2030", "#181926",
	},
	"mocha": {
		"#f5e0dc", "#f2cdcd", "#f5c2e7", "#cba6f7", "#f38ba8", "#eba0ac", "#fab387", "#f9e2af", "#a6e3a1",
		"#94e2d5", "#89dceb", "#74c7ec", "#89b4fa", "#b4befe", "#cdd6f4", "#bac2de", "#a6adc8", "#9399b2",
		"#7f849c", "#6c7086", "#585b70", "#45475a", "#313244", "#1e1e2e", "#181825", "#11111b",
	},
}

// NormalizeTheme maps aliases ("frappé", "Mocha", "") to a theme name; ok is
// false for unknown names.
func normalizeTheme(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "terminal", "default", "none":
		return "terminal", true
	case "frappé":
		return "frappe", true
	}
	if strings.HasPrefix(s, "catppuccin-") {
		s = strings.TrimPrefix(s, "catppuccin-")
	}
	_, ok := palettes[s]
	return s, ok
}

func makeTheme(name string) *theme {
	p, ok := palettes[name]
	if !ok {
		return nil
	}
	c := lipgloss.Color
	t := &theme{name: name, label: themeLabels[name], dark: name != "latte", p: p, fg: c(p.Text), bg: c(p.Base)}
	// The terminal mapping Catppuccin uses for its terminal ports.
	black, white, bblack, bwhite := p.Surface1, p.Subtext1, p.Surface2, p.Subtext0
	if !t.dark {
		black, white, bblack, bwhite = p.Subtext1, p.Surface2, p.Subtext0, p.Surface1
	}
	for i, h := range []string{
		black, p.Red, p.Green, p.Yellow, p.Blue, p.Pink, p.Teal, white,
		bblack, p.Red, p.Green, p.Yellow, p.Blue, p.Pink, p.Teal, bwhite,
	} {
		t.ansi[i] = c(h)
	}
	return t
}

var (
	themeName = "terminal"
	curTheme  *theme // nil: terminal colors
)

// useTheme switches the styles to a theme (cheap when it is already active).
func useTheme(name string) {
	name, ok := normalizeTheme(name)
	if !ok {
		name = "terminal"
	}
	if name == themeName {
		return
	}
	themeName, curTheme = name, makeTheme(name)
	buildStyles(curTheme)
}

// mapColor turns ANSI colors 0–15 into the theme's; nil becomes def.
func (t *theme) mapColor(c, def color.Color) color.Color {
	switch v := c.(type) {
	case nil:
		return def
	case xansi.BasicColor:
		if v < 16 {
			return t.ansi[v]
		}
	case xansi.IndexedColor:
		if v < 16 {
			return t.ansi[v]
		}
	}
	return c
}

// paint recolors every cell of the finished frame: default colors become
// the theme's text and background, ANSI 0–15 the theme's palette. Colors
// set explicitly (256-color, truecolor) are kept.
//
// With only 256 colors (to256), RGB colors are matched to the nearest of the
// 240 fixed xterm colors here: the generic conversion turns the dark
// Catppuccin bases into navy blue.
func (t *theme) paint(canvas *lipgloss.Canvas, to256 bool) {
	if t == nil {
		return
	}
	for y := 0; y < canvas.Height(); y++ {
		for x := 0; x < canvas.Width(); x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			c := *cell
			c.Style.Fg = t.mapColor(c.Style.Fg, t.fg)
			c.Style.Bg = t.mapColor(c.Style.Bg, t.bg)
			if c.Style.UnderlineColor != nil {
				c.Style.UnderlineColor = t.mapColor(c.Style.UnderlineColor, t.fg)
			}
			if to256 {
				c.Style.Fg, c.Style.Bg = nearest256(c.Style.Fg), nearest256(c.Style.Bg)
				if c.Style.UnderlineColor != nil {
					c.Style.UnderlineColor = nearest256(c.Style.UnderlineColor)
				}
			}
			canvas.SetCell(x, y, &c)
		}
	}
}

// cursorColor is the color of the terminal cursor in a tab.
func (t *theme) cursorColor() color.Color {
	if t == nil {
		return nil
	}
	return lipgloss.Color(t.p.Rosewater)
}

// themeEmulator tells the emulator the theme's colors, so programs that ask
// the terminal for its background (vim, OSC 11) get the right answer.
func themeEmulator(e interface {
	SetDefaultForegroundColor(color.Color)
	SetDefaultBackgroundColor(color.Color)
	SetIndexedColor(int, color.Color)
}, t *theme) {
	if t == nil {
		e.SetDefaultForegroundColor(nil)
		e.SetDefaultBackgroundColor(nil)
		for i := 0; i < 16; i++ {
			e.SetIndexedColor(i, nil)
		}
		return
	}
	e.SetDefaultForegroundColor(t.fg)
	e.SetDefaultBackgroundColor(t.bg)
	for i, c := range t.ansi {
		e.SetIndexedColor(i, c)
	}
}

// glamourStyle builds the help's Markdown style from the palette, starting
// from glamour's dark style and replacing its colors by role.
func (t *theme) glamourStyle() (ansi.StyleConfig, error) {
	p := t.p
	roles := map[string]string{
		"document.color": p.Text, "heading.color": p.Blue, "h1.color": p.Base, "h1.background_color": p.Mauve,
		"h6.color": p.Teal, "hr.color": p.Overlay0, "link.color": p.Sapphire, "link_text.color": p.Teal,
		"image.color": p.Pink, "image_text.color": p.Overlay1, "code.color": p.Peach, "code.background_color": p.Surface0,
		"code_block.color": p.Subtext0,
		"text":             p.Text, "error": p.Red, "comment": p.Overlay1, "comment_preproc": p.Peach, "keyword": p.Mauve,
		"keyword_reserved": p.Pink, "keyword_namespace": p.Yellow, "keyword_type": p.Yellow, "operator": p.Sky,
		"punctuation": p.Overlay2, "name": p.Text, "name_builtin": p.Red, "name_tag": p.Blue,
		"name_attribute": p.Yellow, "name_class": p.Yellow, "name_decorator": p.Peach, "name_function": p.Blue,
		"literal_number": p.Peach, "literal_string": p.Green, "literal_string_escape": p.Pink,
		"generic_deleted": p.Red, "generic_inserted": p.Green, "generic_subheading": p.Overlay1,
		"background": p.Mantle,
	}
	raw, err := json.Marshal(styles.DarkStyleConfig)
	if err != nil {
		return ansi.StyleConfig{}, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ansi.StyleConfig{}, err
	}
	for k, v := range doc {
		if m, ok := v.(map[string]any); ok {
			for _, f := range []string{"color", "background_color"} {
				if _, has := m[f]; has {
					if c, ok := roles[k+"."+f]; ok {
						m[f] = c
					}
				}
			}
		}
	}
	if cb, ok := doc["code_block"].(map[string]any); ok {
		if ch, ok := cb["chroma"].(map[string]any); ok {
			for tok, v := range ch {
				m, ok := v.(map[string]any)
				if !ok {
					continue
				}
				c, ok := roles[tok]
				if !ok {
					continue
				}
				switch {
				case tok == "background":
					m["background_color"] = c
				case tok == "error":
					m["color"], m["background_color"] = p.Base, c
				default:
					m["color"] = c
				}
			}
		}
	}
	raw, err = json.Marshal(doc)
	if err != nil {
		return ansi.StyleConfig{}, err
	}
	var out ansi.StyleConfig
	err = json.Unmarshal(raw, &out)
	return out, err
}

// inputStyles are the text input styles for the theme: bubbles' defaults
// use ANSI 7 for blurred text, which is pale on a light flavour.
func (t *theme) inputStyles() textinput.Styles {
	if t == nil {
		return textinput.DefaultStyles(true)
	}
	c := lipgloss.Color
	s := textinput.DefaultStyles(t.dark)
	for _, st := range []*textinput.StyleState{&s.Focused, &s.Blurred} {
		st.Placeholder = lipgloss.NewStyle().Foreground(c(t.p.Overlay0))
		st.Suggestion = st.Placeholder
		st.Prompt = lipgloss.NewStyle().Foreground(c(t.p.Overlay2))
	}
	s.Focused.Text = lipgloss.NewStyle().Foreground(c(t.p.Text))
	s.Blurred.Text = lipgloss.NewStyle().Foreground(c(t.p.Subtext0))
	s.Cursor.Color = c(t.p.Rosewater)
	return s
}

// styleInputs applies the theme to every text input of this (View's) copy
// of the model; it is cheap and runs each frame.
func (m *Model) styleInputs() {
	s := curTheme.inputStyles()
	set := func(in *textinput.Model) { in.SetStyles(s) }
	set(&m.filter)
	set(&m.tunInput)
	set(&m.help.search)
	set(&m.inline.input)
	for i := range m.form.inputs {
		set(&m.form.inputs[i])
	}
	for i := range m.key.inputs {
		set(&m.key.inputs[i])
	}
	if len(m.settings.fields) > 0 {
		fields := make([]setField, len(m.settings.fields)) // don't touch the shared array
		copy(fields, m.settings.fields)
		for i := range fields {
			set(&fields[i].input)
		}
		m.settings.fields = fields
	}
}

var near256 = map[[3]uint8]xansi.IndexedColor{}

// nearest256 maps an RGB color to the closest xterm color 16–255 (the cube
// and the grays, whose values are fixed); indexed colors stay as they are.
func nearest256(c color.Color) color.Color {
	switch c.(type) {
	case nil, xansi.BasicColor, xansi.IndexedColor:
		return c
	}
	r, g, b, _ := c.RGBA()
	key := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	if i, ok := near256[key]; ok {
		return i
	}
	best, bestD := 16, -1
	for i := 16; i < 256; i++ {
		cr, cg, cb := xterm256(i)
		dr, dg, db := int(key[0])-cr, int(key[1])-cg, int(key[2])-cb
		// weighted: the eye is most sensitive to green
		d := 2*dr*dr + 4*dg*dg + 3*db*db
		if bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	near256[key] = xansi.IndexedColor(best)
	return xansi.IndexedColor(best)
}

func xterm256(i int) (r, g, b int) {
	if i >= 232 {
		v := 8 + (i-232)*10
		return v, v, v
	}
	i -= 16
	steps := [6]int{0, 95, 135, 175, 215, 255}
	return steps[i/36], steps[i/6%6], steps[i%6]
}
