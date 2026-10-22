package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holdemlab/sshdeck/docs"
	"github.com/holdemlab/sshdeck/internal/i18n"
)

// The help screen (prefix + ?) shows docs/help.md rendered as Markdown,
// in a scrollable, searchable viewer.

type helpView struct {
	vp      viewport.Model
	plain   []string // rendered lines without styling, for search
	key     string   // what the current rendering was made for
	search  textinput.Model
	typing  bool // the search line is open
	query   string
	matches []int
	match   int
	err     string
}

// helpMarkdown is help.md with the configured prefix filled in.
func (m Model) helpMarkdown() string {
	md := docs.HelpFor(string(i18n.Current()))
	if p := m.opts.Settings.Prefix; p != "" && p != "ctrl+a" {
		md = strings.ReplaceAll(md, "ctrl+a", p)
	}
	return md
}

// renderHelp (re)renders the Markdown for the current size and theme.
func (m *Model) renderHelp() {
	w, h := m.helpSize()
	innerW, innerH := w-4, h-3 // border + padding; one line for the footer
	key := fmt.Sprintf("%d|%t|%s|%s|%s", innerW, m.darkBG, m.opts.Settings.Prefix, i18n.Current(), themeName)
	hv := &m.help
	if hv.key != key {
		style := glamour.WithStandardStyle("dark")
		if !m.darkBG {
			style = glamour.WithStandardStyle("light")
		}
		if curTheme != nil {
			if cfg, err := curTheme.glamourStyle(); err == nil {
				style = glamour.WithStyles(cfg)
			}
		}
		out := m.helpMarkdown()
		r, err := glamour.NewTermRenderer(style, glamour.WithWordWrap(innerW-2))
		if err == nil {
			out, err = r.Render(out)
		}
		if err != nil {
			hv.err = err.Error()
			out = m.helpMarkdown() // fall back to the raw text
		}
		out = strings.Trim(out, "\n")
		hv.vp = viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(innerH))
		hv.vp.SetContent(out)
		hv.plain = strings.Split(ansi.Strip(out), "\n")
		hv.key = key
		hv.matches = nil
		hv.query = ""
	}
	hv.vp.SetWidth(innerW)
	hv.vp.SetHeight(innerH)
}

func (m *Model) openHelp() {
	m.overlay = overlayHelp
	m.help.typing = false
	m.renderHelp()
	m.help.vp.GotoTop()
	si := textinput.New()
	si.Prompt = "/"
	si.Placeholder = T("search…")
	m.help.search = si
}

func (m Model) helpKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	hv := &m.help
	if hv.typing {
		switch k.String() {
		case "esc":
			hv.typing = false
			return m, nil
		case "enter":
			hv.typing = false
			m.findInHelp(hv.search.Value())
			return m, nil
		}
		var cmd tea.Cmd
		hv.search, cmd = hv.search.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "esc", "q", "?":
		m.overlay = overlayNone
		return m, nil
	case "/":
		hv.typing = true
		hv.search.SetValue("")
		return m, hv.search.Focus()
	case "n":
		m.nextMatch(1)
		return m, nil
	case "N":
		m.nextMatch(-1)
		return m, nil
	case "g", "home":
		hv.vp.GotoTop()
		return m, nil
	case "G", "end":
		hv.vp.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	hv.vp, cmd = hv.vp.Update(k)
	return m, cmd
}

// findInHelp jumps to the first line containing q at or after the view.
func (m *Model) findInHelp(q string) {
	hv := &m.help
	hv.query, hv.matches, hv.match = strings.TrimSpace(q), nil, 0
	if hv.query == "" {
		return
	}
	needle := strings.ToLower(hv.query)
	for i, line := range hv.plain {
		if strings.Contains(strings.ToLower(line), needle) {
			hv.matches = append(hv.matches, i)
		}
	}
	if len(hv.matches) == 0 {
		return
	}
	for i, line := range hv.matches {
		if line >= hv.vp.YOffset() {
			hv.match = i
			break
		}
	}
	hv.vp.SetYOffset(max(hv.matches[hv.match]-1, 0))
}

func (m *Model) nextMatch(dir int) {
	hv := &m.help
	if len(hv.matches) == 0 {
		return
	}
	hv.match = (hv.match + dir + len(hv.matches)) % len(hv.matches)
	hv.vp.SetYOffset(max(hv.matches[hv.match]-1, 0))
}

func (m Model) renderHelpBox() string {
	hv := m.help
	var footer string
	switch {
	case hv.typing:
		footer = hv.search.View()
	case hv.query != "" && len(hv.matches) == 0:
		footer = errStyle.Render(T("not found: %s", hv.query)) + mutedStyle.Render("  · "+T("/ search again · esc close"))
	case hv.query != "":
		footer = warnStyle.Render(fmt.Sprintf("%q %d/%d", hv.query, hv.match+1, len(hv.matches))) +
			mutedStyle.Render("  · "+T("n next · N prev · / search · esc close"))
	default:
		footer = mutedStyle.Render(T("↑↓ pgup pgdn g G scroll · / search · esc close · %3.0f%%", hv.vp.ScrollPercent()*100))
	}
	if hv.err != "" {
		footer = errStyle.Render("markdown: "+hv.err) + "  " + footer
	}
	w, h := m.helpSize()
	body := hv.vp.View() + "\n" + ansi.Truncate(footer, w-4, "…")
	return helpBox.Width(w).Height(h).Render(body)
}
