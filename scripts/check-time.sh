#!/usr/bin/env bash
# Enforce ADR-0003 decision 5: kernel, driver and channel code take time only
# from the injected Clock, and business deadlines never use
# context.WithTimeout/WithDeadline.
set -u

roots=(internal/core internal/drivers internal/channels)
pattern='\b(time\.(Now|Since|Until|NewTicker|NewTimer|Tick|After|AfterFunc|Sleep)|context\.(WithTimeout|WithDeadline))[[:space:]]*\('

hits=""
for root in "${roots[@]}"; do
	[ -d "$root" ] || continue
	out="$(grep -rnE --include='*.go' "$pattern" "$root" 2>/dev/null | grep -v '_test\.go:' || true)"
	# Whole-line comments may name the forbidden calls; block comments and
	# string literals remain a documented blind spot.
	out="$(printf '%s\n' "$out" | grep -vE ':[0-9]+:[[:space:]]*//' || true)"
	if [ -n "$out" ]; then hits="${hits}${out}"$'\n'; fi
done

# An aliased import ("import t \"time\"") would bypass the call grep.
alias_hits="$(grep -rnE --include='*.go' '^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]+"(time|context)"' "${roots[@]}" 2>/dev/null | grep -v '_test\.go:' || true)"

if [ -n "$hits" ] || [ -n "$alias_hits" ]; then
	if [ -n "$hits" ]; then
		echo "direct time/context-deadline calls are forbidden in core/drivers/channels:" >&2
		printf '%s' "$hits" >&2
	fi
	if [ -n "$alias_hits" ]; then
		echo "aliased time/context imports are forbidden in core/drivers/channels:" >&2
		printf '%s\n' "$alias_hits" >&2
	fi
	exit 1
fi

echo "OK: no direct time or context deadline calls in core/drivers/channels"
