#!/usr/bin/env bash
# Print the frozen v2-draft spec. The working tree keeps only the current spec.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git fetch --tags >/dev/null 2>&1 || true
git show spec-v2-draft:.scratch/collector/spec.md
