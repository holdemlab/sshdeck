<!-- The title becomes the commit message and decides the release:
     feat: … (minor) · fix: … (patch) · docs:/test:/chore:/ci: … (no release).
     See CONTRIBUTING.md. -->

## What and why

<!-- What does this change, and what problem does it solve? Link the issue if there is one. -->

## Checklist

- [ ] `make check` passes
- [ ] Tests cover the change (the built-in SSH server in `internal/sshtest` helps for connection code)
- [ ] User-visible changes are documented in `docs/` (English) and `docs/uk/` (Ukrainian), and `docs/help.md` / `docs/uk/help.md` if it is a key or a main feature
- [ ] New interface strings have a Ukrainian translation in `internal/i18n/uk.go` (the i18n test checks this)
- [ ] The title follows Conventional Commits (`feat:`, `fix:`, `docs:` …)
