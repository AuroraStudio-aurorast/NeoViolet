#!/bin/sh
# Build libopenmpt for Windows inside an MSYS2 CLANG64 / CLANGARM64 shell, using
# libopenmpt's own makefile build system (upstream's Makefile is the only build
# system that compiles the vendored minimp3/stb_vorbis decoders as their own
# translation units; the autotools route rejects hand-injected objects because
# its libtool refuses non-libtool objects when --host names a cross target).
#
# The result is a single DLL with no third-party runtime dependency: only
# KERNEL32 and the UCRT api-set DLLs, which ship with Windows 10 and later.
# The vendored decoders are compiled in, so there is no mpg123/ogg/vorbis/zlib
# DLL to ship and no LGPL component in the closure.
#
# Local invocation (MSYS2 shell, from the repo root):
#   WINDOWS_ARCH=x86_64 VERSION=0.8.9 WORK=/tmp/win \
#   SMOKE_SRC=$PWD/deps/libopenmpt-smoke.c sh deps/libopenmpt-build-windows.sh
#
# Environment:
#   WINDOWS_ARCH   x86_64 | aarch64      (required; asserted against the DLL)
#   VERSION        libopenmpt version    (required; asserted against the library)
#   SMOKE_SRC      path to libopenmpt-smoke.c (required)
#   WORK           scratch directory     (default: $PWD/win-build)
#   SRC            extracted makefile tarball inside WORK
#                  (default: $WORK/vendored/libopenmpt-$VERSION+release, where the
#                  shared fetch step puts it)
#   OUT            output directory      (default: $WORK/out)
#   JOBS           make -j               (default: nproc)
#   NV_SKIP_BUILD=1 NV_DLL=<path>        run the gates only, against an existing DLL
#                                        (used to exercise this script's checks locally)
set -eu

WINDOWS_ARCH="${WINDOWS_ARCH:?set WINDOWS_ARCH to x86_64 or aarch64}"
VERSION="${VERSION:?set VERSION to the libopenmpt version}"
WORK="${WORK:-$PWD/win-build}"

# The CI step hands over a native Windows path (it comes from %RUNNER_TEMP%); the
# rest of this script uses MSYS2 paths. Convert only when it looks like one, so
# the script also runs unchanged on a POSIX host for gate testing.
to_posix() {
	case "$1" in
	*\\* | [A-Za-z]:*) cygpath -u "$1" 2>/dev/null || printf '%s' "$1" ;;
	*) printf '%s' "$1" ;;
	esac
}
WORK="$(to_posix "$WORK")"

SRC="${SRC:-$WORK/vendored/libopenmpt-$VERSION+release}"
OUT="${OUT:-$WORK/out}"
JOBS="${JOBS:-}"
MIN_OS="${WINDOWS_MIN:-win10}"

# 0x8664 = IMAGE_FILE_MACHINE_AMD64, 0xAA64 = IMAGE_FILE_MACHINE_ARM64.
case "$WINDOWS_ARCH" in
x86_64) EXPECT_TRIPLE=x86_64-w64 EXPECT_MACHINE=0x8664 ;;
aarch64) EXPECT_TRIPLE=aarch64-w64 EXPECT_MACHINE=0xAA64 ;;
*)
	echo "FATAL: WINDOWS_ARCH must be x86_64 or aarch64, got: $WINDOWS_ARCH" >&2
	exit 1
	;;
esac

mkdir -p "$WORK" "$OUT"

if [ "${NV_SKIP_BUILD:-0}" = 1 ]; then
	# Gates-only mode: needs no compiler, so the checks below can be exercised
	# on any machine that has the DLL.
	[ -n "${NV_DLL:-}" ] || { echo "FATAL: NV_SKIP_BUILD=1 needs NV_DLL=<path>" >&2; exit 1; }
	DLL="$NV_DLL"
