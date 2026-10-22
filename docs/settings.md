# Settings (`config.yaml`)

The file `~/.config/sshdeck/config.yaml` is created with comments on first
run. You can change settings on the settings screen in sshdeck or by hand.

## Options

| Option | Default | Allowed values | What it does | When it applies |
|---|---|---|---|---|
| `language` | `auto` | `auto`, `uk`, `en` | language of the interface, built-in help and comments in `config.yaml`, see [below](#language) | immediately |
| `theme` | `terminal` | `terminal`, `latte`, `frappe`, `macchiato`, `mocha` | color scheme, see [below](#color-schemes-catppuccin) | immediately |
| `prefix` | `ctrl+a` | a key with `ctrl` or `alt`; not `ctrl+c`, `ctrl+m`, `ctrl+i`, `ctrl+[` | command key in a tab, see [keys.md](keys.md) | immediately |
| `sidebar_width` | `32` | 16–120 | width of the session tree, in columns | immediately |
| `scrollback` | `5000` | 0–1,000,000 | how many lines of history each tab keeps | immediately |
| `term` | `xterm-256color` | non-empty string | `TERM` value sent to the server | next connection |
| `connect_timeout` | `15s` | > 0, e.g. `10s`, `1m` | TCP connection timeout for each hop | next connection |
| `keepalive` | `30s` | `0` turns it off | keepalive interval; with no reply for 3 × interval the connection is closed | next connection |
| `known_hosts` | `~/.config/sshdeck/known_hosts` | path | server keys file, see [authentication.md](authentication.md#known_hosts) | next connection |
| `use_agent` | `true` | `true` / `false` | take keys from ssh-agent (`$SSH_AUTH_SOCK`) | next connection |
| `identities` | `~/.ssh/id_ed25519`, `~/.ssh/id_ecdsa`, `~/.ssh/id_rsa` | list of paths | keys tried for **every** session after its own key; missing files are skipped | next connection |
| `remember_passwords` | `true` | `true` / `false` | remember accepted passwords and key passphrases in memory until exit; nothing is written to disk; turning it off clears what was remembered, see [authentication.md](authentication.md#remembering-passwords-in-memory) | immediately |
| `confirm_close` | `true` | `true` / `false` | ask before closing a tab with a live connection | immediately |
| `local_shell` | `true` | `true` / `false` | show the `local` entry at the top of the tree, see [below](#local-shell) | immediately |
| `local_command` | empty (`$SHELL`) | a command line | what local tabs run instead of `$SHELL`, e.g. `zsh -l` | new local tabs |
| `local_color`, `local_fill` | empty | palette name, `#rrggbb` | color mark of the `local` entry and its tabs, like [session colors](sessions.md#color-marks) | immediately |

Durations are written like this: `500ms`, `15s`, `2m`, `1m30s`; `0` means "off".

## Settings screen: `ctrl+a ,`

You can also open it with `,` in the sidebar.

```
Settings  ~/.config/sshdeck/config.yaml

▸ Language       ● auto ($LANG)  ○ Українська  ○ English
                 auto follows $LANG · space or ← → to change
  Theme          ○ terminal  ○ Latte  ○ Frappé  ○ Macchiato  ● Mocha
  Prefix key     ctrl+a
  Sidebar        32
  Scrollback     5000
  TERM           xterm-256color
  Timeout        15s
  Keepalive      30s
  known_hosts    ~/.config/sshdeck/known_hosts
  ssh-agent      [x] on
  Keys           ~/.ssh/id_ed25519, ~/.ssh/id_ecdsa, ~/.ssh/id_rsa
  Local shell    [x] on
  Local cmd      $SHELL
  Remember pw    [x] on
  Confirm close  [x] on
```

| Key | Action |
|---|---|
| `tab` / `shift+tab`, `↑↓` | move between fields |
| `enter` on Prefix key | wait for a new key; the next key you press becomes the prefix (`esc` keeps the old one) |
| `space`, `enter`, `← →`, `y` / `n` on toggles | turn on / off |
| `space`, `enter`, `← →` on Language | next / previous language |
| `space`, `enter`, `← →` on Theme | next / previous scheme; the screen changes at once, before you save |
| `ctrl+s` | save and apply |
| `esc` | exit without saving; if something changed, it asks first: `y` discard, `s` save, `n` go back |
| `ctrl+e` | open `config.yaml` in `$VISUAL` / `$EDITOR` |

The Keys field is a comma-separated list of paths.

An invalid value (for example, a width of 2632 or `lots` in scrollback) is not
saved: an error appears at the bottom, and the screen stays open.

On save the file is written **from scratch, together with the explanations**.
If you added your own comments to it, they are not kept.

## Language

The interface and the documentation are available in Ukrainian and English.

| `language` | Result |
|---|---|
| `auto` | by locale: the first non-empty of `LC_ALL`, `LC_MESSAGES`, `LANG`; `uk_*` (e.g. `uk_UA.UTF-8`) means Ukrainian, anything else means English |
| `uk` | Ukrainian |
| `en` | English |

The language switches **immediately** after you save the settings: labels, the
status line, dialogs, the built-in help (`ctrl+a ?`). Messages already printed
in a tab (for example, `Connecting to …`) stay as they were; new connections
write in the new language.

Comments in `config.yaml` are also written in the selected language when the
file is saved.

A few things always stay in English: option names in YAML, keys, technical
errors from libraries and from `sessions.yaml` (for example, `yaml: line 3: …`).

Documentation: English in [`docs/`](.), Ukrainian in [`docs/uk/`](uk/).

## Color schemes (Catppuccin)

| `theme` | Result |
|---|---|
| `terminal` | as before: sshdeck uses your terminal's colors and background |
| `latte` | [Catppuccin](https://catppuccin.com/palette) Latte, light |
| `frappe` | Catppuccin Frappé, dark, the softest |
| `macchiato` | Catppuccin Macchiato, dark |
| `mocha` | Catppuccin Mocha, the darkest |

A Catppuccin scheme paints **the whole window**, not just sshdeck's frame:
- background and text: the flavour's `base` and `text`;
- the tree, tabs, status bar, dialogs and input fields use the palette;
- **programs in the tabs** too: the 16 ANSI colors (`ls --color`, the shell
  prompt, `git diff`) become palette colors, the same way Catppuccin's terminal
  themes do it; `red` becomes the palette's `red`, `blue` its `blue`, and so on;
- the cursor is `rosewater`;
- the help (`ctrl+a ?`) uses the palette as well.

Colors a program sets explicitly (256-color or truecolor, e.g. a vim color
scheme) stay as the program set them. Programs that ask the terminal for its
background (vim with `background=auto`) get the palette's background.

On the settings screen the scheme is **previewed immediately** while you cycle
through them with `← →`. `ctrl+s` saves it; `esc` → `y` goes back to the
previous one.

**Truecolor.** The palettes are 24-bit colors. Modern terminals (kitty,
WezTerm, Alacritty, GNOME Terminal, Konsole, Windows Terminal, iTerm2) support
them. If your terminal has only 256 colors, sshdeck picks the nearest ones
itself, but the shades are coarser. If you run sshdeck inside tmux, let tmux
pass truecolor through:

```
# ~/.tmux.conf
set -as terminal-features ",*:RGB"
```

and check that `COLORTERM=truecolor` is set (most terminals set it themselves).

## Local shell

The first entry in the tree, `local  this computer`, opens a tab with a shell
on your own computer: no SSH, no password, no host key. It is handy for a
quick local command next to your remote tabs, or to run `ssh-keygen`, `git` or
an editor without leaving sshdeck.

- `enter` on `local`, or `ctrl+a l` from anywhere, opens a local tab. You can
  open as many as you like; `ctrl+a d` opens another one.
- The tab works like any other: scrollback (`ctrl+a [`), themes, resizing,
  `r` to start a new shell after `exit`.
- It runs `$SHELL` (or `/bin/sh`) in your home directory. `local_command`
  runs something else through `/bin/sh -c`: `zsh -l`, `fish`, `tmux new -A -s
  main`. Programs see `TERM` from the `term` setting, `COLORTERM=truecolor`
  and `SSHDECK=1` (handy in a shell prompt).
- `C` on `local` sets its colors (stored as `local_color` / `local_fill` in
  `config.yaml`); `e` opens these settings.
- It is not part of `sessions.yaml`, so it cannot be renamed, moved, copied
  or deleted. To hide it, turn off **Local shell** in the settings
  (`local_shell: false`).
- Tunnels and `ctrl+a k` (install a key) need an SSH session, so they are not
  available in a local tab.
- Closing the tab hangs up the shell like closing a terminal window
  (`SIGHUP`).
- Linux, macOS and other Unix systems. On Windows the entry is not shown.

## Prefix and tmux

If you use tmux with the prefix `ctrl+a` on your servers (or locally), there is
a conflict:
- **tmux on the server, inside a tab.** sshdeck catches `ctrl+a`. You can pass
  it to tmux with a double `ctrl+a ctrl+a`, but that's inconvenient.
- **sshdeck inside a local tmux.** tmux takes the key first.

Solution: give sshdeck a different key, for example `ctrl+]` (almost never
used), `ctrl+\` or `alt+a`. Then `ctrl+a` belongs fully to tmux.

## Editing by hand

- You can edit the file in any editor, including via `ctrl+e` on the settings
  screen. After you exit the editor, the file is checked and applied right
  away.
- If it has an error, the previous settings stay, and you see what is wrong.
- If `config.yaml` is already broken at startup, sshdeck starts with the
  defaults and shows the error
  `… — using defaults, fix it with ctrl+a ,`. Save the settings through the
  screen, and the file is rewritten correctly.

Unknown options in the file are ignored. Missing options take their default
values.
