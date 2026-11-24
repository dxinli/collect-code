#!/usr/bin/env bash
# Claim a startable tracker ticket: re-read the issue, then write
# "In progress" and leave a claim comment. See docs/agents/issue-tracker.md.
#
# Usage:
#   scripts/claim.sh [<issue-number>]
#
# Without an issue number, reads the startable set
# (status:todo -label:human -is:blocked) and tries each issue in order.
# Exit codes: 0 claimed or nothing to claim; 1 the requested issue could not be
# claimed; 2 usage error. An issue with the human label, an open blocker, or a
# status other than Todo is never written.
set -euo pipefail

HUMAN_LABEL="human"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
usage_error() {
	printf 'error: %s\n' "$*" >&2
	printf 'run with --help for usage\n' >&2
	exit 2
}

usage() {
	cat <<'EOF'
Usage:
  scripts/claim.sh [<issue-number>]

Without an issue number, scripts/claim.sh reads the startable set
(status:todo -label:human -is:blocked) and tries each issue in order.
A non-startable issue is never written: state must be open, status Todo,
blocked_by must be 0, and the human label must be absent. Each write is
printed before it runs; the script writes immediately (no dry-run).

Exit codes: 0 claimed or nothing to claim; 1 the requested issue could not be
claimed; 2 usage error.
EOF
	exit 0
}

TARGET=""
if [ $# -gt 1 ]; then
	usage_error "at most one issue number is accepted"
fi
if [ $# -eq 1 ]; then
	case "$1" in
		-h | --help) usage ;;
		"" | *[!0-9]*) usage_error "issue number must be numeric: $1" ;;
		*) TARGET="$1" ;;
	esac
fi

REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"

why_unstartable() {
	local n="$1" row state status blocked labels
	row="$(gh api "repos/$REPO/issues/$n" --jq \
		'[.state, (.status.name // ""), (.issue_dependencies_summary.blocked_by // 0 | tostring), ([.labels[].name] | join(","))] | @tsv')" ||
		die "cannot read issue #$n"
	IFS=$'\t' read -r state status blocked labels <<<"$row"
	case "$state" in
		open) ;;
		*) echo "state is $state"; return ;;
	esac
	case "$status" in
		Todo) ;;
		*) echo "status is ${status:-none}"; return ;;
	esac
	if [ "$blocked" -ne 0 ]; then
		echo "blocked by $blocked open issue(s)"
		return
	fi
	case ",$labels," in
		*,human,*) echo "carries the human label" ;;
	esac
}

try_claim() {
	local n="$1" reason
	reason="$(why_unstartable "$n")"
	if [ -n "$reason" ]; then
		say "#$n: skipped ($reason)"
		return 1
	fi
	say "+ gh api -X PATCH repos/$REPO/issues/$n -f status=\"In progress\""
	gh api -X PATCH "repos/$REPO/issues/$n" -f status="In progress" --jq '.status.name' >/dev/null
	say "+ gh issue comment $n (claim note)"
	gh issue comment "$n" --body "🤖 认领：status → \`In progress\`。" >/dev/null
	say "#$n: claimed (status In progress)"
	return 0
}

if [ -n "$TARGET" ]; then
	try_claim "$TARGET" || die "#$TARGET could not be claimed"
	exit 0
fi

raw=""
if ! raw="$(gh issue list --state open --search "status:todo -label:$HUMAN_LABEL -is:blocked" \
	--json number --jq '.[].number')"; then
	die "cannot list startable issues (status:todo -label:$HUMAN_LABEL -is:blocked)"
fi

candidates=()
while IFS= read -r n; do
	if [ -n "$n" ]; then
		candidates+=("$n")
	fi
done <<<"$raw"

if [ "${#candidates[@]}" -eq 0 ]; then
	say "no startable issues (status:todo -label:$HUMAN_LABEL -is:blocked)"
	exit 0
fi

say "candidates: ${candidates[*]}"
for n in "${candidates[@]}"; do
	if try_claim "$n"; then
		exit 0
	fi
done

say "no claimable issue in the startable set"
exit 0
