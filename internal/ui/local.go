package ui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/localshell"
)

// The "local" entry: always first in the tree (unless turned off with
// local_shell: false), it opens a tab with a shell on this computer. It is
// not part of sessions.yaml, so it cannot be renamed, moved or deleted; its
// command and colors live in config.yaml.

const localName = "local"

func (m Model) localEnabled() bool {
	return m.opts.Settings.LocalShell && localshell.Supported()
}

// localRow is the tree row of the local entry.
func localRow() node {
	return node{kind: localNode, name: localName, key: "l:" + localName}
}

// withLocal puts the local entry in front of the tree rows, or, while
// searching, in front of the matches if it matches too.
func (m Model) withLocal(rows []node, filter string) []node {
	if !m.localEnabled() {
		return rows
	}
	if q := strings.ToLower(strings.TrimSpace(filter)); q != "" && !strings.Contains(localName, q) && !strings.Contains(strings.ToLower(T("this computer")), q) {
		return rows
	}
	return append([]node{localRow()}, rows...)
}

// localSession is what a local tab carries as its "session": the name and
// the color mark from the settings.
func localSession(s config.Settings) config.Resolved {
	r := config.Resolved{}
	r.Name = localName
	r.Color, r.Fill = s.LocalColor, s.LocalFill
	return r
}

// openLocalTab opens a tab with a local shell.
func (m *Model) openLocalTab() {
	if !localshell.Supported() {
		m.setStatus(false, "%v", localshell.ErrUnsupported)
		return
	}
	r := m.termRect()
	m.nextID++
	t := newTab(m.nextID, localSession(m.opts.Settings), r.Dx(), r.Dy(), m.opts.Settings.Scrollback, m.opts.Notifier, false)
	t.local = true
	m.tabs = append(m.tabs, t)
	m.active = len(m.tabs) - 1
	m.focus = focusTerm
	m.scrolling = false
	m.connectTab(t)
}

// hasOpenLocalTab reports whether a local tab is running.
func (m Model) hasOpenLocalTab() bool {
	for _, t := range m.tabs {
		if t.local && t.live() {
			return true
		}
	}
	return false
}

// localKey handles the tree keys that act on the selected entry when it is
// the local one; ok is false for keys that work as usual (moving, search,
// new session or folder at the top level…).
func (m Model) localKey(k tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch k.String() {
	case "enter", "right", "l":
		m.openLocalTab()
	case "t":
		m.setStatus(false, "tunnels need an SSH session; the local shell has none")
	case "e":
		m.openSettings()
		m.settings.setFocus(sLocal)
		m.setStatus(true, "the local shell is set up in the settings")
	case "C":
		m.openColorDialog()
	case "R", "f2", "x", "y", "c", "d", "delete":
		m.setStatus(false, "the local shell is built in; hide it in the settings (local_shell)")
	default:
		return m, nil, false
	}
	return m, nil, true
}

// localDetails are the lines under the tree for the local entry.
func (m Model) localDetails() []string {
	host, _ := os.Hostname()
	lines := []string{T("this computer") + mutedStyle.Render(" "+host)}
	lines = append(lines, mutedStyle.Render(T("runs")+" ")+localshell.Describe(m.opts.Settings.LocalCommand))
	return lines
}
