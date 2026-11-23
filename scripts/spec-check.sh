#!/usr/bin/env bash
# Spec package checks. Subcommands: lint | version | contracts | traceability | refs | links | all
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

BASE_REF="${SPEC_BASE:-}"
if [ -z "$BASE_REF" ]; then
  if git rev-parse --verify --quiet origin/main >/dev/null; then
    BASE_REF=origin/main
  elif git rev-parse --verify --quiet main >/dev/null; then
    BASE_REF=main
  else
    BASE_REF=HEAD
  fi
fi
if git rev-parse --verify --quiet "$BASE_REF" >/dev/null; then
  BASE=$(git merge-base "$BASE_REF" HEAD 2>/dev/null || echo "$BASE_REF")
else
  BASE=HEAD
fi

fail() { echo "spec-check: FAIL: $*" >&2; exit 1; }
note() { echo "spec-check: $*"; }

changed_files() {
  {
    git diff --name-only "$BASE"...HEAD 2>/dev/null || true
    git diff --name-only HEAD 2>/dev/null || true
  } | sed '/^$/d' | sort -u
}

# Expand range shorthands such as REQ-INVOKE-03/04 and REQ-QUALITY-02..04.
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

spec_lint() {
  local spec=docs/spec/spec.md
  [ -f "$spec" ] || fail "missing $spec"

  if grep -nE '^```(json|yaml|yml|go|golang|bash|sh|text)' "$spec" >/dev/null; then
    grep -nE '^```(json|yaml|yml|go|golang|bash|sh|text)' "$spec" >&2 || true
    fail "spec contains a fenced code block; move code/config/JSON to code or contracts"
  fi

  if grep -nE '\bD[0-9]{1,2}\b|\bP[0-9]+=[A-Z]\b|\b[SNW][0-9]{1,2}\b' "$spec" >/dev/null; then
    grep -nE '\bD[0-9]{1,2}\b|\bP[0-9]+=[A-Z]\b|\b[SNW][0-9]{1,2}\b' "$spec" >&2 || true
    fail "spec contains decision/review identifiers; move rationale to ADRs"
  fi

  if grep -nE '评审结论|被否决的方案|已定稿|第二轮评审' "$spec" >/dev/null; then
    grep -nE '评审结论|被否决的方案|已定稿|第二轮评审' "$spec" >&2 || true
    fail "spec contains review archaeology"
  fi

  if grep -nE '\b[0-9]{2}[a-z]\b' "$spec" >/dev/null; then
    grep -nE '\b[0-9]{2}[a-z]\b' "$spec" >&2 || true
    fail "spec contains ticket numbers"
  fi

  if grep -nE '^(type|func|const|var|package) ' "$spec" >/dev/null; then
    grep -nE '^(type|func|const|var|package) ' "$spec" >&2 || true
    fail "spec contains implementation code"
  fi

  local -A contract_headings=()
  local f h
  for f in docs/contracts/*.md; do
    while IFS= read -r h; do
      contract_headings["$h"]=1
    done < <(grep -E '^#{1,6} ' "$f" | sed -E 's/^#+ +//')
  done
  while IFS= read -r h; do
    if [ -n "${contract_headings[$h]:-}" ]; then
      fail "spec duplicates contract heading: $h"
    fi
  done < <(grep -E '^#{1,6} ' "$spec" | sed -E 's/^#+ +//')

  note "lint ok"
}

spec_version() {
  local files ver
  files=$(changed_files)
  if echo "$files" | grep -qE '^(docs/spec/spec\.md|docs/contracts/)'; then
    echo "$files" | grep -qx 'docs/spec/VERSION' || fail "spec package changed without docs/spec/VERSION"
    echo "$files" | grep -qx 'docs/spec/CHANGELOG.md' || fail "spec package changed without docs/spec/CHANGELOG.md"
  fi
  [ -f docs/spec/VERSION ] || fail "missing docs/spec/VERSION"
  ver=$(tr -d '[:space:]' < docs/spec/VERSION)
  [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "VERSION must be pure MAJOR.MINOR.PATCH, got '$ver'"
  grep -q "^## $ver" docs/spec/CHANGELOG.md || fail "CHANGELOG has no section for $ver"
  note "version ok ($ver)"
}

spec_contracts() {
  local m topic code sv
  [ -f internal/config/types.go ] && [ -f internal/config/defaults.go ] && [ -f internal/config/rules.go ] \
    || fail "config authority files are missing"
  [ -f docs/contracts/config.md ] || fail "missing docs/contracts/config.md"
  [ -f docs/contracts/external.md ] || fail "missing docs/contracts/external.md"
  [ -f docs/contracts/store.md ] || fail "missing docs/contracts/store.md"
  [ -f docs/contracts/simulator.md ] || fail "missing docs/contracts/simulator.md"

  while IFS= read -r m; do
    grep -Fq "\`$m\`" docs/contracts/store.md || fail "store contract does not document method $m"
  done < <(sed -n '/^type Store interface {/,/^}/p' internal/interfaces/store.go \
    | grep -oE '^[[:space:]]+[A-Z][A-Za-z0-9]+' | sed -E 's/^[[:space:]]+//' | sort -u)

  grep -q 'RuleWriteFromParam.*"V73"' internal/config/rules.go || fail "V73 is not defined in rules.go"
  grep -Fq 'CFG-V73' docs/contracts/config.md || fail "config contract does not document CFG-V73"

  while IFS= read -r topic; do
    grep -Fq "$topic" docs/contracts/external.md || fail "external contract missing default topic $topic"
  done < <(grep -oE '"v1/\{product_key\}[^"]*"' internal/config/defaults.go | tr -d '"' | sort -u)

  while IFS= read -r code; do
    grep -Fq "| $code |" docs/contracts/external.md || fail "external contract missing code $code"
  done < <(grep -oE '\-> [0-9]{4}' internal/interfaces/errors.go | awk '{print $2}' | sort -u)

  while IFS= read -r sv; do
    grep -qE -- "--$sv\b" docs/contracts/simulator.md || fail "simulator contract missing flag --$sv"
  done < <(grep -E 'Var\(' internal/sim/run.go | grep -oE '"[a-z][a-z-]+"' | tr -d '"' | sort -u)

  local schema
  schema=$(grep -oE 'manifestSchemaVersion = [0-9]+' internal/sim/manifest.go | grep -oE '[0-9]+')
  grep -Fq "\"schema_version\": $schema" docs/contracts/simulator.md \
    || fail "simulator contract schema_version does not match internal/sim/manifest.go"

  note "contracts ok"
}

spec_traceability() {
  local defined rows duplicates
  defined=$( {
    grep -oE 'REQ-[A-Z]+-[0-9]{2}' docs/spec/spec.md
    grep -rhoE '(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' docs/contracts/*.md
  } | sort -u )

  rows=$(grep -E '^\| (REQ|EXT|CFG|STORE|SIM)-[A-Z0-9-]+ \|' docs/traceability.md \
    | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/,"",$2); gsub(/^[ \t]+|[ \t]+$/,"",$3); print $2"|"$3}')

  duplicates=$(cut -d'|' -f1 <<< "$rows" | sort | uniq -d)
  [ -z "$duplicates" ] || fail "duplicate traceability rows: $duplicates"

  while IFS= read -r id; do
    [ -n "$id" ] || continue
    grep -q "^$id|" <<< "$rows" || fail "traceability missing $id"
  done <<< "$defined"

  while IFS='|' read -r id status; do
    [ -n "$id" ] || continue
    case "$status" in
      covered|not-applicable|planned\(*\)) ;;
      *) fail "invalid traceability status for $id: $status" ;;
    esac
    grep -qx "$id" <<< "$defined" || fail "traceability references unknown id $id"
  done <<< "$rows"

  note "traceability ok ($(wc -l <<< "$defined" | tr -d ' ') ids)"
}

spec_refs() {
  local f bad
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    case "$f" in
      .scratch/collector/plans/spec-v3.md) continue ;;
    esac
    if head -n 10 "$f" | grep -q 'spec-refs: historical-file' \
      && head -n 10 "$f" | grep -q '历史文档'; then
      continue
    fi
    bad=$(awk 'BEGIN{f=0} /^```/{f=!f; next} !f{
      if (tolower($0) ~ /historical/ || tolower($0) ~ /policy-example/) next
      if ($0 ~ /spec[[:space:]]*§/ || $0 ~ /\.scratch\/collector\/spec\.md/) print FNR": "$0
    }' "$f" || true)
    [ -z "$bad" ] || fail "unmarked historical spec reference in $f:\n$bad"
  done < <(git -c core.quotePath=false ls-files '.scratch/collector/plans/*.md' '.scratch/collector/diffs/*.md' 'docs/代码走读/*.md')

  local defined refs id
  defined=$( {
    grep -oE 'REQ-[A-Z]+-[0-9]{2}' docs/spec/spec.md
    grep -rhoE '(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' docs/contracts/*.md
  } | sort -u )
  refs=$(grep -rhoE 'REQ-[A-Z]+-[0-9]{2}(\.\.|/)[0-9]{2}|REQ-[A-Z]+-[0-9]{2}|(EXT|STORE|SIM)-[A-Z0-9]+-[0-9]{2}|CFG-[A-Z0-9]+(-[0-9]{2})?' \
    .scratch/collector docs/代码走读 2>/dev/null | expand_refs || true)
  while IFS= read -r id; do
    [ -n "$id" ] || continue
    grep -qx "$id" <<< "$defined" || fail "plan/diff references unknown anchor $id"
  done <<< "$refs"

  note "refs ok"
}

spec_links() {
  local f target path
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    while IFS= read -r target; do
      [ -n "$target" ] || continue
      target="${target%% *}"
      case "$target" in
        http:*|https:*|mailto:*|'#'*|/*) continue ;;
      esac
      target="${target%%#*}"
      [ -n "$target" ] || continue
      path="$(dirname "$f")/$target"
      [ -e "$path" ] || fail "broken relative link in $f: $target"
    done < <(grep -oE '\]\([^)]+\)' "$f" | sed -E 's/^\]\((.*)\)$/\1/' || true)
  done < <(find docs -name '*.md' -print; printf '%s\n' *.md)
  note "links ok"
}

case "${1:-all}" in
  lint) spec_lint ;;
  version) spec_version ;;
  contracts) spec_contracts ;;
  traceability) spec_traceability ;;
  refs) spec_refs ;;
  links) spec_links ;;
  all)
    spec_lint
    spec_version
    spec_contracts
    spec_traceability
    spec_refs
    spec_links
    ;;
  *) echo "usage: $0 [lint|version|contracts|traceability|refs|links|all]" >&2; exit 2 ;;
esac
