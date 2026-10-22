# Jump hosts

A jump host is an intermediate server that sshdeck goes through to reach the
target (like `ssh -J` / `ProxyJump`). For example, servers in a private network
are reachable only through a bastion or a proxy.

## How it works

sshdeck connects to the jump host, asks it to open a TCP channel to the next
server, and then runs the SSH handshake and authentication with the target
server **through that channel**, directly from your machine.

What follows from this:
- **Each host authenticates separately, with its own key, and all keys stay
  only with you.** You don't need to put the target server's key on the jump
  host, and nobody there can see it.
- The jump host sees only encrypted traffic. No shell is started on it.
- Host keys are checked for each hop of the chain separately, in your
  known_hosts.
- The jump host must allow TCP forwarding (`AllowTcpForwarding` in sshd; on by
  default).

## Two ways to set a jump

### 1. A reference to another session (recommended)

```yaml
jump: infra/proxy
```

The `jump` value is matched against the full paths of sessions in the tree. If
such a path exists, **all settings of that session** are used for this hop:
host, port, user, identity, and even its own `jump`. So chains build
themselves.

All inheritance for the jump session comes from **its own** folder, not from
the folder of the session that connects through it.

### 2. A `[user@]host[:port]` string

```yaml
jump: ops@bastion.example.com:2200
jump: bastion.example.com            # port 22
jump: admin@[2001:db8::1]:22         # IPv6 in square brackets
```

Such a hop **has no settings of its own**:
- if `user` is not given in the string, the target session's user is used;
- **the key is the same as for the target session** (`identity`).

So for a jump host with a different key, use way 1 (see
[below](#different-keys-for-the-jump-host-and-the-target-server)).

If the string happens to match the path of an existing session (for example,
`proxy`, and there is a session `proxy` in the root), the session is used.

## A chain through several hosts

Separate them with commas:

```yaml
jump: infra/edge, infra/inner
```

The order is: you → `edge` → `inner` → target server.

You also get a chain if the jump session has its own `jump`:

```yaml
- name: infra
  sessions:
    - {name: edge,  host: edge.example.com,  user: jumper}
    - {name: inner, host: 10.0.0.2,          jump: infra/edge}
- name: prod
  jump: infra/inner       # you → edge → inner → prod/*
```

sshdeck detects loops (`a` through `b`, `b` through `a`) and shows the error
`jump loop through …`.

You can see the whole route:
- in the tab's status line: `jumper@edge → user@inner → deploy@web-01`;
- in the tab while connecting: `Connecting to … via …` for each hop.

## Different keys for the jump host and the target server

Make the jump host a separate session with its own `identity` and refer to it:

```yaml
folders:
  - name: infra
    sessions:
      - name: proxy
        host: proxy.example.com
        user: jumper
        identity: ~/.ssh/id_proxy        # key for the jump host

  - name: prod
    jump: infra/proxy
    identity: ~/.ssh/id_prod             # key for the servers in prod
    user: deploy
    sessions:
      - {name: web-01, host: 10.0.10.11}
      - {name: web-02, host: 10.0.10.12}
```

The chain for `prod/web-01`:

| Hop | User | Key | Settings from |
|---|---|---|---|
| 1. `proxy.example.com` | `jumper` | `~/.ssh/id_proxy` | session `infra/proxy` |
| 2. `10.0.10.11` | `deploy` | `~/.ssh/id_prod` | folder `prod` |

The same in the UI:
1. On the `proxy` session press `e` and set Key file to `~/.ssh/id_proxy`.
2. On the `prod` folder press `e`. Jump: `infra/proxy`, Key file: `~/.ssh/id_prod`.

### What to avoid

```yaml
- name: prod
  jump: jumper@proxy.example.com     # a string, not a session
  identity: ~/.ssh/id_prod
```

Here sshdeck also offers the `id_prod` key to `proxy.example.com` first,
because the string has no `identity` of its own. This may work if the right key
is in ssh-agent or in the `identities` list in the [settings](settings.md):
sshdeck tries them all (see the
[order](authentication.md#authentication-order)). But you shouldn't rely on it.

### "Too many authentication failures"

By default sshd allows 6 authentication attempts (`MaxAuthTries`). Each offered
key is one attempt. The session key (`identity`) always goes first, so if it is
set and correct, there is no problem. The problem can happen when `identity`
is not set and ssh-agent and `identities` together hold many keys: the server
closes the connection before it gets to the right one. sshdeck then shows
`too many authentication failures: … set identity for this session, or
use_agent: false …`.

What you can do:
- set `identity` for the session (or its folder);
- turn off the agent in the settings (`use_agent: false`);
- keep fewer keys in the agent;
- remove extra keys from the `identities` list.

## Tunnels through a jump

Tunnels (`L`, `D`, `R`) run on the connection to the **target** server, that
is, after all jump hosts. `L 5432:localhost:5432` in the session `prod/db`
gives you access to port 5432 on `prod/db` itself. More: [tunnels.md](tunnels.md).
