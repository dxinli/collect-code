#!/usr/bin/env bash
# Classify a change into the CI path groups, guard the top-level layout, and
# self-test the rules. See docs/agents/ci.md.
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  scripts/ci-scope.sh --event <pull_request|push|workflow_dispatch> \
      [--base <sha>] [--head <sha>] [--github-output]
  scripts/ci-scope.sh --files <path>
  scripts/ci-scope.sh --check-top-level
  scripts/ci-scope.sh --self-test
  scripts/ci-scope.sh -h | --help

Modes:
  --event ...          Classify the changed files for a GitHub event and print
                       go=/spec=; --github-output appends them to $GITHUB_OUTPUT.
  --files <path>       Classify a newline-separated path list (for local tests).
  --check-top-level    Fail if a tracked top-level entry is not classified.
  --self-test          Run the classification and guard cases.

Path groups:
  Both   Makefile, .github/workflows/ci.yml
  Go     cmd/, internal/, configs/, *.go, go.mod, go.sum, go.work*,
         .golangci.*, scripts/check-*, scripts/verify-static.sh
  Spec   docs/, GLOSSARY.md, AGENTS.md, CLAUDE.md, scripts/spec-*.sh, .scratch/
EOF
}

die() { printf 'ci-scope: error: %s\n' "$*" >&2; exit 1; }
usage_error() {
	printf 'ci-scope: error: %s\n' "$*" >&2
	printf 'run with --help for usage\n' >&2
	exit 2
}

GO=false
SPEC=false

