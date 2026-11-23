#!/usr/bin/env bash
# List open ready-for-agent issues whose blockers are all closed.
# Requires an authenticated gh CLI; the issue tracker is GitHub Issues.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

if ! repo="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null)"; then
  echo "tickets: gh is not installed, not authenticated, or this is not a GitHub repo" >&2
  exit 1
fi

rows="$(gh issue list --state open --label ready-for-agent --limit 200 \
  --json number,title,blockedBy \
  --jq '.[] |
    [ .number,
      .title,
      ([.blockedBy.nodes[] | select(.state == "OPEN")] | length),
      ([.blockedBy.nodes[] | select(.state == "OPEN") | "#\(.number)"] | join(","))
    ] | @tsv' | sort -n)"

ready=()
blocked=()
while IFS=$'\t' read -r number title open blockers; do
  [ -n "${number:-}" ] || continue
  if [ "$open" = "0" ]; then
    ready+=("$number  $title")
  else
    blocked+=("$number  $title  (blocked by $blockers)")
  fi
done <<< "$rows"

printf 'ready-for-agent: %d ready, %d blocked\n' "${#ready[@]}" "${#blocked[@]}"

if [ "${#ready[@]}" -gt 0 ]; then
  echo
  echo "ready now:"
  printf '  #%s\n' "${ready[@]}"
fi

if [ "${#blocked[@]}" -gt 0 ]; then
  echo
  echo "blocked:"
  printf '  #%s\n' "${blocked[@]}"
fi
