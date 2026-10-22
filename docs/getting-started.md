# Getting started

## Installing

**Prebuilt for Linux.** Every release on
[Releases](https://github.com/holdemlab/sshdeck/releases) has Linux builds for
amd64 and arm64: a static binary with no dependencies, and `.deb`, `.rpm` and
`.apk` packages.

```sh
# the binary: put it anywhere in your PATH, no sudo needed
tar xzf sshdeck_*_linux_amd64.tar.gz
install -m 755 sshdeck ~/.local/bin/
sha256sum -c --ignore-missing checksums.txt   # optional: check the download

# or a package
sudo apt install ./sshdeck_*_amd64.deb        # Debian, Ubuntu
sudo dnf install ./sshdeck-*.x86_64.rpm       # Fedora, RHEL
```

On macOS, Windows and other systems, install with Go or build from source;
sshdeck builds there as well.

**With Go** (1.26 or newer):

```sh
go install github.com/holdemlab/sshdeck/cmd/sshdeck@latest
```

The binary lands in `$(go env GOPATH)/bin`.

## Building from source

You need Go 1.26 or newer (Bubble Tea v2 requires it). If you have an older Go,
1.21 or later, the `go` command downloads the required version by itself.

```sh
git clone https://github.com/holdemlab/sshdeck
cd sshdeck
make                # same as: go build -o bin/sshdeck ./cmd/sshdeck
./bin/sshdeck
```

Without `make`, the `go build` line above works on any system.

| Command | What it does |
|---|---|
| `make` / `make build` | `bin/sshdeck` for this machine, with the version from `git describe` |
| `make run ARGS="…"` | build and start, passing flags (`ARGS="-sessions /tmp/s.yaml"`) |
| `make install` | install into `$(go env GOPATH)/bin` |
| `make test` | tests |
| `make race` | tests with the race detector, as in CI |
| `make cover` | coverage report in `coverage.html` |
| `make lint` | gofmt, go vet, go mod tidy checks |
| `make check` | everything CI checks on a pull request |
| `make linux` | static Linux binaries for amd64 and arm64 in `dist/`; copy one to any machine, no sudo needed |
| `make snapshot` | the full release build with GoReleaser, without publishing |
| `make next-version` | the version the next merge into `main` would release, and why |
| `make clean` | remove `bin/`, `dist/` and coverage files |

## Files

Everything lives in `~/.config/sshdeck/` (more precisely, in
`$XDG_CONFIG_HOME/sshdeck/` if that variable is set):

| File | What's in it | Details |
|---|---|---|
| `config.yaml` | app settings; created with comments on first run | [settings.md](settings.md) |
| `sessions.yaml` | the tree of sessions and folders | [sessions.md](sessions.md) |
| `known_hosts` | keys of servers you trust | [authentication.md](authentication.md#known_hosts) |

Separately, sshdeck keeps a state file `~/.local/state/sshdeck/state.yaml` (or
`$XDG_STATE_HOME/sshdeck/state.yaml`): which folders in the tree are collapsed.
It is not a settings file, and you can delete it at any time (see
[sessions.md](sessions.md#collapsing-folders)).

sshdeck does not read `~/.ssh/config` and does not use the system `ssh`
program. All settings live only in these files.

## Command-line flags

| Flag | Default | What it does |
|---|---|---|
| `-sessions FILE` | `~/.config/sshdeck/sessions.yaml` | use a different sessions file |
| `-config FILE` | `~/.config/sshdeck/config.yaml` | use a different settings file (the template is created only for the default path) |
| `-version` | | print the version and exit |

## Your first session

1. Run `sshdeck`. On the left is an empty session tree.
2. Press `N`, type `prod` and press `enter`: a folder appears.
3. Press `n` and fill in the form:
   - Name: `web-01`;
   - Host: `10.0.10.11`;
   - User: `deploy`;
   - Key file: `~/.ssh/id_ed25519`.

   Save with `ctrl+s`.
4. Press `enter` on `web-01`, and a tab opens. On the first connection sshdeck
   shows the server key fingerprint and asks `yes/no`, just like ssh
   (see [known_hosts](authentication.md#known_hosts)).
5. You are now in the server's shell. All keys go to the server, and you
   control sshdeck with the prefix `ctrl+a`:
   - `ctrl+a s` — back to the session list;
   - `ctrl+a 1`…`9` — switch tabs;
   - `ctrl+a ?` — help.

What's next:
- [Sessions and folders](sessions.md): file format, inheritance, editing in the UI.
- [Jump hosts](jump-hosts.md): connecting through intermediate servers, different keys.
- [Tunnels](tunnels.md): forwarding private services to local ports.
- [Keys](keys.md): the full list.