else
	# The MSYS2 environment decides the target: clang in CLANG64 targets
	# x86_64-w64-windows-gnu, clang in CLANGARM64 targets aarch64-w64-windows-gnu.
	# Assert that up front, so a wrong msystem fails immediately instead of
	# producing a library for the wrong architecture.
	command -v clang >/dev/null || { echo "FATAL: clang is not on PATH (wrong MSYS2 environment?)" >&2; exit 1; }
	TRIPLE="$(clang -dumpmachine)"
	echo "clang -dumpmachine: $TRIPLE (expecting $EXPECT_TRIPLE-*)"
	case "$TRIPLE" in
	"$EXPECT_TRIPLE"*) ;;
	*)
		echo "FATAL: clang targets $TRIPLE but WINDOWS_ARCH=$WINDOWS_ARCH" >&2
		exit 1
		;;
	esac
	SMOKE_SRC="${SMOKE_SRC:?set SMOKE_SRC to the path of deps/libopenmpt-smoke.c}"
	SMOKE_SRC="$(to_posix "$SMOKE_SRC")"
	JOBS="${JOBS:-$(nproc)}"

	echo "=== build: libopenmpt $VERSION for $WINDOWS_ARCH (CONFIG unset -> MSYS2 host flavour) ==="
	[ -d "$SRC" ] || { echo "FATAL: no source tree at $SRC" >&2; exit 1; }
	cd "$SRC"
	# Notes on this command line:
	#  - No CONFIG=mingw-w64: the Makefile detects HOST_FLAVOUR=MSYS2 from MSYSTEM
	#    and picks config-defaults.mk, which is the only path that knows about
	#    CLANG64. config-mingw-w64.mk has no arm64 branch and aborts with
	#    "unknown WINDOWS_ARCH" when CONFIG is set, and its cross-only defaults
	#    (automatic NO_* switches) do not apply to a native MSYS2 build anyway.
	#  - CC/CXX are set explicitly: config-defaults.mk's MSYS2 mapping does not
	#    recognise CLANGARM64 and falls back to WINDOWS_ARCH=x86, so the default
	#    tool names would be i686 ones. AR/RC have to be set for the same reason.
	#  - The NO_* switches are explicit because the cross branch's automatic
	#    "no external dependencies" defaults only apply to cross builds.
	#  - PKG_CONFIG=false keeps any host .pc file from being linked in.
	#  - -static pulls in libc++/libunwind/compiler-rt statically; the UCRT
	#    itself stays a system DLL, which is what we want.
	make -j"$JOBS" VERBOSE=1 \
		MINGW_COMPILER=clang \
		WINDOWS_VERSION="$MIN_OS" \
		CC="clang -O2 -static" \
		CXX="clang++ -O2 -static" \
		AR=ar \
		SHARED_LIB=1 STATIC_LIB=0 EXAMPLES=0 OPENMPT123=0 TEST=0 FUZZ=0 \
		NO_ZLIB=1 NO_MPG123=1 NO_OGG=1 NO_VORBIS=1 NO_VORBISFILE=1 \
		NO_SNDFILE=1 NO_FLAC=1 NO_PORTAUDIO=1 NO_PORTAUDIOCPP=1 \
		NO_PULSEAUDIO=1 NO_SDL2=1 \
		PKG_CONFIG=false

	DLL=bin/libopenmpt.dll
	[ -f "$DLL" ] || DLL="$(ls bin/*libopenmpt*.dll 2>/dev/null | head -1 || true)"
	[ -n "$DLL" ] && [ -f "$DLL" ] || { echo "FATAL: the build produced no DLL" >&2; ls -l bin/ | head -20 >&2; exit 1; }
fi

echo "=== gates: $DLL ==="
[ -f "$DLL" ] || { echo "FATAL: no DLL at $DLL" >&2; exit 1; }
echo "size: $(wc -c < "$DLL") bytes"

# Architecture, read straight out of the PE header: the DOS stub holds e_lfanew
# (u32 at 0x3C), which points at "PE\0\0" followed by the machine type (u16).
# Reading it with od keeps this check independent of which binutils or LLVM tools
# the environment happens to provide, and it cannot be fooled the way "the binary
# ran" can be (Windows 11 ARM emulates x86_64).
LFANEW="$(od -An -t u4 -j 60 -N 4 "$DLL" | tr -d ' ')"
SIGNATURE="$(od -An -t x2 -j "$LFANEW" -N 2 "$DLL" | tr -d ' ')"
MACHINE="$(od -An -t u2 -j $((LFANEW + 4)) -N 2 "$DLL" | tr -d ' ')"
printf 'PE: e_lfanew=%s signature=%s machine=0x%04X (expecting %s)\n' "$LFANEW" "$SIGNATURE" "$MACHINE" "$EXPECT_MACHINE"
[ "$SIGNATURE" = 4550 ] || { echo "FATAL: not a PE image" >&2; exit 1; }
[ "$(printf '0x%04X' "$MACHINE")" = "$EXPECT_MACHINE" ] || {
	echo "FATAL: the DLL is not built for $WINDOWS_ARCH" >&2
	exit 1
}

# objdump comes from the environment's own binutils, so it understands the
# architecture this environment targets.
command -v objdump >/dev/null || { echo "FATAL: objdump is not on PATH" >&2; exit 1; }
echo "objdump: $(objdump --version | head -1)"

