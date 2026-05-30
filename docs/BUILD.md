# Build Guide

This document describes how to build NeoViolet from source — locally and via CI/CD.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Makefile Targets](#makefile-targets)
- [Platform-Specific Instructions](#platform-specific-instructions)
  - [Linux](#linux)
  - [macOS](#macos)
  - [Windows](#windows)
- [Build Variants](#build-variants)
- [Installing](#installing)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

- **Go 1.26+** (see `go.mod` for the exact version)
- **`make`** — used for all build targets
- **C compiler** — required by CGo dependencies (the audio playback engine uses CGo for system audio)
- **`pkg-config`** — used to detect `libopenmpt` automatically
- **`libopenmpt`** (optional) — enables enhanced tracker module playback (`.mptm` and improved MOD/XM/IT/S3M support)
- **SoundFont file** (optional, `.sf2`) — required for MIDI playback (`mid` files) (Recommend SF2: [GeneralUser GS](https://schristiancollins.com/generaluser))

> The Makefile auto-detects `libopenmpt` via `pkg-config`. If found, the `openmpt` build tag is added. If not found, tracker support still works with the built-in `gotracker/playback` library, but some formats (e.g. `mptm`) won't play.

---

## Quick Start

```bash
# Clone the repository
git clone https://github.com/AuroraStudio-aurorast/neoviolet.git
cd neoviolet

# Build (libopenmpt auto-detected if available)
make build

# Binary is placed at ./neoviolet (or ./neoviolet.exe on Windows)
```

---

## Makefile Targets

| Target             | Description                                          |
|--------------------|------------------------------------------------------|
| `build`            | Production build with stripped debug info            |
| `build/race`       | Build with Go race detector                          |
| `build/debug`      | Build with debug symbols (compatible with `dlv`)     |
| `build/noopenmpt`  | Build without libopenmpt support                     |
| `run ARGS=...`     | Build and run with optional arguments                |
| `test`             | Run all tests                                        |
| `test/race`        | Run tests with race detector                         |
| `test/verbose`     | Run tests verbosely (`-v`)                           |
| `test/short`       | Run short tests (skips integration)                  |
| `test/cover`       | Run tests with coverage profile and HTML report      |
| `vet`              | Run `go vet`                                         |
| `lint`             | Run `golangci-lint` (falls back to `go vet`)         |
| `tidy`             | Run `go mod tidy`                                    |
| `clean`            | Remove build artifacts                               |
| `install`          | Install binary to `$GOPATH/bin`                      |

---

## Platform-Specific Instructions

### Linux

**Install dependencies:**

```bash
# Debian / Ubuntu
sudo apt-get update
sudo apt-get install -y clang llvm-dev pkg-config make libasound2-dev

# Optional: enhanced tracker module support
sudo apt-get install -y libopenmpt-dev

# Fedora
sudo dnf install clang pkg-config make alsa-lib-devel
sudo dnf install libopenmpt-devel   # optional

# Arch Linux
sudo pacman -S clang pkg-config make alsa-lib
sudo pacman -S libopenmpt           # optional
```

**Build:**

```bash
make build                              # production binary
CGO_ENABLED=1 GOARCH=amd64 make build   # explicit arch target
```

The resulting binary is `./neoviolet` — statically linked Go code with dynamically linked system audio and optional libopenmpt.

### macOS

**Install dependencies:**

```bash
brew update
brew install pkg-config make

# Optional: enhanced tracker module support
brew install libopenmpt
```

**Build:**

```bash
make build                              # production binary (auto-detects arch)
CGO_ENABLED=1 GOARCH=arm64 make build   # Apple Silicon
CGO_ENABLED=1 GOARCH=amd64 make build   # Intel Mac
```

The resulting binary is `./neoviolet`.

> [!NOTE]
> macOS may display a security warning on first launch for unsigned binaries. To bypass, run:
> ```bash
> xattr -dr com.apple.quarantine ./neoviolet
> ```

### Windows

> [!NOTE]
> In theory, this project could use [vcpkg](https://vcpkg.io/en/package/libopenmpt.html) to manage dependencies and compile with MSVC, but this has not yet been explored; here, we are using MSYS2.

Building on Windows requires **[MSYS2](https://www.msys2.org/)**.

**Using MSYS2:**

1. Install [MSYS2](https://www.msys2.org/) and follow the setup instructions.
2. Launch the appropriate MSYS2 environment (Recommend: CLANG64 for x86_64, CLANGARM64 for Arm64).

**Install dependencies (CLANG64 — amd64):**

```bash
# Install Go via MSYS2 is recommended!
pacman -S \
  mingw-w64-clang-x86_64-clang \
  mingw-w64-clang-x86_64-pkgconf \
  mingw-w64-clang-x86_64-libopenmpt \
  mingw-w64-clang-x86_64-libogg \
  mingw-w64-clang-x86_64-libvorbis \
  mingw-w64-clang-x86_64-zlib \
  mingw-w64-clang-x86_64-mpg123 \
  mingw-w64-clang-x86_64-go \
  make curl ca-certificates
```

**Install dependencies (CLANGARM64 — arm64):**

```bash
# Install Go via MSYS2 is recommended!
pacman -S \
  mingw-w64-clang-aarch64-clang \
  mingw-w64-clang-aarch64-pkgconf \
  mingw-w64-clang-aarch64-libopenmpt \
  mingw-w64-clang-aarch64-libogg \
  mingw-w64-clang-aarch64-libvorbis \
  mingw-w64-clang-aarch64-zlib \
  mingw-w64-clang-aarch64-mpg123 \
  mingw-w64-clang-aarch64-go \ # Recommended
  make curl ca-certificates
```

**Build:**

```bash
export CGO_ENABLED=1
export CC=clang
export CXX=clang++
export CGO_CFLAGS="$(pkg-config --cflags libopenmpt)"
export CGO_LDFLAGS="$(pkg-config --libs libopenmpt)"
make build
```

**Collect DLLs for distribution:**


```bash
mkdir -p dist
cp neoviolet.exe dist/
# Use objdump to find required DLLs and copy them from $MSYSTEM_PREFIX/bin
```

The resulting bundle is `dist/` containing `neoviolet.exe` and all required DLLs.

---

## Build Variants

### Production Build (`make build`)

```bash
make build
# Equivalent to: go build -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
# With openmpt tag: go build -tags openmpt -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
```

- Strips debug info (`-s -w`)
- Auto-detects `libopenmpt` and adds `openmpt` build tag if available
- Output: `./neoviolet` (or `./neoviolet.exe` on Windows)

### Debug Build (`make build/debug`)

```bash
make build/debug
# Equivalent to: go build -gcflags="all=-N -l" -o neoviolet ./cmd/neoviolet
```

- Disables optimizations and inlining
- Compatible with `dlv` (Delve debugger)
- Useful for breakpoint debugging and stack traces

### Race Build (`make build/race`)

```bash
make build/race
# Equivalent to: go build -race -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
```

- Enables Go's race detector
- Runtime overhead: ~5-10x slower, ~2x memory
- Use during development to catch data races

### Without libopenmpt (`make build/noopenmpt`)

```bash
make build/noopenmpt
# Equivalent to: go build -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
```

- Explicitly builds without libopenmpt support
- Useful when libopenmpt is installed but you want a build without it
- Tracker playback still works for most formats via `gotracker/playback`

### Manual Go Build

```bash
# Minimum build
CGO_ENABLED=1 go build -o neoviolet ./cmd/neoviolet

# Full build with openmpt
CGO_ENABLED=1 go build -tags openmpt -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
```

---

## Installing

```bash
# Install binary to $GOPATH/bin
make install

# Linux desktop integration (desktop file + icons)
make install/desktop
make install/icons
make install/all        # binary + desktop file
```

---

## Testing

```bash
# Run all tests
make test

# With race detector
make test/race

# Verbose output
make test/verbose

# Short tests (skips integration/format-heavy tests)
make test/short

# Coverage report
make test/cover
# Produces coverage.out and coverage.html
```

---

## Troubleshooting

### CGo / pkg-config Errors

```
pkg-config: exec: "pkg-config": executable file not found in $PATH
```

Install `pkg-config`:
- **macOS**: `brew install pkg-config`
- **Linux**: `sudo apt install pkg-config` (Debian/Ubuntu)
- **Windows (MSYS2)**: `pacman -S mingw-w64-clang-x86_64-pkgconf`

### libopenmpt Not Found

```
# checking: pkg-config --exists libopenmpt → not found
# Build proceeds without openmpt tag
```

This is **not an error**. The build succeeds without libopenmpt — only `.mptm` files and some advanced tracker features are unavailable. To install libopenmpt, see [Prerequisites](#prerequisites).

### Missing ALSA on Linux

```
#github.com/ebitengine/oto/v3/internal/mux
... alsa error: libasound.so.2: cannot open shared object file
```

Install ALSA development headers:
```bash
sudo apt install libasound2-dev   # Debian/Ubuntu
sudo dnf install alsa-lib-devel   # Fedora
```

### macOS Code Signing

```
"neoviolet" cannot be opened because the developer cannot be verified.
```

This is expected for locally built unsigned binaries. Either:
```bash
xattr -dr com.apple.quarantine ./neoviolet   # Remove quarantine flag
```
Or go to **System Settings → Privacy & Security** and click **Open Anyway**.

### Windows DLL Issues

If `neoviolet.exe` fails to launch with a missing DLL error, ensure all transitive DLLs are collected. The CI workflow resolves this automatically; for local builds:

1. Run `objdump -p neoviolet.exe | grep "DLL Name"` to list dependencies
2. Copy each from your MSYS2 `$MINGW_PREFIX/bin` directory
3. Repeat recursively for each non-system DLL

System DLLs (KERNEL32, ntdll, USER32, etc.) do not need to be bundled.

You can also collect these DLLs manually according to the error messages.

### GOARCH=386 Build Issue

When building a version for the 32-bit x86 architecture (`GOARCH=386`), you may encounter the error like:

```
# github.com/gotracker/playback/instrument
... ##[error]../../../go/pkg/mod/github.com/gotracker/playback@v1.5.0/instrument/opl2.go:80:27: cannot use math.MaxInt64 (untyped int constant 9223372036854775807) as int value in struct literal (overflows)
```

Due to dependencies, the project does not support 32-bit architecture. We do not plan to fix this issue; you should compile a 64-bit version, such as x86_64 (`GOARCH=amd64`).

### Go Version Mismatch

```
go: go.mod requires go >= 1.26.1 (running go 1.xx.x)
```

Install or update Go from [go.dev](https://go.dev/dl/).