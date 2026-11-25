#!/usr/bin/env bash
# Cross-branch spec status. Run at: before plan review, before implementation, before opening a PR.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
. "$(git rev-parse --show-toplevel)/scripts/path-policy.sh"

BASE_REF="${SPEC_BASE:-origin/main}"
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  BASE_REF=main
fi
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  echo "spec-status: cannot find origin/main or main; using HEAD as base" >&2
  BASE_REF=HEAD
fi

branch="${SPEC_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"
base=$(git merge-base "$BASE_REF" HEAD 2>/dev/null || echo "$BASE_REF")
changed=$(git -c core.quotePath=false diff --name-only "$base"...HEAD 2>/dev/null || true)
behind=$(git rev-list --count HEAD.."$BASE_REF" 2>/dev/null || echo 0)

local_version=$(tr -d '[:space:]' < docs/spec/VERSION 2>/dev/null || true)
base_version=""
if git cat-file -e "$base:docs/spec/VERSION" 2>/dev/null; then
  base_version=$(git show "$base:docs/spec/VERSION" | tr -d '[:space:]')
fi
echo "spec-status: branch=$branch base=$BASE_REF"
echo "spec-status: local version=${local_version:-missing}"
if [ -n "$base_version" ]; then
  echo "spec-status: base version=$base_version"
fi

if [ "$behind" -gt 0 ]; then
  echo "spec-status: FAIL: branch is $behind commit(s) behind $BASE_REF; merge or rebase before proceeding" >&2
  exit 1
fi

spec_changed=$(echo "$changed" | grep -E "$SPEC_DELTA_ALLOW_RE" || true)
outside_spec=$(echo "$changed" | grep -vE "$SPEC_DELTA_ALLOW_RE" || true)

has_base_version=0
if [ -n "$base_version" ]; then
  has_base_version=1
fi

if [ -n "$local_version" ] && [ -n "$base_version" ] && [ "$local_version" != "$base_version" ]; then
  older=$(printf '%s\n%s\n' "$base_version" "$local_version" | sort -V | head -n1)
  if [ "$older" = "$local_version" ]; then
    echo "spec-status: FAIL: local version $local_version is older than base $base_version" >&2
    exit 1
  fi
fi

case "$branch" in
  spec-delta/*)
    [ -z "$outside_spec" ] || { echo "spec-status: FAIL: spec-delta must only change docs/spec or docs/contracts:" >&2; echo "$outside_spec" >&2; exit 1; }
    [ -n "$spec_changed" ] || { echo "spec-status: FAIL: spec-delta must change docs/spec or docs/contracts" >&2; exit 1; }
    ;;
esac

if [ -n "$spec_changed" ]; then
  case "$branch" in
    spec-delta/*)
      echo "$changed" | grep -qx 'docs/spec/VERSION' || { echo "spec-status: FAIL: spec-delta must update docs/spec/VERSION" >&2; exit 1; }
      echo "$changed" | grep -qx 'docs/spec/CHANGELOG.md' || { echo "spec-status: FAIL: spec-delta must update docs/spec/CHANGELOG.md" >&2; exit 1; }
      if [ "$has_base_version" -eq 1 ]; then
        if [ "$(git show "$base:docs/spec/VERSION" | tr -d '[:space:]')" = "$(tr -d '[:space:]' < docs/spec/VERSION)" ]; then
          echo "spec-status: FAIL: spec-delta changed the package but VERSION did not" >&2
          exit 1
        fi
      fi
      ;;
    *)
      if [ "$has_base_version" -eq 1 ]; then
        echo "spec-status: FAIL: feature branch modifies the spec package:" >&2
        echo "$spec_changed" >&2
        echo "spec-status: open a spec-delta/<topic> branch instead" >&2
        exit 2
      fi
      # Bootstrap: the base has no spec package yet. Allow creating it,
      # but never mix code into the bootstrap change.
      [ -z "$outside_spec" ] || { echo "spec-status: FAIL: bootstrap branch must only change the spec package:" >&2; echo "$outside_spec" >&2; exit 1; }
      echo "spec-status: note: bootstrapping the spec package (base has no docs/spec/VERSION)"
      ;;
  esac
fi

expand_refs() {
  while IFS= read -r tok; do
    [ -n "$tok" ] || continue
    case "$tok" in
      REQ-*-[0-9][0-9]..[0-9][0-9]|REQ-*-[0-9][0-9]/[0-9][0-9])
        prefix="${tok%..*}"
        prefix="${prefix%/*}"
        end="${tok##*..}"
        end="${end##*/}"
        base="${prefix%-*}"
        start="${prefix##*-}"
        s=$((10#$start))
        e=$((10#$end))
        for ((i=s; i<=e; i++)); do
          printf '%s-%02d\n' "$base" "$i"
        done
        ;;
      *) printf '%s\n' "$tok" ;;
    esac
  done | sort -u
}

refs=$(for f in $changed; do
  if printf '%s\n' "$f" | grep -qE "$SPEC_DELTA_ALLOW_RE"; then continue; fi
  [ -f "$f" ] || continue
  grep -hoE 'REQ-[A-Z]+-[0-9]{2}(\.\.|/)[0-9]{2}|REQ-[A-Z]+-[0-9]{2}|(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' "$f" 2>/dev/null || true
done | expand_refs || true)

for id in $refs; do
  if ! grep -rq "$id" docs/spec docs/contracts; then
    echo "spec-status: FAIL: branch references $id but the current spec package does not define it" >&2
    exit 1
  fi
done

echo "spec-status: ok"
