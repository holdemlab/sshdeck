# sshdeck — help

A tabbed SSH session manager. On the left is the session tree, on the right the
terminal of the active tab, tabs on top, hints at the bottom.

**Main rule:** when the focus is in the terminal, all keys go to the server.
You control sshdeck itself with the **prefix**: `ctrl+a`, then a command. You
can change the prefix in the settings (`ctrl+a ,`).

> Open this help with `ctrl+a ?` or `?` in the sidebar.
> Scroll: `↑↓` `j k`, `pgup pgdn`, `g G`; search `/`, then `n`; exit `esc` or `q`.

## Quick start

1. `N` — create a folder (`/name` at the top level, `a/b` nested).
2. `n` — new session: Name, Host, User, Key file; save with `ctrl+s`.
3. `enter` — connect. The new tab asks about the server key (`yes`) and,
   if needed, a password.
4. `ctrl+a s` — back to the tree; `ctrl+a 1`…`9` — go to a tab.
5. Tired of typing the password? `ctrl+a k` installs your key on the server.

## Prefix commands

Always work: both in the terminal and in the tree.

| Keys | Action |
|---|---|
| `ctrl+a 1`…`9` | go to tab |
| `ctrl+a n` / `ctrl+a p` | next / previous tab |
| `ctrl+a s` | focus: tree ↔ terminal |
| `ctrl+a b` | hide / show the tree |
| `ctrl+a w` | close tab |
| `ctrl+a r` | reconnect a closed tab |
| `ctrl+a d` | another tab with the same session |
| `ctrl+a l` | a shell on this computer (local tab) |
| `ctrl+a t` | tab tunnels |
| `ctrl+a e` | edit the tab's session |
| `ctrl+a k` | install your key on the server |
| `ctrl+a [` | scroll history |
| `ctrl+a ,` | settings |
| `ctrl+a ?` | this help |
| `ctrl+a q` | quit |
| `ctrl+a ctrl+a` | send `ctrl+a` itself to the server |

## Session tree

| Keys | Action |
|---|---|
| `↑↓` `j k` | move |
| `←` `h` / `→` `l` | collapse / expand folder |
| `H` or `-` / `L` or `*` | collapse / expand **all** |
| `enter` | open session (on a folder: collapse / expand; on `local`: a shell on this computer) |
| `t` | open tunnels only, no shell |
| `/` | search by name, path, host |
| `n` | new session |
| `N` | new folder (`a/b` nested, `/name` at the top level) |
| `R` | rename in place |
| `e` | edit session or folder (form) |
| `C` | colors: text and fill (e.g. `prod` in red) |
| `c` | duplicate next to it |
| `y`, then `p` | copy into the selected folder (`P` — to the top level) |
| `x`, then `p` | move into the selected folder (`P` — to the top level) |
| `d` | delete (with confirmation) |
| `E` | open `sessions.yaml` in the editor |
| `r` | reload `sessions.yaml` |
| `tab` | to the terminal |
| `q` | quit |

Collapsed folders are remembered between runs.

## Forms

| Keys | Action |
|---|---|
| `tab` / `shift+tab`, `↑↓` | between fields |
| `ctrl+s` | save |
| `esc` | exit (if something changed, asks: `y` discard, `s` save, `n` back) |
| `ctrl+u` | delete text before the cursor |

## Inheritance

`user`, `port`, `jump` and `key file` are set on a folder and apply to
everything inside. The nearest value wins: **session → folder → … → root**.
The final result is shown under the tree (`user@host`, `via …`).

## Jump hosts

The **Jump** field is a path to another session (`infra/proxy`) or a string
`user@host:port`. Several hosts separated by commas make a chain.

- Each host authenticates with **its own** key, from your machine; you don't
  need to put the target server's key on the jump host.
- If the jump host uses a different key, make it a **separate session** with
  its own Key file and refer to it. A `user@host` string uses the target
  server's key.
