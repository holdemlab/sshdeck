// Package config loads and saves the session tree (sessions.yaml).
//
// The tree consists of folders that contain sessions and nested folders.
// Connection settings (user, port, jump host, identity file) set on a folder
// are inherited by everything inside it, unless overridden.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/holdemlab/sshdeck/internal/tunnel"
)

// Defaults are connection settings that can be set on a folder or a session.
// Empty / zero fields mean "inherit from the parent".
type Defaults struct {
	User string `yaml:"user,omitempty"`
	Port int    `yaml:"port,omitempty"`
	// Jump is the host to connect through: either another session given by
	// its path ("infra/proxy") or "user@host:port". Several hops can be
	// chained with commas: "proxy1,proxy2".
	Jump     string `yaml:"jump,omitempty"`
	Identity string `yaml:"identity,omitempty"` // path to a private key
	// Color and Fill mark a session or a folder (and everything inside) in
	// the tree and on its tabs: a palette name ("red", "peach", …), "#rrggbb",
	// or "none" to stop inheriting. See colors.go.
	Color string `yaml:"color,omitempty"`
	Fill  string `yaml:"fill,omitempty"`
}

// Session is a single SSH target.
type Session struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"` // hostname or IP
	Defaults `yaml:",inline"`
	// Command runs instead of the login shell (like "ssh host command").
	Command string `yaml:"command,omitempty"`
	// Forwards are tunnels started with the connection, in ssh notation:
	// "L 8080:localhost:80", "D 1080", "R 9000:localhost:3000".
	Forwards []string `yaml:"forwards,omitempty"`
	Note     string   `yaml:"note,omitempty"`
}

// Folder groups sessions and other folders.
type Folder struct {
	Name     string `yaml:"name"`
	Defaults `yaml:",inline"`
	Folders  []*Folder  `yaml:"folders,omitempty"`
	Sessions []*Session `yaml:"sessions,omitempty"`
}

// Config is the root of sessions.yaml.
type Config struct {
	Defaults `yaml:",inline"`
	Folders  []*Folder  `yaml:"folders,omitempty"`
	Sessions []*Session `yaml:"sessions,omitempty"`
}

// Resolved is a session with all inherited settings applied.
type Resolved struct {
	Session
	Path []string // folder names from the root, e.g. ["prod", "web"]
}

// FolderPath returns the folder path joined with "/".
func (r Resolved) FolderPath() string { return strings.Join(r.Path, "/") }

// ID returns the session's full path, e.g. "prod/web/web-01".
func (r Resolved) ID() string {
	return strings.Join(append(append([]string{}, r.Path...), r.Name), "/")
}

// Dir returns $XDG_CONFIG_HOME/sshdeck (usually ~/.config/sshdeck).
func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sshdeck"), nil
}

// DefaultPath returns the default sessions.yaml location.
func DefaultPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions.yaml"), nil
}

