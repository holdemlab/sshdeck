#!/usr/bin/env bash
# Prints the next release tag from the commits since the last vX.Y.Z tag,
# following Conventional Commits (https://www.conventionalcommits.org):
#
#   feat: …                          → minor  (v0.1.0 → v0.2.0)
#   fix: / perf: / refactor: / other → patch  (v0.1.0 → v0.1.1)
#   feat!: … or "BREAKING CHANGE:"   → major  (v1.2.3 → v2.0.0);
#                                      before 1.0 it bumps the minor instead
#   docs: test: chore: ci: style: build: only → no release (prints nothing)
#   "[skip release]" in any commit message    → no release
#
# With no tag yet, the first release is v0.1.0.
# Usage: scripts/next-version.sh [-v]   (-v explains the decision on stderr)
set -euo pipefail

verbose=false
[[ "${1:-}" == "-v" ]] && verbose=true
say() { if $verbose; then echo "$*" >&2; fi; }

last=$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' --exclude '*-*' 2>/dev/null || true)
if [[ -z "$last" ]]; then
  say "no previous tag: first release"
  echo "v0.1.0"
  exit 0
fi

range="$last..HEAD"
if [[ -z "$(git rev-list "$range")" ]]; then
  say "no commits since $last"
  exit 0
fi

bump=none
# %x1e separates commits: subject + body of each one
while IFS= read -r -d $'\x1e' msg; do
  msg="${msg#$'\n'}"
  [[ -z "$msg" ]] && continue
  subject="${msg%%$'\n'*}"
  if [[ "$msg" == *"[skip release]"* ]]; then
    say "skip: '[skip release]' in: $subject"
    exit 0
  fi
  if [[ "$msg" =~ (^|$'\n')BREAKING[\ -]CHANGE: ]] || [[ "$subject" =~ ^[a-zA-Z]+(\([^\)]*\))?!: ]]; then
    level=major
  elif [[ "$subject" =~ ^feat(\([^\)]*\))?: ]]; then
    level=minor
  elif [[ "$subject" =~ ^(docs|test|tests|chore|ci|style|build)(\([^\)]*\))?: ]]; then
    level=none
  elif [[ "$subject" =~ ^Merge\  ]]; then
    level=none # merge commits carry no meaning; their parents are counted
  else
    level=patch
  fi
  say "$level: $subject"
  case "$level:$bump" in
    major:*) bump=major ;;
    minor:none | minor:patch) bump=minor ;;
    patch:none) bump=patch ;;
  esac
done < <(git log --format='%s%n%b%x1e' "$range")

if [[ "$bump" == none ]]; then
  say "only docs/test/chore/ci changes since $last: no release"
  exit 0
fi

IFS=. read -r major minor patch <<<"${last#v}"
patch="${patch%%-*}" # ignore a pre-release suffix
case "$bump" in
  major)
    if ((major == 0)); then
      say "breaking change before 1.0: bumping minor"
      minor=$((minor + 1)) patch=0
    else
      major=$((major + 1)) minor=0 patch=0
    fi
    ;;
  minor) minor=$((minor + 1)) patch=0 ;;
  patch) patch=$((patch + 1)) ;;
esac
say "$last → $bump"
echo "v$major.$minor.$patch"
