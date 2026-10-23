package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holdemlab/sshdeck/internal/config"
)

// "Copy to…" (Y) and "Move to…" (X): a list of folders to pick the
// destination from, with the top level first. Typing filters the list.

type destPicker struct {
	node  node
	copy  bool
	all   [][]string // every possible destination; nil = top level
	shown [][]string // after the filter
	query string
	sel   int
	top   int // first visible line
}

// openDestPicker starts "Copy to…" (copy) or "Move to…" for the selection.
func (m *Model) openDestPicker(copy bool) {
	n, ok := m.current()
	if !ok || n.kind == localNode {
		m.setStatus(false, "nothing selected")
		return
	}
	d := destPicker{node: n, copy: copy, all: [][]string{nil}}
	var walk func(path []string, folders []*config.Folder)
	walk = func(path []string, folders []*config.Folder) {
		for _, f := range folders {
			p := append(append([]string{}, path...), f.Name)
			// a folder cannot move into itself
			if !copy && n.kind == folderNode && strings.HasPrefix(strings.Join(p, "/")+"/", n.id()+"/") {
				continue
			}
			d.all = append(d.all, p)
			walk(p, f.Folders)
		}
	}
	walk(nil, m.cfg.Folders)
	d.filter("")
	// start at where it is now
	here := strings.Join(containerOf(n, true), "/")
	if n.kind == folderNode {
		here = strings.Join(n.path[:len(n.path)-1], "/")
	}
	for i, p := range d.shown {
		if strings.Join(p, "/") == here {
			d.sel = i
		}
	}
	m.dest = d
	m.overlay = overlayDest
}

func (d *destPicker) filter(q string) {
	d.query = q
	d.shown = d.shown[:0]
	q = strings.ToLower(q)
	for _, p := range d.all {
		if q == "" || strings.Contains(strings.ToLower(displayPath(p)), q) {
			d.shown = append(d.shown, p)
		}
	}
	d.sel, d.top = 0, 0
}

// visible is how many destinations fit on the screen.
func (m Model) destVisible() int { return max(m.height-12, 3) }

func (d *destPicker) scrollTo(vis int) {
	d.sel = min(max(d.sel, 0), max(len(d.shown)-1, 0))
	if d.sel < d.top {
		d.top = d.sel
	}
	if d.sel >= d.top+vis {
		d.top = d.sel - vis + 1
	}
}

func (m Model) destKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := &m.dest
	switch k.String() {
	case "esc":
		if d.query != "" {
			d.filter("")
			return m, nil
		}
		m.overlay = overlayNone
	case "up", "ctrl+p", "shift+tab":
		d.sel--
	case "down", "ctrl+n", "tab":
		d.sel++
	case "pgup":
		d.sel -= m.destVisible()
	case "pgdown":
		d.sel += m.destVisible()
	case "home":
		d.sel = 0
	case "end":
		d.sel = len(d.shown) - 1
	case "enter":
		return m.pickDest(d.sel)
	case "backspace":
		if r := []rune(d.query); len(r) > 0 {
			d.filter(string(r[:len(r)-1]))
		}
	default:
		if k.Text != "" {
			d.filter(d.query + k.Text)
		}
	}
	d.scrollTo(m.destVisible())
	return m, nil
}

// pickDest copies or moves the item into destination i.
func (m Model) pickDest(i int) (tea.Model, tea.Cmd) {
	d := m.dest
	if i < 0 || i >= len(d.shown) {
		return m, nil
	}
	m.overlay = overlayNone
	dest := append([]string{}, d.shown[i]...)
	if d.copy {
		m.pasteCopy(d.node, dest)
		if m.statusOK {
			m.setStatus(true, "copied %s → %s", d.node.id(), displayPath(dest))
		}
	} else {
		m.moveNode(d.node, dest)
	}
	return m, nil
}

func (m Model) renderDestPicker() string {
	d := m.dest
	var b strings.Builder
	title := T("Copy %s to…", d.node.id())
	if !d.copy {
		title = T("Move %s to…", d.node.id())
	}
	b.WriteString(titleStyle.Render(title) + "\n")
	if d.query != "" {
		b.WriteString(prefixStyle.Render("/"+d.query) + "\n\n")
	} else {
		b.WriteString(mutedStyle.Render(T("type to filter")) + "\n\n")
	}
	vis := m.destVisible()
	width := 40
	for _, p := range d.shown {
		width = max(width, lipgloss.Width(displayPath(p))+4)
	}
	width = min(width, m.width-12)
	if len(d.shown) == 0 {
		b.WriteString(mutedStyle.Render(T("no matches")) + "\n")
	}
	for i := d.top; i < len(d.shown) && i < d.top+vis; i++ {
		p := d.shown[i]
		label := "  " + strings.Repeat("  ", max(len(p)-1, 0)) + "▸ " + lastName(p)
		if d.query != "" || len(p) == 0 {
			label = "  " + displayPath(p)
		}
		label = ansi.Truncate(label, width, "…")
		label += strings.Repeat(" ", max(width-lipgloss.Width(label), 0))
		if i == d.sel {
			label = selectedStyle.Render(label)
		}
		b.WriteString(label + "\n")
	}
	if more := len(d.shown) - d.top - vis; more > 0 {
		b.WriteString(mutedStyle.Render(T("… %d more", more)) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render(T("↑↓ choose · enter here · esc cancel")))
	return b.String()
}

func lastName(p []string) string {
	if len(p) == 0 {
		return T("top level")
	}
	return p[len(p)-1]
}

// destMouse: click a destination to pick it, wheel to scroll, a click
// outside the box cancels.
func (m Model) destMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	box := overlayStyle.Render(m.renderDestPicker())
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x0, y0 := max((m.width-w)/2, 0), max((m.height-h)/2, 0)
	ev := msg.Mouse()
	d := &m.dest
	switch e := msg.(type) {
	case tea.MouseWheelMsg:
		if e.Button == tea.MouseWheelUp {
			d.sel -= wheelLines
		} else if e.Button == tea.MouseWheelDown {
			d.sel += wheelLines
		}
		d.scrollTo(m.destVisible())
	case tea.MouseClickMsg:
		if ev.X < x0 || ev.X >= x0+w || ev.Y < y0 || ev.Y >= y0+h {
			m.overlay = overlayNone
			return m, nil
		}
		// border + padding (2 lines), title, filter line, blank line
		i := d.top + ev.Y - y0 - 5
		if e.Button == tea.MouseLeft && i >= d.top && i < len(d.shown) && i < d.top+m.destVisible() {
			return m.pickDest(i)
		}
	}
	return m, nil
}
