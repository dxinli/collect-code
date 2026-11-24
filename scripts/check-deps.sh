#!/usr/bin/env bash
# Enforce the kernel import boundaries. core and drivers may import
# interfaces (and their own sub-packages) but never a concrete implementation
# or the test simulator. The store implementations may import interfaces; only
# bbolt may import the embedded database. Test-only store packages must stay
# out of the production closure.
#
# go list .Deps is the transitive production closure: it excludes the package
# itself and test-only imports, so _test.go files may use internal/testutil or
# the shared contract suite.
set -u
export GOFLAGS=

MODULE="github.com/dxinli/collect-code"
fail=0

for required in internal/core internal/interfaces \
	internal/store/memory internal/store/storetest internal/store/bbolt; do
	if [ ! -d "$required" ]; then
		echo "FAIL: $required is missing" >&2
		exit 1
	fi
done

check_root() {
	local root="$1" forbidden="$2"
	[ -d "$root" ] || return 0
	local pkgs
	if ! pkgs="$(go list ./"$root"/... 2>/dev/null)"; then
		echo "FAIL: go list ./$root/... failed" >&2
		fail=1
		return
	fi
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
	done <<< "$pkgs"
}

# check_store_root inspects direct imports, not the transitive closure:
#   - module packages may only be internal/interfaces;
#   - non-stdlib imports (a dot in the path) are allowed only for
#     internal/interfaces, plus the exact root go.etcd.io/bbolt package when
#     allow_bbolt=yes. Subpackages such as go.etcd.io/bbolt/errors and
#     golang.org/x/sys/unix are rejected.
check_store_root() {
	local root="$1" allow_bbolt="$2"
	[ -d "$root" ] || return 0
	local pkgs errfile
	errfile="$(mktemp)"
	if ! pkgs="$(go list ./"$root"/... 2>"$errfile")"; then
		echo "FAIL: go list ./$root/... failed: $(tr '\n' ' ' <"$errfile")" >&2
		rm -f "$errfile"
		fail=1
		return
	fi
	rm -f "$errfile"
	while IFS= read -r pkg; do
		[ -n "$pkg" ] || continue
		local imports bad bad_external
		if ! imports="$(go list -f '{{range .Imports}}{{println .}}{{end}}' "$pkg")"; then
			echo "FAIL: go list $pkg failed" >&2
			fail=1
			continue
		fi
		bad="$(printf '%s\n' "$imports" | grep -F "${MODULE}/" | grep -vxF "${MODULE}/internal/interfaces" || true)"
		if [ -n "$bad" ]; then
			echo "FAIL: $pkg imports module packages other than interfaces:" >&2
			printf '%s\n' "$bad" >&2
			fail=1
		fi
		if [ "$allow_bbolt" = "yes" ]; then
			bad_external="$(printf '%s\n' "$imports" | grep -E '\.' \
				| grep -vxF "${MODULE}/internal/interfaces" | grep -vxF 'go.etcd.io/bbolt' || true)"
		else
			bad_external="$(printf '%s\n' "$imports" | grep -E '\.' \
				| grep -vxF "${MODULE}/internal/interfaces" || true)"
		fi
		if [ -n "$bad_external" ]; then
			echo "FAIL: $pkg imports forbidden external packages:" >&2
			printf '%s\n' "$bad_external" >&2
			fail=1
		fi
	done <<< "$pkgs"
}

common="${MODULE}/internal/(channels|adapters|store|testutil|clock/realclock|sim)(/|$)|^go\\.etcd\\.io/bbolt(/|$)"
core_forbidden="${common}|${MODULE}/internal/drivers(/|$)"
drivers_forbidden="${common}|${MODULE}/internal/core(/|$)"
check_root internal/core "$core_forbidden"
check_root internal/drivers "$drivers_forbidden"

# The channel implementation is a concrete northbound transport: it may use
# interfaces and topics, but never the kernel, drivers, simulator or test-only
# packages. topics is a pure helper and may not import this module at all.
check_root internal/topics "${MODULE}/"
channels_forbidden="${common}|${MODULE}/internal/(drivers|core)(/|$)"
check_root internal/channels "$channels_forbidden"

# interfaces stays a pure contract: it imports nothing from this module.
check_root internal/interfaces "${MODULE}/"

check_store_root internal/store/memory no
check_store_root internal/store/bbolt yes
check_store_root internal/store/storetest no

# Production assembly may use the bbolt implementation but never the test-only
# in-memory store or the shared contract suite.
app_forbidden="${MODULE}/internal/store/(memory|storetest)(/|$)"
check_root internal/app "$app_forbidden"
check_root cmd "$app_forbidden"

# The simulator is a test peer: it must implement Modbus framing and register
# semantics itself, never reuse the driver under test. The test closure is
# checked too so the independent client cannot import internal/drivers.
sim_forbidden="${MODULE}/internal/(drivers|core)(/|$)"
check_root internal/sim "$sim_forbidden"
if ! simdeps="$(go list -deps -test ./internal/sim 2>/dev/null)"; then
	echo "FAIL: go list -deps -test ./internal/sim failed" >&2
	fail=1
else
	if hits="$(printf '%s\n' "$simdeps" | grep -E "$sim_forbidden" || true)"; [ -n "$hits" ]; then
		echo "FAIL: internal/sim test closure imports forbidden packages:" >&2
		printf '%s\n' "$hits" >&2
		fail=1
	fi
fi

if [ "$fail" -ne 0 ]; then
	exit 1
fi

echo "OK: kernel import boundaries hold"
