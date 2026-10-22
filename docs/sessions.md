# Sessions and folders

Sessions are stored in `~/.config/sshdeck/sessions.yaml` as a tree: folders
contain sessions and other folders. You can edit the tree in sshdeck (see
[below](#editing-in-sshdeck)) or by hand in the file.

## File format

```yaml
# root: settings for all sessions
user: admin

folders:
  - name: infra
    sessions:
      - name: proxy
        host: proxy.example.com
        user: jumper
        identity: ~/.ssh/id_proxy

  - name: prod
    jump: infra/proxy          # everything in prod goes through proxy
    identity: ~/.ssh/id_prod
    folders:
      - name: web
        user: deploy
        sessions:
          - name: web-01
            host: 10.0.10.11
            note: nginx + app
    sessions:
      - name: db-main
        host: 10.0.20.5
        port: 2222
        forwards:
          - L 5432:localhost:5432
      - name: db-top
        host: 10.0.20.5
        port: 2222
        command: htop

sessions:
  - name: homelab
    host: 192.168.1.10
    user: me
```

A fuller example: [`examples/sessions.yaml`](../examples/sessions.yaml).

### Session fields

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | name in the tree; unique within the folder; no `/` |
| `host` | yes | host name or IP |
| `user` | no | user; if empty, taken from the folder or the root, and if not set anywhere, the local `$USER` |
| `port` | no | port; inherited by default, otherwise 22 |
| `jump` | no | what to connect through, see [jump-hosts.md](jump-hosts.md) |
| `identity` | no | path to the private key, `~` is expanded; see [authentication.md](authentication.md) |
| `command` | no | command to run instead of the login shell (like `ssh host command`), for example `htop` |
| `forwards` | no | tunnels that start together with the connection, see [tunnels.md](tunnels.md) |
| `color` | no | text color in the tree and on the tab, see [color marks](#color-marks) |
| `fill` | no | fill (background) color in the tree and on the tab |
| `note` | no | free-form note; shown at the bottom of the sidebar |

### Folder fields

| Field | Meaning |
|---|---|
| `name` | name; unique among folders on the same level |
| `user`, `port`, `jump`, `identity` | defaults for everything inside |
| `color`, `fill` | color mark of the folder and everything inside |
| `folders` | nested folders |
| `sessions` | sessions in this folder |

The root of the file can also have `user`, `port`, `jump`, `identity`. They
apply to all sessions.

## Inheritance

`user`, `port`, `jump` and `identity` are looked up from the session up the
tree: **session → its folder → parent folder → … → root**. The nearest
non-empty value wins.

For `prod/web/web-01` from the example above:

| Field | Value | From |
|---|---|---|
| user | `deploy` | folder `prod/web` |
| port | 22 | not set anywhere |
| jump | `infra/proxy` | folder `prod` |
| identity | `~/.ssh/id_prod` | folder `prod` |

You can see the result in the sidebar under the tree: `user@host`, `via …`. In
the edit form an inherited value is shown as `empty = inherit`.

Limitation: you can only override an inherited value with another one; you
cannot "clear" it for a single session. If `jump` is set at the root and one
session must connect directly, keep it in a separate branch where `jump` is not
set.

## Color marks

You can mark a folder or a session with colors so environments are told apart
at a glance: for example `prod` with a red fill, `staging` with orange text.

```yaml
folders:
  - name: prod
    fill: red          # fill: everything in prod on a red background
    sessions:
      - {name: web-01, host: 10.0.0.1}
      - {name: lab, host: 10.0.0.9, fill: none}   # no fill
  - name: staging
    color: peach       # text color
```

| Field | What it sets |
|---|---|
| `color` | the text color of the name |
| `fill` | the fill (background) of the name; the text then becomes dark or light automatically so it stays readable. You can still set the text color separately |

**Values:**
- a Catppuccin palette name: `rosewater`, `flamingo`, `pink`, `mauve`, `red`,
  `maroon`, `peach`, `yellow`, `green`, `teal`, `sky`, `sapphire`, `blue`,
  `lavender`, and the neutral `text`, `subtext`, `overlay`, `surface`. The
  aliases `orange`, `purple`, `cyan`, `gray`, `white`, `magenta` work too;
- `#rrggbb` or `#rgb`: any color;
- `none`: no color, even if a folder above has a mark;
- empty: inherit from the folder.

Palette names are drawn in the colors of **the selected theme**: `red` in Mocha
is Mocha's red, in Latte it is Latte's red. With the `terminal` theme, sshdeck
uses the Mocha palette on a dark terminal and Latte on a light one. A
`#rrggbb` color is always the same.

**Inheritance** works like `user` or `jump`: a folder's mark applies to all
nested folders and sessions until something below sets its own (or `none`).

**Where it shows:**
- in the tree: the folder or session name; with a fill it looks like a colored
  label;
- on the tab: a connection's tab is drawn in its session's colors. If only a
  text color is set, the active tab is filled with it. The active tab is always
  bold and underlined;
- changes show on open tabs immediately.

**How to set it:**
- `C` in the tree opens the color picker for the selected folder or session:
  ```
  Colors of prod/web

  ▸ Text      ↑ ∅ ████████████████████████████████████  inherit
              ▔▔
    Fill      ↑ ∅ ████████████████████████████████████  red
                      ▔▔
  Preview:  web
  ```
  `← →` picks a color, `↑↓` switches between text and fill, `x` goes back to
  "inherit", `enter` saves, `esc` cancels. `↑` on the strip means "inherit",
  `∅` means `none`. Below the strip you see how the name will look;
- the session and folder forms (`e`, `n`) have **Color** and **Fill** fields:
  `← →` picks from the palette, or type `#rrggbb`. A preview is shown below the
  fields;
- by hand in `sessions.yaml`.

## Editing in sshdeck

All actions work in the sidebar. If the focus is in the terminal, press
`ctrl+a s` first.

### Folders

- **`N`** opens a name field right in the tree, in the selected folder or next
  to the selected session. The status line shows where exactly
  (`New folder in prod/web`). `a/b/c` creates all levels at once.
  **A leading `/` means the top level**, wherever the cursor is: `/infra`
  creates a folder in the root, `/infra/dev` creates `infra` in the root and
  `dev` inside it. `enter` creates, `esc` cancels (if you have already typed
  something, it asks first). If such a folder already exists, the field stays
  open with an error.
- **`R`** renames in place.
- **`e` on a folder** opens a form:
  - Name;
  - Inside — the parent folder; this lets you move the folder, empty means
    the top level;
  - User, Port, Jump, Key file — inherited by everything inside.
- **`d`** deletes the folder with all its contents. Before deleting, a dialog
  shows how many sessions are inside and warns you if any of them are used as
  jump hosts.

### Sessions

- **`n`** creates a new session in the selected folder: the Folder field is
  filled in automatically. An empty Folder field means the top level. If you
  type a folder that doesn't exist yet in Folder, it will be created.
- **`e`** opens the edit form with all fields; `ctrl+s` saves, `esc` cancels.
  If you have already changed something, `esc` first asks whether to discard
  the changes (see
  [keys.md](keys.md#protection-against-an-accidental-esc)). The Tunnels field takes
  comma-separated entries: `L 8080:localhost:80, D 1080`.
- **`ctrl+a e`** edits the session of the current tab when the focus is in the
  terminal.
- **`c`** makes a copy next to it: a form opens with the same fields and a free
  name (`…-copy`), so you can change the host right away. See
  [Copying](#copying) for details.
- **`R`** renames in place.
- **`d`** deletes (with confirmation).

### Moving

- **`x`** cuts a session or folder (it is marked with `✂`).
- **`p`** pastes into the selected folder. If the cursor is on a session, into
  that session's folder.
- **`P`** pastes at the top level.
- **`esc`** cancels the move.

You can't put a folder inside itself. A moved folder keeps its settings.
The old folder stays, even if empty, and you can delete it with `d`.

### Copying

You can copy both sessions and folders with all their contents.

- **`c`** duplicates the selection next to the original:
  - **session**: a form for the copy opens, and you can adjust it before
    saving (`esc` creates nothing);
  - **folder**: the copy is created at once with the name `…-copy`, and inline
    rename opens (`enter` keeps the name as is).
- **`y`**, then **`p`** copies to another folder (like `x`/`p`, but the
  original stays in place):
  - `y` marks a session or folder (`⧉`);
  - `p` pastes a copy into the selected folder (if the cursor is on a session,
    into its folder);
  - `P` pastes at the top level;
  - you can press `p` several times to make several copies;
  - `esc` ends copying.

Rules:
- **Names.** If the name is already taken in the target folder, `-copy` is
  added, then `-copy-2`, `-copy-3`… In another folder the copy keeps the same
  name if it is free there.
- **Everything is copied:** host, user, port, key, command, tunnels, note,
  folder settings and all nested folders. The copy is fully independent of the
  original.
- **Jump inside a copied folder is switched to the copy.** If in
  `acme/dev` the session `main` goes through `acme/dev/proxy`, then in
  the copy `acme/stage` its `main` goes through `acme/stage/proxy`.
  This way a copy of an environment is self-contained right away. References
  to sessions outside the copied folder (for example, to a shared bastion) are
  not changed.
- You can copy a folder into itself: the state at the moment of pasting is
  copied, with no endless nesting.

### What updates automatically

- When a session or folder is renamed or moved, all `jump:` references to it
  (and to sessions inside the folder) are rewritten to the new path.
- Open tabs get the new settings of their session. They take effect on the
  next reconnect (`ctrl+a r`); the current connection is not interrupted.
- Deleting a session does not close its open tabs.

### Collapsing folders

- **Single folders:** `←` / `h` collapses a folder (or moves to the parent),
  `→` / `l` expands, `enter` on a folder toggles.
- **All at once:** `H` or `-` collapses all folders, including nested ones. The
  cursor moves to the top-level folder it was in. `L` or `*` expands all.
- **The state is remembered between runs**: sshdeck restores which folders are
  collapsed on the next start. It is stored not in `sessions.yaml` but in a
  separate state file `~/.local/state/sshdeck/state.yaml` (or
  `$XDG_STATE_HOME/sshdeck/state.yaml`). You can delete the file at any time:
  then everything is expanded.
- **The state follows the folder:** after a rename or move the folder stays
  collapsed; a copy of a folder is collapsed the same way as the original;
  deleted folders are removed from the state file.
- During search the tree is flat, and collapsing applies again as soon as the
  search is cleared.

### Search

`/` searches by name, path and host (fuzzy search: `pw1` finds
`prod/web/web-01`). During search the tree becomes a flat list. `enter` opens
the session, `esc` clears the search.

## Editing by hand

- **`E`** in the sidebar opens `sessions.yaml` in `$VISUAL` or `$EDITOR`
  (`vi` by default). After you exit, the file is checked and reloaded. If it
  has an error, sshdeck keeps the previous state and shows the error with a
  line number.
- **`r`** in the sidebar reloads the file if it was changed from outside.

What is checked on load:
- every session has a `name` and a `host`;
- names are not repeated within one folder;
- every entry in `forwards` is written correctly.

> When sessions are changed through the UI, the file is rewritten completely,
> and **comments in `sessions.yaml` are lost**. If comments matter to you,
> edit the file only by hand (`E`).
