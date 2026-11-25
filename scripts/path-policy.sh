#!/usr/bin/env bash
# Live path policy shared by the CI guard and the spec scripts.
# Sourced by consumers (variables only); run with --self-test to verify wiring.
# Do not copy these values elsewhere; the self-test rejects a second copy.

PLAN_ALLOW_RE='^(docs/|\.scratch/|AGENTS\.md$|CLAUDE\.md$|GLOSSARY\.md$)'
SPEC_DELTA_ALLOW_RE='^(docs/spec/|docs/contracts/)'
LOCAL_ONLY_DIRS=(docs/代码走读)

policy_self_test() {
  set -euo pipefail
  cd "$(git rev-parse --show-toplevel)"

  local fail=0 path got
  check_re() {
    local re="$1" value="$2" expected="$3"
    if printf '%s\n' "$value" | grep -qE "$re"; then got=allow; else got=reject; fi
    if [ "$got" != "$expected" ]; then
      echo "path-policy: FAIL: '$value' expected $expected, got $got" >&2
      fail=1
    fi
  }

  # Plan allowlist.
  for path in docs/a .scratch/b AGENTS.md CLAUDE.md GLOSSARY.md; do
    check_re "$PLAN_ALLOW_RE" "$path" allow
  done
  for path in README.md scripts/x internal/x.go .github/workflows/ci.yml configs/x; do
    check_re "$PLAN_ALLOW_RE" "$path" reject
  done

  # Spec-delta allowlist.
  for path in docs/spec/spec.md docs/contracts/config.md docs/spec/VERSION; do
    check_re "$SPEC_DELTA_ALLOW_RE" "$path" allow
  done
  for path in docs/agents/ci.md configs/x internal/x.go; do
    check_re "$SPEC_DELTA_ALLOW_RE" "$path" reject
  done

  # Consumers source this file.
  local f
  for f in .github/workflows/ci.yml scripts/spec-status.sh scripts/spec-check.sh scripts/spec-impact.sh; do
    if ! grep -qF 'path-policy.sh' "$f"; then
      echo "path-policy: FAIL: $f does not reference path-policy.sh" >&2
      fail=1
    fi
  done
  local sources
  sources=$(grep -cF '. scripts/path-policy.sh' .github/workflows/ci.yml || true)
  if [ "$sources" -lt 2 ]; then
    echo "path-policy: FAIL: both guard steps must source path-policy.sh (found $sources)" >&2
    fail=1
  fi

  # No second executable copy of either allowlist.
  for f in scripts/*.sh .github/workflows/*.yml; do
    [ "$f" = "scripts/path-policy.sh" ] && continue
    if grep -qF 'docs/spec/|docs/contracts/' "$f"; then
      echo "path-policy: FAIL: $f contains a second spec-delta allowlist" >&2
      fail=1
    fi
    if grep -qF 'docs/|\.scratch/|AGENTS\.md$' "$f"; then
      echo "path-policy: FAIL: $f contains a second plan allowlist" >&2
      fail=1
    fi
  done

  # Every local-only dir must also be ignored by git.
  local d
  for d in "${LOCAL_ONLY_DIRS[@]}"; do
    if ! grep -qxF "$d/" .gitignore; then
      echo "path-policy: FAIL: $d is not gitignored" >&2
      fail=1
    fi
  done

  # A broken link under a local-only dir must be ignored by spec-check links.
  local -a probes=() created=()
  cleanup() {
    local p
    if [ "${#probes[@]}" -gt 0 ]; then
      for p in "${probes[@]}"; do rm -f "$p"; done
    fi
    if [ "${#created[@]}" -gt 0 ]; then
      for d in "${created[@]}"; do rmdir "$d" 2>/dev/null || true; done
    fi
  }
  trap cleanup EXIT
  for d in "${LOCAL_ONLY_DIRS[@]}"; do
    if [ ! -d "$d" ]; then
      mkdir -p "$d"
      created+=("$d")
    fi
    local probe="$d/_policy_probe.md"
    printf '[missing](no-such-file.md)\n' >"$probe"
    probes+=("$probe")
  done
  if ! ./scripts/spec-check.sh links >/dev/null 2>&1; then
    echo "path-policy: FAIL: spec-check links scanned a local-only dir" >&2
    fail=1
  fi
  cleanup
  trap - EXIT

  if [ "$fail" -ne 0 ]; then
    exit 1
  fi
  echo "path-policy: self-test ok"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  case "${1:-}" in
    --self-test) policy_self_test ;;
    *)
      echo "usage: $0 --self-test" >&2
      exit 2
      ;;
  esac
fi
