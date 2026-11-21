#!/bin/sh
set -eu

# Validate a resolved Rust target without choosing a toolchain or installing it.
if [ "$#" -ne 3 ]; then
    echo "usage: check-rust.sh GOOS GOARCH RUST_TARGET" >&2
    exit 1
fi
case "$2" in
    386) arch=i686 ;;
    amd64) arch=x86_64 ;;
    arm64) arch=aarch64 ;;
    *) echo "error: unsupported Rust architecture: $2" >&2; exit 1 ;;
esac
case "$1/$3" in
    darwin/"$arch"-apple-darwin | linux/"$arch"-unknown-linux-* | windows/"$arch"-pc-windows-*) ;;
    *) echo "error: Rust target $3 does not match GOOS=$1 GOARCH=$2; set CARGO_BUILD_TARGET" >&2; exit 1 ;;
esac
libdir=$(rustc --print target-libdir --target "$3")
if command -v cygpath >/dev/null 2>&1; then
    libdir=$(cygpath -u "$libdir")
fi
for lib in "$libdir"/libstd-*.rlib; do
    if [ -f "$lib" ]; then
        exit 0
    fi
done
echo "error: Rust standard library for $3 is missing; install it for the selected toolchain" >&2
exit 1
