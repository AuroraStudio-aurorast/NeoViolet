#!/bin/sh
# Build libopenmpt for macOS and gate the result.
#
# One arm64 runner builds both slices in a single pass, so no Intel runner and no
# per-arch artifacts are involved. Because `lipo -info` reports a slice that holds
# nothing but headers as present, every slice is also checked for symbol
# substance, and the Intel slice's functional evidence is a Rosetta run of the
# same smoke test.
#
# Recipe: autotools skeleton + system zlib + vendored minimp3/stb_vorbis, -O2
# -flto, deployment target $MACOS_MIN, and zero non-system dependencies.
#
# Local invocation (from the repo root, after extracting both tarballs into WORK):
#   WORK=$PWD/work VERSION=0.8.9 sh deps/libopenmpt-build-macos.sh
#
# Environment:
#   WORK        workspace with the extracted sources and the output (required)
#   VERSION     libopenmpt version (required: it locates the source tree)
#   MACOS_MIN   deployment target          (default: 13.0)
#   ARCHS       architectures to build     (default: "arm64 x86_64")
#   SMOKE_SRC   path to libopenmpt-smoke.c (default: ./deps/libopenmpt-smoke.c)
#   SRC, INC    source tree / vendored include dir (defaults derived from WORK)
#   OUT         output directory           (default: $WORK/out)
#   NV_SKIP_BUILD=1 NV_DYLIB=<path>        run the gates only, against an existing dylib
set -eu

WORK="${WORK:?set WORK to the workspace holding the extracted sources}"
VERSION="${VERSION:?set VERSION to the libopenmpt version}"
MACOS_MIN="${MACOS_MIN:-13.0}"
ARCHS="${ARCHS:-arm64 x86_64}"
OUT="${OUT:-$WORK/out}"
SRC="${SRC:-$WORK/libopenmpt-$VERSION+release.autotools}"
INC="${INC:-$WORK/vendored/libopenmpt-$VERSION+release/include}"
SMOKE_SRC="${SMOKE_SRC:-$PWD/deps/libopenmpt-smoke.c}"

MIN="-mmacosx-version-min=$MACOS_MIN"
FLAGS="-O2 -flto $MIN"
ARCH_FLAGS=""
for arch in $ARCHS; do
	ARCH_FLAGS="$ARCH_FLAGS -arch $arch"
done

mkdir -p "$OUT"

if [ "${NV_SKIP_BUILD:-0}" = 1 ]; then
	DYLIB="${NV_DYLIB:?NV_SKIP_BUILD=1 needs NV_DYLIB=<path>}"
else
	echo "=== build: libopenmpt $VERSION for $ARCHS (minos $MACOS_MIN) ==="
	# The vendored decoders only ship in the makefile tarball; the autotools
	# tarball has no include/ directory and no configure switch for them.
	[ -d "$SRC" ] || { echo "FATAL: no source tree at $SRC" >&2; exit 1; }
	[ -d "$INC/stb_vorbis" ] || { echo "FATAL: no vendored decoders at $INC" >&2; exit 1; }

	# With MPT_WITH_STBVORBIS, libopenmpt defines STB_VORBIS_HEADER_ONLY, so its
	# #include <stb_vorbis/stb_vorbis.c> only pulls in declarations. The
	# implementation has to be compiled as its own object, the way upstream's own
	# makefile does it, and with the same arch and LTO flags so no slice ends up
	# mixing bitcode with native objects.
	cc -c $ARCH_FLAGS $FLAGS \
		-DMPT_WITH_STBVORBIS -DSTB_VORBIS_NO_PULLDATA_API -DSTB_VORBIS_NO_STDIO \
		-I"$INC" "$INC/stb_vorbis/stb_vorbis.c" -o "$WORK/stb_vorbis.o"

	mkdir -p "$WORK/build"
	cd "$WORK/build"
	# -arch has to go into CC/CXX: libtool compiles its "master object" without
	# CFLAGS/CXXFLAGS, so that object would keep the host architecture and ld
	# would drop the other slice, leaving a library whose Intel half is an empty
	# stub.
	# MINIMP3_IMPLEMENTATION provides minimp3's implementation (only
	# SampleFormatMP3.cpp calls into it).
	"$SRC/configure" \
		CC="clang $ARCH_FLAGS" \
		CXX="clang++ $ARCH_FLAGS" \
		CFLAGS="$FLAGS -I$INC" \
		CXXFLAGS="$FLAGS -I$INC -DMPT_WITH_MINIMP3 -DMINIMP3_IMPLEMENTATION -DMPT_WITH_STBVORBIS" \
		LDFLAGS="$ARCH_FLAGS -flto $MIN" \
		LIBS="$WORK/stb_vorbis.o" \
		--with-zlib \
		--disable-static --enable-shared \
		--disable-openmpt123 --disable-examples --disable-tests \
		--without-mpg123 --without-ogg --without-vorbis --without-vorbisfile \
		--without-sndfile --without-flac \
		--without-portaudio --without-portaudiocpp \
		--prefix="$WORK/build/prefix"
	make -j"$(sysctl -n hw.ncpu)"
	make install
	DYLIB="$WORK/build/prefix/lib/libopenmpt.0.dylib"
