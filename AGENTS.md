# AGENTS.md

## Build & Test

```bash
# Build
make build               # Production binary → ./build/neoviolet
make build/race          # With race detector
make build/debug         # With debug symbols

# Run (build + execute)
make run ARGS="<file>"   # Pass audio file to play directly
make install             # Install to $GOPATH/bin
./build/neoviolet        # Start the player

# Test
make test                # All tests
make test/verbose        # With verbose output
make test/race           # With race detector
make test/cover          # Coverage report
make test/short          # Skip integration tests

# Code quality
make vet                 # go vet
make lint                # golangci-lint (fallback: go vet)
make tidy                # go mod tidy
make clean               # Clean artifacts
```

## Environment Setup

### Prerequisites

- **Go 1.26+** — [Download](https://go.dev/dl/). For Windows, use MSYS2 build of Go.
- **libopenmpt** (optional) — enables enhanced tracker module playback. Without it, playback falls back to the built-in `gotracker` engine.

### CGo Requirement

This project uses **CGo extensively** — audio decoding, and tracker module playback all link against C libraries. `CGO_ENABLED` must be `1` and a C compiler must be available:

```bash
# Always required
export CGO_ENABLED=1
export CC=clang        # clang recommended; GCC also works
export CXX=clang++

# Windows (MSYS2) only — pass libopenmpt flags via pkg-config
export CGO_CFLAGS="$(pkg-config --cflags libopenmpt 2>/dev/null || true)"
export CGO_LDFLAGS="$(pkg-config --libs libopenmpt 2>/dev/null || true)"
```

**For AI agents:** Before any `go build` or `make build` step, verify that `CGO_ENABLED=1` and `CC` are set. The build will fail with linker errors if CGo is disabled. Treat this as a precondition — do not proceed without it.

### libopenmpt Check & Install

Check if `libopenmpt` is available:

```bash
pkg-config --exists libopenmpt && echo "installed" || echo "not installed"
```

If missing, install via your package manager:

| OS | Command |
|---|---|
| macOS (Homebrew) | `brew install libopenmpt` |
| Ubuntu/Debian | `sudo apt install libopenmpt-dev` |
| Fedora | `sudo dnf install libopenmpt-devel` |
| Arch Linux | `sudo pacman -S libopenmpt` |
| Windows (MSYS2, CLANG64) | `pacman -S mingw-w64-clang-x86_64-libopenmpt` |
| Windows (MSYS2, CLANGARM64) | `pacman -S mingw-w64-clang-aarch64-libopenmpt` |

**For AI agents:** Run the check above before building. If `libopenmpt` is not installed, ask the user whether to install it — do not install without confirmation.

### First Run

On first launch, NeoViolet automatically opens a setup wizard to configure audio directories and preferences. The wizard can be re-triggered by deleting the `config.json` next to the binary.

## Project Overview

NeoViolet is a **terminal music player** (Bubble Tea TUI) written in Go. It plays audio files with album art display, lyrics syncing, and media controls.

**Tech:** Go 1.26+, Bubble Tea v2, Lipgloss, gopxl/beep (audio), go-meltysynth (MIDI), godbus/dbus (MPRIS).

## Architecture

```
cmd/neoviolet/          # Entry point: init logger → load config → launch TUI
internal/
  audio/                # Playback engine (beep adapter, MIDI/tracker support)
    format/             # Individual format decoders
      alac/             # ALAC decoder (internal, based on MIT-licensed C-to-Go port)
      mp4/              # Minimal MP4/M4A demuxer (box iteration, sample table, magic cookie)
      alacstream/       # beep.StreamSeekCloser wrapper for ALAC-over-MP4
    synth/              # MIDI synthesis via SoundFont
  config/               # JSON config with defaults, first-run detection
  cover/                # Album art extraction from audio metadata
  lyrics/               # Lyrics parsing with pluggable format registry
    lrc.go, ttml.go, qrc.go, yrc.go, eslrc.go, lys.go, embedded.go
  ui/                   # Bubble Tea model/update/view
    model.go            # Central Model struct, NewModel(), Init()
    types.go            # Message types, state structs, keymap
    update.go           # Message dispatch + handlers
    update_keyboard.go  # Keyboard input handling
    view.go             # Layout rendering with Lipgloss
    audio_state.go      # Playback state machine helpers
  mediactl/             # OS media control bridge
    controller_linux.go # MPRIS via D-Bus
    controller_stub.go  # No-op on other platforms
  accent/               # K-means color extraction from album art
  logger/               # Structured logging
docs/                   # README, ACKNOWLEDGEMENTS
testdata/               # Shared test fixtures (*.mp3, *.flac, *.m4a, *.lrc, etc.)
.github/workflows/      # CI: multi-platform builds (linux/mac/windows × amd64/arm64)
```

## Architecture & Design Decisions

- **Bubble Tea Elm architecture:** Model → Update (via `tea.Msg`) → View. All state mutations happen through message handlers in `update.go`; never mutate model state outside of `Update()`.
- **Pluggable lyrics parsers:** Each format (LRC, TTML, QRC, YRC, ESLRC, LYS, embedded) implements `LyricParser` and registers via `lyrics.RegisterParser()`. The registry iterates in priority order and uses the first successful parse.
- **Platform abstraction via build tags:** Media control selects implementation at compile time — `controller_linux.go` (MPRIS/D-Bus) vs `controller_stub.go` (no-op). Add new platform support by creating a new `controller_<os>.go`.
- **Config-first startup:** On first run, `config.ConfigExists()` returns false, triggering a setup wizard (`ui/wizard`) before the main TUI launches. Config is persisted as JSON next to the binary.
- **Cross-platform audio:** `gopxl/beep` provides the core playback loop; format-specific decoders in `audio/format/` handle WAV/MP3/FLAC/OGG/ALAC (M4A). MIDI requires a SoundFont `.sf2` file. Tracker modules (MOD/XM/IT/S3M) use `gotracker/playback` with an optional `libopenmpt` backend enabled via build tag.

## Common Tasks

- **Add a new lyrics format:** Create a new file in `internal/lyrics/`, implement `LyricParser`, call `RegisterParser()` in `init()`.
- **Add a new audio format:** Add a decoder in `internal/audio/format/`, add magic byte detection in `Decode()`'s switch in `internal/audio/format/decoder.go`, and add the extension to `SupportedFormats()`.
- **Modify keyboard shortcuts:** Edit the `KeyMap` struct in `internal/ui/types.go` and the corresponding handler in `internal/ui/update_keyboard.go`.
- **Tweak UI styling:** Lipgloss style definitions are in `internal/ui/view.go` and `internal/ui/types.go` (config structs).

## External Dependencies

| Dependency | Usage |
|---|---|
| charm.land/bubbletea/v2 | TUI framework |
| charm.land/lipgloss/v2 | Terminal styling |
| github.com/gopxl/beep/v2 | Audio playback |
| github.com/dhowden/tag | Audio metadata/tags/cover art |
| github.com/jpodeszfa/go-meltysynth | MIDI SoundFont synthesis |
| github.com/gotracker/playback | MOD/XM/IT/S3M playback |
| github.com/godbus/dbus/v5 | Linux MPRIS integration |
| github.com/alicebob/alac (forked internal) | ALAC decoder for M4A files |