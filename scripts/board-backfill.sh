#!/usr/bin/env bash
# Migrate the issue tracker to native issue statuses, one spec anchor, and the
# reduced label set. Dry-run by default; --apply mutates the tracker.
#
# Usage: scripts/board-backfill.sh [--dry-run] [--apply] [--yes]
#
# Requires an authenticated gh user with issue write access and access to the
# native issue statuses preview. Idempotent: it only fills null statuses, only
# adds missing sub-issue links, and only removes deprecated labels.
set -euo pipefail

APPLY=false
ASSUME_YES=false
for arg in "$@"; do
	case "$arg" in
		--dry-run) APPLY=false ;;
		--apply) APPLY=true ;;
		--yes) ASSUME_YES=true ;;
		-h | --help)
			sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
			exit 0
			;;
		*)
			echo "unknown argument: $arg" >&2
			exit 2
			;;
	esac
done

REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
ROOT="$(git rev-parse --show-toplevel)"
SPEC_VERSION="$(tr -d '[:space:]' <"$ROOT/docs/spec/VERSION")"

ANCHOR_TITLE="Spec: 采集引擎行为契约"
ANCHOR_LABEL="spec-parent"
HUMAN_LABEL="human"

DEPRECATED_LABELS=(ready-for-agent ready-for-human in-progress done wontfix)
UNUSED_DEFAULT_LABELS=(accessibility documentation duplicate "good first issue" "help wanted" invalid question)

anchor=""
status_writes=0
links_added=0
links_skipped=0
labels_removed=0
labels_deleted=0
labels_created=0
retitled=false

say() { printf '%s\n' "$*"; }
plan() { printf '  [dry-run] %s\n' "$*"; }

has_label() {
	case ",$1," in
		*",$2,"*) return 0 ;;
		*) return 1 ;;
	esac
}

label_exists() {
	gh label list --limit 200 --json name --jq '.[].name' | grep -Fxq -- "$1"
}

label_in_use() {
	local issues prs
	issues="$(gh issue list --state all --label "$1" --json number --jq 'length' 2>/dev/null || echo 0)"
	prs="$(gh pr list --state all --label "$1" --json number --jq 'length' 2>/dev/null || echo 0)"
	[ "$issues" -gt 0 ] || [ "$prs" -gt 0 ]
}

ensure_label() {
	local name="$1" color="$2" desc="$3"
	if label_exists "$name"; then
		say "label $name: exists"
		return
	fi
	if $APPLY; then
		gh label create "$name" --color "$color" --description "$desc" >/dev/null
		say "label $name: created"
	else
		plan "create label $name"
	fi
	labels_created=$((labels_created + 1))
}

delete_label() {
	local name="$1" force="${2:-false}"
	if ! label_exists "$name"; then
		say "label $name: absent"
		return
	fi
	if ! $force && label_in_use "$name"; then
		say "label $name: in use, not deleted"
		return
	fi
	if $APPLY; then
		gh label delete "$name" --yes >/dev/null
		say "label $name: deleted"
	else
		plan "delete label $name"
	fi
	labels_deleted=$((labels_deleted + 1))
}

remove_label() {
	local n="$1" name="$2"
	if $APPLY; then
		gh issue edit "$n" --remove-label "$name" >/dev/null
		say "  #$n remove label $name"
	else
		plan "#$n remove label $name"
	fi
	labels_removed=$((labels_removed + 1))
}

set_status() {
	local n="$1" target="$2" got
	if $APPLY; then
		got="$(gh api -X PATCH "repos/$REPO/issues/$n" -f status="$target" --jq '.status.name')"
		if [ "$got" != "$target" ]; then
			echo "error: #$n status is '$got', expected '$target'" >&2
			exit 1
		fi
		say "  #$n status -> $target"
	else
		plan "#$n status -> $target"
	fi
	status_writes=$((status_writes + 1))
}

status_for_open() {
	local labels="$1"
	if has_label "$labels" in-progress; then
		echo "In progress"
		return
	fi
	if has_label "$labels" needs-info || has_label "$labels" needs-triage; then
		echo "Backlog"
		return
	fi
	if has_label "$labels" ready-for-agent || has_label "$labels" ready-for-human; then
		echo "Todo"
		return
	fi
	echo "Backlog"
}

status_for_closed() {
	local reason="$1"
	case "$reason" in
		completed) echo "Done" ;;
		not_planned) echo "Not planned" ;;
		*) echo "" ;;
	esac
}

