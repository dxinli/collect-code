#!/usr/bin/env bash
# pr-comments.sh — classify a ticket PR's comments under the comment-driven
# protocol (ADR-0012): approved / invalid / feedback.
#
# The record semantics mirror the ci.yml guard exactly and must stay in sync
# with it (same line regex, owner-only records, same sha semantics). The guard
# remains the final gate; this script only classifies for the watch agent.
#
# Usage:
#   scripts/pr-comments.sh <pr-number> [--since <ISO8601>]
#   scripts/pr-comments.sh --self-test
#
# Output (stable, greppable, one finding per line):
#   approved plan|code <sha> <author>
#   invalid  plan|code <sha> <author> <reason>
#   feedback <author>
# Reasons: unresolvable | not-ancestor | from-base | contains-code | not-head.
# A comment is a record only via a bare whole line; quoted or inline-code
# record lines (agent-posted templates) never count. Comments with no record
# line, non-owner comments, and any prose beyond a record line are feedback.

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
. "$SCRIPT_DIR/path-policy.sh"

RECORD_PLAN_RE='^/plan-approved [0-9a-f]{7,40}[[:space:]]*$'
RECORD_CODE_RE='^/code-approved [0-9a-f]{7,40}[[:space:]]*$'

die() { printf 'error: %s\n' "$*" >&2; exit 2; }

# classify_line <line>: stdout "plan <sha>" | "code <sha>" | "" (no record).
classify_line() {
  local line=${1%$'\r'}
  if printf '%s\n' "$line" | grep -qE "$RECORD_PLAN_RE"; then
    printf 'plan %s\n' "$(printf '%s\n' "$line" | awk '{print $2}')"
  elif printf '%s\n' "$line" | grep -qE "$RECORD_CODE_RE"; then
    printf 'code %s\n' "$(printf '%s\n' "$line" | awk '{print $2}')"
  fi
}

# validate_sha <kind> <sha>: stdout a reason or nothing when valid.
# Needs BASE and HEAD (full shas) in the environment — injectable for tests.
validate_sha() {
  local kind=$1 sha=$2 resolved files nonplan
  case "$kind" in
    plan)
      if ! git cat-file -e "$sha^{commit}" 2>/dev/null; then
        echo unresolvable
      elif ! git merge-base --is-ancestor "$sha" "$HEAD" 2>/dev/null; then
        echo not-ancestor
      elif git merge-base --is-ancestor "$sha" "$BASE" 2>/dev/null; then
        echo from-base
      elif ! files=$(git -c core.quotePath=false diff --name-only "$BASE...$sha" 2>/dev/null); then
        echo unresolvable
      else
        nonplan=$(printf '%s\n' "$files" | grep -vE "$PLAN_ALLOW_RE" || true)
        if [ -n "$nonplan" ]; then echo contains-code; fi
      fi
      ;;
    code)
      if ! resolved=$(git rev-parse --verify "$sha^{commit}" 2>/dev/null); then
        echo unresolvable
      elif [ "$resolved" != "$(git rev-parse --verify "$HEAD^{commit}" 2>/dev/null || echo no-head)" ]; then
        echo not-head
      fi
      ;;
    *) die "unknown kind: $kind" ;;
  esac
}