// Session returns the session named name in the folder at path, or nil.
func (c *Config) Session(path []string, name string) *Session {
	list, err := c.sessionsAt(path, false)
	if err != nil {
		return nil
	}
	for _, s := range *list {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Find looks a session up by its full path ("prod/web/web-01") and returns
// it resolved.
func (c *Config) Find(id string) (Resolved, bool) {
	parts := SplitPath(id)
	if len(parts) == 0 {
		return Resolved{}, false
	}
	path, name := parts[:len(parts)-1], parts[len(parts)-1]
	list, err := c.sessionsAt(path, false)
	if err != nil {
		return Resolved{}, false
	}
	for _, s := range *list {
		if s.Name == name {
			r, err := c.Resolve(path, s)
			return r, err == nil
		}
	}
	return Resolved{}, false
}

// Load reads the config file. A missing file yields an empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// Save writes the config atomically (temp file + rename), mode 0600.
func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeAtomic(path, buf.Bytes())
}

// writeAtomic writes data via a temp file + rename, mode 0600.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Validate checks required fields and name uniqueness within each folder.
func (c *Config) Validate() error {
	if err := c.Defaults.validateColors(); err != nil {
		return err
	}
	return validateLevel(nil, c.Folders, c.Sessions)
}

func validateLevel(path []string, folders []*Folder, sessions []*Session) error {
	where := "/" + strings.Join(path, "/")
	seen := map[string]bool{}
	for _, s := range sessions {
		if s.Name == "" {
			return fmt.Errorf("session without name in %s", where)
		}
		if s.Host == "" {
			return fmt.Errorf("session %q in %s has no host", s.Name, where)
		}
		if seen["s:"+s.Name] {
			return fmt.Errorf("duplicate session %q in %s", s.Name, where)
		}
		if err := s.Defaults.validateColors(); err != nil {
			return fmt.Errorf("session %q in %s: %w", s.Name, where, err)
		}
		for _, fw := range s.Forwards {
			if _, err := tunnel.Parse(fw); err != nil {
				return fmt.Errorf("session %q in %s: forwards: %w", s.Name, where, err)
			}
		}
		seen["s:"+s.Name] = true
	}
	for _, f := range folders {
		if f.Name == "" {
			return fmt.Errorf("folder without name in %s", where)
		}
		if seen["f:"+f.Name] {
			return fmt.Errorf("duplicate folder %q in %s", f.Name, where)
		}
		seen["f:"+f.Name] = true
		if err := f.Defaults.validateColors(); err != nil {
			return fmt.Errorf("folder %q in %s: %w", f.Name, where, err)
		}
		if err := validateLevel(append(append([]string{}, path...), f.Name), f.Folders, f.Sessions); err != nil {
			return err
		}
	}
	return nil
}

// merge returns child with empty fields filled from parent.
func merge(parent, child Defaults) Defaults {
	if child.User == "" {
		child.User = parent.User
	}
	if child.Port == 0 {
		child.Port = parent.Port
	}
	if child.Jump == "" {
		child.Jump = parent.Jump
	}
	if child.Identity == "" {
		child.Identity = parent.Identity
	}
	if child.Color == "" {
		child.Color = parent.Color
	}
	if child.Fill == "" {
		child.Fill = parent.Fill
	}
	return child
}

// Resolve returns s with settings inherited along path applied.
func (c *Config) Resolve(path []string, s *Session) (Resolved, error) {
	d := c.Defaults
	folders := c.Folders
	for _, name := range path {
		f := findFolder(folders, name)
		if f == nil {
			return Resolved{}, fmt.Errorf("folder %q not found", strings.Join(path, "/"))
		}
		d = merge(d, f.Defaults)
		folders = f.Folders
	}
	r := Resolved{Session: *s, Path: append([]string{}, path...)}
	r.Defaults = merge(d, s.Defaults)
	return r, nil
}

func findFolder(folders []*Folder, name string) *Folder {
	for _, f := range folders {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// sessionsAt returns a pointer to the session slice at path, creating
// folders along the way if create is true.
func (c *Config) sessionsAt(path []string, create bool) (*[]*Session, error) {
	sessions := &c.Sessions
	folders := &c.Folders
	for _, name := range path {
		f := findFolder(*folders, name)
		if f == nil {
			if !create {
				return nil, fmt.Errorf("folder %q not found", strings.Join(path, "/"))
			}
			f = &Folder{Name: name}
			*folders = append(*folders, f)
		}
		sessions = &f.Sessions
		folders = &f.Folders
	}
	return sessions, nil
}

// Put adds a session at path, or replaces the session named oldName there.
// Folders in path are created if needed.
func (c *Config) Put(path []string, oldName string, s *Session) error {
	if s.Name == "" || s.Host == "" {
		return errors.New("name and host are required")
	}
	list, err := c.sessionsAt(path, true)
	if err != nil {
		return err
	}
	replace := -1
	for i, existing := range *list {
		if oldName != "" && existing.Name == oldName {
			replace = i
		} else if existing.Name == s.Name {
			return fmt.Errorf("session %q already exists in /%s", s.Name, strings.Join(path, "/"))
		}
	}
	if replace >= 0 {
		(*list)[replace] = s
	} else {
		*list = append(*list, s)
	}
	return nil
}

// Delete removes the session named name at path.
func (c *Config) Delete(path []string, name string) error {
	list, err := c.sessionsAt(path, false)
	if err != nil {
		return err
	}
	for i, s := range *list {
		if s.Name == name {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("session %q not found", name)
}

// SplitPath turns "prod/web" into ["prod", "web"], dropping empty parts.
func SplitPath(p string) []string {
	var out []string
	for _, part := range strings.Split(p, "/") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
