package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.yaml")
	st, err := LoadState(path)
	if err != nil || len(st.Collapsed) != 0 {
		t.Fatalf("missing file: %+v %v", st, err)
	}
	if err := SaveState(path, State{Collapsed: []string{"prod/web", "infra"}}); err != nil {
		t.Fatal(err)
	}
	st, err = LoadState(path)
	if err != nil || !reflect.DeepEqual(st.Collapsed, []string{"infra", "prod/web"}) {
		t.Fatalf("got %+v %v", st, err)
	}
	os.WriteFile(path, []byte("collapsed: [unclosed"), 0o600)
	if _, err := LoadState(path); err == nil {
		t.Error("broken file accepted")
	}
}

func TestStatePathXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdgstate")
	p, _ := StatePath()
	if p != "/tmp/xdgstate/sshdeck/state.yaml" {
		t.Errorf("path = %s", p)
	}
}
