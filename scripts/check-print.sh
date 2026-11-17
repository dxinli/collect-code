#!/usr/bin/env bash
# Enforce the logging contract: production code logs through slog or an
# injected io.Writer, never through package-level printing.
set -u

hits="$(grep -rnE '\b(fmt\.(Print|Printf|Println)|log\.(Print|Printf|Println))\b' \
	--include='*.go' internal cmd 2>/dev/null | grep -v '_test.go' || true)"

if [ -n "$hits" ]; then
	echo "direct printing is not allowed; use slog or an io.Writer:" >&2
	echo "$hits" >&2
	exit 1
fi

echo "OK: no direct printing in production code"
