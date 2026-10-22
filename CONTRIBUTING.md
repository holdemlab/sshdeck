# Contributing to sshdeck

Thanks for helping! Bug reports, ideas, translations and pull requests are all
welcome. For larger changes, open an issue first so we can agree on the
approach before you spend time on it.

## Getting started

You need Go 1.26 or newer.

```sh
git clone https://github.com/holdemlab/sshdeck
cd sshdeck
make          # bin/sshdeck
make check    # what CI runs on a pull request: gofmt, vet, tidy, race tests
```

`make help` lists all tasks.

Try your build without touching your real config:

```sh
./bin/sshdeck -sessions /tmp/s.yaml -config /tmp/c.yaml
```

## Project layout

```
cmd/sshdeck/          entry point, flags
internal/config/      sessions.yaml and config.yaml: types, load/save, inheritance, tree operations
internal/sshclient/   SSH client: routing through jump hosts, authentication, known_hosts, pty, key install
internal/tunnel/      tunnels L / R / D (SOCKS5)
internal/sshtest/     an in-process SSH server for tests
internal/ui/          Bubble Tea model: tabs, tree, forms, dialogs, themes
internal/i18n/        interface translations
docs/                 English docs; docs/uk/ Ukrainian; help.md files are embedded in the binary
```

## Guidelines

- **Tests.** New behaviour comes with tests. Connection code is tested
  against `internal/sshtest`, a real SSH server running inside the test, so
  no network or external `sshd` is needed. UI behaviour is tested by sending
  key presses to the model (see `send` and `keys` in
  `internal/ui/model_test.go`). Run `make check` before you push.
- **Formatting.** `gofmt` and `go vet` must be clean; CI checks both, and
  `go mod tidy`.
- **Docs.** Every user-visible change is described in the docs, in both
  languages: `docs/` (English) and `docs/uk/` (Ukrainian). New keys and main
  features also go into the built-in help, `docs/help.md` and
  `docs/uk/help.md`. If you don't write Ukrainian, write the English part and
  say so in the pull request; a maintainer will add the translation.
- **Interface strings** are written in English and wrapped in `T(...)`
  (`setStatus` translates its format by itself). The English text is the key:
  add the Ukrainian translation to `internal/i18n/uk.go`. The tests fail if a
  string has no translation, or if a translation has different `%s` / `%d`
  verbs than the original.
- **Security.** Passwords and passphrases never reach the disk or logs. Host
  keys are never accepted without asking. When in doubt, ask in the issue.
## Commits and pull requests

Small, focused pull requests are easiest to review. Pull requests are
squash-merged, so **the pull request title becomes the commit message**, and it
decides the next version. Write it in the
[Conventional Commits](https://www.conventionalcommits.org) form:

| Title | Release after the merge |
|---|---|
| `feat: color marks for folders` | minor: v0.3.0 → v0.4.0 |
| `fix: crash on resize`, `perf: …`, `refactor: …`, anything else | patch: v0.3.0 → v0.3.1 |
| `feat!: …`, or `BREAKING CHANGE:` in the description | major (before 1.0: minor) |
| `docs: …`, `test: …`, `chore: …`, `ci: …`, `style: …`, `build: …` | none |
| anything with `[skip release]` | none |

A scope is fine: `fix(tunnel): …`. `make next-version` shows what your branch
would release.

## Adding a language

1. Add the language code to `internal/i18n` (`Lang`, `Resolve`, `Valid`) and a
   map like `uk.go`.
2. Extend the completeness test to check the new map.
3. Add `docs/<code>/` with the translated docs and embed its `help.md` in
   `docs/docs.go`.
4. Add the choice to the settings screen (`languageChoices`) and to
   `config.yaml` validation.

## CI and releases

Everything runs on GitHub Actions:

| Workflow | When | What |
|---|---|---|
| `ci.yml` | every pull request into `main` | `make lint`, `make race` on Linux and macOS, Linux binaries as a downloadable artifact |
| `autotag.yml` | every push to `main` (a merged pull request) | runs CI again, then `scripts/next-version.sh` picks the version from the commits since the last tag and pushes the tag |
| `release.yml` | a new `vX.Y.Z` tag | tests, then GoReleaser builds Linux binaries for amd64 and arm64, `.deb` / `.rpm` / `.apk` packages and `checksums.txt`, and publishes the GitHub release with notes generated from the commits |

So a release needs no manual steps: merge a `feat:` or `fix:` pull request and
the tag and the release follow. A tag pushed by hand (`git tag v1.0.0 && git
push origin v1.0.0`) releases too, and *Actions → Release → Run workflow*
re-runs a release for an existing tag. A dry run of the build: `make snapshot`.

Recommended repository settings: allow squash merging only, and protect
`main` so pull requests need the `lint` and `test` checks to pass.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
