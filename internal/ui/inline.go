package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holdemlab/sshdeck/internal/config"
)

// Quick tree editing: create a folder or rename right in the tree (no
// form), and move sessions/folders with cut & paste.

type inlineMode int

const (
	inlineNone inlineMode = iota
	inlineNewFolder
	inlineRename
)

type inlineEdit struct {
	mode   inlineMode
	parent []string // folder the new folder goes into / the renamed item lives in
	target node     // renamed item
	input  textinput.Model
	orig   string // value when the input opened
	ask    bool   // "discard?" is showing
}

func newInlineInput(value, placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	in.SetValue(value)
	in.CursorEnd()
	return in
}

// containerOf is the folder an action on n applies to: a folder itself, or
// the folder a session lives in.
func containerOf(n node, ok bool) []string {
	if !ok {
		return nil
	}
	return append([]string{}, n.path...)
}

// startNewFolder opens an inline input for a folder inside the selection.
func (m *Model) startNewFolder() tea.Cmd {
	n, ok := m.current()
	m.inline = inlineEdit{mode: inlineNewFolder, parent: containerOf(n, ok),
		input: newInlineInput("", T("name · a/b nested · /name top level")), orig: ""}
	if ok && n.kind == folderNode && m.collapsed[n.key] {
		m.setCollapsed(n.key, false) // show where it goes
	}
	m.setStatus(true, "New folder in %s — /name puts it at the top level, a/b/c nests", displayPath(m.inline.parent))
	return m.inline.input.Focus()
}

// startRename opens an inline input over the selected row.
func (m *Model) startRename() tea.Cmd {
	n, ok := m.current()
	if !ok {
		m.setStatus(false, "nothing selected")
		return nil
	}
	parent := n.path
	if n.kind == folderNode {
		parent = n.path[:len(n.path)-1]
	}
	m.inline = inlineEdit{mode: inlineRename, parent: append([]string{}, parent...), target: n,
		input: newInlineInput(n.name, T("new name")), orig: n.name}
	return m.inline.input.Focus()
}

func (m Model) updateInline(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && m.inline.ask {
		m.inline.ask = false
		m.status = ""
		switch k.String() {
		case "y":
			m.inline = inlineEdit{}
			return m, nil
		case "s", "enter":
			if err := m.applyInline(); err != nil {
				m.setStatus(false, "%v", err)
				return m, nil
			}
			m.inline = inlineEdit{}
		}
		return m, nil // anything else: keep editing
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			if strings.TrimSpace(m.inline.input.Value()) != strings.TrimSpace(m.inline.orig) {
				m.inline.ask = true
				m.status, m.statusOK = T("Discard?  y discard · enter save · n keep editing"), false
				return m, nil
			}
			m.inline = inlineEdit{}
			return m, nil
		case "enter":
			if err := m.applyInline(); err != nil {
				m.setStatus(false, "%v", err)
				return m, nil // keep the input open to fix it
			}
			m.inline = inlineEdit{}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.inline.input, cmd = m.inline.input.Update(msg)
	return m, cmd
}

func (m *Model) applyInline() error {
	value := strings.TrimSpace(m.inline.input.Value())
	switch m.inline.mode {
	case inlineNewFolder:
		names := config.SplitPath(value)
		if len(names) == 0 {
			return fmt.Errorf("%s", T("type a folder name"))
		}
		parent := m.inline.parent
		if strings.HasPrefix(value, "/") {
			parent = nil // "/name": from the top level, wherever the cursor is
		}
		path := append(append([]string{}, parent...), names...)
		cfg, err := config.Load(m.opts.ConfigPath)
		if err != nil {
			return err
		}
		if err := cfg.AddFolder(path, config.Defaults{}); err != nil {
			return err
		}
		if err := config.Save(m.opts.ConfigPath, cfg); err != nil {
			return err
		}
		m.cfg = cfg
		m.filter.SetValue("")
		m.refreshRows()
		m.selectKey("f:" + strings.Join(path, "/"))
		m.setStatus(true, "created folder %s — n adds a session in it", strings.Join(path, "/"))

	case inlineRename:
		if value == "" || strings.Contains(value, "/") {
			return fmt.Errorf("%s", T("name must not be empty or contain /"))
		}
		t := m.inline.target
		if value == t.name {
			return nil
		}
		if t.kind == folderNode {
			return m.moveFolder(t.path, append(append([]string{}, m.inline.parent...), value))
		}
		cp := *t.session
		cp.Name = value
		if err := m.saveSession(t.path, &cp, &sessionRef{path: t.path, name: t.name}); err != nil {
			return err
		}
		m.refreshRows()
		m.selectKey("s:" + strings.Join(append(append([]string{}, t.path...), value), "/"))
		m.setStatus(true, "renamed to %s", value)
	}
	return nil
}

// moveFolder renames/moves a folder, keeping its settings.
func (m *Model) moveFolder(from, to []string) error {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		return err
	}
	f := cfg.FolderAt(from)
	if f == nil {
		return fmt.Errorf("%s", T("folder %s not found", strings.Join(from, "/")))
	}
	if err := cfg.UpdateFolder(from, to, f.Defaults); err != nil {
		return err
	}
	if err := config.Save(m.opts.ConfigPath, cfg); err != nil {
		return err
	}
	oldID, newID := strings.Join(from, "/"), strings.Join(to, "/")
	m.cfg = cfg
	m.refreshTabSessions(func(id string) string {
		if strings.HasPrefix(id, oldID+"/") {
			return newID + strings.TrimPrefix(id, oldID)
		}
		return id
	})
	m.moveCollapsed(oldID, newID, false) // keep fold state under the new name
	m.refreshRows()
	m.selectKey("f:" + newID)
	m.setStatus(true, "moved %s → %s", oldID, displayPath(to))
	return nil
}

