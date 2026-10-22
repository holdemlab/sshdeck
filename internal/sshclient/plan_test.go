package sshclient

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/holdemlab/sshdeck/internal/config"
)

func mustConfig(t *testing.T, y string) *config.Config {
	t.Helper()
	var c config.Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestPlanJumpBySessionAndAddress(t *testing.T) {
	c := mustConfig(t, `
user: shared
folders:
  - name: infra
    sessions:
      - {name: proxy, host: proxy.example.com, port: 2200, user: jumper}
      - {name: inner, host: 10.0.0.2, jump: infra/proxy}
  - name: prod
    jump: infra/inner, ops@[2001:db8::1]:23
    sessions:
      - {name: web, host: 10.1.1.1}
`)
	target, ok := c.Find("prod/web")
	if !ok {
		t.Fatal("prod/web not found")
	}
	hops, err := Plan(c, target)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range hops {
		got = append(got, h.User+"@"+h.Addr())
	}
	want := "jumper@proxy.example.com:2200 shared@10.0.0.2:22 ops@[2001:db8::1]:23 shared@10.1.1.1:22"
	if strings.Join(got, " ") != want {
		t.Errorf("hops = %s\nwant   %s", strings.Join(got, " "), want)
	}
	if hops[len(hops)-1].Name != "prod/web" {
		t.Errorf("target name = %q", hops[len(hops)-1].Name)
	}
}

func TestPlanLoop(t *testing.T) {
	c := mustConfig(t, `
sessions:
  - {name: a, host: a, jump: b}
  - {name: b, host: b, jump: a}
`)
	a, _ := c.Find("a")
	if _, err := Plan(c, a); err == nil || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseHop(t *testing.T) {
	for in, want := range map[string]string{
		"host":              "@host:22",
		"u@host:2222":       "u@host:2222",
		"u@1.2.3.4":         "u@1.2.3.4:22",
		"[::1]:2022":        "@[::1]:2022",
		"me@x@corp@bastion": "me@x@corp@bastion:22",
	} {
		h, err := ParseHop(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got := h.User + "@" + h.Addr(); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"", "u@", "h:99999", "[::1"} {
		if _, err := ParseHop(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestExampleSessionsFile(t *testing.T) {
	c, err := config.Load("../../examples/sessions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, ok := c.Find("prod/web/web-01")
	if !ok {
		t.Fatal("prod/web/web-01 missing from the example")
	}
	hops, err := Plan(c, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(hops) != 2 || hops[0].User+"@"+hops[0].Host != "jumper@proxy.example.com" || hops[1].User != "deploy" {
		t.Errorf("hops = %+v", hops)
	}
	r, _ = c.Find("staging/api-stg")
	hops, _ = Plan(c, r)
	if len(hops) != 2 || hops[0].Addr() != "bastion.stg.example.com:2200" || hops[0].Identity != "~/.ssh/id_staging" {
		t.Errorf("staging hops = %+v", hops)
	}
}
