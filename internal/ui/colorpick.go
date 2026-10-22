package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/holdemlab/sshdeck/internal/config"
)

// The color picker: a strip of swatches, ← → to choose. The first two
// entries are "inherit" (empty: take the folder's mark) and "none" (no mark,
// even if the folder has one). Other colors can be typed as #rrggbb.

var pickOptions = append([]string{"", "none"}, config.ColorNames...)

// pickIndex is the position of v in pickOptions, -1 for a custom #hex.
func pickIndex(v string) int {
	n, _ := config.NormalizeColor(v)
	for i, o := range pickOptions {
		if o == n {
			return i
		}
	}
	return -1
}

// pickStep moves v by dir through the options.
func pickStep(v string, dir int) string {
	i := pickIndex(v)
	if i < 0 {
		i = 0
		if dir < 0 {
			i = 1
		}
	}
	return pickOptions[(i+dir+len(pickOptions))%len(pickOptions)]
}

// pickName is what a value is called on screen.
func pickName(v string) string {
	n, ok := config.NormalizeColor(v)
	switch {
	case !ok:
		return T("not a color: %s", strings.TrimSpace(v))
	case n == "":
		return T("inherit")
	case n == "none":
		return T("none")
	}
	return n
}

// swatch is a two-cell sample of a value.
func swatch(v string) string {
	n, _ := config.NormalizeColor(v)
	switch n {
	case "":
		return mutedStyle.Render("↑ ")
	case "none":
		return mutedStyle.Render("∅ ")
	}
	if c := markColor(n); c != nil {
		return lipgloss.NewStyle().Foreground(c).Render("██")
	}
	return errStyle.Render("??")
}

// pickerStrip draws all options with a caret under the chosen one, plus its
// name; a custom #hex is shown after the strip.
func pickerStrip(v string) string {
	var top, bottom strings.Builder
	sel := pickIndex(v)
	caret := lipgloss.NewStyle().Foreground(accent)
	for i, o := range pickOptions {
		top.WriteString(swatch(o))
		if i == sel {
			bottom.WriteString(caret.Render("▔▔"))
		} else {
			bottom.WriteString("  ")
		}
	}
	if sel < 0 {
		top.WriteString(" " + swatch(v))
		bottom.WriteString(" " + caret.Render("▔▔"))
	}
	return top.String() + "  " + pickName(v) + "\n" + bottom.String()
}

// pickerValue is the compact form: sample and name.
func pickerValue(v string) string {
	return swatch(v) + " " + mutedStyle.Render(pickName(v))
}

// ---- quick color dialog (C in the tree) ----

type colorDialog struct {
	target node      // the folder or session being marked
	values [2]string // color, fill
	row    int       // 0 color, 1 fill
	err    string
}

func (m *Model) openColorDialog() {
	n, ok := m.current()
	if !ok {
		m.setStatus(false, "nothing selected")
		return
	}
	d := m.ownDefaults(n)
	m.colors = colorDialog{target: n, values: [2]string{d.Color, d.Fill}}
	m.overlay = overlayColors
}

// ownDefaults are the settings set on the node itself (not inherited).
func (m Model) ownDefaults(n node) config.Defaults {
	if n.kind == folderNode {
		if f := m.cfg.FolderAt(n.path); f != nil {
			return f.Defaults
		}
		return config.Defaults{}
	}
	if n.session != nil {
		return n.session.Defaults
	}
	return config.Defaults{}
}

func (m Model) colorKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := &m.colors
	switch k.String() {
	case "esc", "q":
		m.overlay = overlayNone
	case "up", "down", "tab", "shift+tab", "k", "j":
		d.row = 1 - d.row
	case "left", "h":
		d.values[d.row] = pickStep(d.values[d.row], -1)
	case "right", "l", "space":
		d.values[d.row] = pickStep(d.values[d.row], 1)
	case "backspace", "delete", "x":
		d.values[d.row] = ""
	case "enter", "ctrl+s":
		if err := m.saveMark(d.target, d.values[0], d.values[1]); err != nil {
			d.err = err.Error()
			return m, nil
		}
		m.overlay = overlayNone
		m.setStatus(true, "colors of %s saved", d.target.id())
	}
	return m, nil
}

// saveMark writes color / fill of a folder or session to sessions.yaml.
func (m *Model) saveMark(n node, colorV, fillV string) error {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		return err
	}
	switch n.kind {
	case folderNode:
		f := cfg.FolderAt(n.path)
		if f == nil {
			return errNotFound(n.id())
		}
		f.Color, f.Fill = colorV, fillV
	default:
		s := cfg.Session(n.path, n.name)
		if s == nil {
			return errNotFound(n.id())
		}
		s.Color, s.Fill = colorV, fillV
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := config.Save(m.opts.ConfigPath, cfg); err != nil {
		return err
	}
	m.cfg = cfg
	m.refreshRows()
	m.refreshTabSessions(func(id string) string { return id })
	return nil
}

func errNotFound(id string) error {
	return fmt.Errorf("%s", T("%s is no longer in %s", id, "sessions.yaml"))
}

func (m Model) renderColorDialog() string {
	d := m.colors
	var b strings.Builder
	b.WriteString(titleStyle.Render(T("Colors of %s", d.target.id())) + "\n\n")
	labels := [2]string{T("Text"), T("Fill")}
	for i := range 2 {
		marker := "  "
		if d.row == i {
			marker = lipgloss.NewStyle().Foreground(accent).Render("▸ ")
		}
		lines := strings.Split(pickerStrip(d.values[i]), "\n")
		b.WriteString(marker + labelStyle.Width(9).Render(labels[i]) + " " + lines[0] + "\n")
		b.WriteString(strings.Repeat(" ", 12) + lines[1] + "\n")
	}
	// preview with inheritance applied
	eff := m.cfg.FolderDefaults(d.target.path)
	if d.target.kind == folderNode && len(d.target.path) > 0 {
		eff = m.cfg.FolderDefaults(d.target.path[:len(d.target.path)-1])
	}
	c, f := d.values[0], d.values[1]
	if c == "" {
		c = eff.Color
	}
	if f == "" {
		f = eff.Fill
	}
	st, _ := mark(c, f)
	b.WriteString("\n" + mutedStyle.Render(T("Preview:")+" ") + chip(st, d.target.name, markColor(f) != nil) + "\n")
	if d.target.kind == folderNode {
		b.WriteString(mutedStyle.Render(T("Sessions and folders inside take these colors unless they have their own.")) + "\n")
	}
	if d.err != "" {
		b.WriteString(errStyle.Render("✗ "+d.err) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render(T("← → color · ↑↓ text / fill · x inherit · enter save · esc cancel")) + "\n")
	b.WriteString(mutedStyle.Render(T("↑ inherit from the folder · ∅ no color · #rrggbb in the edit form (e)")))
	return b.String()
}
