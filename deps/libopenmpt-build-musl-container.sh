#!/bin/sh
# Build libopenmpt for musl (Alpine) and gate the result.
#
# This script runs INSIDE an Alpine container: the CI host is glibc, so both the
# object code and the smoke test (which dlopens the library) have to come from
# musl. deps/libopenmpt-build-musl.sh is the host-side entry point that starts
# this container; both files exist so that "run the platform script" is the same
# step in CI and on a laptop.
#
# Environment:
#   MUSL_ARCH  arm64 | x86_64  (required; asserted against the container)
#   WORK       workspace, default /work: holds the extracted sources and output
#   VERSION    library version from deps/libopenmpt.lock (required: it locates
#              the extracted source tree)
#
# Recipe: the same one the macOS dependency build uses (autotools skeleton +
# system zlib + vendored minimp3/stb_vorbis, LTO, no non-system dependencies),
# with the differences musl imposes:
#   * no -arch / min-version flags; one native arch per container
#   * the vendored decoder object needs -fPIC, or the shared link fails
#   * libtool always links libstdc++/libgcc_s dynamically: -static-libstdc++ in
#     LDFLAGS has no effect. Alpine's own libopenmpt package needs both as well,
#     so they are accepted rather than fought.
set -eu

MUSL_ARCH="${MUSL_ARCH:?MUSL_ARCH must be arm64 or x86_64}"
WORK="${WORK:-/work}"
VERSION="${VERSION:?VERSION must be set (it locates the extracted source tree)}"

case "$MUSL_ARCH" in
	arm64) UNAME=aarch64; MACHINE_RE='AArch64' ;;
	x86_64) UNAME=x86_64; MACHINE_RE='X86-64' ;;
	*) echo "ERROR: unsupported MUSL_ARCH '$MUSL_ARCH'" >&2; exit 2 ;;
esac

SRC="$WORK/libopenmpt-$VERSION+release.autotools"
INC="$WORK/vendored/libopenmpt-$VERSION+release/include"
BUILD="$WORK/build-musl-$MUSL_ARCH"
OUT="$WORK/out"
ART="$OUT/libopenmpt-musl-$MUSL_ARCH.so"

echo "=== libopenmpt musl build: arch=$MUSL_ARCH libc=$(ldd /bin/sh 2>/dev/null | head -1 || true)"

# The container must really be the requested architecture; a wrong --platform
# would otherwise silently produce a library for the runner's arch.
running="$(uname -m)"
[ "$running" = "$UNAME" ] || {
	echo "ERROR: container is $running but MUSL_ARCH=$MUSL_ARCH expects $UNAME" >&2
	exit 1
}
echo "container: $(uname -s) $(uname -r) $(uname -m), alpine $(cat /etc/alpine-release)"

echo "--- toolchain"
# build-base gives gcc/g++/make/binutils; zlib-dev the headers libopenmpt wants
# for --with-zlib (musl's own libz.so.1 comes with the zlib package and is
# already in the alpine base image).
apk add --no-cache build-base zlib-dev >/dev/null
echo "gcc:  $(gcc -dumpversion) ($(gcc -dumpmachine))"
echo "make: $(make --version | head -1)"
echo "zlib: $(ls /usr/lib/libz.so.1 2>/dev/null || echo missing)"

test -x "$SRC/configure" || { echo "ERROR: no source tree at $SRC" >&2; exit 1; }
test -d "$INC/stb_vorbis" || { echo "ERROR: no vendored decoders at $INC" >&2; exit 1; }

# With MPT_WITH_STBVORBIS, libopenmpt defines STB_VORBIS_HEADER_ONLY, so its
# #include <stb_vorbis/stb_vorbis.c> only pulls in declarations. The
# implementation has to be its own translation unit, exactly as upstream's own
# makefile does it, compiled with the same -fPIC (Linux, unlike macOS needs it
# explicitly) and -flto so no bitcode is mixed with native objects.
echo "--- vendored decoder object"
gcc -O2 -flto -fPIC \
	-DMPT_WITH_STBVORBIS -DSTB_VORBIS_NO_PULLDATA_API -DSTB_VORBIS_NO_STDIO \
	-I"$INC" -c "$INC/stb_vorbis/stb_vorbis.c" -o "$WORK/stb_vorbis.o"
echo "stb_vorbis.o: $(wc -c <"$WORK/stb_vorbis.o") bytes"

