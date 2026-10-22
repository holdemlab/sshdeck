package ui

import (
	"strings"

	"github.com/holdemlab/sshdeck/internal/config"
)

// Folded folders are remembered in state.yaml between runs. Keys in
// m.collapsed are "f:<folder path>".

func (m *Model) loadState() {
	if m.opts.StatePath == "" {
		return
	}
	st, err := config.LoadState(m.opts.StatePath)
	if err != nil {
		m.setStatus(false, "%v — folds not restored", err)
		return
	}
	for _, p := range st.Collapsed {
		m.collapsed["f:"+p] = true
	}
	m.refreshRows()
}

// saveState writes the folded folders that still exist.
func (m *Model) saveState() {
	if m.opts.StatePath == "" {
		return
	}
	var st config.State
	for key, on := range m.collapsed {
		if !on || !strings.HasPrefix(key, "f:") {
			continue
		}
		path := strings.TrimPrefix(key, "f:")
		if m.cfg.FolderAt(config.SplitPath(path)) == nil {
			delete(m.collapsed, key) // folder is gone
			continue
		}
		st.Collapsed = append(st.Collapsed, path)
	}
	if err := config.SaveState(m.opts.StatePath, st); err != nil {
		m.setStatus(false, "cannot save folds: %v", err)
	}
}

// setCollapsed folds or unfolds one folder and remembers it.
func (m *Model) setCollapsed(key string, on bool) {
	if on {
		m.collapsed[key] = true
	} else {
		delete(m.collapsed, key)
	}
	m.refreshRows()
	m.saveState()
}

// collapseAll folds every folder; the cursor goes to the top-level folder
// that contained it.
func (m *Model) collapseAll() {
	cur, ok := m.current()
	for _, n := range buildTree(m.cfg, nil) {
		if n.kind == folderNode {
			m.collapsed[n.key] = true
		}
	}
	m.refreshRows()
	if ok {
		if len(cur.path) > 0 {
			m.selectKey("f:" + cur.path[0])
		} else {
			m.selectKey(cur.key) // top-level session stays selected
		}
	}
	m.saveState()
	m.setStatus(true, "all folders folded — L or * unfolds all")
}

// expandAll unfolds every folder, keeping the cursor on the same row.
func (m *Model) expandAll() {
	cur, ok := m.current()
	m.collapsed = map[string]bool{}
	m.refreshRows()
	if ok {
		m.selectKey(cur.key)
	}
	m.saveState()
	m.setStatus(true, "all folders unfolded — H or - folds all")
}

// moveCollapsed carries fold state from folder oldID (and its subfolders)
// to newID, after a rename/move (copy=false) or a copy (copy=true).
func (m *Model) moveCollapsed(oldID, newID string, copy bool) {
	for key, on := range m.collapsed {
		p := strings.TrimPrefix(key, "f:")
		if !on || !(p == oldID || strings.HasPrefix(p, oldID+"/")) {
			continue
		}
		if !copy {
			delete(m.collapsed, key)
		}
		m.collapsed["f:"+newID+strings.TrimPrefix(p, oldID)] = true
	}
	m.saveState()
}
