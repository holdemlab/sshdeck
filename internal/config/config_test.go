package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `
user: admin
folders:
  - name: prod
    jump: bastion
    port: 2222
    folders:
      - name: web
        user: deploy
        sessions:
          - name: web-01
            host: 10.0.0.1
          - name: web-02
            host: 10.0.0.2
            port: 22
            jump: other
    sessions:
      - name: db
        host: db.internal
sessions:
  - name: home
    host: home.lan
`

func load(t *testing.T) (*Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sessions.yaml")
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return c, path
}

func TestResolveInheritance(t *testing.T) {
	c, _ := load(t)
	web := c.Folders[0].Folders[0]

	r, err := c.Resolve([]string{"prod", "web"}, web.Sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	if r.User != "deploy" || r.Port != 2222 || r.Jump != "bastion" {
		t.Errorf("web-01 resolved to %+v", r.Defaults)
	}

	r, _ = c.Resolve([]string{"prod", "web"}, web.Sessions[1])
	if r.Port != 22 || r.Jump != "other" || r.User != "deploy" {
		t.Errorf("web-02 overrides lost: %+v", r.Defaults)
	}

	r, _ = c.Resolve(nil, c.Sessions[0])
	if r.User != "admin" || r.Jump != "" {
		t.Errorf("root session: %+v", r.Defaults)
	}
	if r.FolderPath() != "" {
		t.Errorf("root FolderPath = %q", r.FolderPath())
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || len(c.Folders)+len(c.Sessions) != 0 {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestValidateRejectsDuplicates(t *testing.T) {
	c := &Config{Sessions: []*Session{{Name: "a", Host: "x"}, {Name: "a", Host: "y"}}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate error")
	}
	c = &Config{Sessions: []*Session{{Name: "a"}}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected missing host error")
	}
}

func TestPutDeleteSaveRoundTrip(t *testing.T) {
	c, path := load(t)

	// new folder path is created
	if err := c.Put([]string{"staging", "api"}, "", &Session{Name: "api-1", Host: "api1"}); err != nil {
		t.Fatal(err)
	}
	// duplicate name is refused and nothing changes
	if err := c.Put([]string{"staging", "api"}, "", &Session{Name: "api-1", Host: "other"}); err == nil {
		t.Fatal("expected duplicate error")
	}
	// rename in place
	if err := c.Put([]string{"prod"}, "db", &Session{Name: "db-main", Host: "db.internal"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete([]string{"prod", "web"}, "web-02"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete([]string{"prod", "web"}, "missing"); err == nil {
		t.Fatal("expected not found")
	}

	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}
	c2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c2.Folders[1].Folders[0].Sessions[0].Host; got != "api1" {
		t.Errorf("staging/api/api-1 host = %q", got)
	}
	if got := c2.Folders[0].Sessions[0].Name; got != "db-main" {
		t.Errorf("renamed session = %q", got)
	}
	if n := len(c2.Folders[0].Folders[0].Sessions); n != 1 {
		t.Errorf("prod/web has %d sessions, want 1", n)
	}
	// folder defaults survive the round trip
	if c2.Folders[0].Jump != "bastion" || c2.User != "admin" {
		t.Errorf("defaults lost: %+v / %+v", c2.Folders[0].Defaults, c2.Defaults)
	}
}

func TestSplitPath(t *testing.T) {
	got := SplitPath(" /prod//web/ ")
	if len(got) != 2 || got[0] != "prod" || got[1] != "web" {
		t.Errorf("SplitPath = %q", got)
	}
	if SplitPath("") != nil {
		t.Error("empty path should be nil")
	}
}

func TestColorInheritance(t *testing.T) {
	c := &Config{Folders: []*Folder{{
		Name:     "prod",
		Defaults: Defaults{Color: "Orange", Fill: "#ABC"},
		Folders:  []*Folder{{Name: "db", Sessions: []*Session{{Name: "a", Host: "a"}, {Name: "b", Host: "b", Defaults: Defaults{Color: "none"}}}}},
	}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if f := c.FolderAt([]string{"prod"}); f.Color != "peach" || f.Fill != "#aabbcc" {
		t.Errorf("not normalized: %q %q", f.Color, f.Fill)
	}
	a, _ := c.Find("prod/db/a")
	b, _ := c.Find("prod/db/b")
	if a.Color != "peach" || a.Fill != "#aabbcc" || b.Color != "none" || b.Fill != "#aabbcc" {
		t.Errorf("a=%+v b=%+v", a.Defaults, b.Defaults)
	}
	if d := c.FolderDefaults([]string{"prod", "db"}); d.Color != "peach" {
		t.Errorf("folder defaults %+v", d)
	}
	c.Folders[0].Fill = "ultraviolet"
	if err := c.Validate(); err == nil {
		t.Error("bad color accepted")
	}
}
