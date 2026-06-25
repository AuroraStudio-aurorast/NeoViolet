# Build Guide

This document describes how to build NeoViolet from source — locally and via CI/CD.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Makefile Targets](#makefile-targets)
- [GUI (neoviolet-gui)](#gui-neoviolet-gui)
  - [GUI Prerequisites](#gui-prerequisites)
  - [Building](#gui-building)
  - [macOS App Bundle](#macos-app-bundle)
- [APE (Monkey's Audio) Toolchain](#ape-monkeys-audio-toolchain)
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
- **Rust with cargo** — required for building `apecli` (the APE/Monkey's Audio decoder helper) and `neoviolet-gui` (the native GUI wrapper). Without it, `make build` still succeeds (both sub-builds print a warning and exit cleanly). APE playback falls back to ffmpeg or macOS `afconvert`; the GUI binary is simply not produced. Install via [rustup](https://rustup.rs/).
- **`make`** — used for all build targets
- **C compiler** — required by CGo dependencies (the audio playback engine uses CGo for system audio)
- **`pkg-config`** — used to detect `libopenmpt` automatically
- **`libopenmpt`** (optional) — enables enhanced tracker module playback (`.mptm` and improved MOD/XM/IT/S3M support)
- **SoundFont file** (optional, `.sf2`) — required for MIDI playback (`mid` files) (Recommend SF2: [GeneralUser GS](https://schristiancollins.com/generaluser))
- **X11 development libraries** (Linux only) — required by the GUI (`gpui-ce`); see [GUI Prerequisites](#gui-prerequisites)

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
| `build`            | Production build with stripped debug info (builds `apecli`, `neoviolet-gui` automatically) |
| `build/race`       | Build with Go race detector                          |
| `build/debug`      | Build with debug symbols (compatible with `dlv`)     |
| `build/noopenmpt`  | Build without libopenmpt support                     |
| `build/osxappbundle` | Build all and pack into an app bundle for macOS    |
| `apetools`         | Build `apecli` Rust helper (release mode)            |
| `apetools/debug`   | Build `apecli` in debug mode                         |
| `gui`              | Build `neoviolet-gui` GUI wrapper (release mode)     |
| `gui/debug`        | Build `neoviolet-gui` in debug mode                  |
| `run/gui ARGS=...` | Build GUI and run with optional arguments            |
| `run ARGS=...`     | Build TUI and run with optional arguments            |
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

## GUI (neoviolet-gui)

`neoviolet-gui` is a native desktop GUI wrapper that embeds the NeoViolet terminal player in a native window. Built with Rust (`gpui-ce` + `yororen-ui`). It requires the `neoviolet` binary at runtime, searched as a sibling next to the GUI or via `$PATH`.

### GUI Prerequisites

In addition to the [core prerequisites](#prerequisites), the GUI requires:

**Linux** — X11 development libraries:

```bash
# Debian / Ubuntu
sudo apt-get install libxcb1-dev libxkbcommon-dev libxkbcommon-x11-dev

# Fedora
sudo dnf install libxcb-devel libxkbcommon-devel libxkbcommon-x11-devel

# Arch Linux
sudo pacman -S libxcb libxkbcommon libxkbcommon-x11
```

**macOS** — No extra dependencies. The Makefile automatically uses `runtime_shaders` to avoid requiring the Metal Toolchain.

**Windows (MSYS2)** — Covered by the standard [Windows MSYS2 setup](#windows); no extra packages needed.

### Building

```bash
make gui                # Release build → ./neoviolet-gui
make gui/debug          # Debug build (no optimizations)
make build              # Build everything (TUI + APE + GUI) in one step
make run/gui ARGS=...   # Build GUI and launch it
```

If `cargo` is not installed, the GUI targets print a warning and exit cleanly.

### macOS App Bundle

```bash
make build/osxappbundle
```

Packages `neoviolet`, `neoviolet-gui`, and `apecli` into `dist/NeoViolet GUI.app/`. The script (`tools/osx-appbundle-builder/build.sh`) handles icon compilation (require Xcode 26+), `Info.plist`, and ad-hoc code signing.

---

## APE (Monkey's Audio) Toolchain

APE format support requires a separate Rust helper binary (`apecli`) built from `tools/apecli/`. The Makefile handles this automatically.

### How It Works

- **`make build`** depends on **`apetools`**, so `apecli` is built before the Go binary on every production build.
- The `apetools` target checks for `cargo` — if missing, it prints a warning and exits cleanly; the Go build proceeds and APE playback falls back to ffmpeg or macOS `afconvert`.
- The built `apecli` binary is copied to the repo root so the Go runtime can find it.

### Build Targets

| Target | Command | Description |
|---|---|---|
| `make build` (default) | `make build` | Runs `apetools` → `go build` — one-step build with APE support |
| `make apetools` | `make apetools` | `cargo build --release` in `tools/apecli/` + copy to repo root |
| `make apetools/debug` | `make apetools/debug` | `cargo build` (debug) — useful for Rust debugging |
| `make clean` | `make clean` | Removes `apecli` binary + `cargo clean` |

### Without cargo

If Rust/cargo is not installed, all build targets still work. The build output includes a warning:

```
Warning: cargo not found, apecli will not be built (ffmpeg/mac fallback still works)
```

Install Rust via [rustup](https://rustup.rs/) and run `make apetools` to build `apecli` afterwards.

---

## Platform-Specific Instructions

### Linux

**Install dependencies:**

> [!NOTE]
> Exclude **Go** and **Rust**

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

Produces `./neoviolet`, `./apecli` (if cargo is available), and `./neoviolet-gui` (if cargo is available). The Go binary is statically linked Go code with dynamically linked system audio and optional libopenmpt.

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
  mingw-w64-clang-x86_64-rust \
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
  mingw-w64-clang-aarch64-go \
  mingw-w64-clang-aarch64-rust \
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

Both `neoviolet.exe` and `neoviolet-gui.exe` (if built) need their transitive DLLs collected:

```bash
mkdir -p dist
cp neoviolet.exe dist/
cp neoviolet-gui.exe dist/ 2>/dev/null || true
# Use objdump to find required DLLs and copy them from $MSYSTEM_PREFIX/bin
# See .github/workflows/build.yml for the full recursive DLL collection script
```

The resulting bundle is `dist/` containing `neoviolet.exe`, `neoviolet-gui.exe` (if built), and all required DLLs.

---

## Build Variants

### Production Build (`make build`)

```bash
make build
# Step 1: cargo build --release in tools/apecli/     (if cargo is available)
# Step 2: cargo build --release in tools/neoviolet-gui/ (if cargo is available)
# Step 3: go build -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
# With openmpt tag: go build -tags openmpt -ldflags="-s -w" -o neoviolet ./cmd/neoviolet
```

- Builds the `apecli` Rust helper automatically (if cargo is available)
- Builds the `neoviolet-gui` Rust GUI automatically (if cargo is available)
- Strips debug info (`-s -w`)
- Auto-detects `libopenmpt` and adds `openmpt` build tag if available
- Output: `./neoviolet`, `./neoviolet-gui`, `./apecli` (or `.exe` on Windows)

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

> **Note:** `make test` does **not** rebuild `apecli`. If you've modified the Rust source in `tools/apecli/`, run `make apetools` first, then `make test`.

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

### apecli Build Failure (Rust)

```
error[E0463]: can't find crate for `ape_decoder`
```

This means the Rust `ape-decoder` crate failed to download or compile. Ensure:

```bash
# Verify cargo is installed
cargo --version

# Test the build directly
cd tools/apecli && cargo build --release
```

If `cargo` is missing, install [rustup](https://rustup.rs/) and retry.

### GUI Build Failure — Missing X11 Libraries (Linux)

```
error: linking with `cc` failed: exit status: 1
  = note: /usr/bin/ld: cannot find -lxcb
  = note: /usr/bin/ld: cannot find -lxkbcommon
```

The GUI (`gpui-ce`) requires X11 development headers on Linux. Install them:

```bash
# Debian / Ubuntu
sudo apt-get install libxcb1-dev libxkbcommon-dev libxkbcommon-x11-dev

# Fedora
sudo dnf install libxcb-devel libxkbcommon-devel libxkbcommon-x11-devel

# Arch
sudo pacman -S libxcb libxkbcommon libxkbcommon-x11
```

After installing, re-run `make gui` or `make build`.

### GUI Build Failure — Metal Shader Compilation (macOS)

```
error: failed to run `xcrun metal` …
```

`gpui-ce` compiles GPU shaders ahead of time by default, which requires the full Xcode Metal Toolchain (cannot be detected sometimes even though you have installed). The Makefile avoids this by adding `--no-default-features -F gpui/runtime_shaders` on macOS, compiling shaders at runtime instead via the system Metal framework. If you invoke `cargo build` directly, pass the flag manually:

```bash
cargo build --release --no-default-features -F gpui/runtime_shaders
```

### GUI Can't Find neoviolet Binary

```
Error: neoviolet binary not found
```

Place `neoviolet` next to `neoviolet-gui` (sibling in the same directory), or ensure it's on `$PATH`:

```bash
cp neoviolet "$(dirname "$(which neoviolet-gui)")/"
# Windows (MSYS2):
cp neoviolet.exe "$(dirname "$(which neoviolet-gui.exe)")/"
```

### GUI Window Fails to Open (Linux)

```
Error: X11 connection broken / Wayland protocol error
```

`gpui-ce` natively supports both X11 and Wayland (both are default features). If the window fails to open, ask related communities for help.

### Go Version Mismatch

```
go: go.mod requires go >= 1.26.1 (running go 1.xx.x)
```

Install or update Go from [go.dev](https://go.dev/dl/).