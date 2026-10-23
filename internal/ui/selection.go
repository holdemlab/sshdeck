package ui

import (
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// Text selection inside a tab. The terminal's own selection (Shift + drag)
// spans the whole window, sidebar included; this one stays inside the tab:
// drag with the left button, the text is copied to the clipboard when the
// button is released. Double click selects a word, triple click a line.
// Middle click pastes the last copied text into the tab.
//
// Positions are lines of the tab's history: scrollback lines first, then the
// screen, so a selection stays put while the history is scrolled.

type selPos struct{ line, col int }

func (p selPos) before(q selPos) bool {
	return p.line < q.line || (p.line == q.line && p.col < q.col)
}

type selection struct {
	tab      int // tab id; 0: nothing selected
	from, to selPos
	dragging bool
	clicks   int // 1, 2 (word) or 3 (line) for the current click series
	at       time.Time
}

// ordered returns the selection's start and end.
func (s selection) ordered() (selPos, selPos) {
	if s.to.before(s.from) {
		return s.to, s.from
	}
	return s.from, s.to
}

func (s selection) has(t *Tab) bool { return t != nil && s.tab == t.id }

// histPos turns a cell of the tab's view into a history position.
func (m Model) histPos(t *Tab, x, y int) selPos {
	off := 0
	if m.scrolling {
		off = t.scroll
	}
	return selPos{line: t.emu.ScrollbackLen() - off + y, col: x}
}

// histCell returns the cell at a history position, or nil.
func histCell(t *Tab, p selPos) *uv.Cell {
	sb := t.emu.ScrollbackLen()
	if p.line < sb {
		return t.emu.ScrollbackCellAt(p.col, p.line)
	}
	return t.emu.CellAt(p.col, p.line-sb)
}

// lineText is one history line as text, trailing blanks removed.
func lineText(t *Tab, line, from, to int) string {
	var b strings.Builder
	for x := from; x <= to && x < t.emu.Width(); x++ {
		c := histCell(t, selPos{line, x})
		switch {
		case c == nil:
			b.WriteByte(' ')
		case c.Content == "" && c.Width == 0:
			// the second half of a wide character
		case c.Content == "":
			b.WriteByte(' ')
		default:
			b.WriteString(c.Content)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// selectedText is the selection as text, lines joined with \n.
func (m Model) selectedText(t *Tab) string {
	a, b := m.sel.ordered()
	var lines []string
	for line := a.line; line <= b.line; line++ {
		from, to := 0, t.emu.Width()-1
		if line == a.line {
			from = a.col
		}
		if line == b.line {
			to = b.col
		}
		lines = append(lines, lineText(t, line, from, to))
	}
	return strings.Join(lines, "\n")
}

// wordAround widens p to the word under it.
func wordAround(t *Tab, p selPos) (selPos, selPos) {
	isWord := func(x int) bool {
		c := histCell(t, selPos{p.line, x})
		return c != nil && c.Content != "" && strings.TrimSpace(c.Content) != "" && !strings.ContainsAny(c.Content, "\"'`()[]{}<>|;,")
	}
	if !isWord(p.col) {
		return p, p
	}
	from, to := p.col, p.col
	for from > 0 && isWord(from-1) {
		from--
	}
	for to < t.emu.Width()-1 && isWord(to+1) {
		to++
	}
	return selPos{p.line, from}, selPos{p.line, to}
}

// selectMouse handles the left button for selecting; it reports a command
// that copies the text when the button is released.
func (m *Model) selectMouse(t *Tab, msg tea.MouseMsg, x, y int) tea.Cmd {
	p := m.histPos(t, x, y)
	switch msg.(type) {
	case tea.MouseClickMsg:
		now := time.Now()
		clicks := 1
		if m.sel.has(t) && now.Sub(m.sel.at) < doubleClick && m.sel.from.line == p.line {
			clicks = m.sel.clicks%3 + 1
		}
		m.sel = selection{tab: t.id, from: p, to: p, dragging: true, clicks: clicks, at: now}
		switch clicks {
		case 2:
			m.sel.from, m.sel.to = wordAround(t, p)
			m.sel.dragging = false
			return m.copySelection(t)
		case 3:
			m.sel.from, m.sel.to = selPos{p.line, 0}, selPos{p.line, t.emu.Width() - 1}
			m.sel.dragging = false
			return m.copySelection(t)
		}
	case tea.MouseMotionMsg:
		if m.sel.has(t) && m.sel.dragging {
			m.sel.to = p
		}
	case tea.MouseReleaseMsg:
		if m.sel.has(t) && m.sel.dragging {
			m.sel.to = p
			m.sel.dragging = false
			if m.sel.from == m.sel.to {
				m.sel.tab = 0 // a plain click selects nothing
				return nil
			}
			return m.copySelection(t)
		}
	}
	return nil
}

// copySelection puts the selected text on the clipboard: through the
// terminal (OSC 52, works over SSH and tmux) and through the system tools
// when they are there (wl-copy, xclip, xsel, pbcopy).
func (m *Model) copySelection(t *Tab) tea.Cmd {
	text := m.selectedText(t)
	if text == "" {
		m.sel.tab = 0
		return nil
	}
	m.lastCopy = text
	m.setStatus(true, "copied %d characters", len([]rune(text)))
	return tea.Batch(tea.SetClipboard(text), systemCopy(text))
}

// systemCopy copies with the first clipboard tool found, in the background.
func systemCopy(text string) tea.Cmd {
	return func() tea.Msg {
		var tools [][]string
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			tools = append(tools, []string{"wl-copy"})
		}
		tools = append(tools, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"}, []string{"pbcopy"})
		for _, tool := range tools {
			if _, err := exec.LookPath(tool[0]); err != nil {
				continue
			}
			if tool[0] != "pbcopy" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
				continue // no desktop session: OSC 52 has to do
			}
			cmd := exec.Command(tool[0], tool[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if cmd.Start() == nil {
				// wl-copy and xclip stay around to serve the clipboard: don't wait
				go func() { _ = cmd.Wait() }()
				return nil
			}
		}
		return nil
	}
}

// drawSelection highlights the selected cells of the tab's view.
func (m Model) drawSelection(canvas *lipgloss.Canvas, t *Tab, r uv.Rectangle) {
	if !m.sel.has(t) {
		return
	}
	a, b := m.sel.ordered()
	for y := 0; y < r.Dy() && y < t.emu.Height(); y++ {
		line := m.histPos(t, 0, y).line
		if line < a.line || line > b.line {
			continue
		}
		from, to := 0, t.emu.Width()-1
		if line == a.line {
			from = a.col
		}
		if line == b.line {
			to = b.col
		}
		for x := from; x <= to && x < r.Dx(); x++ {
			c := canvas.CellAt(r.Min.X+x, r.Min.Y+y)
			if c == nil {
				continue
			}
			cp := *c
			cp.Style.Attrs ^= uv.AttrReverse
			canvas.SetCell(r.Min.X+x, r.Min.Y+y, &cp)
		}
	}
}
