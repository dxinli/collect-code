#!/usr/bin/env bash
# Local impact scan. Not used by CI; CI only does merge-base/version checks.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

BASE_REF="${1:-${SPEC_BASE:-origin/main}}"
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  BASE_REF=main
fi
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  BASE_REF=HEAD
fi

version=$(tr -d '[:space:]' < docs/spec/VERSION 2>/dev/null || echo unknown)
echo "spec-impact: version=$version base=$BASE_REF"

echo
echo "Changed anchors in CHANGELOG for $version:"
awk -v ver="$version" '
  /^## / { if (seen) exit; if ($0 ~ "^## " ver) seen=1 }
  seen { print }
' docs/spec/CHANGELOG.md \
  | grep -oE 'REQ-[A-Z]+-[0-9]{2}|(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' \
  | sort -u || true

echo
echo "Local branches (ahead/behind $BASE_REF, spec changes):"
git for-each-ref --format='%(refname:short)' refs/heads | while IFS= read -r b; do
  ahead=$(git rev-list --count "$BASE_REF".."$b" 2>/dev/null || echo 0)
  behind=$(git rev-list --count "$b".."$BASE_REF" 2>/dev/null || echo 0)
  spec=$(git -c core.quotePath=false diff --name-only "$BASE_REF"..."$b" 2>/dev/null | grep -cE '^(docs/spec/|docs/contracts/)' || true)
  echo "  $b: ahead=$ahead behind=$behind spec_files=$spec"
done

echo
echo "Worktrees:"
git worktree list --porcelain | awk '/^worktree /{print "  "$2} /^branch /{print "    "$2}'

echo
echo "Tickets/plans referencing the current anchors:"
{
  grep -rhoE 'REQ-[A-Z]+-[0-9]{2}|(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' \
    .scratch 2>/dev/null || true
} | sort | uniq -c | sort -nr | sed 's/^/  /'
