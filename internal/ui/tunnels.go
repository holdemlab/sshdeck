package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/tunnel"
)

// tunnelTickMsg refreshes the tunnels panel while it is open.
type tunnelTickMsg struct{}

func tunnelTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tunnelTickMsg{} })
}

// tunnelKey handles keys while the tunnels panel is open.
func (m Model) tunnelKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := m.activeTab()
	if t == nil {
		m.overlay = overlayNone
		return m, nil
	}
	if m.tunAdding && m.tunAsk {
		m.tunAsk = false
		switch k.String() {
		case "y":
			m.tunAdding, m.tunErr = false, ""
			m.tunInput.Blur()
			return m, nil
		case "s", "enter":
			// fall through to the enter handling below
			k = tea.KeyPressMsg{Code: tea.KeyEnter}
		default:
			return m, nil
		}
	}
	if m.tunAdding {
		switch k.String() {
		case "esc":
			if strings.TrimSpace(m.tunInput.Value()) != "" {
				m.tunAsk = true
				return m, nil
			}
			m.tunAdding, m.tunErr = false, ""
			m.tunInput.Blur()
			return m, nil
		case "enter":
			sp, err := tunnel.Parse(m.tunInput.Value())
			if err == nil {
				err = t.addForward(sp)
			}
			if err != nil {
				m.tunErr = err.Error()
				return m, nil
			}
			m.tunAdding, m.tunErr = false, ""
			m.tunInput.Blur()
			m.tunCursor = len(t.forwardList()) - 1
			return m, nil
		}
		var cmd tea.Cmd
		m.tunInput, cmd = m.tunInput.Update(k)
		return m, cmd
	}

	list := t.forwardList()
	m.tunErr = ""
	switch k.String() {
	case "esc", "q", "t":
		m.overlay = overlayNone
	case "up", "k":
		m.tunCursor = max(m.tunCursor-1, 0)
	case "down", "j":
		m.tunCursor = min(m.tunCursor+1, max(len(list)-1, 0))
	case "space", "enter":
		if m.tunCursor < len(list) {
			if err := t.toggleForward(list[m.tunCursor]); err != nil {
				m.tunErr = err.Error()
			}
		}
	case "a", "n":
		m.tunAdding = true
		m.tunInput.SetValue("")
		return m, m.tunInput.Focus()
	case "x", "d", "delete", "backspace":
		if m.tunCursor < len(list) {
			t.removeForward(list[m.tunCursor])
			m.tunCursor = max(min(m.tunCursor, len(list)-2), 0)
		}
	case "w", "s":
		if err := m.saveForwards(t); err != nil {
			m.tunErr = err.Error()
		} else {
			m.setStatus(true, "saved tunnels of %s to %s", t.session.Name, m.opts.ConfigPath)
		}
	}
	return m, nil
}

// saveForwards writes the tab's current forwards into its session in
// sessions.yaml, so they start automatically next time.
func (m *Model) saveForwards(t *Tab) error {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		return err
	}
	s := cfg.Session(t.session.Path, t.session.Name)
	if s == nil {
		return fmt.Errorf("%s", T("session %s is no longer in %s", t.session.ID(), m.opts.ConfigPath))
	}
	s.Forwards = nil
	for _, f := range t.forwardList() {
		s.Forwards = append(s.Forwards, f.Spec.String())
		f.FromConfig = true
	}
	if err := config.Save(m.opts.ConfigPath, cfg); err != nil {
		return err
	}
	m.cfg = cfg
	t.session.Forwards = s.Forwards
	return nil
}

func (m Model) renderTunnels(t *Tab) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(T("Tunnels — %s", t.session.Name)) + "\n")
	state, _ := t.getState()
	switch state {
	case stateConnecting:
		b.WriteString(warnStyle.Render(T("connecting… tunnels start once connected")) + "\n")
	case stateClosed:
		b.WriteString(errStyle.Render(T("not connected — tunnels start after reconnect (r)")) + "\n")
	default:
		b.WriteString(mutedStyle.Render(T("via")+" "+t.routeString()) + "\n")
	}
	b.WriteString("\n")

	list := t.forwardList()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render(T("No tunnels. Press a to add one, e.g.")) + "\n")
		b.WriteString(mutedStyle.Render(T("  L 5432:db.internal:5432   local port → service behind the server")) + "\n")
		b.WriteString(mutedStyle.Render(T("  D 1080                    SOCKS5 proxy through the server")) + "\n")
		b.WriteString(mutedStyle.Render(T("  R 9000:localhost:3000     server port → service on this machine")) + "\n")
	}
	const width = 62
	for i, f := range list {
		st := f.Stats()
		var icon, info string
		switch st.State {
		case tunnel.Listening:
			icon = okStyle.Render("●")
			info = T("%d open · %d total · %s", st.Active, st.Total, humanBytes(st.Bytes))
			if f.Spec.BindPort == 0 && st.Addr != "" {
				info = T("on %s", st.Addr) + " · " + info
			}
		case tunnel.Failed:
			icon = errStyle.Render("✗")
			info = errStyle.Render(fmt.Sprint(st.Err))
		default:
			icon = mutedStyle.Render("○")
			info = mutedStyle.Render(T("off"))
			if f.Wanted.Load() {
				info = mutedStyle.Render(T("waiting for connection"))
			}
		}
		line := fmt.Sprintf("%s %c  %s", icon, f.Spec.Kind, f.Spec.Describe())
		if !f.FromConfig {
			line += mutedStyle.Render(T("  (unsaved)"))
		}
		line = ansi.Truncate(line, width, "…")
		line += strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
		if i == m.tunCursor && !m.tunAdding {
			line = selectedStyle.Render(ansi.Strip(line))
		}
		b.WriteString(line + "\n")
		b.WriteString("      " + ansi.Truncate(info, width-6, "…") + "\n")
	}
	b.WriteString("\n")
	if m.tunAdding && m.tunAsk {
		b.WriteString(m.tunInput.View() + "\n")
		b.WriteString(prefixStyle.Render(T("Discard this tunnel?  y discard · s add it · n keep editing")) + "\n")
	} else if m.tunAdding {
		b.WriteString(m.tunInput.View() + "\n")
		b.WriteString(mutedStyle.Render(T("enter add · esc cancel · bind defaults to 127.0.0.1, *:port for all interfaces")) + "\n")
	} else {
		b.WriteString(mutedStyle.Render(T("space on/off · a add · x remove · w save to session · esc close")) + "\n")
	}
	if m.tunErr != "" {
		b.WriteString(errStyle.Render("✗ "+m.tunErr) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
