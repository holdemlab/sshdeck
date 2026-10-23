package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/holdemlab/sshdeck/internal/config"
)

// Mouse support (setting mouse: true):
//   - tab bar: click switches tabs, middle click closes one;
//   - tree: click selects, double click opens a session or folds a folder,
//     the wheel scrolls; a click on "/ search" starts a search;
//   - the sidebar edge can be dragged to change its width (saved);
//   - terminal: programs that turned mouse reporting on (vim, htop, mc,
//     tmux) get clicks, drags and the wheel; otherwise the wheel scrolls the
//     history, or sends arrow keys in full-screen programs;
//   - help: the wheel scrolls.
// Text is selected inside a tab by dragging (selection.go); the terminal's
// own selection, across the whole window, works with Shift held.

const (
	doubleClick = 400 * time.Millisecond
	wheelLines  = 3
)

type mouseState struct {
	lastClick time.Time
	lastRow   int  // tree row of the last click
	dragging  bool // the sidebar edge is being dragged
	termDown  bool // a button went down in the terminal: its release goes there too
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.opts.Settings.Mouse {
		return m, nil
	}
	ev := msg.Mouse()

	// a drag of the sidebar edge owns the mouse until the button is released
	if m.mouse.dragging {
		switch msg.(type) {
		case tea.MouseMotionMsg:
			m.setSidebarWidth(ev.X + 1)
		case tea.MouseReleaseMsg:
			m.mouse.dragging = false
			m.saveSidebarWidth()
		}
		return m, nil
	}

	switch {
	case m.overlay == overlayHelp:
		if w, ok := msg.(tea.MouseWheelMsg); ok {
			if w.Button == tea.MouseWheelUp {
				m.help.vp.ScrollUp(wheelLines)
			} else if w.Button == tea.MouseWheelDown {
				m.help.vp.ScrollDown(wheelLines)
			}
		}
		return m, nil
	case m.overlay != overlayNone, m.settingsOpen, m.prefixArmed:
		return m, nil // dialogs are driven by the keyboard
	}

	tr := m.termRect()
	sw := m.sidebarWidth()
	switch {
	case ev.Y == 0:
		if c, ok := msg.(tea.MouseClickMsg); ok {
			return m.clickTabBar(c)
		}
	case m.mouse.termDown || (ev.X >= tr.Min.X && ev.Y >= tr.Min.Y && ev.Y < tr.Max.Y):
		return m.mouseTerminal(msg, ev.X-tr.Min.X, ev.Y-tr.Min.Y)
	case sw > 0 && ev.X == sw-1 && ev.Y < m.height-1:
		if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft {
			m.mouse.dragging = true
		}
	case sw > 0 && ev.X < sw && ev.Y < m.height-1:
		return m.mouseSidebar(msg, ev)
	}
	return m, nil
}

// clickTabBar: left click switches to a tab, middle click closes it.
func (m Model) clickTabBar(c tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	for i, sp := range m.tabSpans() {
		if c.X < sp[0] || c.X >= sp[1] {
			continue
		}
		switch c.Button {
		case tea.MouseLeft:
			m.switchTab(i)
		case tea.MouseMiddle:
			m.active = i
			if m.tabs[i].live() && m.opts.Settings.ConfirmClose {
				m.overlay = overlayConfirmClose
			} else {
				m.closeTab(i)
			}
		}
		return m, nil
	}
	return m, nil
}

// mouseSidebar handles the tree: rows start below the title and search lines.
func (m Model) mouseSidebar(msg tea.MouseMsg, ev tea.Mouse) (tea.Model, tea.Cmd) {
	if m.side == sideForm || m.inline.mode != inlineNone {
		return m, nil
	}
	const firstRow = 3 // tab bar, "Sessions", "/ search"
	switch e := msg.(type) {
	case tea.MouseWheelMsg:
		switch e.Button {
		case tea.MouseWheelUp:
			m.cursor -= wheelLines
		case tea.MouseWheelDown:
			m.cursor += wheelLines
		}
		m.clampCursor()
	case tea.MouseClickMsg:
		if e.Button != tea.MouseLeft {
			return m, nil
		}
		m.focus = focusSidebar
		if ev.Y == firstRow-1 { // the search line
			m.side = sideFilter
			m.cursor = 0
			m.refreshRows()
			return m, m.filter.Focus()
		}
		line := ev.Y - firstRow
		if line < 0 || line >= m.treeHeight() {
			return m, nil
		}
		row := m.offset + line
		if row >= len(m.rows) {
			return m, nil
		}
		now := time.Now()
		double := row == m.mouse.lastRow && now.Sub(m.mouse.lastClick) < doubleClick
		m.mouse.lastClick, m.mouse.lastRow = now, row
		m.cursor = row
		m.clampCursor()
		n := m.rows[row]
		// a click on a folder's arrow folds it at once
		onArrow := n.kind == folderNode && ev.X-1 >= n.depth*2 && ev.X-1 < n.depth*2+2
		switch {
		case onArrow:
			m.setCollapsed(n.key, !m.collapsed[n.key])
			m.mouse.lastClick = time.Time{}
		case double && n.kind == localNode:
			m.openLocalTab()
		case double:
			m.activate(false)
			m.mouse.lastClick = time.Time{}
		}
	}
	return m, nil
}

