#!/bin/sh
set -eu

# The Go generator owns content-addressed resource sources. This script only
# compiles one source and prints the absolute path suitable for the linker.
if [ "$#" -ne 2 ]; then
    echo "usage: compile.sh RESOURCE.rc ARCH" >&2
    exit 1
fi
rc=$1
case "$2" in
    386) machine=X86; format=pe-i386 ;;
    amd64) machine=X64; format=pe-x86-64 ;;
    arm64) machine=ARM64; format= ;;
    *) echo "error: unsupported Windows resource architecture: $2" >&2; exit 1 ;;
esac
test -f "$rc"
case "$rc" in
    *.rc) obj=${rc%.rc}.o ;;
    *) echo "error: expected a .rc source: $rc" >&2; exit 1 ;;
esac

if [ ! -f "$obj" ]; then
    tmp=$(mktemp -d "$(dirname "$rc")/tmp.XXXXXX")
    trap 'rm -rf "$tmp"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    if command -v "${LLVM_RC:-llvm-rc}" >/dev/null 2>&1 && command -v "${LLVM_CVTRES:-llvm-cvtres}" >/dev/null 2>&1; then
        "${LLVM_RC:-llvm-rc}" -no-preprocess -FO "$tmp/resource.res" "$rc" >&2
        "${LLVM_CVTRES:-llvm-cvtres}" "-machine:$machine" -timestamp:0 "-out:$tmp/resource.o" "$tmp/resource.res" >&2
    elif [ -n "$format" ] && command -v "${WINDRES:-windres}" >/dev/null 2>&1; then
        "${WINDRES:-windres}" -F "$format" -i "$rc" -O coff -o "$tmp/resource.o" >&2
    else
        echo "error: Windows $2 resources need llvm-rc + llvm-cvtres, or windres for x86/x86_64 (override LLVM_RC/LLVM_CVTRES/WINDRES)" >&2
        exit 1
    fi
    mv "$tmp/resource.o" "$obj"
fi

link_path=$(cd "$(dirname "$obj")" && pwd -P)/$(basename "$obj")
if command -v cygpath >/dev/null 2>&1; then
    link_path=$(cygpath -m "$link_path")
fi
printf '%s\n' "$link_path"
