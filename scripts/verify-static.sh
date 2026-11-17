#!/usr/bin/env bash
# Verify that a built binary is a static Linux executable for the expected
# architecture. Reads the artifact only; never executes a cross-arch binary.
set -u

bin="${1:-}"
arch="${2:-}"

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

[ -n "$bin" ] || fail "usage: verify-static.sh <binary> <amd64|arm64>"
[ -n "$arch" ] || fail "usage: verify-static.sh <binary> <amd64|arm64>"
[ -f "$bin" ] || fail "missing artifact: $bin"

export LC_ALL=C

file_out="$(file "$bin")"
echo "$file_out" | grep -q 'ELF 64-bit' || fail "not an ELF 64-bit binary: $file_out"

case "$arch" in
amd64)
	echo "$file_out" | grep -Eq 'x86-64|x86_64' || fail "not amd64: $file_out"
	;;
arm64)
	echo "$file_out" | grep -Eq 'aarch64|ARM64' || fail "not arm64: $file_out"
	;;
*)
	fail "unsupported architecture: $arch"
	;;
esac

# The strongest, toolchain-independent assertion: build metadata.
version_out="$(go version -m "$bin" 2>&1)" || fail "cannot read build metadata: $version_out"
echo "$version_out" | grep -q 'GOOS=linux' || fail "GOOS is not linux"
echo "$version_out" | grep -q "GOARCH=$arch" || fail "GOARCH is not $arch"
echo "$version_out" | grep -q 'CGO_ENABLED=0' || fail "CGO_ENABLED is not 0"

# Supplementary: a static binary has no dynamic interpreter.
if command -v readelf >/dev/null 2>&1; then
	if readelf -l "$bin" 2>/dev/null | grep -q 'INTERP'; then
		fail "binary has a dynamic interpreter"
	fi
elif command -v ldd >/dev/null 2>&1; then
	ldd_out="$(ldd "$bin" 2>&1 || true)"
	if ! echo "$ldd_out" | grep -q 'not a dynamic executable'; then
		# "not executable" for a cross-arch binary does not prove static.
		fail "cannot prove static (ldd: $ldd_out)"
	fi
else
	fail "neither readelf nor ldd is available"
fi

echo "OK: $bin ($arch, static)"
