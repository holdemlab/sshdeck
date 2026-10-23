# Keys

The screen has three parts:
- the tab bar at the top: `sshdeck`, then the tabs `1 web-01 ●`, `2 db ⇄1 ●`,
  and the version on the right (`v0.2.0`);
- the sidebar with the session tree on the left;
- the active tab's terminal on the right.

At the bottom is the status line with hints.

**Focus** is either on the sidebar or on the terminal. When focus is in the
terminal, **all keys go to the server**, except the prefix. The prefix is
`ctrl+a` by default; you can change it in
[settings](settings.md#prefix-and-tmux).

## Prefix commands (always work)

Press the prefix, release it, then the command key. After the prefix, the
status line is highlighted and shows the available commands.

| After prefix | Action |
|---|---|
| `1`–`9` | go to the tab with this number |
| `n` / `→` / `tab` | next tab |
| `p` / `←` / `shift+tab` | previous tab |
| `s` | switch focus: sidebar ↔ terminal |
| `b` | hide / show the sidebar (terminal at full width) |
| `w` / `x` | close the tab (asks first if the connection is live and `confirm_close: true`) |
| `r` | reconnect a closed tab |
| `d` | open another tab with the same session |
| `l` | open a [local shell](settings.md#local-shell) tab |
| `t` | [tunnels panel](tunnels.md#tunnels-panel-ctrla-t) of the current tab |
| `e` | edit the current tab's session |
| `k` | install your public key on the tab's server, so you log in without a password from then on ([details](authentication.md#installing-a-key-on-the-server-ctrla-k)) |
| `,` | [settings](settings.md) |
| `[` | scrollback mode (see below) |
| `?` / `h` | help (see [below](#in-app-help)) |
| `q` | quit (asks first if there are live connections) |
| prefix again | send the prefix itself to the server (for example, `ctrl+a` for bash or tmux) |

## Sidebar

| Key | Action |
|---|---|
| `↑↓`, `j k`, `ctrl+p` / `ctrl+n` | move |
| `g` / `home`, `G` / `end` | to the top / to the bottom |
| `←` / `h` | fold the folder or go to the parent |
| `→` / `l` | unfold the folder |
| `H` or `-` | fold all folders (the cursor moves to the top-level folder) |
| `L` or `*` | unfold all folders |
| `enter` | open the session in a tab; on a folder, fold / unfold; on `local`, a [local shell](settings.md#local-shell) |
| `t` | open the session in [tunnels-only](tunnels.md#tunnels-only-mode) mode |
| `/` | search (`enter` opens, `esc` clears, `↑↓` / `tab` select) |
| `n` | new session in the selected folder |
| `N` or `+` | new folder right in the tree, inside the selected one; `a/b/c` creates nested folders, `/name` creates one at the top level |
| `R` or `f2` | rename in place |
| `e` | edit the session or folder (form) |
| `C` | colors of the folder or session: text and fill ([details](sessions.md#color-marks)) |
| `c` | duplicate next to it: a session opens a copy form, a folder is copied and renamed |
| `y`, then `p` / `P` | copy into the selected folder / to the top level (`p` again makes one more copy, `esc` finishes) |
| `d` or `delete` | delete the session or folder (asks first) |
| `x`, then `p` / `P` | move into the selected folder / to the top level |
| `E` | open `sessions.yaml` in the editor |
| `r` | reload `sessions.yaml` |
| `,` | settings |
| `?` | help |
| `tab` / `esc` | focus on the terminal (if there are tabs); `esc` also clears search, move and copy |
| `q` / `ctrl+c` | quit |

More on editing the tree: [sessions.md](sessions.md#editing-in-sshdeck).

## Forms (session, folder, settings)

| Key | Action |
|---|---|
| `tab` / `↓`, `shift+tab` / `↑` | next / previous field |
| `enter` | next field; on the last one, save |
| `ctrl+s` | save |
| `esc` | cancel (if something changed, asks first, see below) |
| `ctrl+u` | delete text before the cursor |

### Protection against an accidental `esc`

If something changed in the session form, the folder form or the settings
screen, `esc` does not close it at once, but asks:

```
Discard changes?  y discard · s save · n / esc keep editing
```

| Key | Action |
|---|---|
| `y` | close without saving |
| `s` / `ctrl+s` | save |
| `n`, `esc` or any other key | go back to editing, what you typed stays in place |

If nothing changed, `esc` closes the form at once.

The same works for:
- lines in the tree (`N` — new folder, `R` — rename): if the text changed,
  `esc` asks in the status line `Discard? y discard · enter save · n keep editing`;
- the add-tunnel line in the tunnels panel (`a`): if you already typed
  something, `esc` asks `Discard this tunnel? y discard · s add it · n keep editing`.

## Tab

**State in the tab bar:**

| Marker | Meaning |
|---|---|
| `◌` | connecting (or waiting for a password or key confirmation) |
| `●` | connected |
| `✗` | closed or error; the reason is in the tab and in the status line |
| `⇄N` | active tunnels |
| `⇄ name` | tab in tunnels-only mode |

**Closed tab:**
- `r` or `enter` reconnects;
- the history stays on screen.

**Scrollback mode (`prefix [`):**

| Key | Action |
|---|---|
| `↑↓`, `j k` | one line |
| `pgup` / `ctrl+b` / `ctrl+u` | one page up |
| `pgdn` / `ctrl+f` / `ctrl+d` / `space` | one page down |
| `g` / `G` | to the top / to the bottom |
| `esc` / `q` / `enter` | exit |

The top right corner shows `scroll N/M`. Full-screen programs (vim, htop) don't
write to the history.

**Copy and paste:**
- drag over text in the tab: it is copied to the clipboard when you let go;
  middle click pastes it. See [selecting text](#selecting-text). With
  `mouse: false`, select with your terminal as usual;
- pasting from the clipboard is passed to the server; if the program on the
  server asked for bracketed paste, it gets it.

**Window size.** Resizing the window or hiding the sidebar tells the server the
new size at once. The current size is shown in the status line, for example
`88×28`.

## Mouse

On by default; `mouse: false` in the [settings](settings.md) turns it off.

| Where | What it does |
|---|---|
| tab bar | click: switch to the tab; middle click: close it (asks first if it is connected) |
| tree | click: select; double click: open the session / fold the folder; click on `▾` `▸`: fold at once; wheel: move |
| `/ search` line | click: start a search |
| sidebar edge `│` | drag: change the sidebar width; it is saved in `config.yaml` |
| terminal | drag: select text, copied when you let go; double click: a word; triple click: a line; middle click: paste the copied text |
| terminal, programs with mouse support | vim with `set mouse=a`, htop, mc, tmux get clicks, drags and the wheel; **Alt** + drag still selects text |
| terminal, wheel | scrolls the history like `prefix [`; in full-screen programs without mouse support (less, man) it sends `↑` / `↓` |
| help | wheel: scroll |

### Selecting text

Drag with the left button inside a tab: the selection stays in the tab (the
sidebar and the bars are not included) and works in the scrolled history too.
When you let go, the text goes to the clipboard, and the status bar says
`copied N characters`. Typing or switching tabs clears the highlight.

The text is copied two ways at once:
- through the terminal (OSC 52): works over SSH and inside tmux (it needs
  `set -g set-clipboard on`); kitty, WezTerm, Alacritty, foot, iTerm2 and
  Windows Terminal support it;
- with `wl-copy`, `xclip` or `xsel` on Linux, `pbcopy` on macOS, if installed.
  GNOME Terminal and Konsole need one of these: `sudo apt install wl-clipboard`
  (Wayland) or `xclip` (X11).

Paste with middle click, or as usual with `ctrl+shift+v`.

**Shift + drag** is still the terminal's own selection, across the whole
window.

## In-app help

`ctrl+a ?` (or `?` in the tree) opens [`help.md`](help.md) with formatting
(headings, tables, code). Your prefix is substituted in the text if it is not
`ctrl+a`. The theme, light or dark, is picked from the terminal background.

| Key | Action |
|---|---|
| `↑↓`, `j k`, `pgup` / `pgdn`, `space` | scroll |
| `g` / `G` | to the top / to the bottom |
| `/` | search (`enter` finds) |
| `n` / `N` | next / previous match |
| `esc`, `q`, `?` | close |
