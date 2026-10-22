# Tunnels (port forwarding)

Tunnels forward ports through the SSH connection. For example, a database or a
web panel visible only from the server becomes available on `localhost` of your
machine. Tunnels also work through [jump hosts](jump-hosts.md): they always go
through the connection to the **target** server.

## Types and format

The syntax is the same as `ssh -L/-R/-D`:

```
L [bind:]port:host:hostport     local port → host:hostport, as seen from the server
D [bind:]port                   SOCKS proxy: connections leave from the server
R [bind:]port:host:hostport     port on the server → host:hostport, as seen from your machine
```

| Example | What happens |
|---|---|
| `L 5432:localhost:5432` | `127.0.0.1:5432` on your machine → port 5432 **on the server itself** |
| `L 9090:grafana.internal:3000` | `127.0.0.1:9090` on your machine → `grafana.internal:3000`, the server resolves the name |
| `L *:8080:web:80` | the same, but listens on all interfaces of your machine |
| `L 0:db:5432` | a random free port; the panel shows which one |
| `D 1080` | SOCKS5 and SOCKS4a on `127.0.0.1:1080` |
| `R 9000:localhost:3000` | port 9000 on the server → `localhost:3000` on your machine |

Rules:
- the default bind address is `127.0.0.1`: the port is reachable only from your
  machine; `localhost` also means `127.0.0.1`;
- `*` or `0.0.0.0` opens the port on all interfaces. Careful: other machines on
  the network can then use the tunnel;
- IPv6 addresses go in brackets: `L 8080:[2001:db8::1]:443`;
- the letter can be lowercase and without a space (`l8080:…`); if there is no
  letter, `L` is assumed.

### SOCKS (`D`)

- SOCKS5 (no auth, CONNECT command) and SOCKS4/4a are supported.
- The server resolves host names if the client passes a name, not an IP.
  Examples: `curl --socks5-hostname 127.0.0.1:1080 http://internal/`, or a
  SOCKS5 proxy in the browser with the "DNS through proxy" option.
- If the `no_proxy` / `NO_PROXY` variable is set (for example, `localhost`),
  curl and many other programs bypass the proxy for those addresses. Then a
  request to `localhost` through `D` goes around the tunnel.

### Reverse tunnels (`R`)

- The server itself opens the port.
- Whether you can bind to a non-localhost address on the server depends on its
  `GatewayPorts` setting in sshd.
- Port 0 means the server picks a free port; the panel shows the actual
  address.

## Tunnels in a session (autostart)

```yaml
- name: db-main
  host: 10.0.20.5
  forwards:
    - L 5432:localhost:5432
    - L 9090:grafana.internal:3000
    - D 1080
```

Or in the UI: the **Tunnels** field in the session form (`e`), entries
separated by commas.

These tunnels:
- start as soon as the connection is up;
- stop when it drops;
- start again after a reconnect (`r`).

A badly written tunnel stops the session from being saved: the form shows an
error, and the file fails to load with a message saying which session has the
problem.

## Tunnels-only mode

`t` in the sidebar opens a session **without a shell**: the `⇄ db-main` tab only
holds the connection for the tunnels. It shows a timestamped event log:

```
Connected. Tunnels only, no shell.
19:02:11 L 5432:localhost:5432 listening on 127.0.0.1:5432
19:02:40 L 9090:grafana.internal:3000: cannot reach grafana.internal:3000: …
```

This is handy when you only need a database or a web panel. The session's
`command` does not run in this mode.

## Tunnels panel: `ctrl+a t`

Shows the tunnels of the current tab (any tab, regular or `⇄`):

```
╭────────────────────────────────────────────────────────────────────╮
│  Tunnels — web-01                                                  │
│  via jumper@proxy → deploy@web-01                                  │
│                                                                    │
│  ● L  127.0.0.1:18080 → localhost:8099                             │
│        0 open · 1 total · 288 B                                    │
│  ○ D  127.0.0.1:11080 (SOCKS5)  (unsaved)                          │
│        off                                                         │
│  ✗ L  127.0.0.1:5432 → localhost:5432                              │
│        bind: address already in use                                │
│                                                                    │
│  space on/off · a add · x remove · w save to session · esc close   │
╰────────────────────────────────────────────────────────────────────╯
```

Markers:
- `●` — running;
- `○` — off or waiting for the connection;
- `✗` — error (the reason is on the line below).

The stats line under a tunnel: connections open now, total connections, bytes
transferred. It updates every second.

| Key | Action |
|---|---|
| `↑↓` `j k` | select |
| `space` / `enter` | turn on / off |
| `a` | add a tunnel (line at the bottom; `enter` adds, `esc` cancels; if you already typed something, `esc` asks first) |
| `x` | remove from the tab |
| `w` | save the tab's current set of tunnels to the session (`forwards` in sessions.yaml) |
| `esc` / `q` / `t` | close the panel |

**Saving.**
- Tunnels added with `a` are marked `(unsaved)`: they live while the tab is
  open.
- `w` writes all of the tab's tunnels to the session, and they will start
  automatically.
- The "off" state is not saved: after `w`, all tunnels start on the next
  connection.

**Indicators.** The number of active tunnels is shown in the tab bar
(`web-01 ⇄2`) and in the status line (`⇄ 2 tunnel(s)`). Tunnel errors of a
regular tab are shown in the status line, so they don't clutter the terminal.

## Common errors

| Message | Cause |
|---|---|
| `bind: address already in use` | the local port is already taken: another tunnel, another tab or a program |
| `bind: permission denied` | a port < 1024 on Linux needs root; use a port ≥ 1024 |
| `cannot reach host:port: … connect failed` | the server cannot reach the target: wrong host or port, or a firewall |
| `listener closed` (in `R`) | the connection to the server dropped |
| the server refuses `R` | forbidden on the server (`AllowTcpForwarding`, `GatewayPorts`) |