# classify_body <author> <body>: print findings for one comment.
# Uses $OWNER (repo owner) plus validate_sha's BASE/HEAD.
classify_body() {
  local author=$1 body=$2 normalized line rec kind sha reason
  local had_record=0 had_extra=0
  if [ "$author" != "$OWNER" ]; then
    if [ -n "$(printf '%s' "$body" | tr -d '[:space:]')" ]; then
      echo "feedback $author"
    fi
    return 0
  fi
  normalized=$(printf '%s' "$body" | tr -d '\r')
  while IFS= read -r line || [ -n "$line" ]; do
    rec=$(classify_line "$line")
    if [ -n "$rec" ]; then
      had_record=1
      kind=${rec%% *}
      sha=${rec##* }
      reason=$(validate_sha "$kind" "$sha")
      if [ -z "$reason" ]; then
        echo "approved $kind $sha $author"
      else
        echo "invalid $kind $sha $author $reason"
      fi
    elif [ -n "$(printf '%s' "$line" | tr -d '[:space:]')" ]; then
      had_extra=1
    fi
  done <<<"$normalized"
  if [ "$had_record" -eq 0 ] || [ "$had_extra" -eq 1 ]; then
    if [ "$had_record" -eq 0 ] && [ "$had_extra" -eq 0 ]; then
      return 0
    fi
    echo "feedback $author"
  fi
}

fetch_and_classify() {
  local pr=$1 since=${2:-1970-01-01T00:00:00Z}
  local repo author body_b64 ts body
  repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
  OWNER=$(gh repo view --json owner --jq .owner.login)
  BASE=$(gh pr view "$pr" --json baseRefOid --jq .baseRefOid)
  HEAD=$(gh pr view "$pr" --json headRefOid --jq .headRefOid)
  gh api --paginate "repos/$repo/issues/$pr/comments" \
    --jq ".[] | select(.created_at > \"$since\") | [.user.login, (.body | @base64)] | @tsv" |
    while IFS=$'\t' read -r author body_b64; do
      body=$(printf '%s' "$body_b64" | base64 -d)
      classify_body "$author" "$body"
    done
  gh api --paginate "repos/$repo/pulls/$pr/reviews" \
    --jq ".[] | select(.state == \"COMMENTED\" or .state == \"APPROVED\") | select(.submitted_at > \"$since\") | [.user.login, (.body // \"\" | @base64)] | @tsv" |
    while IFS=$'\t' read -r author body_b64; do
      body=$(printf '%s' "$body_b64" | base64 -d)
      classify_body "$author" "$body"
    done
}

# ---------------- self-test ----------------

st_fail=0
expect() {
  local want=$1 got=$2 label=$3
  if [ "$want" != "$got" ]; then
    echo "pr-comments: FAIL: $label: want '$want', got '$got'" >&2
    st_fail=1
  fi
}

self_test() {
  # A. Line classification matrix (guard-parity regex).
  expect "plan 6b2b304" "$(classify_line '/plan-approved 6b2b304')" "bare plan line"
  expect "plan 6b2b304" "$(classify_line '/plan-approved 6b2b304   ')" "trailing spaces"
  expect "plan 6b2b304" "$(classify_line "$(printf '/plan-approved 6b2b304\r')")" "CRLF line"
  expect "code abc1234" "$(classify_line '/code-approved abc1234')" "bare code line"
  expect "" "$(classify_line '/plan-approved 6b2b304 附带文字')" "trailing junk"
  expect "" "$(classify_line ' /plan-approved 6b2b304')" "leading space"
  expect "" "$(classify_line '> /plan-approved 6b2b304')" "quoted template"
  expect "" "$(classify_line '`/plan-approved 6b2b304`')" "inline-code template"
  expect "" "$(classify_line '/plan-approved G123abc')" "non-hex sha"
  expect "" "$(classify_line '/plan-approved 6b2b30')" "sha too short"
  expect "" "$(classify_line '/plan-approved 01234567890123456789012345678901234567890')" "sha too long"
  expect "" "$(classify_line '/plan-approved 6B2B304')" "uppercase hex (guard parity)"
  expect "" "$(classify_line '中文反馈：计划可行，请补风险点')" "Chinese feedback line"
  expect "" "$(classify_line '')" "empty line"

  # B. sha semantics against a temp repo (BASE/HEAD injected).
  local tmp
  tmp=$(mktemp -d)
  (
    set -e
    cd "$tmp"
    git init -q -b main
    git config user.email st@example.com
    git config user.name self-test
    git config core.autocrlf false
    git config core.safecrlf false
    mkdir -p "docs/中文 目录" internal
    echo a >"docs/中文 目录/说明.md"
    git add "docs/中文 目录/说明.md"
    git commit -qm base
    git switch -qc work
    echo b >>"docs/中文 目录/说明.md"
    git add "docs/中文 目录/说明.md"
    git commit -qm plan-non-ascii
    echo c >>AGENTS.md
    git add AGENTS.md
    git commit -qm plan-root-md
    echo p >README.md
    git add README.md
    git commit -qm plan-root-md-outside
    echo g >"internal/主程序.go"
    git add "internal/主程序.go"
    git commit -qm code-non-ascii
  )
  local base head p1 p2 p3 code bogus
  base=$(git -C "$tmp" rev-parse main)
  head=$(git -C "$tmp" rev-parse work)
  p1=$(git -C "$tmp" rev-parse work~3)
  p2=$(git -C "$tmp" rev-parse work~2)
  p3=$(git -C "$tmp" rev-parse work~1)
  code=$(git -C "$tmp" rev-parse work)
  bogus=0000000000000000000000000000000000000000
  pushd "$tmp" >/dev/null
  BASE=$base HEAD=$head
  export BASE HEAD
  expect "" "$(validate_sha plan "$p1")" "plan non-ASCII path valid"
  expect "" "$(validate_sha plan "$p2")" "plan root AGENTS.md valid"
  expect contains-code "$(validate_sha plan "$p3")" "plan root README.md is code"
  expect contains-code "$(validate_sha plan "$code")" "plan code commit rejected"
  expect from-base "$(validate_sha plan "$base")" "plan base commit rejected"
  expect unresolvable "$(validate_sha plan "$bogus")" "plan bogus sha"
  expect "" "$(validate_sha code "$code")" "code head valid"
  expect not-head "$(validate_sha code "$p1")" "code non-head rejected"
  expect unresolvable "$(validate_sha code "$bogus")" "code bogus sha"
  popd >/dev/null
  rm -rf "$tmp"

  # C. classify_body end-to-end fixtures (owner/non-owner, mixed comment).
  # Fixtures use a bogus sha so the verdict does not depend on the repo the
  # self-test runs in.
  OWNER=owner
  BASE=unused
  HEAD=unused
  expect "feedback other" "$(classify_body other '/plan-approved 6b2b304')" "non-owner record is feedback"
  expect "feedback owner" "$(classify_body owner '整体可以，但风险点要展开')" "owner feedback"
  expect "" "$(classify_body owner '   ')" "whitespace-only comment"
  expect "" "$(classify_body owner '')" "empty comment"
  local got
  got=$(classify_body owner "$(printf '/plan-approved %s\n中文：另请更新文档目录' "$bogus")")
  expect "invalid plan $bogus owner unresolvable
feedback owner" "$got" "mixed comment: record + prose"
  got=$(classify_body owner "$(printf '> /plan-approved %s\n有意见：见上' "$bogus")")
  expect "feedback owner" "$got" "quoted template is feedback"

  if [ "$st_fail" -ne 0 ]; then
    echo "pr-comments: self-test FAILED" >&2
    exit 1
  fi
  echo "pr-comments: self-test ok"
}

case "${1:-}" in
  --self-test) self_test ;;
  ''|-h|--help)
    sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
    exit 0
    ;;
  *)
    [[ "$1" =~ ^[1-9][0-9]*$ ]] || die "first argument must be a PR number"
    PR=$1
    shift
    SINCE=1970-01-01T00:00:00Z
    while [ $# -gt 0 ]; do
      case "$1" in
        --since) [ $# -ge 2 ] || die "--since needs a value"; SINCE=$2; shift 2 ;;
        *) die "unknown option: $1" ;;
      esac
    done
    fetch_and_classify "$PR" "$SINCE"
    ;;
esac