classify_path() {
	local path="$1"
	GO=false
	SPEC=false
	case "$path" in
		Makefile|.github/workflows/ci.yml)
			GO=true
			SPEC=true
			;;
		cmd/*|internal/*|configs/*|*.go|go.mod|go.sum|go.work*|.golangci.*|scripts/check-*|scripts/verify-static.sh)
			GO=true
			;;
		docs/*|GLOSSARY.md|AGENTS.md|CLAUDE.md|scripts/spec-*.sh|.scratch/*)
			SPEC=true
			;;
	esac
}

GO_ANY=false
SPEC_ANY=false

reset_scope() {
	GO_ANY=false
	SPEC_ANY=false
}

add_file() {
	classify_path "$1"
	if [ "$GO" = true ]; then
		GO_ANY=true
	fi
	if [ "$SPEC" = true ]; then
		SPEC_ANY=true
	fi
}

scope_from_file() {
	local file="$1" path
	reset_scope
	while IFS= read -r path; do
		[ -n "$path" ] || continue
		add_file "$path"
	done <"$file"
}

scope_for_event() {
	local event="$1" base="$2" head="$3" range files path
	reset_scope
	if [ "$event" = workflow_dispatch ] || [ -z "$base" ] || [ -z "$head" ] ||
		[ "$base" = "0000000000000000000000000000000000000000" ]; then
		GO_ANY=true
		SPEC_ANY=true
		return 0
	fi
	if [ "$event" = pull_request ]; then
		range="$base...$head"
	else
		range="$base..$head"
	fi
	if ! files=$(git -c core.quotePath=false diff --name-only "$range" 2>/dev/null); then
		printf '::warning::ci-scope: cannot compute changed files; running all checks\n'
		GO_ANY=true
		SPEC_ANY=true
		return 0
	fi
	while IFS= read -r path; do
		[ -n "$path" ] || continue
		add_file "$path"
	done <<<"$files"
}

emit_scope() {
	printf 'go=%s\n' "$GO_ANY"
	printf 'spec=%s\n' "$SPEC_ANY"
	printf 'scope: go=%s spec=%s\n' "$GO_ANY" "$SPEC_ANY"
	if [ "$GO_ANY" = false ] && [ "$SPEC_ANY" = false ]; then
		printf '::notice::No Go/spec checks apply to this change; ci passes by design.\n'
		if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
			printf '### CI scope\n\nNo Go/spec checks apply to this change (paths outside the Go and spec groups).\n' \
				>>"$GITHUB_STEP_SUMMARY"
		fi
	fi
}

write_github_output() {
	[ -n "${GITHUB_OUTPUT:-}" ] || die "--github-output requires GITHUB_OUTPUT"
	{
		printf 'go=%s\n' "$GO_ANY"
		printf 'spec=%s\n' "$SPEC_ANY"
	} >>"$GITHUB_OUTPUT"
}

TOP_LEVEL_OK=true

classify_top_level() {
	local entry="$1"
	TOP_LEVEL_OK=true
	case "$entry" in
		cmd|internal|configs|go.mod|go.sum|go.work*|.golangci.*) ;;
		docs|.scratch|GLOSSARY.md|AGENTS.md|CLAUDE.md) ;;
		scripts|.github|Makefile) ;;
		.gitignore|.gitattributes) ;;
		*) TOP_LEVEL_OK=false ;;
	esac
}

check_top_level() {
	local entries entry
	entries=$(git -c core.quotePath=false ls-files | cut -d/ -f1 | sort -u)
	[ -n "$entries" ] || die "no tracked files"
	while IFS= read -r entry; do
		[ -n "$entry" ] || continue
		classify_top_level "$entry"
		if [ "$TOP_LEVEL_OK" = false ]; then
			die "unclassified top-level entry '$entry'; add it to scripts/ci-scope.sh and docs/agents/ci.md"
		fi
	done <<<"$entries"
	printf 'ci-scope: top-level ok\n'
}

self_test() (
	local tmp summary repo failures=0 total=0 old_summary noop_out
	tmp=$(mktemp)
	summary=$(mktemp)
	repo=$(mktemp -d)
	old_summary="${GITHUB_STEP_SUMMARY:-}"
	trap 'rm -f "$tmp" "$summary"; rm -rf "$repo"' EXIT

	# Assertions count failures and return 0 so the whole matrix runs;
	# self_test exits 1 after all cases when anything failed.
	assert_scope() {
		local desc="$1" want_go="$2" want_spec="$3"
		shift 3
		total=$((total + 1))
		printf '%s\n' "$@" >"$tmp"
		scope_from_file "$tmp"
		if [ "$GO_ANY" != "$want_go" ] || [ "$SPEC_ANY" != "$want_spec" ]; then
			printf 'ci-scope: FAIL %s: got go=%s spec=%s, want go=%s spec=%s\n' \
				"$desc" "$GO_ANY" "$SPEC_ANY" "$want_go" "$want_spec" >&2
			failures=$((failures + 1))
		fi
	}

	assert_top() {
		local desc="$1" entry="$2" want="$3"
		total=$((total + 1))
		classify_top_level "$entry"
		if [ "$TOP_LEVEL_OK" != "$want" ]; then
			printf 'ci-scope: FAIL top-level %s: known=%s, want %s\n' \
				"$desc" "$TOP_LEVEL_OK" "$want" >&2
			failures=$((failures + 1))
		fi
	}

	assert_event() {
		local desc="$1" event="$2" base="$3" head="$4" want_go="$5" want_spec="$6"
		total=$((total + 1))
		scope_for_event "$event" "$base" "$head" >/dev/null 2>&1 || true
		if [ "$GO_ANY" != "$want_go" ] || [ "$SPEC_ANY" != "$want_spec" ]; then
			printf 'ci-scope: FAIL event %s: got go=%s spec=%s, want go=%s spec=%s\n' \
				"$desc" "$GO_ANY" "$SPEC_ANY" "$want_go" "$want_spec" >&2
			failures=$((failures + 1))
		fi
	}

	assert_scope "docs-only" false true docs/agents/ci.md
	assert_scope "go-only" true false internal/core/state/state.go
	assert_scope "both" true true Makefile
	assert_scope "workflow both" true true .github/workflows/ci.yml
	assert_scope "none" false false .gitignore
	assert_scope "agents doc" false true AGENTS.md
	assert_scope "claude doc" false true CLAUDE.md
	assert_scope "non-ascii docs" false true docs/需求文档_1.md
	assert_scope "go.work glob" true false go.work.foo
	assert_scope "golangci glob" true false .golangci.toml
	assert_scope "spec script" false true scripts/spec-check.sh
	assert_scope "go script" true false scripts/check-deps.sh
	assert_scope "ordinary script" false false scripts/ticket-new.sh
	assert_scope "mixed files" true true docs/agents/ci.md internal/core/state/state.go

	assert_top "go dir" cmd true
	assert_top "spec dir" docs true
	assert_top "mixed file" Makefile true
	assert_top "spec file" AGENTS.md true
	assert_top "ignored file" .gitignore true
	assert_top "ignored file" .gitattributes true
	assert_top "unknown dir" tools false

	assert_event "workflow_dispatch" workflow_dispatch "" "" true true
	assert_event "zero base" push 0000000000000000000000000000000000000000 HEAD true true
	assert_event "diff failure" push does-not-exist HEAD true true

	# Git C-quotes non-ASCII paths unless quotePath=false; the temp repo makes
	# sure a docs-only change with a Chinese filename still selects spec.
	git -C "$repo" init -q
	mkdir -p "$repo/docs"
	: >"$repo/docs/需求文档_1.md"
	git -C "$repo" -c user.email=ci@example.invalid -c user.name=ci \
		add docs/需求文档_1.md
	git -C "$repo" -c user.email=ci@example.invalid -c user.name=ci \
		commit -qm probe
	empty_tree=$(git -C "$repo" hash-object -t tree /dev/null)
	total=$((total + 1))
	if ! (
		cd "$repo"
		scope_for_event push "$empty_tree" HEAD >/dev/null 2>&1
		[ "$GO_ANY" = false ] && [ "$SPEC_ANY" = true ]
	); then
		printf 'ci-scope: FAIL non-ascii docs event range\n' >&2
		failures=$((failures + 1))
	fi

	total=$((total + 1))
	GO_ANY=false
	SPEC_ANY=false
	: >"$summary"
	noop_out=$(GITHUB_STEP_SUMMARY="$summary" emit_scope)
	if ! printf '%s\n' "$noop_out" | grep -q '::notice::No Go/spec checks apply'; then
		printf 'ci-scope: FAIL no-op notice was not printed\n' >&2
		failures=$((failures + 1))
	fi
	total=$((total + 1))
	if ! grep -q 'No Go/spec checks apply' "$summary"; then
		printf 'ci-scope: FAIL no-op summary was not written\n' >&2
		failures=$((failures + 1))
	fi
	if [ -n "$old_summary" ]; then
		export GITHUB_STEP_SUMMARY="$old_summary"
	else
		unset GITHUB_STEP_SUMMARY
	fi

	if [ "$failures" -ne 0 ]; then
		printf 'ci-scope: self-test FAILED (%d/%d)\n' "$failures" "$total" >&2
		exit 1
	fi
	printf 'ci-scope: self-test ok (%d cases)\n' "$total"
)

MODE=""
EVENT=""
BASE=""
HEAD=""
FILES=""
GITHUB_OUTPUT_FLAG=false

while [ $# -gt 0 ]; do
	case "$1" in
		--event)
			[ $# -ge 2 ] || usage_error "--event needs a value"
			[ -z "$MODE" ] || usage_error "choose only one mode"
			MODE=event
			EVENT="$2"
			shift 2
			;;
		--base)
			[ $# -ge 2 ] || usage_error "--base needs a value"
			BASE="$2"
			shift 2
			;;
		--head)
			[ $# -ge 2 ] || usage_error "--head needs a value"
			HEAD="$2"
			shift 2
			;;
		--files)
			[ $# -ge 2 ] || usage_error "--files needs a value"
			[ -z "$MODE" ] || usage_error "choose only one mode"
			MODE=files
			FILES="$2"
			shift 2
			;;
		--check-top-level)
			[ -z "$MODE" ] || usage_error "choose only one mode"
			MODE=top
			shift
			;;
		--self-test)
			[ -z "$MODE" ] || usage_error "choose only one mode"
			MODE=self
			shift
			;;
		--github-output)
			GITHUB_OUTPUT_FLAG=true
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*) usage_error "unknown argument: $1" ;;
	esac
done

case "$MODE" in
	event|top|self) cd "$(git rev-parse --show-toplevel)" ;;
esac

case "$MODE" in
	event)
		scope_for_event "$EVENT" "$BASE" "$HEAD"
		emit_scope
		if $GITHUB_OUTPUT_FLAG; then
			write_github_output
		fi
		;;
	files)
		[ -f "$FILES" ] || die "file not found: $FILES"
		scope_from_file "$FILES"
		emit_scope
		;;
	top)
		check_top_level
		;;
	self)
		self_test
		;;
	"")
		usage_error "choose a mode"
		;;
esac
