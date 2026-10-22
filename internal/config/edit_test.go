package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const editYAML = `
jump: infra/proxy
folders:
  - name: infra
    sessions:
      - {name: proxy, host: p}
      - {name: inner, host: i, jump: infra/proxy}
  - name: prod
    jump: infra/inner, bastion.example.com
    folders:
      - name: web
        sessions:
          - {name: web-01, host: w}
    sessions:
      - {name: db, host: d}
sessions:
  - {name: home, host: h}
`

func parse(t *testing.T) *Config {
	t.Helper()
	var c Config
	if err := yaml.Unmarshal([]byte(editYAML), &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestRenameFolderRewritesJumps(t *testing.T) {
	c := parse(t)
	if err := c.UpdateFolder([]string{"infra"}, []string{"core"}, Defaults{User: "ops"}); err != nil {
		t.Fatal(err)
	}
	if c.FolderAt([]string{"infra"}) != nil || c.FolderAt([]string{"core"}).User != "ops" {
		t.Fatal("folder not renamed / defaults not set")
	}
	if c.Folders[0].Name != "core" {
		t.Errorf("rename moved the folder: order %s, %s", c.Folders[0].Name, c.Folders[1].Name)
	}
	if c.Jump != "core/proxy" {
		t.Errorf("root jump = %q", c.Jump)
	}
	if got := c.FolderAt([]string{"prod"}).Jump; got != "core/inner, bastion.example.com" {
		t.Errorf("prod jump = %q", got)
	}
	if got := c.Session([]string{"core"}, "inner").Jump; got != "core/proxy" {
		t.Errorf("inner jump = %q", got)
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

func TestMoveFolder(t *testing.T) {
	c := parse(t)
	if err := c.UpdateFolder([]string{"prod", "web"}, []string{"web"}, Defaults{}); err != nil {
		t.Fatal(err)
	}
	if c.FolderAt([]string{"web"}) == nil || c.FolderAt([]string{"prod", "web"}) != nil {
		t.Fatal("move failed")
	}
	if _, ok := c.Find("web/web-01"); !ok {
		t.Error("session did not move with its folder")
	}
	// into a new parent that does not exist yet
	if err := c.UpdateFolder([]string{"web"}, []string{"stage", "frontends"}, Defaults{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find("stage/frontends/web-01"); !ok {
		t.Error("move into new parent failed")
	}
}

func TestUpdateFolderErrors(t *testing.T) {
	c := parse(t)
	cases := []struct {
		old, new []string
		want     string
	}{
		{[]string{"prod"}, []string{"prod", "web", "x"}, "into itself"},
		{[]string{"prod"}, []string{"infra"}, "already exists"},
		{[]string{"prod"}, []string{"home"}, "session named"},
		{[]string{"nope"}, []string{"x"}, "not found"},
		{[]string{"prod"}, nil, "required"},
	}
	for _, tc := range cases {
		err := c.UpdateFolder(tc.old, tc.new, Defaults{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v → %v: err = %v, want %q", tc.old, tc.new, err, tc.want)
		}
	}
	if err := c.Validate(); err != nil {
		t.Errorf("failed updates changed the tree: %v", err)
	}
}

func TestDeleteFolderAndJumpUsers(t *testing.T) {
	c := parse(t)
	if n := c.JumpUsers("infra/proxy", false); n != 2 { // root + infra/inner
		t.Errorf("users of infra/proxy = %d", n)
	}
	if n := c.JumpUsers("infra", true); n != 3 { // root, prod, inner
		t.Errorf("users of folder infra = %d", n)
	}
	if err := c.DeleteFolder([]string{"prod", "web"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find("prod/web/web-01"); ok {
		t.Error("session inside deleted folder still there")
	}
	if err := c.DeleteFolder([]string{"prod", "web"}); err == nil {
		t.Error("deleting twice should fail")
	}
}

func TestRenameSessionRefs(t *testing.T) {
	c := parse(t)
	if n := c.RenameRefs("infra/proxy", "infra/gw", false); n != 2 {
		t.Errorf("changed %d refs", n)
	}
	if c.Jump != "infra/gw" {
		t.Errorf("root jump = %q", c.Jump)
	}
	// a folder-style rename must not touch exact refs to a session
	// named like the folder prefix
	c.RenameRefs("infra/gw", "x", true)
	if c.Jump != "infra/gw" {
		t.Errorf("folder rename touched session ref: %q", c.Jump)
	}
}

func TestCopySession(t *testing.T) {
	c := parse(t)
	name, err := c.CopySession([]string{"prod"}, "db", []string{"prod"})
	if err != nil || name != "db-copy" {
		t.Fatalf("same folder: %q %v", name, err)
	}
	name, _ = c.CopySession([]string{"prod"}, "db", []string{"prod"})
	if name != "db-copy-2" {
		t.Errorf("second copy = %q", name)
	}
	// into another folder: name kept, folder created if needed
	name, err = c.CopySession([]string{"prod"}, "db", []string{"new", "place"})
	if err != nil || name != "db" {
		t.Fatalf("other folder: %q %v", name, err)
	}
	// deep copy: changing the copy leaves the original alone
	c.Session([]string{"new", "place"}, "db").Host = "changed"
	if c.Session([]string{"prod"}, "db").Host != "d" {
		t.Error("copy shares data with the original")
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

func TestCopyFolderRedirectsInnerJumps(t *testing.T) {
	c := parse(t)
	// infra/inner jumps through infra/proxy; prod jumps through infra/inner
	newPath, err := c.CopyFolder([]string{"infra"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(newPath, "/") != "infra-copy" {
		t.Fatalf("path = %v", newPath)
	}
	if got := c.Session([]string{"infra-copy"}, "inner").Jump; got != "infra-copy/proxy" {
		t.Errorf("inner jump in the copy = %q", got)
	}
	if got := c.Session([]string{"infra"}, "inner").Jump; got != "infra/proxy" {
		t.Errorf("original changed: %q", got)
	}
	// references from outside still point to the original
	if got := c.FolderAt([]string{"prod"}).Jump; got != "infra/inner, bastion.example.com" {
		t.Errorf("prod jump = %q", got)
	}

	// nested copy into another folder keeps its name; outside refs untouched
	newPath, err = c.CopyFolder([]string{"prod"}, []string{"archive"})
	if err != nil || strings.Join(newPath, "/") != "archive/prod" {
		t.Fatalf("copy into archive: %v %v", newPath, err)
	}
	if _, ok := c.Find("archive/prod/web/web-01"); !ok {
		t.Error("subfolders not copied")
	}
	if got := c.FolderAt([]string{"archive", "prod"}).Jump; got != "infra/inner, bastion.example.com" {
		t.Errorf("outside jump changed: %q", got)
	}

	// copying a folder into itself is finite
	newPath, err = c.CopyFolder([]string{"prod"}, []string{"prod"})
	if err != nil || strings.Join(newPath, "/") != "prod/prod" {
		t.Fatalf("into itself: %v %v", newPath, err)
	}
	if c.FolderAt([]string{"prod", "prod", "prod"}) != nil {
		t.Error("copy recursed")
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}