func displayPath(p []string) string {
	if len(p) == 0 {
		return T("top level")
	}
	return strings.Join(p, "/")
}

// clipboard holds a cut (move) or copied item until it is pasted.
type clipboard struct {
	node
	copy bool
}

// cut marks the selection for moving.
func (m *Model) cut() {
	n, ok := m.current()
	if !ok {
		m.setStatus(false, "nothing selected")
		return
	}
	m.clip = &clipboard{node: n}
	m.setStatus(true, "cut %s — select a folder and press p (P = top level, esc = cancel)", n.id())
}

// yank marks the selection for copying.
func (m *Model) yank() {
	n, ok := m.current()
	if !ok {
		m.setStatus(false, "nothing selected")
		return
	}
	m.clip = &clipboard{node: n, copy: true}
	m.setStatus(true, "copied %s — select a folder and press p (P = top level, esc = done)", n.id())
}

// paste moves or copies the clipboard item into the selected folder
// (top=true: top level).
func (m *Model) paste(top bool) {
	if m.clip == nil {
		m.setStatus(false, "nothing to paste — press x (cut) or y (copy) first")
		return
	}
	var dest []string
	if !top {
		n, ok := m.current()
		dest = containerOf(n, ok)
	}
	if m.clip.copy {
		m.pasteCopy(m.clip.node, dest)
		return // the clipboard stays: paste again for more copies
	}
	c := m.clip.node
	var err error
	if c.kind == folderNode {
		if strings.HasPrefix(strings.Join(dest, "/")+"/", c.id()+"/") {
			m.setStatus(false, "cannot move %s into itself", c.id())
			return
		}
		err = m.moveFolder(c.path, append(append([]string{}, dest...), c.name))
	} else {
		if strings.Join(dest, "/") == strings.Join(c.path, "/") {
			m.clip = nil
			m.setStatus(true, "%s is already in %s", c.name, displayPath(dest))
			return
		}
		cfg := m.cfg.Session(c.path, c.name)
		if cfg == nil {
			err = fmt.Errorf("%s no longer exists", c.id())
		} else {
			cp := *cfg
			err = m.saveSession(dest, &cp, &sessionRef{path: c.path, name: c.name})
			if err == nil {
				m.refreshRows()
				m.selectKey("s:" + strings.Join(append(append([]string{}, dest...), c.name), "/"))
				m.setStatus(true, "moved %s → %s", c.name, displayPath(dest))
			}
		}
	}
	if err != nil {
		m.setStatus(false, "%v", err)
		return
	}
	m.clip = nil
}

// pasteCopy copies a session or folder into dest.
func (m *Model) pasteCopy(c node, dest []string) {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		m.setStatus(false, "%v", err)
		return
	}
	var key, what string
	if c.kind == folderNode {
		var newPath []string
		newPath, err = cfg.CopyFolder(c.path, dest)
		key, what = "f:"+strings.Join(newPath, "/"), strings.Join(newPath, "/")
		if err == nil {
			defer m.moveCollapsed(c.id(), strings.Join(newPath, "/"), true) // the copy folds like the original
		}
	} else {
		var name string
		name, err = cfg.CopySession(c.path, c.name, dest)
		p := strings.Join(append(append([]string{}, dest...), name), "/")
		key, what = "s:"+p, p
	}
	if err == nil {
		err = cfg.Validate()
	}
	if err == nil {
		err = config.Save(m.opts.ConfigPath, cfg)
	}
	if err != nil {
		m.setStatus(false, "copy failed: %v", err)
		return
	}
	m.cfg = cfg
	m.filter.SetValue("")
	m.refreshRows()
	m.selectKey(key)
	m.setStatus(true, "copied to %s — p pastes another copy, esc when done", what)
}

// duplicate copies the selection next to itself: a session opens in the
// form to adjust it, a folder is copied and opened for renaming.
func (m *Model) duplicate() tea.Cmd {
	n, ok := m.current()
	if !ok {
		m.setStatus(false, "nothing selected")
		return nil
	}
	if n.kind == sessionNode {
		cp := *n.session
		cp.Forwards = append([]string(nil), n.session.Forwards...)
		cp.Name = m.cfg.FreeName(n.path, n.name)
		m.form = newForm(T("Copy of %s", n.name), n.path, &cp, nil)
		m.side = sideForm
		return nil
	}
	m.pasteCopy(n, n.path[:len(n.path)-1])
	if !m.statusOK {
		return nil
	}
	m.setStatus(true, "copied %s — type a new name (enter keeps it)", n.id())
	return m.startRename()
}

func (m *Model) selectKey(key string) {
	for i, n := range m.rows {
		if n.key == key {
			m.cursor = i
			m.clampCursor()
			return
		}
	}
}

// inlineRow renders the input for a new folder below the selected row.
func (m Model) inlineRow(depth, width int) string {
	prefix := strings.Repeat("  ", depth) + "+ "
	return ansi.Truncate(prefix+m.inline.input.View(), width, "")
}