// mouseTerminal sends the event to the program in the tab if it asked for
// the mouse; otherwise the wheel scrolls.
func (m Model) mouseTerminal(msg tea.MouseMsg, x, y int) (tea.Model, tea.Cmd) {
	t := m.activeTab()
	if t == nil {
		return m, nil
	}
	if _, ok := msg.(tea.MouseClickMsg); ok {
		m.focus = focusTerm
	}
	ev := msg.Mouse()
	w, h := t.emu.Width(), t.emu.Height()
	x, y = min(max(x, 0), w-1), min(max(y, 0), h-1)
	pos := uv.Mouse{X: x, Y: y, Button: ev.Button, Mod: ev.Mod}

	// programs that use the mouse get it; Alt + drag selects text anyway
	if state, _ := t.getState(); state == stateConnected && t.wantsMouse() && !m.scrolling && ev.Mod&tea.ModAlt == 0 {
		switch msg.(type) {
		case tea.MouseClickMsg:
			m.mouse.termDown = true
			t.emu.SendMouse(uv.MouseClickEvent(pos))
		case tea.MouseReleaseMsg:
			m.mouse.termDown = false
			t.emu.SendMouse(uv.MouseReleaseEvent(pos))
		case tea.MouseMotionMsg:
			t.emu.SendMouse(uv.MouseMotionEvent(pos))
		case tea.MouseWheelMsg:
			t.emu.SendMouse(uv.MouseWheelEvent(pos))
		}
		t.scroll = 0
		return m, nil
	}
	switch msg.(type) {
	case tea.MouseClickMsg:
		switch ev.Button {
		case tea.MouseLeft:
			m.mouse.termDown = true
			return m, m.selectMouse(t, msg, x, y)
		case tea.MouseMiddle: // paste what was copied last
			if m.lastCopy != "" {
				t.paste(m.lastCopy)
			}
			return m, nil
		}
	case tea.MouseMotionMsg:
		if ev.Button == tea.MouseLeft {
			return m, m.selectMouse(t, msg, x, y)
		}
	case tea.MouseReleaseMsg:
		m.mouse.termDown = false
		return m, m.selectMouse(t, msg, x, y)
	}
	wheel, ok := msg.(tea.MouseWheelMsg)
	if !ok {
		return m, nil
	}
	up := wheel.Button == tea.MouseWheelUp
	if !up && wheel.Button != tea.MouseWheelDown {
		return m, nil
	}
	// full-screen programs without mouse support (less, man, vim): arrows
	if t.emu.IsAltScreen() {
		if state, _ := t.getState(); state == stateConnected {
			k := tea.KeyPressMsg{Code: tea.KeyDown}
			if up {
				k.Code = tea.KeyUp
			}
			for range wheelLines {
				t.sendKey(k)
			}
		}
		return m, nil
	}
	// the shell: scroll the history, like prefix + [
	if up {
		t.scroll += wheelLines
		m.scrolling = true
	} else {
		t.scroll -= wheelLines
	}
	t.scroll = min(t.scroll, t.emu.ScrollbackLen())
	if t.scroll <= 0 {
		t.scroll, m.scrolling = 0, false
	}
	return m, nil
}

// setSidebarWidth resizes the sidebar while its edge is dragged.
func (m *Model) setSidebarWidth(w int) {
	w = min(max(w, 16), min(120, m.width-20))
	if w == m.opts.Settings.SidebarWidth {
		return
	}
	m.opts.Settings.SidebarWidth = w
	m.resizeTabs()
}

// saveSidebarWidth keeps the dragged width in config.yaml.
func (m *Model) saveSidebarWidth() {
	if m.opts.SettingsPath == "" {
		return
	}
	saved, err := config.LoadSettings(m.opts.SettingsPath, false)
	if err != nil {
		m.setStatus(false, "%v", err)
		return
	}
	if saved.SidebarWidth == m.opts.Settings.SidebarWidth {
		return
	}
	saved.SidebarWidth = m.opts.Settings.SidebarWidth
	if err := config.SaveSettings(m.opts.SettingsPath, saved); err != nil {
		m.setStatus(false, "%v", err)
		return
	}
	m.setStatus(true, "sidebar width %d saved", saved.SidebarWidth)
}
