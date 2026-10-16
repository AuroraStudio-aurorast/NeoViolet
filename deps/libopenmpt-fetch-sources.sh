#!/bin/sh
# Fetches the two libopenmpt tarballs named by deps/libopenmpt.lock, verifies
# their checksums and extracts them into the build directory.
#
#   WORK   build directory (required): the tarballs, the source tree and
#          everything the platform scripts derive from them live under it.
#
# The lock file is taken from the environment when the caller has already loaded
# it (the workflow does) and sourced otherwise, so the script also runs standalone.
#
# The autotools tarball is the build skeleton for macOS and musl; the makefile
# tarball carries the vendored decoders that replace the external libraries, and it
# is the only source Windows builds from. Both are fetched everywhere, so the
# sources and their checksums are verified in exactly one place.
set -eu

: "${WORK:?WORK is not set}"
DEPS_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -z "${AUTOTOOLS_URL:-}" ]; then
	# shellcheck disable=SC1090
	. "$DEPS_DIR/libopenmpt.lock"
fi

for key in AUTOTOOLS_URL AUTOTOOLS_SHA256 VENDOR_URL VENDOR_SHA256; do
	eval "value=\${$key:-}"
	[ -n "$value" ] || { echo "FATAL: $key is not set" >&2; exit 1; }
done

# On the Windows runners WORK arrives as a native path (D:\a\_temp\...) that the
# msys2 shell cannot use as it is.
case "$WORK" in
	*\\* | [A-Za-z]:*) WORK="$(cygpath -u "$WORK" 2>/dev/null || printf '%s' "$WORK")" ;;
esac

mkdir -p "$WORK"
cd "$WORK"

# macOS ships shasum, Linux and MSYS2 ship sha256sum.
hash_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

fetch() {
	url="$1"
	file="$2"
	expected="$3"
	echo "=== fetch: $file"
	curl -fsSL --retry 3 --retry-delay 5 --connect-timeout 20 -o "$file" "$url"
	actual="$(hash_of "$file")"
	if [ "$actual" != "$expected" ]; then
		echo "FATAL: checksum mismatch for $file" >&2
		echo "  expected: $expected" >&2
		echo "  actual:   $actual" >&2
		exit 1
	fi
	echo "checksum verified"
}

fetch "$AUTOTOOLS_URL" autotools.tar.gz "$AUTOTOOLS_SHA256"
fetch "$VENDOR_URL" vendored.tar.gz "$VENDOR_SHA256"

tar xzf autotools.tar.gz
mkdir -p vendored
tar xzf vendored.tar.gz -C vendored

echo "FETCH OK"
