// Package buildinfo tells which build of sshdeck is running.
//
// Release builds set the values with the linker:
//
//	-ldflags "-X github.com/holdemlab/sshdeck/internal/buildinfo.Version=v1.2.3
//	          -X github.com/holdemlab/sshdeck/internal/buildinfo.Commit=<sha>
//	          -X github.com/holdemlab/sshdeck/internal/buildinfo.Date=<RFC 3339>"
//
// Without them (go build, go install …@v1.2.3) the module version and the
// git commit Go records in the binary are used.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// Set by -ldflags -X; see the package comment.
var (
	Version = ""
	Commit  = ""
	Date    = ""
)

// Info describes the running binary.
type Info struct {
	Version  string // "v1.2.3", "v1.2.3-4-gabc1234-dirty" or "dev"
	Commit   string // short commit hash, "" if unknown
	Date     string // build time (releases) or commit time, RFC 3339; "" if unknown
	Modified bool   // built from a tree with uncommitted changes
	Go       string // go1.26.7
	Platform string // linux/amd64
}

// Get collects the build information.
func Get() Info {
	in := Info{Version: Version, Commit: Commit, Date: Date, Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if in.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			in.Version = bi.Main.Version // go install …@version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if in.Commit == "" {
					in.Commit = s.Value
				}
			case "vcs.time":
				if in.Date == "" {
					in.Date = s.Value
				}
			case "vcs.modified":
				in.Modified = s.Value == "true"
			}
		}
	}
	if in.Version == "" {
		in.Version = "dev"
	}
	if strings.HasSuffix(in.Version, "dirty") { // "-dirty" (git describe) or "+dirty" (go build)
		in.Modified = true
	}
	if len(in.Commit) > 7 {
		in.Commit = in.Commit[:7]
	}
	return in
}

// Short is the version for the interface: "v1.2.3", or for builds that are
// not a release "dev+abc1234" (with "*" when the tree had changes).
func (in Info) Short() string {
	v := in.Version
	if v == "dev" && in.Commit != "" {
		v += "+" + in.Commit
	}
	if in.Modified && !strings.Contains(v, "dirty") {
		v += "*"
	}
	return v
}

// Long is the text of `sshdeck --version`.
func (in Info) Long() string {
	var b strings.Builder
	fmt.Fprintf(&b, "sshdeck %s\n", in.Version)
	if in.Commit != "" {
		commit := in.Commit
		if in.Modified {
			commit += " (modified)"
		}
		fmt.Fprintf(&b, "commit  %s\n", commit)
	}
	if in.Date != "" {
		date := in.Date
		if t, err := time.Parse(time.RFC3339, in.Date); err == nil {
			date = t.UTC().Format("2006-01-02 15:04 UTC")
		}
		fmt.Fprintf(&b, "date    %s\n", date)
	}
	fmt.Fprintf(&b, "go      %s %s\n", in.Go, in.Platform)
	return b.String()
}
