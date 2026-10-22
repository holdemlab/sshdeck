# Problems and messages

Connection errors are shown in red in the tab and in the status line. The tab
stays open, and `r` reconnects.

## Connecting

| Message | What it means | What to do |
|---|---|---|
| `dial tcp …: connect: connection refused` | nothing is listening on this port | check the session's host and port; check that sshd is running |
| `dial tcp …: i/o timeout` / `context deadline exceeded` | the host does not answer within `connect_timeout` | network, firewall, VPN; maybe you need a jump |
| `dial tcp: lookup …: no such host` | the name does not resolve | for hosts behind a jump, give the address the way the jump host sees it |
| `… administratively prohibited` / `open failed` on a hop after a jump | the jump host does not open the channel further | `AllowTcpForwarding` is off on the jump host, or the target is unreachable from it |
| `authentication failed (no key or password accepted)` | the server accepted no key and no password | check user, `identity`, and that the public key is in `authorized_keys`; see [authentication.md](authentication.md) |
| `too many authentication failures: …` | the server gave up before the right key was offered | set the session's `identity`; or `use_agent: false`, or fewer keys in the agent and `identities`; see [jump-hosts.md](jump-hosts.md#too-many-authentication-failures) |
| asks for the passphrase of the wrong key | versions before this fix opened all keys in advance; now the passphrase is asked only for the key the server accepted | update sshdeck; the `Using key …` line in the tab shows which key was used |
| `host key verification failed` | the key question was answered with something other than `yes` | reconnect and answer `yes` if the fingerprint is correct |
| `HOST KEY FOR … HAS CHANGED` | the server key does not match the saved one | if the change is expected, delete the given line from `known_hosts`; see [known_hosts](authentication.md#server-key-changed) |
| `jump loop through …` | sessions point to each other through `jump` | fix the chain |
| `jump "…": bad port` / `empty host` | the jump string is written wrong | the format is `[user@]host[:port]`, IPv6 in `[…]` |
| `Cannot use key …` | the `identity` file is not found, it is damaged, or it is an encrypted PEM without `.pub` | check the path; for old PEM, put `key.pub` next to it or convert it: `ssh-keygen -p -f key` |
| `pty request: …` | the server did not give a terminal | pty is forbidden on the server (`PermitTTY no`) or restricted in `authorized_keys` |

## Passwords and key installation

| Message | What it means | What to do |
|---|---|---|
| `Remembered password was rejected.` | the password on the server was changed | type the new one; the old one is already forgotten |
| `connect first: the key is installed over the open connection` | `ctrl+a k` in a tab with no connection | connect first (with a password), then `ctrl+a k` |
| `installing the key failed: …` | the server did not let sshdeck write `~/.ssh/authorized_keys` | the server has no `sh`, the home directory is read-only, or running commands is forbidden |
| `… already exists` when creating a key | a file with that name appeared while the dialog was open | sshdeck never overwrites keys; close the dialog and open `ctrl+a k` again, it will offer a free name |
| still asks for the password after installing the key | the key is not set in the session, or the server does not read `authorized_keys` | answer `y` to `Use … for …`; check permissions on the server: `~/.ssh` 700, `authorized_keys` 600, home directory not writable by others |

## While working

| Message | What it means |
|---|---|
| `Connection to … closed.` | the shell on the server ended (`exit`) |
| `remote exited with status N` | the command or shell ended with code N |
| `connection lost` | the connection dropped or the server stopped answering keepalives |
| text cannot be selected with the mouse | the mouse goes to sshdeck: hold **Shift** while selecting (Option in iTerm2), or set `mouse: false` |

## Configs

| Situation | What happens |
|---|---|
| error in `sessions.yaml` at startup | sshdeck does not start and prints the line with the error |
| error in `sessions.yaml` after `E` or `r` | the previous state stays, the status line shows a message with the error line |
| error in `config.yaml` at startup | starts with default values and a message; fix it with `ctrl+a ,` |
| error in `config.yaml` after editing | the previous settings stay, the status line shows a message |
| comments in `sessions.yaml` are gone | the file is rewritten when you make changes in the UI; see [sessions.md](sessions.md#editing-by-hand) |

## Keys

| Situation | Cause and fix |
|---|---|
| `n`, `e`, `d` do nothing | focus is in the terminal, so the keys went to the server; `ctrl+a s` moves focus to the sidebar |
| `ctrl+a` does not reach tmux or bash on the server | sshdeck takes it; press it twice or change the prefix ([settings.md](settings.md#prefix-and-tmux)) |
| sshdeck does not react to the prefix | sshdeck runs inside tmux with the same prefix; change the prefix |
| `esc` and then another key quickly worked as `alt+…` | this is how terminals encode alt; you need a short pause between `esc` and the next key |
