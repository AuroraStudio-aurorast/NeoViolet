#!/bin/sh
set -eu

# Fetching is separate from packaging: saved artifacts can be packaged offline.
PROJECT_ROOT=$(cd "$(dirname "$0")/../.." && pwd -P)
. "$PROJECT_ROOT/deps/libopenmpt.lock"
: "${LIBOPENMPT_DYLIB:?missing LIBOPENMPT_DYLIB}"
asset="libopenmpt-${LIBOPENMPT_VERSION}-macos${MACOS_MIN}-universal.dylib"

verified() {
    [ -f "$1" ] && [ -f "$2" ] || return 1
    expected=$(awk 'NR == 1 { print $1 }' "$2")
    [ "${#expected}" -eq 64 ] || return 1
    case "$expected" in *[!0-9a-f]*) return 1 ;; esac
    actual=$(shasum -a 256 "$1") || return 1
    [ "${actual%% *}" = "$expected" ]
}

if verified "$LIBOPENMPT_DYLIB" "$LIBOPENMPT_DYLIB.sha256"; then
    echo "Using verified libopenmpt: $LIBOPENMPT_DYLIB"
    exit 0
fi

mkdir -p "$(dirname "$LIBOPENMPT_DYLIB")"
tmp=$(mktemp -d "$(dirname "$LIBOPENMPT_DYLIB")/.openmpt-fetch.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
gh release download deps --repo AuroraStudio-aurorast/NeoViolet \
    --pattern "$asset" --pattern "$asset.sha256" --dir "$tmp"
if ! verified "$tmp/$asset" "$tmp/$asset.sha256"; then
    echo "error: checksum mismatch for $asset" >&2
    exit 1
fi
mv "$tmp/$asset" "$LIBOPENMPT_DYLIB"
mv "$tmp/$asset.sha256" "$LIBOPENMPT_DYLIB.sha256"
echo "Downloaded and verified libopenmpt: $LIBOPENMPT_DYLIB"
