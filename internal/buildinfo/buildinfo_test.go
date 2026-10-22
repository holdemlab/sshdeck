package buildinfo

import (
	"strings"
	"testing"
)

func TestShortAndLong(t *testing.T) {
	cases := []struct {
		in    Info
		short string
	}{
		{Info{Version: "v1.2.3", Commit: "abc1234"}, "v1.2.3"},
		{Info{Version: "dev", Commit: "abc1234"}, "dev+abc1234"},
		{Info{Version: "dev", Commit: "abc1234", Modified: true}, "dev+abc1234*"},
		{Info{Version: "v1.2.3-4-gabc1234-dirty", Modified: true}, "v1.2.3-4-gabc1234-dirty"},
		{Info{Version: "dev"}, "dev"},
		{Info{Version: "v0.2.0+dirty", Modified: true}, "v0.2.0+dirty"},
	}
	for _, c := range cases {
		if got := c.in.Short(); got != c.short {
			t.Errorf("%+v: Short = %q, want %q", c.in, got, c.short)
		}
	}
	long := Info{Version: "v1.2.3", Commit: "abc1234", Date: "2026-10-22T14:05:00Z", Go: "go1.26.7", Platform: "linux/amd64"}.Long()
	for _, want := range []string{"sshdeck v1.2.3", "commit  abc1234", "date    2026-10-22 14:05 UTC", "go      go1.26.7 linux/amd64"} {
		if !strings.Contains(long, want) {
			t.Errorf("Long missing %q:\n%s", want, long)
		}
	}
}

func TestLinkerValuesWin(t *testing.T) {
	old := [3]string{Version, Commit, Date}
	defer func() { Version, Commit, Date = old[0], old[1], old[2] }()
	Version, Commit, Date = "v9.9.9", "0123456789abcdef", "2026-01-02T03:04:05Z"
	in := Get()
	if in.Version != "v9.9.9" || in.Commit != "0123456" || in.Date != "2026-01-02T03:04:05Z" {
		t.Errorf("Get = %+v", in)
	}
	if in.Go == "" || !strings.Contains(in.Platform, "/") {
		t.Errorf("runtime fields: %+v", in)
	}
}
