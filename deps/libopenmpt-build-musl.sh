#!/bin/sh
# Host-side entry point for the musl dependency build.
#
# The CI host is glibc and the object code plus the smoke test (which dlopens the
# library) have to come from musl, so this script only starts the Alpine
# container that runs deps/libopenmpt-build-musl-container.sh. Keeping the Docker
# invocation here means the CI step and a local run are the same command:
#
#   WORK=$PWD/work VERSION=0.8.9 MUSL_ARCH=arm64 sh deps/libopenmpt-build-musl.sh
#
# Both architectures build natively: GitHub hosts ubuntu runners for x86_64 and
# arm64, so no emulation is involved and the compiler's output can be executed by
# the smoke test in the same container.
#
# Environment:
#   WORK           absolute workspace path with the extracted sources (required;
#                  mounted into the container as /work)
#   VERSION        libopenmpt version (required)
#   MUSL_ARCH      x86_64 | arm64 (required; the container asserts it against its
#                  own uname -m, so a wrong runner cannot silently ship the host
#                  architecture)
#   MUSL_BASELINE  Alpine release to build in (default: 3.23); it is the
#                  compatibility floor the artifact is meant to run on, because
#                  musl has no symbol versioning
#   SMOKE_SRC      path to libopenmpt-smoke.c (default: ./deps/libopenmpt-smoke.c);
#                  its directory is mounted read-only as /deps
set -eu

WORK="${WORK:?set WORK to the absolute workspace path}"
VERSION="${VERSION:?set VERSION to the libopenmpt version}"
MUSL_ARCH="${MUSL_ARCH:?set MUSL_ARCH to x86_64 or arm64}"
MUSL_BASELINE="${MUSL_BASELINE:-3.23}"
SMOKE_SRC="${SMOKE_SRC:-$PWD/deps/libopenmpt-smoke.c}"

case "$WORK" in
/*) ;;
*)
	echo "FATAL: WORK must be an absolute path (Docker bind mounts need one), got: $WORK" >&2
	exit 1
	;;
esac
[ -f "$SMOKE_SRC" ] || { echo "FATAL: no smoke test at $SMOKE_SRC" >&2; exit 1; }
DEPS_DIR="$(cd "$(dirname "$SMOKE_SRC")" && pwd)"

command -v docker >/dev/null || { echo "FATAL: docker is not available" >&2; exit 1; }

echo "=== build: libopenmpt $VERSION for musl/$MUSL_ARCH in alpine:$MUSL_BASELINE ==="
docker run --rm \
	-v "$WORK:/work" \
	-v "$DEPS_DIR:/deps:ro" \
	-e MUSL_ARCH="$MUSL_ARCH" \
	-e VERSION="$VERSION" \
	alpine:"$MUSL_BASELINE" \
	sh /deps/libopenmpt-build-musl-container.sh
