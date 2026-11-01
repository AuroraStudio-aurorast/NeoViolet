# Build Guide

How to build NeoViolet from source.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Makefile Targets](#makefile-targets)
- [GUI (neoviolet-gui)](#gui-neoviolet-gui)
  - [GUI Prerequisites](#gui-prerequisites)
  - [Building](#building)
  - [macOS App Bundle](#macos-app-bundle)
- [APE (Monkey's Audio) Toolchain](#ape-monkeys-audio-toolchain)
- [Platform-Specific Instructions](#platform-specific-instructions)
  - [Linux](#linux)
  - [macOS](#macos)
  - [Windows](#windows)
- [Build Variants](#build-variants)
- [Installing](#installing)
- [Testing](#testing)
- [Cleaning](#cleaning)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

- **Go 1.26.1+** — the requirement declared in `go.mod`; CI pins the version in `.github/workflows/build.yml`
- **`make`** — the only build entry point
- **Rust with cargo** — builds `apecli` and `neoviolet-gui`. Optional for TUI development: without it,
  `make run`, `make build/tui` and `make build` skip Rust components with a warning and still build
  `neoviolet`. `make run/gui`, `make build/macos` and macOS app bundling require cargo.
  Install via [rustup](https://rustup.rs/).
- **C compiler** — for race builds, race tests and `make check` (the race detector requires cgo), and
  for the Rust components' native dependencies. The Go player uses no CGo bindings; CI release builds
  set `CGO_ENABLED=0`. Ordinary local builds inherit Go's cgo setting, so a working `CC` is also needed
  when `CGO_ENABLED=1`.
- **`pkg-config`** (Linux only) — the GUI's build scripts use it to locate X11, Wayland, fontconfig and
  freetype
- **X11/Wayland development libraries** (Linux only) — required by the GUI; see
  [GUI Prerequisites](#gui-prerequisites)
- **`libopenmpt`** (optional, runtime) — not a build dependency. It is opened at run time and enables
  tracker formats the built-in engine cannot decode; `NEOVIOLET_LIBOPENMPT` overrides the search path.
  Per-platform install commands are in [Platform-Specific Instructions](#platform-specific-instructions).

---

## Quick Start

```bash
# Clone the repository
git clone https://github.com/AuroraStudio-aurorast/neoviolet.git
cd neoviolet

# Build the TUI and optional apecli, then launch the TUI (no GUI build)
make run
```

The TUI binary is placed at `./neoviolet` (`./neoviolet.exe` on Windows). Use `make build/tui` to
build without launching, or `make build` to build all components, including the optional GUI.

---

## Makefile Targets

| Target               | Description                                                                                |
|----------------------|--------------------------------------------------------------------------------------------|
| `build`              | Production TUI build with stripped debug info, plus both Rust components when cargo exists |
| `build/tui`          | Build TUI and optional `apecli`, without GUI                                               |
| `build/race`         | Build Go player only with race detector (requires cgo and a C compiler)                   |
| `build/debug`        | Build Go player only with debug symbols (compatible with `dlv`)                           |
| `build/macos`        | Universal binaries for Apple silicon and Intel (`arm64` + `amd64`, merged with `lipo`)     |
| `build/osxappbundle` | Build all and pack into an app bundle for macOS                                            |
| `apetools`           | Build `apecli` Rust helper (release mode)                                                  |
| `apetools/debug`     | Build `apecli` in debug mode                                                               |
| `gui`                | Build `neoviolet-gui` GUI wrapper (release mode)                                           |
| `gui/debug`          | Build `neoviolet-gui` in debug mode                                                        |
| `run/gui ARGS=...`   | Build TUI, `apecli` and GUI, then launch GUI (cargo required)                              |
| `run ARGS=...`       | Build TUI and optional `apecli`, then launch TUI                                          |
| `test`               | Run Go and Rust tests (Rust skipped without cargo)                                        |
| `test/go`            | Run Go tests only                                                                         |
| `test/race`          | Run Go tests with race detector (requires cgo and a C compiler)                            |
| `test/verbose`       | Run Go tests verbosely (`-v`)                                                              |
| `test/short`         | Run short Go tests (skips integration)                                                     |
| `test/cover`         | Run Go tests with coverage profile and HTML report                                        |
| `test/coverage`      | Run Go tests and enforce the `COVERAGE_MIN` gate                                           |
| `test/rust`          | Run Rust unit tests for `apecli` and `neoviolet-gui`                                       |
| `vet`                | Run `go vet`                                                                               |
| `lint`               | Run `golangci-lint` (falls back to `make vet` + `gofmt`)                                   |
| `lint/rust`          | Run `cargo fmt --check` and clippy with `-D warnings`                                      |
| `check`              | Aggregate lint, vet, Rust tests, race, coverage, tidy and file-size gates                 |
| `check/cgo`          | Check cgo and C compiler prerequisites for race targets                                   |
| `check/linelength`   | Enforce the per-file line limits (500 lines / 800 for tests)                               |
| `tidy`               | Run `go mod tidy`                                                                          |
| `tidy/check`         | Read-only `go mod tidy -diff` (what CI runs)                                               |
| `clean`              | Remove root binaries and coverage reports; retain Rust compilation caches                |
| `clean/all`          | Also clear both Rust compilation caches (when cargo is available)                        |
| `install`            | Install binary to `$GOPATH/bin`                                                            |
| `install/desktop`    | Install the `.desktop` entry                                                               |
| `install/icons`      | Print where to place the icons                                                             |
| `install/all`        | `install` + `install/desktop`                                                              |
| `help`               | List the available targets                                                                 |

---

## GUI (neoviolet-gui)

A native desktop wrapper around the same player, built with Rust (`gpui-ce` + `yororen-ui`).

### GUI Prerequisites

In addition to the [core prerequisites](#prerequisites):

**Linux** — X11/Wayland development libraries. The GUI's build scripts look up `x11`, `wayland-client`,
`fontconfig` and `freetype2` through `pkg-config`, and its build dependency `bindgen` needs libclang:

```bash
# Debian / Ubuntu
sudo apt-get install libxcb1-dev libxkbcommon-dev libxkbcommon-x11-dev \
                     libx11-dev libwayland-dev libfontconfig1-dev \
                     llvm-dev libclang-dev pkg-config

# Fedora
sudo dnf install libxcb-devel libxkbcommon-devel libxkbcommon-x11-devel \
                 libX11-devel wayland-devel fontconfig-devel \
                 clang-devel llvm-devel pkgconf

# Arch Linux
sudo pacman -S libxcb libxkbcommon libxkbcommon-x11 \
               libx11 wayland fontconfig clang pkgconf
```

`freetype2` itself is optional: `freetype-sys` builds its own copy when the system library is absent.

**macOS** — nothing extra; the GUI links only system frameworks.

**Windows** — nothing extra beyond the [MSYS2 toolchain](#windows).

### Building

```bash
make gui                # Release build → ./neoviolet-gui
make gui/debug          # Debug build
make build              # TUI + APE + GUI in one step
make run/gui ARGS=...    # Build TUI + APE + GUI and launch GUI (cargo required)
```

`make gui` builds only the wrapper. `make run/gui` also rebuilds its TUI and apecli dependencies so
the GUI launches the current player. Without cargo, `run/gui` fails before launching any existing
GUI binary. Rust compilation and root-binary copy failures also stop the build.

`make gui/debug` leaves its binary in `tools/neoviolet-gui/target/debug/`; it does not replace the
release binary in the repository root.

### macOS App Bundle

```bash
make build/osxappbundle
```

Packages `neoviolet`, `neoviolet-gui` and `apecli` into `dist/NeoViolet GUI.app/`. The bundle script
(`tools/osx-appbundle-builder/build.sh`) writes `Info.plist`, picks the icon (compiling it with
`actool` only when Xcode 26+ is installed, otherwise using the assets committed to the repo) and signs
the bundle ad hoc.

---

## APE (Monkey's Audio) Toolchain

`make build/tui` depends on `apetools`, which builds `apecli` from `tools/apecli/` and copies it to the
repo root so the Go binary finds it. `make run` uses this TUI build; `make build` also builds the GUI.

Cargo is optional. Without it the target prints

```
Warning: cargo not found, apecli will not be built (ffmpeg/mac fallback still works)
```

and the Go build continues. If no usable apecli is available, APE playback tries ffmpeg, then the
Monkey's Audio `mac` CLI. These are separate installed programs; if none is available, APE playback
fails while the TUI remains usable for other formats.

Build the helper on its own with `make apetools` (release) or `make apetools/debug`. The debug target
leaves its binary in `tools/apecli/target/debug/` and does not replace the root release binary. Tests
do not refresh the release helper; use `make apetools` after changing Rust code that the player uses.

---

## Platform-Specific Instructions

The system packages below exclude Go and Rust — install those separately as described per platform.

### Linux

**Build dependencies** — for the Rust GUI and for race builds/tests. These GUI dependencies are not
needed by `make run` or `make build/tui`. Building the Go player with `CGO_ENABLED=0` needs no C
toolchain; ordinary local builds inherit Go's cgo setting:

```bash
# Debian / Ubuntu
sudo apt-get update
sudo apt-get install -y gcc make pkg-config llvm-dev libclang-dev \
    libxcb1-dev libxkbcommon-dev libxkbcommon-x11-dev \
    libx11-dev libwayland-dev libfontconfig1-dev

# Fedora
sudo dnf install gcc make pkg-config llvm-devel clang-devel \
    libxcb-devel libxkbcommon-devel libxkbcommon-x11-devel \
    libX11-devel wayland-devel fontconfig-devel

# Arch Linux
sudo pacman -S gcc make pkgconf clang \
    libxcb libxkbcommon libxkbcommon-x11 \
    libx11 wayland fontconfig
```

**Optional runtime libraries** — not needed to build:

```bash
sudo apt-get install -y libopenmpt0              # Debian 13+ names it libopenmpt0t64
sudo dnf install libopenmpt                      # Fedora
sudo pacman -S libopenmpt                        # Arch Linux
```

Audio output needs the ALSA runtime library (`libasound2` / `alsa-lib`). Without it the app starts,
plays nothing, and reports no error.

**Build:**

```bash
make build/tui                          # TUI and optional apecli
make build                              # TUI, optional apecli and optional GUI
```

`build/tui` produces `./neoviolet` and, with cargo, `./apecli`. The full `build` also produces
`./neoviolet-gui` when cargo is available.

`GOARCH=amd64 make build` cross-compiles only the Go binary; the Rust helpers are always built for the
host architecture.

### macOS

**Install dependencies:** none. Xcode Command Line Tools provide `make`, and the GUI links only system
frameworks. The only optional package is the runtime library for tracker support:

```bash
brew install libopenmpt
```

**Build:**

```bash
make run          # build TUI + optional apecli and run; no GUI build
make build/tui    # host architecture: TUI + optional apecli
make build        # host architecture: all components
make build/macos  # universal, arm64 + amd64 merged with lipo
```

`make build/macos` requires cargo and the Intel Rust target, which rustup does not install by default:

```bash
rustup target add x86_64-apple-darwin
```

It compiles both slices on the machine it runs on and merges them, so a single Apple silicon host
produces binaries that also run on Intel Macs. `MACOS_MIN` (default `13.0`) is the oldest macOS those
slices claim.

The binaries land in the repository root: `./neoviolet`, `./neoviolet-gui`, `./apecli`.

> [!NOTE]
> macOS may warn about an unsigned binary on first launch. Clear the quarantine flag with
> `xattr -dr com.apple.quarantine ./neoviolet`.

### Windows

Requires **[MSYS2](https://www.msys2.org/)**, which provides `make` and the clang toolchain. Go and
Rust come from their own installers.

1. Install [MSYS2](https://www.msys2.org/) and follow its setup instructions.
2. Launch the environment matching your architecture: **CLANG64** for x86_64, **CLANGARM64** for Arm64.

**Build toolchain:**

```bash
# CLANG64 (amd64)
pacman -S --needed mingw-w64-clang-x86_64-clang make

# CLANGARM64 (arm64)
pacman -S --needed mingw-w64-clang-aarch64-clang make
```

The `clang` package also brings the C++ runtime (`libc++.dll`, `libunwind.dll`) that the Rust GUI
links against.

**Go** — from [go.dev](https://go.dev/dl/), at least the version required by `go.mod`. MSYS2's
`mingw-w64-clang-<arch>-go` package works too.

**Rust** — via [rustup](https://rustup.rs/), using the gnullvm toolchain that matches the clang packages
above, installed as the host toolchain so a plain `cargo build` targets it:

```bash
rustup toolchain install 1.98.0-x86_64-pc-windows-gnullvm   # RUST_VERSION from build.yml
rustup default 1.98.0-x86_64-pc-windows-gnullvm
```

Use the `aarch64` variant of that toolchain name in CLANGARM64.

**Build:**

```bash
export CGO_ENABLED=0
export CC=clang
export CXX=clang++
make build
```

**Optional runtime library:** `pacman -S --needed mingw-w64-clang-x86_64-libopenmpt`, or the `aarch64`
name in CLANGARM64.

**Collect DLLs for distribution:**

```bash
mkdir -p dist
cp neoviolet.exe dist/
cp neoviolet-gui.exe dist/ 2>/dev/null || true
# Then copy the non-system DLLs that objdump reports, recursively:
objdump -p dist/neoviolet-gui.exe | grep "DLL Name"
```

`libopenmpt` is opened at run time, so `objdump -p` never lists it. Copy it into `dist/` yourself,
otherwise `dist/` ships without tracker support (the app still starts).

Either take the DLL this project builds, or MSYS2's:

```bash
# Prebuilt: the asset the release build uses, matching your architecture
case "$(uname -m)" in
  x86_64)        arch=x86_64 ;;
  aarch64|arm64) arch=aarch64 ;;
esac
gh release download deps --repo AuroraStudio-aurorast/neoviolet \
  -p "libopenmpt-*-win-${arch}.dll" -D dist
mv dist/libopenmpt-*-win-${arch}.dll dist/libopenmpt.dll

# MSYS2's copy, whose own imports still have to be collected
cp "$MINGW_PREFIX"/bin/libopenmpt-0.dll dist/
objdump -p dist/libopenmpt-0.dll | grep "DLL Name"
```

The prebuilt DLL compiles its decoders in and needs nothing else; `gh release download deps` also
carries a matching `.sha256`. MSYS2's `libopenmpt-0.dll` instead pulls in its whole runtime closure
(`libogg`, `libvorbis`, `libvorbisfile`, `libmpg123`, `zlib1`, `libc++`, `libunwind`), so walk it the
same recursive way as the executables.

The `deps` release is a pre-release holding the dependency builds for every platform, with the
libopenmpt version and the architecture in each asset name.

The full recursive collection is in `.github/workflows/build.yml`.

---

## Build Variants

### Production Build (`make build`)

```bash
make build
```

- Builds the Go player and, when cargo is available, both Rust components in release mode
- Strips Go debug info (`-s -w`)
- Stamps the version from `git describe --tags --always --dirty` (override with `make build VERSION=...`)
- Output: `./neoviolet`, plus `./neoviolet-gui` and `./apecli` when cargo is available (`.exe` on Windows)

### TUI Build (`make build/tui`)

```bash
make build/tui
make run ARGS="--version"
```

Uses the same Go build flags and version stamp as `make build`, and builds the release apecli helper
when cargo is available. It does not build the GUI. `make run` depends on this target and passes
`ARGS` to the TUI. Go and cargo retain their normal incremental compilation caches.

### Debug Build (`make build/debug`)

```bash
make build/debug
```

Builds only the Go player, disables optimizations and inlining (`-gcflags="all=-N -l"`), and retains
debug symbols for `dlv` (Delve). This target does not inject a version stamp or rebuild Rust helpers.

### Race Build (`make build/race`)

```bash
make build/race
```

Builds only the Go player using the production flags plus Go's race detector (`-race`). Requires
`CGO_ENABLED=1` and a working C compiler; `check/cgo` checks these before compilation.

### Building Without CGo

```bash
CGO_ENABLED=0 make build/tui
```

This disables cgo for the Go player while preserving the Makefile's build flags and version stamp.
The optional apecli still builds when cargo is available. GUI and Rust native dependencies use their
own toolchains; `CGO_ENABLED=0` does not remove those requirements from a full build.

---

## Installing

```bash
make install            # binary to $GOPATH/bin
make install/desktop    # Linux .desktop entry
make install/icons      # prints where to place the icons
make install/all        # install + install/desktop
```

---

## Testing

```bash
make test           # Go + Rust tests; Rust skipped with a warning without cargo
make test/go        # Go tests only
make test/rust      # Rust tests only
make test/race      # Go tests with race detector; requires cgo and CC
make test/verbose   # Go tests with -v
make test/short     # short Go tests; skips integration/format-heavy tests
make test/cover     # Go tests; writes coverage.out and coverage.html
make test/coverage  # Go tests; enforces COVERAGE_MIN and writes coverage.out
make check          # complete local code quality gate; requires cgo and CC
```

All Go test targets honor `TEST_FLAGS` (default `-count=1`). For example:

```bash
make test/go TEST_FLAGS="-count=1 -run TestVersion"
CGO_ENABLED=1 CC=clang make test/race
```

`make check` runs vet, Go and Rust lint, Rust tests, Go race tests, the Go coverage gate, the read-only
module tidy check and the file-size gate. The race and coverage runs cover Go tests without an extra
ordinary full Go test run. CI runs these checks individually; its tool versions are pinned in
`.github/workflows/build.yml`. Rust tests and lint warn and skip without cargo, so such a local run
does not validate the Rust components.

If `golangci-lint` is unavailable, `make lint` falls back to the repository's `make vet` rules and
checks Go formatting. This fallback covers fewer checks than the full linter. Formatting and
file-size scans exclude Git metadata, `.worktrees` and build `target` directories.

Tests do not refresh the Rust release binaries that the application spawns. Use `make apetools` or
`make gui` to update those binaries.

---

## Cleaning

```bash
make clean          # remove root binaries and coverage reports; retain Rust caches
make clean/all      # also run cargo clean in both Rust crates
```

Use `clean` for ordinary development. `clean/all` clears Rust compilation caches and makes subsequent
Rust builds compile dependencies again. Without cargo, it removes root outputs and warns that Rust
cache cleanup was skipped. Neither target removes configuration, history or the app bundle in `dist/`.

---

## Troubleshooting

### GOARCH=386

32-bit x86 is not supported. Building with `GOARCH=386` fails in a dependency:

```
# github.com/gotracker/playback/instrument
... cannot use math.MaxInt64 (untyped int constant 9223372036854775807) as int value
in struct literal (overflows)
```

Build for a 64-bit target instead (`GOARCH=amd64` or `GOARCH=arm64`). This is not going to be fixed.

### Go Version Mismatch

```
go: go.mod requires go >= 1.26.1 (running go 1.xx.x)
```

Install a newer Go from [go.dev](https://go.dev/dl/).

### pkg-config Not Found (Linux)

```
error: failed to run custom build command for `wayland-sys v0.31.11`
...
Try `apt install pkg-config`, or `yum install pkg-config`, or `brew install pkgconf`
```

Only the Linux GUI needs `pkg-config`; the Go binary and the macOS/Windows builds do not.

```bash
sudo apt install pkg-config     # Debian/Ubuntu
sudo dnf install pkgconf        # Fedora
sudo pacman -S pkgconf          # Arch Linux
```

### Metal Shader Compilation (macOS)

```
error: failed to run `xcrun metal` …
```

`gpui-ce` compiles its shaders ahead of time by default, which requires the full Xcode Metal Toolchain.
`make gui` avoids that on macOS by passing `--no-default-features -F gpui/runtime_shaders`, so shaders
are compiled at run time through the system Metal framework instead. Use the Makefile entry points
so those flags are applied automatically:

```bash
make gui
make gui/debug
```
