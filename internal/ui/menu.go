package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// The context menu of the tree: right click on an entry (or m on the
// selected one) lists what can be done with it. Every item is one of the
// tree's keys, shown next to it, so the menu also teaches the keys.

type menuItem struct {
	label string // translated
	key   string // the tree key it runs; "" for a separator
}

type contextMenu struct {
	items []menuItem
	sel   int
	x, y  int  // top-left corner on the screen
	empty bool // opened on the empty part of the tree: top-level actions
}

func sep() menuItem { return menuItem{} }

// menuFor lists the actions for a tree entry.
func (m Model) menuFor(n node, ok bool) []menuItem {
	var items []menuItem
	paste := func() {
		if m.clip != nil {
			items = append(items, menuItem{T("Paste here"), "p"})
		}
	}
	switch {
	case !ok:
		items = []menuItem{{T("New session"), "top:n"}, {T("New folder"), "top:N"}}
		if m.clip != nil {
			items = append(items, menuItem{T("Paste at the top level"), "P"})
		}
		return items
	case n.kind == localNode:
		return []menuItem{{T("Open"), "enter"}, sep(), {T("Colors…"), "C"}, {T("Settings…"), "e"}}
	case n.kind == folderNode:
		fold := menuItem{T("Collapse"), "enter"}
		if m.collapsed[n.key] {
			fold.label = T("Expand")
		}
		items = []menuItem{fold, sep(),
			{T("New session here"), "n"}, {T("New folder here"), "N"}, sep(),
			{T("Edit…"), "e"}, {T("Colors…"), "C"}, {T("Rename"), "R"}, sep(),
			{T("Duplicate"), "c"}, {T("Copy"), "y"}, {T("Cut"), "x"}}
		paste()
	default:
		items = []menuItem{{T("Open"), "enter"}, {T("Open tunnels only"), "t"}, sep(),
			{T("Edit…"), "e"}, {T("Colors…"), "C"}, {T("Rename"), "R"}, sep(),
			{T("Duplicate"), "c"}, {T("Copy"), "y"}, {T("Cut"), "x"}}
		paste()
	}
	return append(items, sep(), menuItem{T("Delete…"), "d"})
}

// openMenu shows the menu for the selected entry (or the empty tree) with
// its corner at x, y.
func (m *Model) openMenu(x, y int, empty bool) {
	n, ok := m.current()
	if empty {
		ok = false
	}
	items := m.menuFor(n, ok)
	m.menu = contextMenu{items: items, x: x, y: y, empty: !ok}
	m.overlay = overlayMenu
	m.focus = focusSidebar
}

// menuKey drives the open menu with the keyboard.
func (m Model) menuKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	mn := &m.menu
	switch s := k.String(); s {
	case "esc", "q", "m":
		m.overlay = overlayNone
	case "up", "k", "shift+tab":
		mn.sel = mn.step(-1)
	case "down", "j", "tab":
		mn.sel = mn.step(1)
	case "enter", "space":
		return m.runMenu(mn.sel)
	default:
		for i, it := range mn.items { // the item's own key runs it
			if it.key != "" && displayKey(it.key) == s {
				return m.runMenu(i)
			}
		}
	}
	return m, nil
}

// step moves the selection over the separators.
func (mn contextMenu) step(dir int) int {
	n := len(mn.items)
	for i, s := 0, mn.sel; i < n; i++ {
		s = (s + dir + n) % n
		if mn.items[s].key != "" {
			return s
		}
	}
	return mn.sel
}

// runMenu closes the menu and runs item i.
func (m Model) runMenu(i int) (tea.Model, tea.Cmd) {
	m.overlay = overlayNone
	if i < 0 || i >= len(m.menu.items) {
		return m, nil
	}
	switch key := m.menu.items[i].key; key {
	case "":
		return m, nil
	case "top:n":
		m.form = newForm(T("New session"), nil, nil, nil)
		m.side = sideForm
		return m, nil
	case "top:N":
		m.inline = inlineEdit{mode: inlineNewFolder, parent: nil,
			input: newInlineInput("", T("name · a/b nested · /name top level")), orig: ""}
		m.setStatus(true, "New folder in %s — /name puts it at the top level, a/b/c nests", displayPath(nil))
		return m, m.inline.input.Focus()
	default:
		return m.updateSidebar(keyMsg(key))
	}
}

// keyMsg builds the key press a menu item stands for.
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// displayKey is how an item's key is shown and typed.
func displayKey(key string) string {
	switch key {
	case "enter":
		return "⏎"
	case "top:n":
		return "n"
	case "top:N":
		return "N"
	}
	return key
}

// menuBox renders the menu; its lines line up with the items.
func (m Model) menuBox() string {
	mn := m.menu
	labels := make([]string, len(mn.items))
	w := 0
	for i, it := range mn.items {
		if it.key != "" {
			labels[i] = it.label
			w = max(w, lipgloss.Width(labels[i])+2+lipgloss.Width(displayKey(it.key)))
		}
	}
	w += 2
	var lines []string
	for i, it := range mn.items {
		if it.key == "" {
			lines = append(lines, mutedStyle.Render(strings.Repeat("─", w)))
			continue
		}
		hint := displayKey(it.key)
		text := " " + labels[i] + strings.Repeat(" ", w-2-lipgloss.Width(labels[i])-lipgloss.Width(hint)) + hint + " "
		if i == mn.sel {
			lines = append(lines, selectedStyle.Render(text))
		} else {
			lines = append(lines, " "+labels[i]+strings.Repeat(" ", w-2-lipgloss.Width(labels[i])-lipgloss.Width(hint))+mutedStyle.Render(hint)+" ")
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Render(strings.Join(lines, "\n"))
}

// menuRect is where the menu is drawn: at its corner, moved inside the screen.
func (m Model) menuRect() uv.Rectangle {
	box := m.menuBox()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x := min(m.menu.x, max(m.width-w, 0))
	y := min(m.menu.y, max(m.height-1-h, 0))
	return uv.Rect(x, y, w, h)
}

func (m Model) drawMenu(canvas *lipgloss.Canvas) {
	box := m.menuBox()
	uv.NewStyledString(box).Draw(canvas, m.menuRect())
}

// menuMouse: a click on an item runs it, a click elsewhere closes the menu.
func (m Model) menuMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	c, ok := msg.(tea.MouseClickMsg)
	if !ok {
		return m, nil
	}
	r := m.menuRect()
	in := c.X > r.Min.X && c.X < r.Max.X-1 && c.Y > r.Min.Y && c.Y < r.Max.Y-1
	if !in {
		m.overlay = overlayNone
		if c.Button == tea.MouseRight { // right click elsewhere: a new menu there
			return m.handleMouse(msg)
		}
		return m, nil
	}
	i := c.Y - r.Min.Y - 1
	if i >= 0 && i < len(m.menu.items) && m.menu.items[i].key != "" && c.Button == tea.MouseLeft {
		return m.runMenu(i)
	}
	return m, nil
}

// menuText is the plain menu, for tests.
func (m Model) menuText() string { return ansi.Strip(m.menuBox()) }
