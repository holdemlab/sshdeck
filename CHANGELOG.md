# Changelog

Versions follow [Semantic Versioning](https://semver.org/) and are released
automatically after each merge into `main` (see
[CONTRIBUTING.md](CONTRIBUTING.md#ci-and-releases)). The notes for every
version are generated from the commit messages on
[GitHub Releases](https://github.com/holdemlab/sshdeck/releases); this file
summarizes the larger milestones. Until 1.0, minor versions may change the
config format.

## [0.1.0] - 2026-10-22

First public release.

### Added

- Tabs with a built-in terminal emulator (vim, htop, mc, colors, resizing,
  scrollback) and a configurable prefix key (`ctrl+a` by default).
- Session tree with nested folders: create, rename, move, copy and delete in
  place, fuzzy search, folds remembered between runs.
- Inheritance of `user`, `port`, `jump`, `identity` and color marks along the
  folder tree.
- Built-in SSH client: jump hosts and chains with a separate key per hop,
  keys with passphrases, ssh-agent, keyboard-interactive and passwords.
  Accepted passwords are kept in memory only, never on disk.
- `ctrl+a k` installs your public key on the server (like `ssh-copy-id`) and
  can create a new ed25519 key.
- Own `known_hosts` with trust on first use and refusal of changed host keys.
  `~/.ssh/config` is never read.
- Tunnels: local (`L`), remote (`R`) and dynamic SOCKS5 (`D`); started with the
  session, added on the fly, or in a tunnels-only tab.
- Settings screen (`ctrl+a ,`) and editing `config.yaml` / `sessions.yaml` in
  `$EDITOR`, with validation.
- Color marks for folders and sessions (text color and fill, `C` in the tree),
  shown in the tree and on the tabs.
- Catppuccin color schemes (Latte, Frappé, Macchiato, Mocha) that also recolor
  the programs running in the tabs, with a live preview.
- English and Ukrainian interface and built-in help (`language: auto | en | uk`).
- Built-in Markdown help (`ctrl+a ?`) with search.

[0.1.0]: https://github.com/holdemlab/sshdeck/releases/tag/v0.1.0
