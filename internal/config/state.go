package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// State is UI state that sshdeck remembers between runs. It is not
// configuration, so it lives apart from config.yaml and sessions.yaml, in
// $XDG_STATE_HOME/sshdeck/state.yaml (usually ~/.local/state/sshdeck).
type State struct {
	// Collapsed lists folders folded in the tree, by path ("prod/web").
	Collapsed []string `yaml:"collapsed,omitempty"`
}

// StatePath returns the default state.yaml location.
func StatePath() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "sshdeck", "state.yaml"), nil
}

// LoadState reads state.yaml; a missing file is an empty state. A broken
// file is not fatal for the caller: it gets an empty state and the error.
func LoadState(path string) (State, error) {
	var st State
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := yaml.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return st, nil
}

// SaveState writes state.yaml atomically.
func SaveState(path string, st State) error {
	sort.Strings(st.Collapsed)
	data, err := yaml.Marshal(st)
	if err != nil {
		return err
	}
	header := []byte("# sshdeck UI state, written automatically. Safe to delete.\n")
	return writeAtomic(path, append(header, data...))
}
