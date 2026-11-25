#!/usr/bin/env bash
# Create a tracker ticket from a brief that follows
# .github/ISSUE_TEMPLATE/ticket.md, then write its native status and optional
# routing/dependency edges. See docs/agents/issue-tracker.md.
#
# Usage:
#   scripts/ticket-new.sh --title <title> --body-file <file> [options]
#
# Options:
#   --title <title>          Issue title (required)
#   --body-file <file>       Ticket brief file (required)
#   --spec-ticket            Attach to spec anchor #71; the title must be
#                            "<NN>[<letter>]: <title>"
#   --status Todo|Backlog    Native issue status (default: Todo)
#   --human                  Add the human routing label
#   --blocked-by <n[,n]>     Add native blocked-by edges
#   --dry-run                Print the commands without running them
#   -h, --help               Show this help
#
# A plain ticket is standalone. A spec ticket is a spec slice, a split-out of
# an existing spec ticket, or a fix of an implementation ticket: it attaches
# #71 and must use the spec title form. chore/hotfix titles are standalone; a
# hotfix of a spec ticket's implementation is created as a spec ticket instead.
# Creation is not transactional: if a later step fails, the script prints the
# created issue URL and exits non-zero.
set -euo pipefail

SPEC_ANCHOR=71
HUMAN_LABEL="human"

TITLE=""
BODY_FILE=""
STATUS="Todo"
HUMAN=false
SPEC_TICKET=false
BLOCKED_BY=""
DRY_RUN=false

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
  scripts/ticket-new.sh --title <title> --body-file <file> [options]

Options:
  --title <title>          Issue title (required)
  --body-file <file>       Ticket brief file (required)
  --spec-ticket            Attach to spec anchor #71; the title must be
                           "<NN>[<letter>]: <title>"
  --status Todo|Backlog    Native issue status (default: Todo)
  --human                  Add the human routing label
  --blocked-by <n[,n]>     Add native blocked-by edges
  --dry-run                Print the commands without running them
  -h, --help               Show this help

The brief must contain the ticket template fields: "**Ticket:**" (with an id),
"**What to build:**", "**Current behavior:**", "**Desired behavior:**",
"**Key interfaces:**", "**Acceptance criteria:**", and "**Out of scope:**".
Without --spec-ticket the issue is standalone. --human requires status Todo.
EOF
	exit 0
}

while [ $# -gt 0 ]; do
	case "$1" in
		--title)
			[ $# -ge 2 ] || usage_error "--title needs a value"
			TITLE="$2"
			shift 2
			;;
		--body-file)
			[ $# -ge 2 ] || usage_error "--body-file needs a value"
			BODY_FILE="$2"
			shift 2
			;;
		--status)
			[ $# -ge 2 ] || usage_error "--status needs a value"
			STATUS="$2"
			shift 2
			;;
		--blocked-by)
			[ $# -ge 2 ] || usage_error "--blocked-by needs a value"
			BLOCKED_BY="$2"
			shift 2
			;;
		--human) HUMAN=true; shift ;;
		--spec-ticket) SPEC_TICKET=true; shift ;;
		--dry-run) DRY_RUN=true; shift ;;
		-h | --help) usage ;;
		*) usage_error "unknown argument: $1" ;;
	esac
done

[ -n "$TITLE" ] || usage_error "--title is required"
[ -n "$BODY_FILE" ] || usage_error "--body-file is required"
[ -f "$BODY_FILE" ] || die "body file not found: $BODY_FILE"
case "$STATUS" in
	Todo | Backlog) ;;
	*) usage_error "--status must be Todo or Backlog (got '$STATUS')" ;;
esac
if [ -n "$BLOCKED_BY" ] && ! [[ "$BLOCKED_BY" =~ ^[0-9]+(,[0-9]+)*$ ]]; then
	usage_error "--blocked-by must be comma-separated issue numbers"
fi
if $HUMAN && [ "$STATUS" != "Todo" ]; then
	usage_error "--human requires --status Todo (see triage-labels.md)"
fi

for field in \
	'**Ticket:**' \
	'**What to build:**' \
	'**Current behavior:**' \
	'**Desired behavior:**' \
	'**Key interfaces:**' \
	'**Acceptance criteria:**' \
	'**Out of scope:**'; do
	grep -Fq -- "$field" "$BODY_FILE" || die "body is missing required field: $field"
done
grep -Eq '^\*\*Ticket:\*\*[[:space:]]+[^<[:space:]]' "$BODY_FILE" ||
	die "body has no ticket id on the **Ticket:** line"

WANT_PARENT=false
if $SPEC_TICKET; then
	[[ "$TITLE" =~ ^[0-9]{2}[a-z]?:[[:space:]]+.+ ]] ||
		usage_error "--spec-ticket requires a title like '06a: <title>' (got '$TITLE')"
	WANT_PARENT=true
fi

create=(gh issue create --title "$TITLE" --body-file "$BODY_FILE")
if $WANT_PARENT; then
	create+=(--parent "$SPEC_ANCHOR")
fi
if $HUMAN; then
	create+=(--label "$HUMAN_LABEL")
fi

if $DRY_RUN; then
	say "dry-run: nothing will be created"
	say "+ $(printf '%q ' "${create[@]}")"
	say "+ gh api -X PATCH repos/<owner>/<repo>/issues/<new> -f status=\"$STATUS\""
	if [ -n "$BLOCKED_BY" ]; then
		say "+ gh issue edit <new> --add-blocked-by $BLOCKED_BY"
	fi
	say "parent: $($WANT_PARENT && echo "#$SPEC_ANCHOR" || echo none)"
	say "status: $STATUS"
	say "human: $HUMAN"
	exit 0
fi

REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
if $WANT_PARENT; then
	gh api "repos/$REPO/issues/$SPEC_ANCHOR" --jq .number >/dev/null 2>&1 ||
		die "spec anchor #$SPEC_ANCHOR not found in $REPO"
fi

say "+ $(printf '%q ' "${create[@]}")"
url="$("${create[@]}")" || die "issue creation failed"
n="${url##*/}"
say "created: $url"

if ! gh api -X PATCH "repos/$REPO/issues/$n" -f status="$STATUS" --jq '.status.name' >/dev/null; then
	die "created $url but failed to set status '$STATUS'; set it manually"
fi

if [ -n "$BLOCKED_BY" ]; then
	if ! gh issue edit "$n" --add-blocked-by "$BLOCKED_BY" >/dev/null; then
		die "created $url but failed to add blocked-by: $BLOCKED_BY"
	fi
fi

say ""
gh api "repos/$REPO/issues/$n" --jq \
	'"status: \(.status.name // "-")\nlabels: \([.labels[].name] | join(","))\nblocked_by: \(.issue_dependencies_summary.blocked_by)\nparent: \(.parent.number // "none")"'