- Renaming and moving sessions updates `jump` references automatically.

## Keys and passwords

Order of attempts: **session key → ssh-agent → keys from settings → password**.

- A key passphrase is asked only when the server has accepted that key.
- The tab shows which key you logged in with: `Using key …`.
- Accepted passwords and passphrases are remembered **in memory** until you
  quit sshdeck: reconnects don't ask again. Nothing is written to disk. Turn
  it off with the `remember_passwords` setting.
- `ctrl+a k` appends your public key to `~/.ssh/authorized_keys` on the server
  (like `ssh-copy-id`). If you have no key, sshdeck creates a new one and can
  set it in the session.

Server keys are stored in sshdeck's own `known_hosts`. A new server shows its
fingerprint and asks `yes/no`. If a server key has changed, sshdeck refuses to
connect.

## Tunnels

Same format as in ssh:

| Entry | What it does |
|---|---|
| `L 5432:localhost:5432` | local port → service on the server |
| `L 8080:grafana.internal:3000` | local port → host visible from the server |
| `D 1080` | SOCKS5 proxy through the server |
| `R 9000:localhost:3000` | port on the server → service on your machine |

- Session tunnels (the **Tunnels** field) start together with the connection.
- `ctrl+a t` — tunnels panel: `space` turns on and off, `a` adds, `x`
  deletes, `w` saves to the session.
- `t` in the tree opens a session for tunnels only, no shell.

## Tabs

| Mark | State |
|---|---|
| `◌` | connecting or waiting for a password |
| `●` | connected |
| `✗` | closed; `r` reconnects |
| `⇄2` | 2 tunnels running |

Scroll history: `ctrl+a [`, then `↑↓` `pgup pgdn` `g G`, exit with `esc`.
Select text with Shift held and copy it with your terminal's own tools.

## Language

Ukrainian or English: `ctrl+a ,` → **Language** (`auto` takes the language
from `$LANG`). Switches immediately, together with this help.

## Color marks

`C` on a folder or session: `← →` color, `↑↓` text / fill, `enter` save. A
folder's mark applies to everything inside and shows on the tabs, so you won't
mix up `prod` and `staging`. In `sessions.yaml` these are `color:` and `fill:`
(a palette name, `#rrggbb` or `none`).

## Colors

`ctrl+a ,` → **Theme**: `terminal` (your terminal's colors) or Catppuccin
`Latte` (light), `Frappé`, `Macchiato`, `Mocha`. The scheme shows at once while
you choose and also colors the programs in the tabs.

## Mouse

Click a tab to switch, middle click to close; click and double click in the
tree; the wheel scrolls the tree, the history and this help; drag the sidebar
edge to resize. vim, htop, mc get the mouse too. **Select text with Shift
held.** Turn it off with `mouse: false`.

## Version

The running version is at the right end of the tab bar and at the bottom of
this help. Details: `sshdeck --version` (commit, build date, Go, platform).

## Files

| File | What's in it |
|---|---|
| `~/.config/sshdeck/sessions.yaml` | sessions and folders |
| `~/.config/sshdeck/config.yaml` | settings (`ctrl+a ,`, or `ctrl+e` there for the editor) |
| `~/.config/sshdeck/known_hosts` | server keys |
| `~/.local/state/sshdeck/state.yaml` | which folders are collapsed |

## If something goes wrong

- **`n`, `e`, `d` don't work:** the focus is in the terminal, press `ctrl+a s`.
- **`ctrl+a` doesn't reach tmux on the server:** press it twice or change the
  prefix, for example to `ctrl+]`.
- **`HOST KEY … HAS CHANGED`:** if the change is expected, delete the line
  shown from `known_hosts`.
- **`too many authentication failures`:** set a Key file for the session or
  turn off ssh-agent in the settings.

The full documentation is in the `docs/` directory of the repository: https://github.com/holdemlab/sshdeck/tree/main/docs
