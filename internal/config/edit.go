package config

import (
	"fmt"
	"strings"
)

// foldersAt returns a pointer to the list of subfolders of the folder at
// path (the root list for an empty path).
func (c *Config) foldersAt(path []string, create bool) (*[]*Folder, error) {
	folders := &c.Folders
	for i, name := range path {
		f := findFolder(*folders, name)
		if f == nil {
			if !create {
				return nil, fmt.Errorf("folder %q not found", strings.Join(path[:i+1], "/"))
			}
			f = &Folder{Name: name}
			*folders = append(*folders, f)
		}
		folders = &f.Folders
	}
	return folders, nil
}

// FolderAt returns the folder at path, or nil.
func (c *Config) FolderAt(path []string) *Folder {
	if len(path) == 0 {
		return nil
	}
	parent, err := c.foldersAt(path[:len(path)-1], false)
	if err != nil {
		return nil
	}
	return findFolder(*parent, path[len(path)-1])
}

// DeleteFolder removes the folder at path with everything inside it.
func (c *Config) DeleteFolder(path []string) error {
	if len(path) == 0 {
		return fmt.Errorf("no folder given")
	}
	parent, err := c.foldersAt(path[:len(path)-1], false)
	if err != nil {
		return err
	}
	name := path[len(path)-1]
	for i, f := range *parent {
		if f.Name == name {
			*parent = append((*parent)[:i], (*parent)[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("folder %q not found", strings.Join(path, "/"))
}

// UpdateFolder sets the defaults of the folder at oldPath and, if newPath
// differs, renames/moves it there. Jump references to sessions inside it
// are rewritten.
func (c *Config) UpdateFolder(oldPath, newPath []string, d Defaults) error {
	if len(newPath) == 0 {
		return fmt.Errorf("folder name is required")
	}
	for _, p := range newPath {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("empty folder name")
		}
	}
	f := c.FolderAt(oldPath)
	if f == nil {
		return fmt.Errorf("folder %q not found", strings.Join(oldPath, "/"))
	}
	oldID, newID := strings.Join(oldPath, "/"), strings.Join(newPath, "/")
	if oldID != newID {
		if strings.HasPrefix(newID+"/", oldID+"/") {
			return fmt.Errorf("cannot move %q into itself", oldID)
		}
		if c.FolderAt(newPath) != nil {
			return fmt.Errorf("folder %q already exists", newID)
		}
		parentPath := newPath[:len(newPath)-1]
		if list, err := c.sessionsAt(parentPath, false); err == nil {
			for _, s := range *list {
				if s.Name == newPath[len(newPath)-1] {
					// allowed by the format, but confusing in the tree
					return fmt.Errorf("a session named %q is already in /%s", s.Name, strings.Join(parentPath, "/"))
				}
			}
		}
		if strings.Join(parentPath, "/") == strings.Join(oldPath[:len(oldPath)-1], "/") {
			f.Name = newPath[len(newPath)-1] // plain rename: keep its place in the list
		} else {
			if err := c.DeleteFolder(oldPath); err != nil {
				return err
			}
			parent, err := c.foldersAt(parentPath, true)
			if err != nil {
				return err
			}
			f.Name = newPath[len(newPath)-1]
			*parent = append(*parent, f)
		}
		c.RenameRefs(oldID, newID, true)
	}
	f.Defaults = d
	return nil
}

// walk calls fn for every session with the path of its folder.
func (c *Config) walk(fn func(path []string, s *Session)) {
	var rec func(path []string, folders []*Folder, sessions []*Session)
	rec = func(path []string, folders []*Folder, sessions []*Session) {
		for _, s := range sessions {
			fn(path, s)
		}
		for _, f := range folders {
			rec(append(append([]string{}, path...), f.Name), f.Folders, f.Sessions)
		}
	}
	rec(nil, c.Folders, c.Sessions)
}

// walkDefaults calls fn for the root, every folder and every session's
// connection settings (the places a jump can be set).
func (c *Config) walkDefaults(fn func(d *Defaults)) {
	fn(&c.Defaults)
	var rec func(folders []*Folder)
	rec = func(folders []*Folder) {
		for _, f := range folders {
			fn(&f.Defaults)
			rec(f.Folders)
		}
	}
	rec(c.Folders)
	c.walk(func(_ []string, s *Session) { fn(&s.Defaults) })
}

func refMatches(ref, id string, folder bool) bool {
	if folder {
		return strings.HasPrefix(ref, id+"/")
	}
	return ref == id
}

// RenameRefs rewrites jump references after a session (folder=false) or a
// folder (folder=true) was renamed from oldID to newID. It returns how many
// references changed.
func (c *Config) RenameRefs(oldID, newID string, folder bool) int {
	n := 0
	c.walkDefaults(func(d *Defaults) {
		if d.Jump == "" {
			return
		}
		parts := strings.Split(d.Jump, ",")
		changed := false
		for i, p := range parts {
			ref := strings.TrimSpace(p)
			if refMatches(ref, oldID, folder) {
				parts[i] = newID + strings.TrimPrefix(ref, oldID)
				changed = true
				n++
			} else {
				parts[i] = ref
			}
		}
		if changed {
			d.Jump = strings.Join(parts, ", ")
		}
	})
	return n
}

// JumpUsers counts places (root, folders, sessions) whose jump goes through
// the session id, or through any session in folder id.
func (c *Config) JumpUsers(id string, folder bool) int {
	n := 0
	c.walkDefaults(func(d *Defaults) {
		for _, p := range strings.Split(d.Jump, ",") {
			if refMatches(strings.TrimSpace(p), id, folder) {
				n++
				return
			}
		}
	})
	return n
}

// AddFolder creates an empty folder at path (parents are created too).
func (c *Config) AddFolder(path []string, d Defaults) error {
	if len(path) == 0 {
		return fmt.Errorf("folder name is required")
	}
	if c.FolderAt(path) != nil {
		return fmt.Errorf("folder %q already exists", strings.Join(path, "/"))
	}
	parent, err := c.foldersAt(path[:len(path)-1], true)
	if err != nil {
		return err
	}
	*parent = append(*parent, &Folder{Name: path[len(path)-1], Defaults: d})
	return nil
}

// ---- copying ----

// cloneSession returns a deep copy.
func cloneSession(s *Session) *Session {
	cp := *s
	cp.Forwards = append([]string(nil), s.Forwards...)
	return &cp
}

// cloneFolder returns a deep copy of a folder and everything inside it.
func cloneFolder(f *Folder) *Folder {
	cp := &Folder{Name: f.Name, Defaults: f.Defaults}
	for _, s := range f.Sessions {
		cp.Sessions = append(cp.Sessions, cloneSession(s))
	}
	for _, sub := range f.Folders {
		cp.Folders = append(cp.Folders, cloneFolder(sub))
	}
	return cp
}

// nameTaken reports whether a session or folder called name exists directly
// inside the folder at path.
func (c *Config) nameTaken(path []string, name string) bool {
	if list, err := c.sessionsAt(path, false); err == nil {
		for _, s := range *list {
			if s.Name == name {
				return true
			}
		}
	}
	if folders, err := c.foldersAt(path, false); err == nil {
		return findFolder(*folders, name) != nil
	}
	return false
}

// FreeName returns base, or base-copy, base-copy-2, … — the first name not
// used in the folder at path.
func (c *Config) FreeName(path []string, base string) string { return c.freeName(path, base) }

func (c *Config) freeName(path []string, base string) string {
	if !c.nameTaken(path, base) {
		return base
	}
	name := base + "-copy"
	for i := 2; c.nameTaken(path, name); i++ {
		name = fmt.Sprintf("%s-copy-%d", base, i)
	}
	return name
}

// CopySession copies a session into the folder at to (created if needed).
// It returns the copy's name (a "-copy" suffix is added when taken).
func (c *Config) CopySession(from []string, name string, to []string) (string, error) {
	s := c.Session(from, name)
	if s == nil {
		return "", fmt.Errorf("session %q not found", strings.Join(append(append([]string{}, from...), name), "/"))
	}
	cp := cloneSession(s)
	cp.Name = c.freeName(to, s.Name)
	list, err := c.sessionsAt(to, true)
	if err != nil {
		return "", err
	}
	*list = append(*list, cp)
	return cp.Name, nil
}

// CopyFolder copies the folder at from, with everything inside, into the
// folder at toParent (top level when empty). Jump references inside the copy
// that point into the original folder are redirected to the copy, so a
// copied environment uses its own jump hosts. It returns the copy's path.
func (c *Config) CopyFolder(from []string, toParent []string) ([]string, error) {
	f := c.FolderAt(from)
	if f == nil {
		return nil, fmt.Errorf("folder %q not found", strings.Join(from, "/"))
	}
	cp := cloneFolder(f) // snapshot first, so copying into itself is finite
	cp.Name = c.freeName(toParent, f.Name)
	newPath := append(append([]string{}, toParent...), cp.Name)

	oldID, newID := strings.Join(from, "/"), strings.Join(newPath, "/")
	var rewrite func(fd *Folder)
	rewriteDefaults := func(d *Defaults) {
		if d.Jump == "" {
			return
		}
		parts := strings.Split(d.Jump, ",")
		for i, p := range parts {
			ref := strings.TrimSpace(p)
			if strings.HasPrefix(ref, oldID+"/") {
				ref = newID + strings.TrimPrefix(ref, oldID)
			}
			parts[i] = ref
		}
		d.Jump = strings.Join(parts, ", ")
	}
	rewrite = func(fd *Folder) {
		rewriteDefaults(&fd.Defaults)
		for _, s := range fd.Sessions {
			rewriteDefaults(&s.Defaults)
		}
		for _, sub := range fd.Folders {
			rewrite(sub)
		}
	}
	rewrite(cp)

	parent, err := c.foldersAt(toParent, true)
	if err != nil {
		return nil, err
	}
	*parent = append(*parent, cp)
	return newPath, nil
}
