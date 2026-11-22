#!/usr/bin/env bash
# Cross-branch spec status. Run at: before plan review, before implementation, before opening a PR.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

BASE_REF="${SPEC_BASE:-origin/main}"
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  BASE_REF=main
fi
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  echo "spec-status: cannot find origin/main or main; using HEAD as base" >&2
  BASE_REF=HEAD
fi

branch=$(git rev-parse --abbrev-ref HEAD)
base=$(git merge-base "$BASE_REF" HEAD 2>/dev/null || echo "$BASE_REF")
changed=$(git diff --name-only "$base"...HEAD 2>/dev/null || true)
behind=$(git rev-list --count HEAD.."$BASE_REF" 2>/dev/null || echo 0)

echo "spec-status: branch=$branch base=$BASE_REF"
echo "spec-status: local version=$(tr -d '[:space:]' < docs/spec/VERSION 2>/dev/null || echo missing)"
if git cat-file -e "$base:docs/spec/VERSION" 2>/dev/null; then
  echo "spec-status: base version=$(git show "$base:docs/spec/VERSION" | tr -d '[:space:]')"
fi

if [ "$behind" -gt 0 ]; then
  echo "spec-status: FAIL: branch is $behind commit(s) behind $BASE_REF; merge or rebase before proceeding" >&2
  exit 1
fi

spec_changed=$(echo "$changed" | grep -E '^(docs/spec/|docs/contracts/)' || true)
code_changed=$(echo "$changed" | grep -E '^(cmd/|internal/|go\.mod$|go\.sum$)' || true)

case "$branch" in
  spec-delta/*)
    [ -n "$spec_changed" ] || echo "spec-status: note: spec-delta branch has no spec package changes"
    echo "$changed" | grep -qx 'docs/spec/VERSION' || { echo "spec-status: FAIL: spec-delta must update docs/spec/VERSION" >&2; exit 1; }
    echo "$changed" | grep -qx 'docs/spec/CHANGELOG.md' || { echo "spec-status: FAIL: spec-delta must update docs/spec/CHANGELOG.md" >&2; exit 1; }
    [ -z "$code_changed" ] || { echo "spec-status: FAIL: spec-delta must not change code: $code_changed" >&2; exit 1; }
    ;;
  docs/spec-v3|spec-bootstrap)
    [ -z "$code_changed" ] || { echo "spec-status: FAIL: bootstrap branch must not change code: $code_changed" >&2; exit 1; }
    ;;
  *)
    if [ -n "$spec_changed" ]; then
      echo "spec-status: FAIL: feature branch modifies the spec package:" >&2
      echo "$spec_changed" >&2
      echo "spec-status: open a spec-delta/<topic> branch instead" >&2
      exit 2
    fi
    ;;
esac

if [ -n "$spec_changed" ]; then
  if git cat-file -e "$base:docs/spec/VERSION" 2>/dev/null; then
    if [ "$(git show "$base:docs/spec/VERSION" | tr -d '[:space:]')" = "$(tr -d '[:space:]' < docs/spec/VERSION)" ]; then
      echo "spec-status: FAIL: spec package changed but VERSION did not" >&2
      exit 1
    fi
  fi
fi

refs=$(for f in $changed; do
  case "$f" in docs/spec/*|docs/contracts/*) continue ;; esac
  [ -f "$f" ] || continue
  grep -hoE 'REQ-[A-Z]+-[0-9]{2}|(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' "$f" 2>/dev/null || true
done | sort -u)

for id in $refs; do
  if ! grep -rq "$id" docs/spec docs/contracts; then
    echo "spec-status: FAIL: branch references $id but the current spec package does not define it" >&2
    exit 1
  fi
done

echo "spec-status: ok"
