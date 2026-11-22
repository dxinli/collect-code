#!/usr/bin/env bash
# Enforce the kernel import boundaries. core and drivers may import
# interfaces (and their own sub-packages) but never a concrete implementation
# or the test simulator.
#
# go list .Deps is the transitive production closure: it excludes the package
# itself and test-only imports, so _test.go files may use internal/testutil.
set -u
export GOFLAGS=

MODULE="github.com/dxinli/collect-code"
fail=0

for required in internal/core internal/interfaces; do
	if [ ! -d "$required" ]; then
		echo "FAIL: $required is missing" >&2
		exit 1
	fi
done

check_root() {
	local root="$1" forbidden="$2"
	[ -d "$root" ] || return 0
	while IFS= read -r pkg; do
		[ -n "$pkg" ] || continue
		local deps hits
		if ! deps="$(go list -f '{{range .Deps}}{{println .}}{{end}}' "$pkg")"; then
			echo "FAIL: go list $pkg failed" >&2
			fail=1
			continue
		fi
		hits="$(printf '%s\n' "$deps" | grep -E "$forbidden" || true)"
		if [ -n "$hits" ]; then
			echo "FAIL: $pkg imports forbidden packages:" >&2
			printf '%s\n' "$hits" >&2
			fail=1
		fi
	done < <(go list ./"$root"/... 2>/dev/null || true)
}

common="${MODULE}/internal/(channels|adapters|store|testutil|clock/realclock|sim)(/|$)|^go\\.etcd\\.io/bbolt(/|$)"
core_forbidden="${common}|${MODULE}/internal/drivers(/|$)"
drivers_forbidden="${common}|${MODULE}/internal/core(/|$)"
check_root internal/core "$core_forbidden"
check_root internal/drivers "$drivers_forbidden"

# interfaces stays a pure contract: it imports nothing from this module.
check_root internal/interfaces "${MODULE}/"

if [ "$fail" -ne 0 ]; then
	exit 1
fi

echo "OK: kernel import boundaries hold"