fi

echo "=== gates: $DYLIB ==="
[ -f "$DYLIB" ] || { echo "FATAL: no dylib at $DYLIB" >&2; exit 1; }
echo "runner arch: $(uname -m), target arches: $ARCHS, runner macOS: $(sw_vers -productVersion)"
lipo -info "$DYLIB"
lipo -detailed_info "$DYLIB"

for arch in $ARCHS; do
	lipo -info "$DYLIB" | grep -qw "$arch" || { echo "FATAL: missing the $arch slice" >&2; exit 1; }
	symbols="$(nm -arch "$arch" "$DYLIB" | wc -l | tr -d ' ')"
	echo "$arch symbol count: $symbols"
	[ "$symbols" -gt 1000 ] || { echo "FATAL: the $arch slice is an empty stub" >&2; exit 1; }
	vtool -show-build -arch "$arch" "$DYLIB" | grep -q "minos $MACOS_MIN" || {
		echo "FATAL: the $arch slice does not target macOS $MACOS_MIN (it drifts with the runner's macOS version)" >&2
		vtool -show-build -arch "$arch" "$DYLIB"
		exit 1
	}
done

# Only real load commands: otool -L also reports LC_ID_DYLIB (the library itself)
# and, for fat files, per-architecture header lines.
NON_SYSTEM="$(otool -l "$DYLIB" | awk '/LC_LOAD_DYLIB/{want=1} want && /^ *name /{print $2; want=0}' | grep -v '^/usr/lib/' || true)"
if [ -n "$NON_SYSTEM" ]; then
	echo "FATAL: the library carries non-system dependencies:" >&2
	echo "$NON_SYSTEM" >&2
	exit 1
fi

cp "$DYLIB" "$OUT/libopenmpt.0.dylib"
echo "size: $(wc -c < "$OUT/libopenmpt.0.dylib") bytes"
otool -l "$OUT/libopenmpt.0.dylib" | awk '/LC_LOAD_DYLIB/{want=1} want && /^ *name /{print "  " $2; want=0}'

if [ "${NV_SKIP_BUILD:-0}" = 1 ]; then
	echo "NV_SKIP_BUILD=1: smoke and checksum phases skipped (gates above were exercised)"
	echo "MACOS GATES OK"
	exit 0
fi

echo "=== functional smoke ==="
# One fat smoke binary: it runs natively as arm64 here and as x86_64 under
# Rosetta below.
cc -O2 $ARCH_FLAGS $MIN "$SMOKE_SRC" -o "$WORK/smoke"
"$WORK/smoke" "$OUT/libopenmpt.0.dylib"

# The Intel slice's functional evidence. lipo/nm based checks only prove the
# slice exists and holds code; running it proves it works.
echo "=== Rosetta smoke (x86_64 slice) ==="
if ! arch -x86_64 /usr/bin/true 2>/dev/null; then
	echo "Rosetta is not installed; installing it to execute the x86_64 slice"
	sudo softwareupdate --install-rosetta --agree-to-license
fi
arch -x86_64 /usr/bin/true
arch -x86_64 "$WORK/smoke" "$OUT/libopenmpt.0.dylib"

shasum -a 256 "$OUT/libopenmpt.0.dylib" > "$OUT/libopenmpt.0.dylib.sha256"
cat "$OUT/libopenmpt.0.dylib.sha256"
echo "MACOS BUILD OK"