fetch_open() {
	gh api --paginate "repos/$REPO/issues?state=open&per_page=100" \
		--jq '.[] | select(.pull_request == null) | [.number,.id,.state,(.status.name // "null"),([.labels[].name]|join(",")),.title] | @tsv'
}

fetch_closed() {
	gh api --paginate "repos/$REPO/issues?state=closed&per_page=100" \
		--jq '.[] | select(.pull_request == null) | [.number,.state_reason,(.status.name // "null"),([.labels[].name]|join(",")),.title] | @tsv'
}

anchor_body() {
	cat <<EOF
**Tracker anchor for the current spec.**

- Authoritative spec: \`docs/spec/spec.md\` (v${SPEC_VERSION})
- Change log: \`docs/spec/CHANGELOG.md\`
- Contracts: \`docs/contracts/\`
- ADRs: \`docs/adr/\`
- Glossary: \`GLOSSARY.md\`

Tickets are native sub-issues of this issue. This is a container, not a work item: it has no status by design, and \`/to-tickets\` must not close or modify it.

Execution state lives in the native issue status; labels are classification/routing only. See \`docs/agents/issue-tracker.md\`.
EOF
}

find_anchor() {
	if ! label_exists "$ANCHOR_LABEL"; then
		echo ""
		return
	fi
	local rows count
	rows="$(gh issue list --state all --label "$ANCHOR_LABEL" --json number,title,state --jq '.[] | [.number,.title,.state] | @tsv')"
	count="$(printf '%s\n' "$rows" | grep -c . || true)"
	if [ "$count" -eq 0 ]; then
		echo ""
		return
	fi
	if [ "$count" -gt 1 ]; then
		echo "error: multiple issues carry $ANCHOR_LABEL:" >&2
		printf '%s\n' "$rows" >&2
		exit 1
	fi
	local n title state
	IFS=$'\t' read -r n title state <<<"$rows"
	if [ "$title" != "$ANCHOR_TITLE" ]; then
		echo "error: $ANCHOR_LABEL is on #$n ($title), expected '$ANCHOR_TITLE'" >&2
		exit 1
	fi
	if [ "$state" != "OPEN" ]; then
		echo "error: spec anchor #$n is $state; it must stay open" >&2
		exit 1
	fi
	echo "$n"
}

ensure_anchor() {
	local existing
	existing="$(find_anchor)"
	if [ -n "$existing" ]; then
		anchor="$existing"
		say "spec anchor: #$anchor (existing)"
		return
	fi
	if $APPLY; then
		local url
		url="$(gh issue create --title "$ANCHOR_TITLE" --body "$(anchor_body)" --label "$ANCHOR_LABEL")"
		anchor="${url##*/}"
		say "spec anchor: #$anchor (created)"
	else
		plan "create spec anchor '$ANCHOR_TITLE' with label $ANCHOR_LABEL"
	fi
}

backfill_open_statuses() {
	local n id state status labels title target
	while IFS=$'\t' read -r n id state status labels title; do
		if [ "$n" = "$anchor" ]; then
			continue
		fi
		if [ "$status" != "null" ]; then
			continue
		fi
		target="$(status_for_open "$labels")"
		set_status "$n" "$target"
	done < <(fetch_open)
}

backfill_closed_statuses() {
	local n reason status labels title target
	while IFS=$'\t' read -r n reason status labels title; do
		if [ "$status" != "null" ]; then
			continue
		fi
		target="$(status_for_closed "$reason")"
		if [ -z "$target" ]; then
			say "  #$n closed ($reason): status left null (unsupported reason)"
			continue
		fi
		set_status "$n" "$target"
	done < <(fetch_closed)
}

existing_subissues() {
	if [ -z "$anchor" ]; then
		echo ""
		return
	fi
	gh api --paginate "repos/$REPO/issues/$anchor/sub_issues" --jq '.[].number' || true
}

link_open_tickets() {
	local existing n id state status labels title padded
	existing="$(existing_subissues)"
	padded=$'\n'"$existing"$'\n'
	while IFS=$'\t' read -r n id state status labels title; do
		if [ "$n" = "$anchor" ]; then
			continue
		fi
		if ! [[ "$title" =~ ^[0-9]{2}[a-z]?: ]]; then
			continue
		fi
		case "$padded" in
			*$'\n'"$n"$'\n'*)
				links_skipped=$((links_skipped + 1))
				continue
				;;
		esac
		if $APPLY; then
			if [ -z "$anchor" ]; then
				echo "error: cannot link without a spec anchor" >&2
				exit 1
			fi
			gh issue edit "$anchor" --add-sub-issue "$n" >/dev/null
			say "  link sub-issue #$n"
		else
			plan "link sub-issue #$n"
		fi
		links_added=$((links_added + 1))
	done < <(fetch_open)
}

remove_deprecated_labels() {
	local n id state status labels title label
	while IFS=$'\t' read -r n id state status labels title; do
		for label in "${DEPRECATED_LABELS[@]}"; do
			if has_label "$labels" "$label"; then
				remove_label "$n" "$label"
			fi
		done
	done < <(fetch_open)
	while IFS=$'\t' read -r n state status labels title; do
		for label in "${DEPRECATED_LABELS[@]}"; do
			if has_label "$labels" "$label"; then
				remove_label "$n" "$label"
			fi
		done
	done < <(fetch_closed)
}

retitle_57() {
	local title new
	title="$(gh issue view 57 --json title --jq .title)"
	new="${title//（epic）/}"
	new="${new% }"
	if [ "$title" = "$new" ]; then
		say "#57 title: already clean"
		return
	fi
	if $APPLY; then
		gh issue edit 57 --title "$new" >/dev/null
		say "#57 title -> $new"
	else
		plan "#57 title -> $new"
	fi
	retitled=true
}

print_counts() {
	local label="$1" open_counts closed_counts startable nostatus blocked
	say "== $label status counts =="
	open_counts="$(gh api --paginate "repos/$REPO/issues?state=open&per_page=100" --jq '.[] | select(.pull_request == null) | (.status.name // "null")' | sort | uniq -c || true)"
	closed_counts="$(gh api --paginate "repos/$REPO/issues?state=closed&per_page=100" --jq '.[] | select(.pull_request == null) | (.status.name // "null")' | sort | uniq -c || true)"
	say "open:"
	printf '%s\n' "$open_counts" | sed 's/^/  /'
	say "closed:"
	printf '%s\n' "$closed_counts" | sed 's/^/  /'
	startable="$(gh issue list --state open --search "status:todo -label:$HUMAN_LABEL -is:blocked" --json number --jq 'length')"
	nostatus="$(gh issue list --state open --search "no:status" --json number --jq 'length')"
	blocked="$(gh issue list --state open --search "is:blocked" --json number --jq 'length')"
	say "startable (status:todo -label:$HUMAN_LABEL -is:blocked): $startable"
	say "open with no status: $nostatus"
	say "open blocked: $blocked"
}

verify() {
	local open_null closed_null
	open_null="$(gh api --paginate "repos/$REPO/issues?state=open&per_page=100" --jq '[.[] | select(.pull_request == null) | select(.status == null)] | length')"
	closed_null="$(gh api --paginate "repos/$REPO/issues?state=closed&per_page=100" --jq '[.[] | select(.pull_request == null) | select(.status == null)] | length')"
	if [ "$open_null" -ne 1 ] || [ "$closed_null" -ne 0 ]; then
		echo "verify failed: open_null=$open_null (expected 1 = spec anchor), closed_null=$closed_null (expected 0)" >&2
		exit 1
	fi
	print_counts "verify"
}

main() {
	say "repo: $REPO"
	say "spec version: $SPEC_VERSION"
	say "mode: $($APPLY && echo apply || echo dry-run)"
	if $APPLY && ! $ASSUME_YES; then
		local answer
		read -r -p "Type 'apply' to mutate the tracker: " answer
		if [ "$answer" != "apply" ]; then
			echo "aborted" >&2
			exit 1
		fi
	fi

	ensure_label "$HUMAN_LABEL" "#1d76db" "Requires human implementation (routing exception)"
	ensure_label "$ANCHOR_LABEL" "#5319e7" "Tracker anchor for the current spec; not a work item"
	ensure_anchor

	backfill_open_statuses
	backfill_closed_statuses
	link_open_tickets
	remove_deprecated_labels

	local label
	for label in "${DEPRECATED_LABELS[@]}"; do
		delete_label "$label" true
	done
	for label in "${UNUSED_DEFAULT_LABELS[@]}"; do
		delete_label "$label"
	done
	retitle_57

	say "== summary ($($APPLY && echo applied || echo dry-run)) =="
	say "status writes: $status_writes"
	say "sub-issue links added: $links_added (already present: $links_skipped)"
	say "label instances removed: $labels_removed"
	say "labels deleted: $labels_deleted; created: $labels_created"
	say "#57 retitled: $retitled"
	if $APPLY; then
		verify
	else
		print_counts "current"
	fi
}

main