echo "--- configure + make -j$(nproc)"
rm -rf "$BUILD"
mkdir -p "$BUILD"
cd "$BUILD"
start=$(date +%s)
# MINIMP3_IMPLEMENTATION provides minimp3's implementation; only
# SampleFormatMP3.cpp calls into it.
"$SRC/configure" \
	CC=gcc \
	CXX=g++ \
	CFLAGS="-O2 -flto -fPIC -I$INC" \
	CXXFLAGS="-O2 -flto -fPIC -I$INC -DMPT_WITH_MINIMP3 -DMINIMP3_IMPLEMENTATION -DMPT_WITH_STBVORBIS" \
	LDFLAGS="-O2 -flto" \
	LIBS="$WORK/stb_vorbis.o" \
	--with-zlib \
	--disable-static --enable-shared \
	--disable-openmpt123 --disable-examples --disable-tests \
	--without-mpg123 --without-ogg --without-vorbis --without-vorbisfile \
	--without-sndfile --without-flac \
	--without-portaudio --without-portaudiocpp \
	--prefix="$BUILD/prefix" >"$WORK/configure.log" 2>&1
make -j"$(nproc)" >"$WORK/make.log" 2>&1
make install >>"$WORK/make.log" 2>&1
echo "built in $(( $(date +%s) - start ))s"

BUILT="$BUILD/prefix/lib/libopenmpt.so.0.5.5"
test -f "$BUILT" || {
	echo "ERROR: the build produced no libopenmpt.so.0.5.5" >&2
	tail -20 "$WORK/make.log" >&2
	exit 1
}

echo "--- package the artifact"
mkdir -p "$OUT"
cp "$BUILT" "$ART"
chmod 755 "$ART"
# Alpine's own package is stripped too; on a 2.1 MB library this is ~15% off and
# the dynamic symbol table survives (checked below).
strip "$ART"
symbols="$(nm -D --defined-only "$ART" | wc -l | tr -d ' ')"
echo "$(basename "$ART"): $(wc -c <"$ART") bytes, $symbols dynamic symbols"
[ "$symbols" -gt 100 ] || { echo "ERROR: stripping dropped the dynamic symbols" >&2; exit 1; }

echo "--- gate: machine, dependencies, symbols"
readelf -h "$ART" | grep -qiE "$MACHINE_RE" || {
	echo "ERROR: the artifact is not built for $MUSL_ARCH" >&2
	readelf -h "$ART" >&2
	exit 1
}

# Only libz, the C++ runtime and musl libc are expected. This is a strict subset
# of what Alpine's own libopenmpt package needs (it also links libmpg123,
# libogg, libvorbis and libvorbisfile); anything else means the slimming
# switches stopped taking effect.
allowed="libz.so.1 libstdc++.so.6 libgcc_s.so.1 libc.musl-$UNAME.so.1"
needed="$(readelf -d "$ART" | sed -n 's/.*(NEEDED).*\[\(.*\)\]/\1/p')"
echo "NEEDED: $(echo "$needed" | tr '\n' ' ')"
for lib in $needed; do
	case " $allowed " in
		*" $lib "*) ;;
		*) echo "ERROR: unexpected dependency '$lib'" >&2; exit 1 ;;
	esac
done
echo "$needed" | grep -q '^libc\.musl-' || { echo "ERROR: artifact is not linked against musl libc" >&2; exit 1; }

echo "--- gate: functional smoke test (deps/libopenmpt-smoke.c)"
SMOKE_SRC="${SMOKE_SRC:-/deps/libopenmpt-smoke.c}"
[ -f "$SMOKE_SRC" ] || { echo "ERROR: no smoke test at $SMOKE_SRC" >&2; exit 1; }
cc -Wall -Wextra -Werror -O2 "$SMOKE_SRC" -o "$WORK/smoke"
# Keep both the output and the exit status: piping into tee would hand the
# pipeline's status to tee, so a smoke test that failed after printing its
# version line would go unnoticed.
if ! "$WORK/smoke" "$ART" > "$WORK/smoke.log" 2>&1; then
	cat "$WORK/smoke.log"
	echo "ERROR: the smoke test failed" >&2
	exit 1
fi
cat "$WORK/smoke.log"

grep -q '^library_version : ' "$WORK/smoke.log" || { echo "ERROR: the smoke test reported no version" >&2; exit 1; }
if [ -n "$VERSION" ]; then
	grep -q "^library_version : $VERSION$" "$WORK/smoke.log" || {
		echo "ERROR: library version does not match deps/libopenmpt.lock ($VERSION)" >&2
		exit 1
	}
fi
for feature in '+ZLIB' '+MINIMP3' '+STBVORBIS'; do
	grep -q -- "$feature" "$WORK/smoke.log" || {
		echo "ERROR: the library is missing $feature; the slimming switches are not what the lock file describes" >&2
		exit 1
	}
done

echo "--- checksum"
cd "$OUT"
sha256sum "$(basename "$ART")" > "$(basename "$ART").sha256"
cat "$(basename "$ART").sha256"
ls -l "$OUT"

echo "=== libopenmpt musl build done: $ART"
echo "MUSL BUILD OK"
