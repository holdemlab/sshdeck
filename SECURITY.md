# Security policy

sshdeck handles SSH credentials and decides which servers to trust, so
security reports are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub:
[Security → Report a vulnerability](https://github.com/holdemlab/sshdeck/security/advisories/new).

Include what you found, how to reproduce it, the version (`sshdeck -version`)
and the impact as you see it. You will get an answer within a week. Once a
fix is released, the advisory is published with credit to you, unless you
prefer otherwise.

## Supported versions

Fixes go into the latest release. Before 1.0 there are no backports to older
versions.

## Scope

Especially interesting:

- host key verification: accepting a changed or unknown key without asking,
  ways to make sshdeck write to `known_hosts` without confirmation;
- credentials: passwords or passphrases reaching the disk, logs or another
  process; keys used for a host they were not meant for;
- jump hosts and tunnels: connecting through or exposing something other than
  what the config says (for example a tunnel listening on all interfaces when
  it should not);
- terminal escape sequences from a server that affect your machine beyond the
  tab (clipboard, files, commands).

Design notes that are intentional and not vulnerabilities: sshdeck keeps
accepted passwords in memory until exit when `remember_passwords` is on (turn
it off to be asked every time), and it never reads `~/.ssh/config`.