# The point of this pipeline: nothing but the OS may be needed at run time.
# KERNEL32 and the UCRT api sets ship with Windows 10+; anything else would be a
# DLL we would have to package and keep patched.
IMPORTS="$(objdump -p "$DLL" | awk '/DLL Name: /{print $3}' | sort -u)"
if [ -z "$IMPORTS" ]; then
	echo "FATAL: objdump found no import table (can this environment read $WINDOWS_ARCH PEs?)" >&2
	exit 1
fi
echo "imports:"; echo "$IMPORTS" | sed 's/^/  /'
UNEXPECTED="$(echo "$IMPORTS" | grep -viE '^(KERNEL32|api-ms-win-crt-[a-z0-9-]*)\.dll$' || true)"
if [ -n "$UNEXPECTED" ]; then
	echo "FATAL: the DLL needs third-party runtime DLLs:" >&2
	echo "$UNEXPECTED" >&2
	echo "hint: if libc++.dll/libunwind.dll are listed, the C++ runtime was linked" >&2
	echo "      dynamically; -static in CC/CXX is the flag that avoids it." >&2
	exit 1
fi

# The Go side loads this library through purego, which resolves every symbol up
# front and gives up on the whole library if one is missing. That contract is
# enforced by the smoke test below, and deliberately not by scanning the export
# table here: the export-table output differs between the GNU and the LLVM
# objdump (MSYS2's clang ships the LLVM one, whose format the GNU pattern does
# not match), so a textual scan would be a gate whose strictness depends on
# which binutils happens to be installed.

if [ "${NV_SKIP_BUILD:-0}" = 1 ]; then
	echo "NV_SKIP_BUILD=1: build and smoke phases skipped (gates above were exercised)"
	echo "WINDOWS GATES OK"
	exit 0
fi

echo "=== functional smoke ==="
# Compiled with the same environment and run twice: normally, and with PATH cut
# down to the system directory so that any third-party runtime DLL (including a
# delay-loaded one, which the import table above would show separately) simply
# cannot be found. The second run is what proves the library works on a machine
# that has only Windows itself.
SMOKE="$WORK/smoke.exe"
clang -Wall -Wextra -Werror -O2 "$SMOKE_SRC" -o "$SMOKE"
DLL_WIN="$(cygpath -w "$DLL" 2>/dev/null || echo "$DLL")"
OUTPUT="$("$SMOKE" "$DLL_WIN" 2>&1)"
echo "$OUTPUT"
echo "$OUTPUT" | grep -q '^PASS$' || { echo "FATAL: the smoke test failed" >&2; exit 1; }
echo "$OUTPUT" | grep -qE "^library_version : $VERSION$" || { echo "FATAL: expected library_version $VERSION" >&2; exit 1; }
for feature in +MINIMP3 +STBVORBIS; do
	echo "$OUTPUT" | grep -q "$feature" || { echo "FATAL: the library does not report $feature" >&2; exit 1; }
done

echo "=== bare-Windows smoke (PATH reduced to the system directory) ==="
SYSTEM_DIR="$(cygpath -u "${SYSTEMROOT:-C:\\Windows}")/System32"
echo "PATH=$SYSTEM_DIR"
BARE_OUTPUT="$(PATH="$SYSTEM_DIR" "$SMOKE" "$DLL_WIN" 2>&1 || true)"
echo "$BARE_OUTPUT"
echo "$BARE_OUTPUT" | grep -q '^PASS$' || {
	echo "FATAL: the DLL needs something that a plain Windows install does not have" >&2
	echo "      (the imports above should have been empty apart from KERNEL32 and the UCRT)" >&2
	exit 1
}

echo "=== package ==="
ART="$OUT/libopenmpt-$VERSION-win-$WINDOWS_ARCH.dll"
# Same reasoning as objdump above: MSYS2's clang ships LLVM's binutils, so accept
# either name and fail loudly when neither exists.
if command -v llvm-strip >/dev/null 2>&1; then
	STRIP=llvm-strip
elif command -v strip >/dev/null 2>&1; then
	STRIP=strip
else
	echo "FATAL: no strip tool (llvm-strip or strip) is on PATH" >&2
	exit 1
fi
echo "strip: $STRIP"
cp "$DLL" "$ART"
"$STRIP" "$ART"
echo "stripped size: $(wc -c < "$ART") bytes"
# The stripped copy is re-verified by the smoke test below, which is also what
# proves the exported symbol contract survived stripping.
"$SMOKE" "$(cygpath -w "$ART" 2>/dev/null || echo "$ART")" | grep -q '^PASS$' || {
	echo "FATAL: the stripped copy no longer passes the smoke test" >&2
	exit 1
}
sha256sum "$ART" > "$ART.sha256"
cat "$ART.sha256"
echo "WINDOWS BUILD OK"
